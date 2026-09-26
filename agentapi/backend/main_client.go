package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// MainClient is the only component allowed to talk to the Sub2API process. It
// deliberately has no database access and keeps per-Agent credentials server-
// side, so they can never be serialized into a browser response.
type MainClient struct {
	cfg         Config
	http        *http.Client
	streamHTTP  *http.Client
	controlHTTP *http.Client
}

type MainAPIError struct {
	Status  int
	Code    string
	Message string
	Body    []byte
}

func (e *MainAPIError) Error() string {
	if e == nil {
		return "main api error"
	}
	if e.Code != "" {
		return fmt.Sprintf("main api error: %s (%d): %s", e.Code, e.Status, e.Message)
	}
	return fmt.Sprintf("main api error (%d): %s", e.Status, e.Message)
}

type mainEnvelope struct {
	Code     json.RawMessage `json:"code"`
	Message  string          `json:"message"`
	Reason   string          `json:"reason"`
	Metadata map[string]any  `json:"metadata"`
	Data     json.RawMessage `json:"data"`
}

type MainAuthResult struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
	User         json.RawMessage
	Requires2FA  bool
	TempToken    string
	RawData      json.RawMessage
}

type MainUserResult struct {
	ID      string
	Email   string
	Balance int64
	Raw     json.RawMessage
}

type MainProvisioningAgent struct {
	AgentID string `json:"agent_id"`
	Status  string `json:"status"`
}

type MainUsageResult struct {
	ID            string
	RequestID     string
	TotalCents    int64
	ActualCents   int64
	HasActualCost bool
	Model         string
	Snapshot      MainUsageSnapshot
	Raw           json.RawMessage
}

// MainUsageSnapshot contains only user-safe facts from Sub2API's admin usage
// DTO. Keep credentials, IP addresses, user/account objects, and other admin
// metadata out of the AgentAPI usage response.
type MainUsageSnapshot struct {
	Source                    string           `json:"usage_source,omitempty"`
	ServiceTier               string           `json:"service_tier,omitempty"`
	ReasoningEffort           string           `json:"reasoning_effort,omitempty"`
	InboundEndpoint           string           `json:"inbound_endpoint,omitempty"`
	InputTokens               int64            `json:"input_tokens"`
	OutputTokens              int64            `json:"output_tokens"`
	CacheCreationTokens       int64            `json:"cache_creation_tokens"`
	CacheReadTokens           int64            `json:"cache_read_tokens"`
	CacheCreation5mTokens     int64            `json:"cache_creation_5m_tokens"`
	CacheCreation1hTokens     int64            `json:"cache_creation_1h_tokens"`
	InputCostNanos            int64            `json:"input_cost_usd_nanos"`
	OutputCostNanos           int64            `json:"output_cost_usd_nanos"`
	CacheCreationCostNanos    int64            `json:"cache_creation_cost_usd_nanos"`
	CacheReadCostNanos        int64            `json:"cache_read_cost_usd_nanos"`
	TotalCostNanos            int64            `json:"total_cost_usd_nanos"`
	ActualCostNanos           int64            `json:"actual_cost_usd_nanos"`
	ActualCostReported        bool             `json:"actual_cost_reported"`
	RateMultiplier            float64          `json:"rate_multiplier"`
	LongContextBillingApplied bool             `json:"long_context_billing_applied"`
	ImageCount                int64            `json:"image_count"`
	ImageInputTokens          int64            `json:"image_input_tokens"`
	ImageInputCostNanos       int64            `json:"image_input_cost_usd_nanos"`
	ImageOutputTokens         int64            `json:"image_output_tokens"`
	ImageOutputCostNanos      int64            `json:"image_output_cost_usd_nanos"`
	RequestedModel            string           `json:"-"`
	UpstreamModel             string           `json:"upstream_model,omitempty"`
	UpstreamResponseModel     string           `json:"upstream_response_model,omitempty"`
	UpstreamModelMismatch     *bool            `json:"upstream_model_mismatch,omitempty"`
	RequestType               string           `json:"request_type,omitempty"`
	BillingMode               string           `json:"billing_mode,omitempty"`
	BillingType               int64            `json:"billing_type"`
	OpenAIWSMode              bool             `json:"openai_ws_mode"`
	NativeCompactionV2        bool             `json:"native_compaction_v2"`
	DurationMs                int64            `json:"duration_ms"`
	FirstTokenMs              int64            `json:"first_token_ms"`
	Stream                    bool             `json:"stream"`
	ImageSize                 string           `json:"image_size,omitempty"`
	ImageInputSize            string           `json:"image_input_size,omitempty"`
	ImageOutputSize           string           `json:"image_output_size,omitempty"`
	ImageSizeSource           string           `json:"image_size_source,omitempty"`
	ImageSizeBreakdown        map[string]int64 `json:"image_size_breakdown,omitempty"`
	MediaType                 string           `json:"media_type,omitempty"`
	CacheTTLOverridden        bool             `json:"cache_ttl_overridden"`
}

func NewMainClient(cfg Config) *MainClient {
	// All of these clients carry credentials or identity-scoping headers. Do
	// not let a server redirect a request to a different origin (or even to a
	// different endpoint) with those headers attached.
	rejectRedirects := func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	var controlTransport http.RoundTripper = http.DefaultTransport
	if defaultTransport, ok := http.DefaultTransport.(*http.Transport); ok {
		transport := defaultTransport.Clone()
		transport.ResponseHeaderTimeout = cfg.MainRequestTimeout
		controlTransport = transport
	}
	return &MainClient{
		cfg:         cfg,
		http:        &http.Client{Timeout: cfg.MainRequestTimeout, CheckRedirect: rejectRedirects},
		streamHTTP:  &http.Client{Timeout: cfg.ModelStreamTimeout, CheckRedirect: rejectRedirects},
		controlHTTP: &http.Client{Transport: controlTransport, CheckRedirect: rejectRedirects},
	}
}

func (c *MainClient) endpoint(path string) string {
	return strings.TrimRight(c.cfg.MainAPIBaseURL, "/") + "/" + strings.TrimLeft(path, "/")
}

func (c *MainClient) request(ctx context.Context, method, target string, body []byte, headers http.Header) (*http.Response, []byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	for key, values := range headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	data, readErr := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if readErr != nil {
		return resp, nil, readErr
	}
	return resp, data, nil
}

func (c *MainClient) jsonRequest(ctx context.Context, method, path string, payload any, token string) (json.RawMessage, error) {
	var body []byte
	var err error
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			return nil, err
		}
	}
	headers := make(http.Header)
	headers.Set("Accept", "application/json")
	if payload != nil {
		headers.Set("Content-Type", "application/json")
	}
	if token != "" {
		headers.Set("Authorization", "Bearer "+token)
	}
	resp, data, err := c.request(ctx, method, c.endpoint(path), body, headers)
	if err != nil {
		return nil, err
	}
	return unwrapMainResponse(resp.StatusCode, data)
}

func unwrapMainResponse(status int, body []byte) (json.RawMessage, error) {
	var envelope mainEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		if status >= 200 && status < 300 {
			return json.RawMessage(body), nil
		}
		return nil, &MainAPIError{Status: status, Message: http.StatusText(status), Body: body}
	}
	if status < 200 || status >= 300 || (len(envelope.Code) > 0 && string(envelope.Code) != "0" && string(envelope.Code) != "null") {
		code := ""
		if envelope.Reason != "" {
			code = envelope.Reason
		} else if len(envelope.Code) > 0 {
			code = strings.Trim(string(envelope.Code), `"`)
		}
		message := strings.TrimSpace(envelope.Message)
		if message == "" {
			message = http.StatusText(status)
		}
		return nil, &MainAPIError{Status: status, Code: code, Message: message, Body: body}
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return json.RawMessage(`{}`), nil
	}
	return envelope.Data, nil
}

func (c *MainClient) Login(ctx context.Context, email, password string) (MainAuthResult, error) {
	data, err := c.jsonRequest(ctx, http.MethodPost, "/auth/login", map[string]any{
		"email": email, "password": password,
	}, "")
	if err != nil {
		return MainAuthResult{}, err
	}
	return decodeAuthResult(data)
}

func (c *MainClient) Register(ctx context.Context, payload map[string]any) (MainAuthResult, error) {
	data, err := c.jsonRequest(ctx, http.MethodPost, "/auth/register", payload, "")
	if err != nil {
		return MainAuthResult{}, err
	}
	return decodeAuthResult(data)
}

func (c *MainClient) Login2FA(ctx context.Context, tempToken, code string) (MainAuthResult, error) {
	data, err := c.jsonRequest(ctx, http.MethodPost, "/auth/login/2fa", map[string]any{
		"temp_token": tempToken, "totp_code": code,
	}, "")
	if err != nil {
		return MainAuthResult{}, err
	}
	return decodeAuthResult(data)
}

func (c *MainClient) Refresh(ctx context.Context, refreshToken string) (MainAuthResult, error) {
	data, err := c.jsonRequest(ctx, http.MethodPost, "/auth/refresh", map[string]any{
		"refresh_token": refreshToken,
	}, "")
	if err != nil {
		return MainAuthResult{}, err
	}
	return decodeAuthResult(data)
}

func (c *MainClient) CurrentUser(ctx context.Context, accessToken string) (json.RawMessage, error) {
	return c.jsonRequest(ctx, http.MethodGet, "/auth/me", nil, accessToken)
}

// UserProfile reads the authenticated user's public profile from Sub2API.
// The caller must filter the response before returning it to an AgentAPI
// browser; the upstream profile DTO contains fields that do not belong in the
// satellite UI.
func (c *MainClient) UserProfile(ctx context.Context, accessToken string) (json.RawMessage, error) {
	return c.jsonRequest(ctx, http.MethodGet, "/user/profile", nil, accessToken)
}

func (c *MainClient) UpdateUserProfile(ctx context.Context, accessToken, username string) (json.RawMessage, error) {
	return c.jsonRequest(ctx, http.MethodPut, "/user", map[string]string{"username": username}, accessToken)
}

func (c *MainClient) ChangeUserPassword(ctx context.Context, accessToken, oldPassword, newPassword string) error {
	_, err := c.jsonRequest(ctx, http.MethodPut, "/user/password", map[string]string{
		"old_password": oldPassword,
		"new_password": newPassword,
	}, accessToken)
	return err
}

func (c *MainClient) Logout(ctx context.Context, refreshToken string) error {
	if refreshToken == "" {
		return nil
	}
	_, err := c.jsonRequest(ctx, http.MethodPost, "/auth/logout", map[string]any{
		"refresh_token": refreshToken,
	}, "")
	return err
}

func (c *MainClient) runtimeJSON(ctx context.Context, method, path string, payload any) (json.RawMessage, error) {
	return c.runtimeJSONWithIdentityProof(ctx, method, path, payload, "", "")
}

func (c *MainClient) runtimeJSONWithIdentityProof(ctx context.Context, method, path string, payload any, userAccessToken, ssoTicket string) (json.RawMessage, error) {
	credential := strings.TrimSpace(c.cfg.RuntimeControlCredential)
	if !strings.HasPrefix(credential, "agt_ctl_") || len(credential) < 40 || len(credential) > 256 {
		return nil, &MainAPIError{Status: http.StatusServiceUnavailable, Code: "AGENT_RUNTIME_CONTROL_CREDENTIAL_MISSING", Message: "per-Agent control credential is not configured"}
	}
	userAccessToken = strings.TrimSpace(userAccessToken)
	ssoTicket = strings.TrimSpace(ssoTicket)
	if userAccessToken != "" && (len(userAccessToken) > 8192 || strings.ContainsAny(userAccessToken, "\r\n")) {
		return nil, fmt.Errorf("main user access token is invalid")
	}
	if ssoTicket != "" && (len(ssoTicket) > 8192 || strings.ContainsAny(ssoTicket, "\r\n")) {
		return nil, fmt.Errorf("main SSO identity ticket is invalid")
	}
	if userAccessToken != "" && ssoTicket != "" {
		return nil, fmt.Errorf("multiple main user identity proofs are not allowed")
	}
	var body []byte
	var err error
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			return nil, err
		}
	}
	headers := make(http.Header)
	headers.Set("Accept", "application/json")
	headers.Set("X-AgentAPI-Runtime-Control", credential)
	if userAccessToken != "" {
		headers.Set("Authorization", "Bearer "+userAccessToken)
	}
	if ssoTicket != "" {
		headers.Set("X-AgentAPI-SSO-Ticket", ssoTicket)
	}
	if payload != nil {
		headers.Set("Content-Type", "application/json")
	}
	resp, data, err := c.request(ctx, method, c.endpoint(path), body, headers)
	if err != nil {
		return nil, err
	}
	return unwrapMainResponse(resp.StatusCode, data)
}

func (c *MainClient) RuntimeGetOwner(ctx context.Context) (MainUserResult, error) {
	data, err := c.runtimeJSON(ctx, http.MethodGet, "/agent-runtime/owner", nil)
	if err != nil {
		return MainUserResult{}, err
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return MainUserResult{}, err
	}
	return MainUserResult{ID: jsonID(raw["id"]), Email: stringValue(raw["email"]), Balance: moneyValue(raw["balance"]), Raw: data}, nil
}

func (c *MainClient) MapRuntimeUser(ctx context.Context, mainUserID, userAccessToken string) error {
	mainUserID = strings.TrimSpace(mainUserID)
	if _, err := strconv.ParseInt(mainUserID, 10, 64); err != nil || mainUserID == "" {
		return fmt.Errorf("main user id is invalid")
	}
	if strings.TrimSpace(userAccessToken) == "" {
		return fmt.Errorf("main user access token is required")
	}
	_, err := c.runtimeJSONWithIdentityProof(ctx, http.MethodPost, "/agent-runtime/users/"+url.PathEscape(mainUserID)+"/map", map[string]any{}, userAccessToken, "")
	return err
}

func (c *MainClient) MapRuntimeUserWithSSOTicket(ctx context.Context, mainUserID, ticket string) error {
	mainUserID = strings.TrimSpace(mainUserID)
	if _, err := strconv.ParseInt(mainUserID, 10, 64); err != nil || mainUserID == "" {
		return fmt.Errorf("main user id is invalid")
	}
	if strings.TrimSpace(ticket) == "" {
		return fmt.Errorf("main SSO identity ticket is required")
	}
	_, err := c.runtimeJSONWithIdentityProof(ctx, http.MethodPost, "/agent-runtime/users/"+url.PathEscape(mainUserID)+"/map", map[string]any{}, "", ticket)
	return err
}

// AdminGetUser is a compatibility wrapper; the runtime API can return only
// the configured Owner, never an arbitrary main-site user.
func (c *MainClient) AdminGetUser(ctx context.Context, mainUserID string) (MainUserResult, error) {
	if strings.TrimSpace(mainUserID) != strings.TrimSpace(c.cfg.OwnerMainUserID) {
		return MainUserResult{}, &MainAPIError{Status: http.StatusForbidden, Code: "AGENT_RUNTIME_SCOPE_MISMATCH", Message: "runtime credential is restricted to its configured Owner"}
	}
	return c.RuntimeGetOwner(ctx)
}

func (c *MainClient) RuntimeUpdateUserStatus(ctx context.Context, mainUserID, status string) error {
	mainUserID, status = strings.TrimSpace(mainUserID), strings.TrimSpace(status)
	if _, err := strconv.ParseInt(mainUserID, 10, 64); err != nil || mainUserID == "" {
		return fmt.Errorf("main user id is invalid")
	}
	if status != "active" && status != "disabled" {
		return fmt.Errorf("unsupported main user status")
	}
	data, err := c.runtimeJSON(ctx, http.MethodPatch, "/agent-runtime/users/"+url.PathEscape(mainUserID)+"/status", map[string]string{"status": status})
	if err != nil {
		return err
	}
	var updated struct {
		UserID json.RawMessage `json:"user_id"`
		Status string          `json:"status"`
	}
	if err := json.Unmarshal(data, &updated); err != nil {
		return fmt.Errorf("decode updated Agent user status: %w", err)
	}
	updatedUserID := strings.TrimSpace(string(updated.UserID))
	if err := json.Unmarshal(updated.UserID, &updatedUserID); err != nil {
		// Sub2API returns its numeric user ID as a JSON number, while some
		// compatible deployments serialize IDs as strings.
		updatedUserID = strings.TrimSpace(string(updated.UserID))
	}
	if strings.TrimSpace(updatedUserID) != mainUserID || strings.TrimSpace(updated.Status) != status {
		return fmt.Errorf("main site did not confirm the requested user status")
	}
	return nil
}

func (c *MainClient) RuntimeUpdateModelAllowlist(ctx context.Context, enabled []string) error {
	models := make([]string, len(enabled))
	copy(models, enabled)
	data, err := c.runtimeJSON(ctx, http.MethodPut, "/agent-runtime/model-policy", map[string][]string{"enabled": models})
	if err != nil {
		return err
	}
	var result struct {
		AgentID string   `json:"agent_id"`
		Enabled []string `json:"enabled"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("decode main-site model scope confirmation: %w", err)
	}
	if strings.TrimSpace(c.cfg.AgentID) == "" || strings.TrimSpace(result.AgentID) != strings.TrimSpace(c.cfg.AgentID) || !sameStringSet(result.Enabled, models) {
		return fmt.Errorf("main site did not confirm the requested per-Agent model scope")
	}
	return nil
}

func sameStringSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	counts := make(map[string]int, len(left))
	for _, value := range left {
		counts[value]++
	}
	for _, value := range right {
		if counts[value] == 0 {
			return false
		}
		counts[value]--
	}
	return true
}

func sameStringSlice(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func (c *MainClient) AdminUpdateUserStatus(ctx context.Context, mainUserID, status string) error {
	return c.RuntimeUpdateUserStatus(ctx, mainUserID, status)
}

func (c *MainClient) RuntimeGetProvisioningAgent(ctx context.Context, agentID string) (MainProvisioningAgent, error) {
	if strings.TrimSpace(agentID) != strings.TrimSpace(c.cfg.AgentID) {
		return MainProvisioningAgent{}, &MainAPIError{Status: http.StatusForbidden, Code: "AGENT_RUNTIME_SCOPE_MISMATCH", Message: "runtime credential is restricted to its own Agent"}
	}
	data, err := c.runtimeJSON(ctx, http.MethodGet, "/agent-runtime/agent", nil)
	if err != nil {
		return MainProvisioningAgent{}, err
	}
	var result MainProvisioningAgent
	if err := json.Unmarshal(data, &result); err != nil {
		return MainProvisioningAgent{}, err
	}
	if result.AgentID != strings.TrimSpace(agentID) || strings.TrimSpace(result.Status) == "" {
		return MainProvisioningAgent{}, fmt.Errorf("main site returned an invalid Agent runtime record")
	}
	return result, nil
}

// RuntimeAgentUpdates subscribes to ID-only wake-up events for this Agent.
// Every event requires a fresh GET /agent; the event stream is never treated
// as an authoritative state source.
func (c *MainClient) RuntimeAgentUpdates(ctx context.Context) (<-chan struct{}, error) {
	credential := strings.TrimSpace(c.cfg.RuntimeControlCredential)
	if !strings.HasPrefix(credential, "agt_ctl_") || len(credential) < 40 || len(credential) > 256 {
		return nil, &MainAPIError{Status: http.StatusServiceUnavailable, Code: "AGENT_RUNTIME_CONTROL_CREDENTIAL_MISSING", Message: "per-Agent control credential is not configured"}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint("/agent-runtime/agent/stream"), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "text/event-stream")
	request.Header.Set("X-AgentAPI-Runtime-Control", credential)
	client := c.controlHTTP
	if client == nil {
		client = http.DefaultClient
	}
	safeClient := *client
	safeClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	response, err := safeClient.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		_ = response.Body.Close()
		_, apiErr := unwrapMainResponse(response.StatusCode, body)
		return nil, apiErr
	}
	if !strings.HasPrefix(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream") {
		_ = response.Body.Close()
		return nil, &MainAPIError{Status: response.StatusCode, Code: "AGENT_RUNTIME_STREAM_INVALID", Message: "main site did not return an event stream"}
	}
	updates := make(chan struct{}, 1)
	go func() {
		defer close(updates)
		defer response.Body.Close()
		scanner := bufio.NewScanner(response.Body)
		scanner.Buffer(make([]byte, 4096), 256<<10)
		eventName := ""
		for scanner.Scan() {
			line := strings.TrimSuffix(scanner.Text(), "\r")
			if line == "" {
				if eventName == "agent" || eventName == "resync" {
					select {
					case updates <- struct{}{}:
					default:
					}
				}
				eventName = ""
				continue
			}
			if strings.HasPrefix(line, "event:") {
				eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			}
		}
	}()
	return updates, nil
}

func (c *MainClient) AdminGetProvisioningAgent(ctx context.Context, agentID string) (MainProvisioningAgent, error) {
	return c.RuntimeGetProvisioningAgent(ctx, agentID)
}

// AdminFindUsage is a compatibility wrapper around the Owner-scoped runtime
// usage query. Sub2API applies the Owner filter again server-side.
func (c *MainClient) AdminFindUsage(ctx context.Context, requestID string) ([]MainUsageResult, error) {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return nil, fmt.Errorf("request id is required")
	}
	path := "/agent-runtime/usage?request_id=" + url.QueryEscape(requestID)
	data, err := c.runtimeJSON(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var envelope struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(data, &envelope); err == nil && envelope.Items != nil {
		return decodeMainUsageItems(envelope.Items)
	}
	var items []json.RawMessage
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	return decodeMainUsageItems(items)
}

func decodeMainUsageItems(items []json.RawMessage) ([]MainUsageResult, error) {
	result := make([]MainUsageResult, 0, len(items))
	for _, rawItem := range items {
		var item map[string]json.RawMessage
		if err := json.Unmarshal(rawItem, &item); err != nil {
			return nil, err
		}
		requestID := rawString(item["request_id"])
		if requestID == "" {
			continue
		}
		actualCostRaw, actualCostPresent := item["actual_cost"]
		actualCost, actualCostValid := decimalNanos(actualCostRaw)
		totalCostRaw, totalCostPresent := item["total_cost"]
		totalCost, totalCostValid := decimalNanos(totalCostRaw)
		if actualCostPresent && !actualCostValid {
			return nil, fmt.Errorf("main usage %q has an invalid actual_cost", requestID)
		}
		if !actualCostPresent && (!totalCostPresent || !totalCostValid) {
			return nil, fmt.Errorf("main usage %q has neither a valid actual_cost nor a fallback total_cost", requestID)
		}
		var snapshot MainUsageSnapshot
		snapshot.Source = "sub2api_owner_runtime_usage"
		snapshot.ServiceTier = rawString(item["service_tier"])
		snapshot.ReasoningEffort = rawString(item["reasoning_effort"])
		snapshot.InboundEndpoint = rawString(item["inbound_endpoint"])
		snapshot.RequestedModel = rawString(item["model"])
		snapshot.InputTokens = rawInt64(item["input_tokens"])
		snapshot.OutputTokens = rawInt64(item["output_tokens"])
		snapshot.CacheCreationTokens = rawInt64(item["cache_creation_tokens"])
		snapshot.CacheReadTokens = rawInt64(item["cache_read_tokens"])
		snapshot.CacheCreation5mTokens = rawInt64(item["cache_creation_5m_tokens"])
		snapshot.CacheCreation1hTokens = rawInt64(item["cache_creation_1h_tokens"])
		snapshot.InputCostNanos, _ = decimalNanos(item["input_cost"])
		snapshot.OutputCostNanos, _ = decimalNanos(item["output_cost"])
		snapshot.CacheCreationCostNanos, _ = decimalNanos(item["cache_creation_cost"])
		snapshot.CacheReadCostNanos, _ = decimalNanos(item["cache_read_cost"])
		snapshot.TotalCostNanos = totalCost
		snapshot.ActualCostNanos = actualCost
		snapshot.ActualCostReported = actualCostPresent
		snapshot.RateMultiplier = rawFloat64(item["rate_multiplier"])
		snapshot.LongContextBillingApplied = rawBool(item["long_context_billing_applied"])
		snapshot.ImageCount = rawInt64(item["image_count"])
		snapshot.ImageInputTokens = rawInt64(item["image_input_tokens"])
		snapshot.ImageInputCostNanos, _ = decimalNanos(item["image_input_cost"])
		snapshot.ImageOutputTokens = rawInt64(item["image_output_tokens"])
		snapshot.ImageOutputCostNanos, _ = decimalNanos(item["image_output_cost"])
		snapshot.UpstreamModel = rawString(item["upstream_model"])
		snapshot.UpstreamResponseModel = rawString(item["upstream_response_model"])
		snapshot.UpstreamModelMismatch = rawBoolPointer(item["upstream_model_mismatch"])
		snapshot.RequestType = rawString(item["request_type"])
		snapshot.BillingMode = rawString(item["billing_mode"])
		snapshot.BillingType = rawInt64(item["billing_type"])
		snapshot.OpenAIWSMode = rawBool(item["openai_ws_mode"])
		snapshot.NativeCompactionV2 = rawBool(item["native_compaction_v2"])
		snapshot.DurationMs = rawInt64(item["duration_ms"])
		snapshot.FirstTokenMs = rawInt64(item["first_token_ms"])
		snapshot.Stream = rawBool(item["stream"])
		snapshot.ImageSize = rawString(item["image_size"])
		snapshot.ImageInputSize = rawString(item["image_input_size"])
		snapshot.ImageOutputSize = rawString(item["image_output_size"])
		snapshot.ImageSizeSource = rawString(item["image_size_source"])
		snapshot.ImageSizeBreakdown = rawInt64Map(item["image_size_breakdown"])
		snapshot.MediaType = rawString(item["media_type"])
		snapshot.CacheTTLOverridden = rawBool(item["cache_ttl_overridden"])
		settlementNanos := actualCost
		if !actualCostPresent {
			settlementNanos = totalCost
		}
		result = append(result, MainUsageResult{
			ID:            jsonID(rawJSONAny(item["id"])),
			RequestID:     requestID,
			TotalCents:    nanosToCents(totalCost),
			ActualCents:   nanosToCents(settlementNanos),
			HasActualCost: actualCostPresent,
			Model:         snapshot.RequestedModel,
			Snapshot:      snapshot,
			Raw:           append(json.RawMessage(nil), rawItem...),
		})
	}
	return result, nil
}

func rawJSONAny(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil {
		return nil
	}
	return value
}

func rawInt64(raw json.RawMessage) int64 {
	if len(raw) == 0 {
		return 0
	}
	var value json.Number
	if json.Unmarshal(raw, &value) == nil {
		parsed, _ := strconv.ParseInt(value.String(), 10, 64)
		if parsed != 0 {
			return parsed
		}
		floatValue, _ := strconv.ParseFloat(value.String(), 64)
		return int64(floatValue)
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		parsed, _ := strconv.ParseInt(text, 10, 64)
		return parsed
	}
	return 0
}

func rawBoolPointer(raw json.RawMessage) *bool {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var value bool
	if json.Unmarshal(raw, &value) != nil {
		return nil
	}
	return &value
}

func rawFloat64(raw json.RawMessage) float64 {
	if len(raw) == 0 {
		return 0
	}
	var number json.Number
	if json.Unmarshal(raw, &number) == nil {
		value, _ := strconv.ParseFloat(number.String(), 64)
		return value
	}
	return 0
}

func rawInt64Map(raw json.RawMessage) map[string]int64 {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var values map[string]json.RawMessage
	if json.Unmarshal(raw, &values) != nil {
		return nil
	}
	result := make(map[string]int64, len(values))
	for key, value := range values {
		result[key] = rawInt64(value)
	}
	return result
}

// decimalNanos converts a USD JSON number into integer nano-USD without first
// rounding it to cents. This preserves the precision of per-token cost fields.
func decimalNanos(raw json.RawMessage) (int64, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, false
	}
	var text string
	if raw[0] == '"' {
		if json.Unmarshal(raw, &text) != nil {
			return 0, false
		}
	} else {
		text = string(raw)
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value > float64(math.MaxInt64)/1e9 || value < float64(math.MinInt64)/1e9 {
		return 0, false
	}
	return int64(math.Round(value * 1e9)), true
}

func nanosToCents(nanos int64) int64 {
	if nanos >= 0 {
		return (nanos + 5_000_000) / 10_000_000
	}
	return (nanos - 5_000_000) / 10_000_000
}

func mustJSON(value any) json.RawMessage {
	data, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return data
}

// RelayModel creates a request to the main /v1 gateway. It returns a fully
// buffered response so the caller can settle the local ledger after a stream
// finishes and so upstream Set-Cookie headers can never reach the browser.
func (c *MainClient) RelayModel(ctx context.Context, path string, query url.Values, body []byte, mainUserID, requestID string) (int, http.Header, []byte, error) {
	return c.RelayModelMethod(ctx, http.MethodPost, path, query, body, mainUserID, requestID)
}

// RelayModelMethod forwards the client's HTTP method while injecting the
// server-side application credential and on-behalf-of identity. Keeping the
// method parameter here means GET endpoints such as /v1/models and video
// polling work correctly without exposing the upstream credential.
func (c *MainClient) RelayModelMethod(ctx context.Context, method, path string, query url.Values, body []byte, mainUserID, requestID string) (int, http.Header, []byte, error) {
	return c.relayModelMethod(ctx, method, path, query, body, "", mainUserID, requestID)
}

// RelayModelMethodWithContentType is the model relay variant used by the HTTP
// handler when the client sends a multipart request. Keeping the original
// media type (including its boundary) lets image-edit and other upload
// endpoints be parsed by Sub2API; all authentication and identity headers are
// still generated exclusively on the server.
func (c *MainClient) RelayModelMethodWithContentType(ctx context.Context, method, path string, query url.Values, body []byte, contentType, mainUserID, requestID string) (int, http.Header, []byte, error) {
	return c.relayModelMethod(ctx, method, path, query, body, contentType, mainUserID, requestID)
}

func (c *MainClient) relayModelMethod(ctx context.Context, method, path string, query url.Values, body []byte, contentType, mainUserID, requestID string) (int, http.Header, []byte, error) {
	resp, err := c.OpenModelResponse(ctx, method, path, query, body, contentType, mainUserID, requestID)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return resp.StatusCode, filteredResponseHeaders(resp.Header), nil, err
	}
	return resp.StatusCode, filteredResponseHeaders(resp.Header), data, nil
}

// OpenModelResponse starts a model request without buffering its response.
// The caller owns resp.Body and must close it. It is used for explicit
// streaming requests; buffered callers should use RelayModelMethodWithContentType.
func (c *MainClient) OpenModelResponse(ctx context.Context, method, path string, query url.Values, body []byte, contentType, mainUserID, requestID string) (*http.Response, error) {
	target := strings.TrimRight(c.cfg.MainModelBaseURL, "/")
	path = strings.TrimPrefix(path, "/v1")
	if path == "" {
		path = "/"
	}
	target += "/" + strings.TrimLeft(path, "/")
	if encoded := query.Encode(); encoded != "" {
		target += "?" + encoded
	}
	headers := make(http.Header)
	headers.Set("Accept", "application/json, text/event-stream")
	contentType = strings.TrimSpace(contentType)
	if contentType == "" {
		contentType = "application/json"
	}
	headers.Set("Content-Type", contentType)
	if c.cfg.AppCredential == "" {
		return nil, &MainAPIError{Status: http.StatusServiceUnavailable, Code: "MAIN_APP_CREDENTIAL_MISSING", Message: "model relay credential is not configured"}
	}
	if strings.TrimSpace(mainUserID) == "" {
		return nil, &MainAPIError{Status: http.StatusServiceUnavailable, Code: "MAIN_USER_MISSING", Message: "mapped Agent user is not configured"}
	}
	headers.Set("Authorization", "Bearer "+c.cfg.AppCredential)
	// Keep the public satellite identity tied to the current Agent Session. The
	// Sub2API Agent model credential separately resolves the Owner billing key
	// after it verifies this user is an active mapping for the same Agent.
	headers.Set("X-Sub2API-On-Behalf-Of", mainUserID)
	headers.Set("X-Sub2API-Satellite", c.cfg.SatelliteSlug)
	if requestID != "" {
		headers.Set("X-Request-ID", requestID)
	}
	if method == "" {
		method = http.MethodPost
	}
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	for key, values := range headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	return c.streamHTTP.Do(req)
}

func filteredResponseHeaders(source http.Header) http.Header {
	result := make(http.Header)
	for key, values := range source {
		lower := strings.ToLower(key)
		if lower == "set-cookie" || lower == "authorization" || strings.HasPrefix(lower, "x-sub2api-") {
			continue
		}
		for _, value := range values {
			result.Add(key, value)
		}
	}
	return result
}

func decodeAuthResult(data json.RawMessage) (MainAuthResult, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return MainAuthResult{}, err
	}
	result := MainAuthResult{RawData: append(json.RawMessage(nil), data...)}
	result.AccessToken = rawString(raw["access_token"])
	result.RefreshToken = rawString(raw["refresh_token"])
	result.ExpiresIn = rawInt(raw["expires_in"])
	result.User = raw["user"]
	result.Requires2FA = rawBool(raw["requires_2fa"])
	result.TempToken = rawString(raw["temp_token"])
	if result.User == nil {
		result.User = json.RawMessage(`{}`)
	}
	return result, nil
}

func rawString(raw json.RawMessage) string {
	var value string
	_ = json.Unmarshal(raw, &value)
	return value
}

func rawInt(raw json.RawMessage) int {
	var value int
	if json.Unmarshal(raw, &value) == nil {
		return value
	}
	var number float64
	if json.Unmarshal(raw, &number) == nil {
		return int(number)
	}
	return 0
}

func rawBool(raw json.RawMessage) bool {
	var value bool
	_ = json.Unmarshal(raw, &value)
	return value
}

func jsonID(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case float64:
		return strconv.FormatInt(int64(typed), 10)
	case json.Number:
		return typed.String()
	case int64:
		return strconv.FormatInt(typed, 10)
	case int:
		return strconv.Itoa(typed)
	default:
		return ""
	}
}

func stringValue(value any) string {
	if typed, ok := value.(string); ok {
		return strings.TrimSpace(typed)
	}
	return ""
}

func moneyValue(value any) int64 {
	switch typed := value.(type) {
	case float64:
		return int64(typed*100 + 0.5)
	case json.Number:
		parsed, _ := strconv.ParseFloat(typed.String(), 64)
		return int64(parsed*100 + 0.5)
	case string:
		parsed, err := parseCents(typed)
		if err == nil {
			return parsed
		}
	}
	return 0
}

func mainUserIDFromJSON(data []byte) string {
	var value map[string]any
	if json.Unmarshal(data, &value) != nil {
		return ""
	}
	for _, key := range []string{"id", "user_id", "uid"} {
		if id := jsonID(value[key]); id != "" {
			return id
		}
	}
	return ""
}

func isTransientMainError(err error) bool {
	var apiErr *MainAPIError
	if errors.As(err, &apiErr) {
		return apiErr.Status == 429 || apiErr.Status >= 500
	}
	return true
}

func contextWithMainTimeout(ctx context.Context, cfg Config) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(ctx, cfg.MainRequestTimeout)
}
