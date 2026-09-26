package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type config struct {
	MainURL                string
	ProvisioningCredential string
	SSOSecretFile          string
	StateDir               string
	ProvisionerPath        string
	Image                  string
	DockerBin              string
	EdgeNetwork            string
	NginxContainer         string
	NginxConfigDir         string
	ExpectedEdgeIPs        []string
	RequestTimeout         time.Duration
	ReadinessTimeout       time.Duration
	PollInterval           time.Duration
	TLSCheckTimeout        time.Duration
	ProvisionCommandLimit  time.Duration
}

type agentRecord struct {
	AgentID         string    `json:"agent_id"`
	RequestID       string    `json:"request_id"`
	Slug            string    `json:"slug"`
	Domain          string    `json:"domain"`
	DisplayName     string    `json:"display_name"`
	OwnerMainUserID int64     `json:"owner_main_user_id"`
	Status          string    `json:"status"`
	CurrentStep     string    `json:"current_step"`
	RecentError     string    `json:"recent_error"`
	CanRetry        bool      `json:"can_retry"`
	DomainStatus    string    `json:"domain_status"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type apiEnvelope struct {
	Code    json.RawMessage `json:"code"`
	Message string          `json:"message"`
	Reason  string          `json:"reason"`
	Data    json.RawMessage `json:"data"`
}

type agentPage struct {
	Items    []agentRecord `json:"items"`
	Total    int           `json:"total"`
	Page     int           `json:"page"`
	PageSize int           `json:"page_size"`
}

type controlAPI struct {
	cfg       config
	client    *http.Client
	streamURL string
}

var (
	errAgentLeaseHeld    = errors.New("agent provisioning lease is held")
	errAgentNotClaimable = errors.New("agent is no longer claimable")
	errAgentLeaseLost    = errors.New("agent provisioning lease was lost")
)

const defaultLeaseRefreshInterval = 20 * time.Second

type leaseClaim struct {
	Agent     agentRecord `json:"agent"`
	Token     string      `json:"lease_token"`
	ExpiresAt time.Time   `json:"lease_expires_at"`
}

func newControlAPI(cfg config) *controlAPI {
	return &controlAPI{
		cfg:       cfg,
		client:    &http.Client{Timeout: cfg.RequestTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		streamURL: strings.TrimRight(cfg.MainURL, "/") + "/api/v1/agent-provisioning/agents/stream",
	}
}

func (a *controlAPI) endpoint(path string) string {
	return strings.TrimRight(a.cfg.MainURL, "/") + "/api/v1/" + strings.TrimLeft(path, "/")
}

func (a *controlAPI) request(ctx context.Context, method, path string, payload any, idempotencyKey, requestID string) (json.RawMessage, error) {
	return a.requestWithLease(ctx, method, path, payload, idempotencyKey, requestID, "")
}

func (a *controlAPI) requestWithLease(ctx context.Context, method, path string, payload any, idempotencyKey, requestID, leaseToken string) (json.RawMessage, error) {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("encode control-plane request")
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, a.endpoint(path), body)
	if err != nil {
		return nil, fmt.Errorf("create control-plane request")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-AgentAPI-Provisioning-Credential", a.cfg.ProvisioningCredential)
	if leaseToken != "" {
		req.Header.Set("X-AgentAPI-Provisioning-Lease", leaseToken)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	if requestID != "" {
		req.Header.Set("X-Request-ID", requestID)
	}
	response, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("control-plane request failed")
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return nil, fmt.Errorf("read control-plane response")
	}
	var envelope apiEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		if response.StatusCode >= 200 && response.StatusCode < 300 {
			return json.RawMessage(data), nil
		}
		return nil, fmt.Errorf("control plane returned HTTP %d", response.StatusCode)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || (len(envelope.Code) > 0 && string(envelope.Code) != "0" && string(envelope.Code) != "null") {
		code := strings.Trim(string(envelope.Code), `"`)
		if envelope.Reason != "" {
			code = envelope.Reason
		}
		if code == "" {
			code = fmt.Sprintf("HTTP_%d", response.StatusCode)
		}
		switch code {
		case "AGENT_PROVISIONING_LEASE_HELD":
			return nil, errAgentLeaseHeld
		case "AGENT_STATE_CONFLICT":
			return nil, errAgentNotClaimable
		case "AGENT_PROVISIONING_LEASE_LOST":
			return nil, errAgentLeaseLost
		default:
			return nil, fmt.Errorf("control plane rejected request (%s)", code)
		}
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return json.RawMessage(`{}`), nil
	}
	return envelope.Data, nil
}

func (a *controlAPI) claimAgent(ctx context.Context, agentID string) (leaseClaim, error) {
	data, err := a.request(ctx, http.MethodPost, "agent-provisioning/agents/"+url.PathEscape(agentID)+"/claim", map[string]any{}, "", "")
	if err != nil {
		return leaseClaim{}, err
	}
	var result leaseClaim
	if err := json.Unmarshal(data, &result); err != nil || result.Agent.AgentID != agentID || len(result.Token) < 32 || result.ExpiresAt.IsZero() {
		return leaseClaim{}, fmt.Errorf("control plane returned an invalid provisioning lease")
	}
	return result, nil
}

func (a *controlAPI) renewLease(ctx context.Context, agentID, token string) error {
	_, err := a.requestWithLease(ctx, http.MethodPost, "agent-provisioning/agents/"+url.PathEscape(agentID)+"/lease", map[string]any{}, "", "", token)
	return err
}

func (a *controlAPI) registerRuntimeCredentials(ctx context.Context, agentID, controlHash, modelHash, leaseToken string) error {
	if !validWorkerAgentID(agentID) || !isSHA256Hash(controlHash) || !isSHA256Hash(modelHash) || controlHash == modelHash || strings.TrimSpace(leaseToken) == "" {
		return fmt.Errorf("deployment bundle returned invalid runtime credential metadata")
	}
	_, err := a.requestWithLease(ctx, http.MethodPost,
		"agent-provisioning/agents/"+url.PathEscape(agentID)+"/runtime-credentials",
		map[string]string{"control_token_hash": controlHash, "model_token_hash": modelHash}, "", "", leaseToken)
	return err
}

func validWorkerAgentID(value string) bool {
	if len(value) != 36 || !strings.HasPrefix(value, "agt_") {
		return false
	}
	for _, char := range value[4:] {
		if !(char >= '0' && char <= '9') && !(char >= 'a' && char <= 'f') {
			return false
		}
	}
	return true
}

func isSHA256Hash(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func (a *controlAPI) getAgent(ctx context.Context, agentID string) (agentRecord, error) {
	data, err := a.request(ctx, http.MethodGet, "agent-provisioning/agents/"+url.PathEscape(agentID), nil, "", "")
	if err != nil {
		return agentRecord{}, err
	}
	var agent agentRecord
	if err := json.Unmarshal(data, &agent); err != nil || agent.AgentID != agentID || agent.UpdatedAt.IsZero() {
		return agentRecord{}, fmt.Errorf("control plane returned an invalid agent record")
	}
	return agent, nil
}

func (a *controlAPI) listAgents(ctx context.Context, status string, page int) (agentPage, error) {
	query := url.Values{"status": {status}, "page": {fmt.Sprint(page)}, "page_size": {"100"}}
	data, err := a.request(ctx, http.MethodGet, "agent-provisioning/agents?"+query.Encode(), nil, "", "")
	if err != nil {
		return agentPage{}, err
	}
	var result agentPage
	if err := json.Unmarshal(data, &result); err != nil {
		return agentPage{}, fmt.Errorf("control plane returned an invalid agent list")
	}
	return result, nil
}

func (a *controlAPI) reportProgress(ctx context.Context, agent agentRecord, step, failureCode string, retryable bool, leaseToken string) (agentRecord, error) {
	payload := map[string]any{"step": step}
	if failureCode != "" {
		payload["failure_code"] = failureCode
		payload["failure_step"] = agent.CurrentStep
		payload["retryable"] = retryable
	}
	key := workerIdempotencyKey(agent, "progress-"+step)
	requestID := workerRequestID(agent, step)
	data, err := a.requestWithLease(ctx, http.MethodPatch, "agent-provisioning/agents/"+url.PathEscape(agent.AgentID)+"/progress", payload, key, requestID, leaseToken)
	if err != nil {
		return agentRecord{}, err
	}
	var result agentRecord
	if err := json.Unmarshal(data, &result); err != nil || result.AgentID != agent.AgentID || result.UpdatedAt.IsZero() {
		return agentRecord{}, fmt.Errorf("control plane returned an invalid progress result")
	}
	return result, nil
}

func (a *controlAPI) activate(ctx context.Context, agent agentRecord, leaseToken string) error {
	payload := map[string]any{"confirm_agent_id": agent.AgentID, "readiness_confirmed": true}
	_, err := a.requestWithLease(ctx, http.MethodPost, "agent-provisioning/agents/"+url.PathEscape(agent.AgentID)+"/activate", payload,
		workerIdempotencyKey(agent, "activate"), workerRequestID(agent, "activate"), leaseToken)
	return err
}

func (a *controlAPI) openStream(ctx context.Context) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, a.streamURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create control-plane event stream request")
	}
	request.Header.Set("Accept", "text/event-stream")
	request.Header.Set("Cache-Control", "no-cache")
	request.Header.Set("X-AgentAPI-Provisioning-Credential", a.cfg.ProvisioningCredential)
	response, err := (&http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}).Do(request)
	if err != nil {
		return nil, fmt.Errorf("control-plane event stream unavailable")
	}
	if response.StatusCode != http.StatusOK || !strings.HasPrefix(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream") {
		response.Body.Close()
		return nil, fmt.Errorf("control-plane event stream returned HTTP %d", response.StatusCode)
	}
	return response, nil
}

func workerIdempotencyKey(agent agentRecord, operation string) string {
	seed := agent.AgentID + "|" + agent.UpdatedAt.UTC().Format(time.RFC3339Nano) + "|" + operation
	sum := sha256.Sum256([]byte(seed))
	return "agent-worker-" + hex.EncodeToString(sum[:24])
}

func workerRequestID(agent agentRecord, operation string) string {
	sum := sha256.Sum256([]byte(agent.AgentID + "|" + agent.UpdatedAt.UTC().Format(time.RFC3339Nano) + "|" + operation))
	return "req_agent_" + hex.EncodeToString(sum[:20])
}

type provisionBackend interface {
	Generate(context.Context, agentRecord) (generatedBundle, error)
	Deploy(context.Context, agentRecord, string) error
	PublishNginx(context.Context, agentRecord, string) error
	CheckDNS(context.Context, agentRecord) error
	CheckTLS(context.Context, agentRecord) error
	CheckReady(context.Context, agentRecord) error
}

type provisionWorker struct {
	cfg                  config
	api                  *controlAPI
	backend              provisionBackend
	queue                chan string
	mu                   sync.Mutex
	queued               map[string]struct{}
	leaseRefreshInterval time.Duration
}

func newWorker(cfg config, api *controlAPI, backend provisionBackend) *provisionWorker {
	return &provisionWorker{cfg: cfg, api: api, backend: backend, queue: make(chan string, 2048), queued: make(map[string]struct{}), leaseRefreshInterval: defaultLeaseRefreshInterval}
}

func (w *provisionWorker) run(ctx context.Context) error {
	if err := w.reconcile(ctx); err != nil {
		return err
	}
	go w.consume(ctx)
	go w.poll(ctx)
	go w.listen(ctx)
	<-ctx.Done()
	return ctx.Err()
}

func (w *provisionWorker) poll(ctx context.Context) {
	ticker := time.NewTicker(w.cfg.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.reconcile(ctx); err != nil {
				// Do not log API response bodies or request credentials.
				fmt.Fprintln(os.Stderr, "agent provisioning reconciliation failed")
			}
		}
	}
}

func (w *provisionWorker) listen(ctx context.Context) {
	backoff := time.Second
	for ctx.Err() == nil {
		response, err := w.api.openStream(ctx)
		if err == nil {
			backoff = time.Second
			err = consumeAgentEvents(response.Body, func(agentID string) { w.enqueue(agentID) })
			_ = response.Body.Close()
		}
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "agent provisioning event stream disconnected; polling remains active")
		}
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		if backoff < 30*time.Second {
			backoff *= 2
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
		}
	}
}

func consumeAgentEvents(reader io.Reader, enqueue func(string)) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	event := ""
	var data strings.Builder
	dispatch := func() {
		switch event {
		case "resync":
			enqueue("")
		case "agent":
			var update struct {
				AgentID string `json:"agent_id"`
			}
			if json.Unmarshal([]byte(data.String()), &update) == nil && strings.TrimSpace(update.AgentID) != "" {
				enqueue(update.AgentID)
			}
		}
		event = ""
		data.Reset()
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			dispatch()
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, ok := strings.Cut(line, ":")
		if !ok {
			field, value = line, ""
		}
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "event":
			event = value
		case "data":
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(value)
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	dispatch()
	return fmt.Errorf("agent provisioning stream closed")
}

func (w *provisionWorker) enqueue(agentID string) {
	w.mu.Lock()
	if _, exists := w.queued[agentID]; exists {
		w.mu.Unlock()
		return
	}
	w.queued[agentID] = struct{}{}
	w.mu.Unlock()
	select {
	case w.queue <- agentID:
	default:
		w.mu.Lock()
		delete(w.queued, agentID)
		w.mu.Unlock()
	}
}

func (w *provisionWorker) consume(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case agentID := <-w.queue:
			w.mu.Lock()
			delete(w.queued, agentID)
			w.mu.Unlock()
			if agentID == "" {
				if err := w.reconcile(ctx); err != nil {
					fmt.Fprintln(os.Stderr, "agent provisioning reconciliation failed")
				}
				continue
			}
			if err := w.process(ctx, agentID); err != nil {
				// Error strings are produced by this worker and never include secret values.
				fmt.Fprintln(os.Stderr, "agent provisioning job failed", agentID, err)
			}
		}
	}
}

func (w *provisionWorker) reconcile(ctx context.Context) error {
	// Failed records only resume after an explicit administrator retry. The retry
	// endpoint moves them back to pending and emits the normal change notification.
	for _, status := range []string{"pending", "provisioning"} {
		for page := 1; ; page++ {
			result, err := w.api.listAgents(ctx, status, page)
			if err != nil {
				return err
			}
			for _, agent := range result.Items {
				w.enqueue(agent.AgentID)
			}
			if len(result.Items) == 0 || page*result.PageSize >= result.Total {
				break
			}
		}
	}
	return nil
}

type workerFailure struct {
	code      string
	retryable bool
	err       error
}

func (e *workerFailure) Error() string { return e.err.Error() }
func (e *workerFailure) Unwrap() error { return e.err }

func (w *provisionWorker) process(ctx context.Context, agentID string) error {
	agent, err := w.api.getAgent(ctx, agentID)
	if err != nil {
		return err
	}
	switch agent.Status {
	case "active":
		return nil
	case "pending":
	case "failed":
		return fmt.Errorf("failed agent awaits an explicit administrator retry")
	case "provisioning":
	case "suspended", "revoked":
		return fmt.Errorf("agent is %s", agent.Status)
	default:
		return fmt.Errorf("agent has unsupported state")
	}
	if err != nil {
		return err
	}
	claim, err := w.api.claimAgent(ctx, agentID)
	if errors.Is(err, errAgentLeaseHeld) || errors.Is(err, errAgentNotClaimable) {
		return nil
	}
	if err != nil {
		return err
	}
	return w.processClaimed(ctx, claim.Agent, claim.Token)
}

func (w *provisionWorker) processClaimed(ctx context.Context, agent agentRecord, leaseToken string) error {
	leaseCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	leaseLost := make(chan struct{}, 1)
	go w.maintainLease(leaseCtx, cancel, agent.AgentID, leaseToken, leaseLost)
	if agent.Status == "pending" && agent.CurrentStep == "queued" {
		var err error
		agent, err = w.api.reportProgress(leaseCtx, agent, "validating", "", false, leaseToken)
		if err != nil {
			return err
		}
	}
	if err := w.provisionClaimed(leaseCtx, agent, leaseToken); err != nil {
		select {
		case <-leaseLost:
			return errAgentLeaseLost
		default:
			return err
		}
	}
	return nil
}

func (w *provisionWorker) maintainLease(ctx context.Context, cancel context.CancelFunc, agentID, token string, lost chan<- struct{}) {
	interval := w.leaseRefreshInterval
	if interval <= 0 {
		interval = defaultLeaseRefreshInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.api.renewLease(ctx, agentID, token); err != nil {
				select {
				case lost <- struct{}{}:
				default:
				}
				cancel()
				return
			}
		}
	}
}

func (w *provisionWorker) provisionClaimed(ctx context.Context, agent agentRecord, leaseToken string) error {
	var preparedBundle *generatedBundle
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		var err error
		var failure *workerFailure
		switch agent.CurrentStep {
		case "validating":
			bundle, generateErr := w.backend.Generate(ctx, agent)
			if generateErr == nil {
				generateErr = w.api.registerRuntimeCredentials(ctx, agent.AgentID, bundle.ControlTokenHash, bundle.ModelTokenHash, leaseToken)
			}
			if generateErr != nil {
				failure = &workerFailure{code: "bundle_generation_failed", retryable: true, err: generateErr}
			} else {
				preparedBundle = &bundle
			}
			if failure == nil {
				agent, err = w.api.reportProgress(ctx, agent, "bundle_generated", "", false, leaseToken)
			}
		case "bundle_generated":
			agent, err = w.api.reportProgress(ctx, agent, "deploying", "", false, leaseToken)
		case "deploying":
			bundle := preparedBundle
			if bundle == nil {
				generated, generateErr := w.backend.Generate(ctx, agent)
				if generateErr != nil {
					failure = &workerFailure{code: "bundle_generation_failed", retryable: true, err: generateErr}
					break
				}
				if registerErr := w.api.registerRuntimeCredentials(ctx, agent.AgentID, generated.ControlTokenHash, generated.ModelTokenHash, leaseToken); registerErr != nil {
					failure = &workerFailure{code: "bundle_generation_failed", retryable: true, err: registerErr}
					break
				}
				bundle = &generated
			}
			if deployErr := w.backend.Deploy(ctx, agent, bundle.Path); deployErr != nil {
				failure = classifyWorkerFailure(deployErr, "deployment_failed")
				break
			}
			nginxConf, readErr := os.ReadFile(filepath.Join(bundle.Path, "nginx.conf"))
			if readErr != nil {
				failure = &workerFailure{code: "deployment_failed", retryable: true, err: fmt.Errorf("deployment proxy configuration is missing")}
				break
			}
			if publishErr := w.backend.PublishNginx(ctx, agent, string(nginxConf)); publishErr != nil {
				failure = classifyWorkerFailure(publishErr, "deployment_failed")
				break
			}
			agent, err = w.api.reportProgress(ctx, agent, "domain_check", "", false, leaseToken)
		case "domain_check":
			if checkErr := w.backend.CheckDNS(ctx, agent); checkErr != nil {
				failure = &workerFailure{code: "dns_failed", retryable: true, err: checkErr}
				break
			}
			agent, err = w.api.reportProgress(ctx, agent, "tls_check", "", false, leaseToken)
		case "tls_check":
			if checkErr := w.backend.CheckTLS(ctx, agent); checkErr != nil {
				failure = &workerFailure{code: "tls_failed", retryable: true, err: checkErr}
				break
			}
			agent, err = w.api.reportProgress(ctx, agent, "readiness_check", "", false, leaseToken)
		case "readiness_check":
			if checkErr := w.backend.CheckReady(ctx, agent); checkErr != nil {
				failure = &workerFailure{code: "readiness_failed", retryable: true, err: checkErr}
				break
			}
			return w.api.activate(ctx, agent, leaseToken)
		default:
			return fmt.Errorf("agent is at an unsupported provisioning step")
		}
		if failure != nil {
			_, reportErr := w.api.reportProgress(ctx, agent, "failed", failure.code, failure.retryable, leaseToken)
			if reportErr != nil {
				return errors.Join(failure, fmt.Errorf("failed to report provisioning failure"))
			}
			return failure
		}
		if err != nil {
			return err
		}
		if agent.Status != "provisioning" || agent.CurrentStep == "" {
			return fmt.Errorf("control plane returned an invalid provisioning state")
		}
	}
}

func classifyWorkerFailure(err error, fallback string) *workerFailure {
	var failure *workerFailure
	if errors.As(err, &failure) {
		return failure
	}
	return &workerFailure{code: fallback, retryable: true, err: err}
}

type hostBackend struct{ cfg config }

func newHostBackend(cfg config) *hostBackend { return &hostBackend{cfg: cfg} }

type bundleResult struct {
	AgentID          string `json:"agent_id"`
	Slug             string `json:"slug"`
	Domain           string `json:"domain"`
	Status           string `json:"status"`
	BundlePath       string `json:"bundle_path"`
	ControlTokenHash string `json:"control_token_hash"`
	ModelTokenHash   string `json:"model_token_hash"`
}

type generatedBundle struct {
	Path             string
	ControlTokenHash string
	ModelTokenHash   string
}

func (b *hostBackend) Generate(ctx context.Context, agent agentRecord) (generatedBundle, error) {
	commandCtx, cancel := context.WithTimeout(ctx, b.cfg.ProvisionCommandLimit)
	defer cancel()
	key := "main-" + agent.RequestID
	args := []string{
		"--state-dir", b.cfg.StateDir,
		"--idempotency-key", key,
		"--agent-id", agent.AgentID,
		"--slug", agent.Slug,
		"--domain", agent.Domain,
		"--display-name", agent.DisplayName,
		"--owner-main-user-id", fmt.Sprint(agent.OwnerMainUserID),
		"--main-url", b.cfg.MainURL,
		"--image", b.cfg.Image,
	}
	output, err := exec.CommandContext(commandCtx, b.cfg.ProvisionerPath, args...).Output()
	if err != nil {
		return generatedBundle{}, fmt.Errorf("deployment bundle generation command failed")
	}
	var result bundleResult
	if err := json.Unmarshal(output, &result); err != nil || result.AgentID != agent.AgentID || result.Slug != agent.Slug || result.Domain != agent.Domain || result.Status != "generated" ||
		!isSHA256Hash(result.ControlTokenHash) || !isSHA256Hash(result.ModelTokenHash) || result.ControlTokenHash == result.ModelTokenHash {
		return generatedBundle{}, fmt.Errorf("deployment bundle generator returned invalid runtime credential metadata")
	}
	bundle, err := bundlePathWithinState(b.cfg.StateDir, result.BundlePath)
	if err != nil {
		return generatedBundle{}, err
	}
	return generatedBundle{Path: bundle, ControlTokenHash: result.ControlTokenHash, ModelTokenHash: result.ModelTokenHash}, nil
}

func bundlePathWithinState(stateDir, bundlePath string) (string, error) {
	root, err := filepath.Abs(stateDir)
	if err != nil {
		return "", fmt.Errorf("invalid provisioning state directory")
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("invalid provisioning state directory")
	}
	bundle, err := filepath.Abs(bundlePath)
	if err != nil {
		return "", fmt.Errorf("invalid deployment bundle path")
	}
	bundle, err = filepath.EvalSymlinks(bundle)
	if err != nil {
		return "", fmt.Errorf("invalid deployment bundle path")
	}
	info, err := os.Stat(bundle)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("invalid deployment bundle directory")
	}
	rel, err := filepath.Rel(root, bundle)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("deployment bundle escaped the configured state directory")
	}
	return bundle, nil
}

func (b *hostBackend) Deploy(ctx context.Context, agent agentRecord, bundle string) error {
	if err := b.ensureNetwork(ctx); err != nil {
		return &workerFailure{code: "network_unavailable", retryable: true, err: err}
	}
	composeEnv := []string{"SUB2API_SSO_SECRET_FILE=" + b.cfg.SSOSecretFile}
	if err := b.docker(ctx, nil, "network", "connect", b.cfg.EdgeNetwork, b.cfg.NginxContainer); err != nil {
		if !b.containerOnNetwork(ctx, b.cfg.NginxContainer) {
			return err
		}
	}
	ctx, cancel := context.WithTimeout(ctx, b.cfg.ProvisionCommandLimit)
	defer cancel()
	args := []string{"compose", "--project-name", "agentapi-" + agent.Slug, "--file", filepath.Join(bundle, "compose.yaml"), "up", "-d"}
	cmd := exec.CommandContext(ctx, b.cfg.DockerBin, args...)
	cmd.Dir = bundle
	cmd.Env = append(os.Environ(), composeEnv...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("agent container deployment command failed")
	}
	return nil
}

func (b *hostBackend) ensureNetwork(ctx context.Context) error {
	if err := b.docker(ctx, nil, "network", "inspect", b.cfg.EdgeNetwork); err == nil {
		return nil
	}
	if err := b.docker(ctx, nil, "network", "create", b.cfg.EdgeNetwork); err != nil {
		if inspectErr := b.docker(ctx, nil, "network", "inspect", b.cfg.EdgeNetwork); inspectErr == nil {
			return nil
		}
		return fmt.Errorf("required Docker edge network is unavailable")
	}
	return nil
}

func (b *hostBackend) containerOnNetwork(ctx context.Context, container string) bool {
	var result []struct {
		Containers map[string]struct {
			Name string `json:"Name"`
		} `json:"Containers"`
	}
	command := exec.CommandContext(ctx, b.cfg.DockerBin, "network", "inspect", b.cfg.EdgeNetwork)
	output, err := command.Output()
	if err != nil || json.Unmarshal(output, &result) != nil || len(result) == 0 {
		return false
	}
	for _, attached := range result[0].Containers {
		if attached.Name == container {
			return true
		}
	}
	return false
}

func (b *hostBackend) docker(ctx context.Context, env []string, args ...string) error {
	commandCtx, cancel := context.WithTimeout(ctx, b.cfg.ProvisionCommandLimit)
	defer cancel()
	command := exec.CommandContext(commandCtx, b.cfg.DockerBin, args...)
	if len(env) > 0 {
		command.Env = append(os.Environ(), env...)
	}
	if err := command.Run(); err != nil {
		return fmt.Errorf("Docker operation failed")
	}
	return nil
}

func (b *hostBackend) PublishNginx(ctx context.Context, agent agentRecord, body string) error {
	if !strings.Contains(body, "# managed-by-agentapi-provision-worker agent_id="+agent.AgentID) {
		return fmt.Errorf("refusing to install an unmanaged nginx configuration")
	}
	name := "agentapi-" + agent.Slug + ".conf"
	path := filepath.Join(b.cfg.NginxConfigDir, name)
	previous, readErr := os.ReadFile(path)
	if readErr == nil && !strings.Contains(string(previous), "# managed-by-agentapi-provision-worker agent_id="+agent.AgentID) {
		return fmt.Errorf("nginx configuration path is already owned by another service")
	}
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return fmt.Errorf("could not inspect existing nginx configuration")
	}
	if err := writeAtomic(path, []byte(body), 0o640); err != nil {
		return fmt.Errorf("could not install nginx configuration")
	}
	if err := b.docker(ctx, nil, "exec", b.cfg.NginxContainer, "nginx", "-t"); err != nil {
		rollbackNginxConfig(path, previous, readErr == nil)
		return fmt.Errorf("nginx configuration validation failed")
	}
	if err := b.docker(ctx, nil, "exec", b.cfg.NginxContainer, "nginx", "-s", "reload"); err != nil {
		rollbackNginxConfig(path, previous, readErr == nil)
		_ = b.docker(ctx, nil, "exec", b.cfg.NginxContainer, "nginx", "-s", "reload")
		return fmt.Errorf("nginx reload failed")
	}
	return nil
}

func rollbackNginxConfig(path string, previous []byte, existed bool) {
	if existed {
		_ = writeAtomic(path, previous, 0o640)
	} else {
		_ = os.Remove(path)
	}
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".agentapi-tmp-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	return nil
}

func (b *hostBackend) CheckDNS(_ context.Context, agent agentRecord) error {
	_, err := b.resolveEdgeIP(agent.Domain)
	return err
}

func (b *hostBackend) resolveEdgeIP(domain string) (net.IP, error) {
	if !strings.HasSuffix(strings.ToLower(domain), ".cc2.cx") {
		return nil, fmt.Errorf("agent domain is outside the managed platform zone")
	}
	ips, err := net.LookupIP(domain)
	if err != nil || len(ips) == 0 {
		return nil, fmt.Errorf("agent domain did not resolve")
	}
	wanted := make(map[string]struct{}, len(b.cfg.ExpectedEdgeIPs))
	for _, value := range b.cfg.ExpectedEdgeIPs {
		ip := net.ParseIP(value)
		if ip == nil {
			return nil, fmt.Errorf("worker has invalid expected edge IP configuration")
		}
		wanted[ip.String()] = struct{}{}
	}
	var selected net.IP
	for _, ip := range ips {
		if _, ok := wanted[ip.String()]; !ok {
			return nil, fmt.Errorf("agent domain resolves outside the configured edge IP allowlist")
		}
		if selected == nil || (selected.To4() == nil && ip.To4() != nil) {
			selected = append(net.IP(nil), ip...)
		}
	}
	if selected == nil {
		return nil, fmt.Errorf("agent domain does not resolve to a configured edge IP")
	}
	return selected, nil
}

func (b *hostBackend) CheckTLS(ctx context.Context, agent agentRecord) error {
	if _, err := b.resolveEdgeIP(agent.Domain); err != nil {
		return err
	}
	requestCtx, cancel := context.WithTimeout(ctx, b.cfg.TLSCheckTimeout)
	defer cancel()
	client := b.pinnedHealthClient(agent.Domain)
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, "https://"+agent.Domain+"/healthz", nil)
	if err != nil {
		return fmt.Errorf("agent health URL is invalid")
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("agent TLS health check failed")
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("agent TLS health endpoint returned HTTP %d", response.StatusCode)
	}
	return nil
}

func (b *hostBackend) CheckReady(ctx context.Context, agent agentRecord) error {
	if _, err := b.resolveEdgeIP(agent.Domain); err != nil {
		return err
	}
	deadline := time.Now().Add(b.cfg.ReadinessTimeout)
	client := b.pinnedHealthClient(agent.Domain)
	for time.Now().Before(deadline) {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+agent.Domain+"/readyz", nil)
		if err != nil {
			return fmt.Errorf("agent readiness URL is invalid")
		}
		response, err := client.Do(request)
		if err == nil {
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return nil
			}
		}
		timer := time.NewTimer(2 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return fmt.Errorf("agent readiness check timed out")
}

func (b *hostBackend) pinnedHealthClient(domain string) *http.Client {
	return &http.Client{
		Timeout:       b.cfg.TLSCheckTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{ServerName: domain, MinVersion: tls.VersionTLS12},
			DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
				if !strings.EqualFold(address, net.JoinHostPort(domain, "443")) {
					return nil, fmt.Errorf("health checker refused unexpected dial target")
				}
				ip, err := b.resolveEdgeIP(domain)
				if err != nil {
					return nil, err
				}
				return (&net.Dialer{Timeout: b.cfg.TLSCheckTimeout}).DialContext(ctx, network, net.JoinHostPort(ip.String(), "443"))
			},
		},
	}
}
