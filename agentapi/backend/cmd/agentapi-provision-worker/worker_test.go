package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const testAgentID = "agt_0123456789abcdef0123456789abcdef"
const testLeaseToken = "worker-provisioning-lease-token-0123456789"

type fakeProvisionBackend struct {
	mu         sync.Mutex
	steps      []string
	bundlePath string
	failAt     string
	failure    error
}

func (b *fakeProvisionBackend) add(step string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.steps = append(b.steps, step)
	if step == b.failAt {
		return b.failure
	}
	return nil
}

func (b *fakeProvisionBackend) Generate(context.Context, agentRecord) (generatedBundle, error) {
	return generatedBundle{Path: b.bundlePath, ControlTokenHash: strings.Repeat("1", 64), ModelTokenHash: strings.Repeat("2", 64)}, b.add("generate")
}
func (b *fakeProvisionBackend) Deploy(context.Context, agentRecord, string) error {
	return b.add("deploy")
}
func (b *fakeProvisionBackend) PublishNginx(context.Context, agentRecord, string) error {
	return b.add("nginx")
}
func (b *fakeProvisionBackend) CheckDNS(context.Context, agentRecord) error {
	return b.add("dns")
}
func (b *fakeProvisionBackend) CheckTLS(context.Context, agentRecord) error {
	return b.add("tls")
}
func (b *fakeProvisionBackend) CheckReady(context.Context, agentRecord) error {
	return b.add("ready")
}

func TestWorkerRunsFullProvisioningFromAuthoritativeControlPlane(t *testing.T) {
	api, state, calls := newTestControlAPI(t, agentRecord{
		AgentID: testAgentID, RequestID: "req-create", Slug: "alpha", Domain: "alpha.cc2.cx",
		DisplayName: "Alpha", OwnerMainUserID: 42, Status: "pending", CurrentStep: "queued",
		UpdatedAt: time.Date(2026, 9, 24, 1, 0, 0, 0, time.UTC),
	})
	backend := newFakeProvisionBackend(t)
	worker := newWorker(config{}, api, backend)
	if err := worker.process(context.Background(), testAgentID); err != nil {
		t.Fatalf("process() error = %v", err)
	}
	if state.Status != "active" || state.CurrentStep != "ready" {
		t.Fatalf("final authority state = status %q step %q", state.Status, state.CurrentStep)
	}
	wantBackend := []string{"generate", "deploy", "nginx", "dns", "tls", "ready"}
	if strings.Join(backend.steps, ",") != strings.Join(wantBackend, ",") {
		t.Fatalf("backend calls = %v, want %v", backend.steps, wantBackend)
	}
	wantAPI := []string{"GET", "POST:claim", "PATCH:validating", "POST:runtime-credentials", "PATCH:bundle_generated", "PATCH:deploying", "PATCH:domain_check", "PATCH:tls_check", "PATCH:readiness_check", "POST:activate"}
	if strings.Join(*calls, ",") != strings.Join(wantAPI, ",") {
		t.Fatalf("control API calls = %v, want %v", *calls, wantAPI)
	}
}

func TestWorkerReportsRetryableFailureAndBoundedFailureCode(t *testing.T) {
	api, state, calls := newTestControlAPI(t, agentRecord{
		AgentID: testAgentID, RequestID: "req-create", Slug: "alpha", Domain: "alpha.cc2.cx",
		DisplayName: "Alpha", OwnerMainUserID: 42, Status: "pending", CurrentStep: "queued",
		UpdatedAt: time.Date(2026, 9, 24, 1, 0, 0, 0, time.UTC),
	})
	backend := newFakeProvisionBackend(t)
	backend.failAt, backend.failure = "deploy", errors.New("docker error output must not be persisted")
	worker := newWorker(config{}, api, backend)
	err := worker.process(context.Background(), testAgentID)
	if err == nil {
		t.Fatal("expected deployment failure")
	}
	if state.Status != "failed" || state.CurrentStep != "deploying" || !state.CanRetry || state.RecentError != "agent container deployment failed" {
		t.Fatalf("failed state contains incorrect or unbounded error: %+v", state)
	}
	if strings.Contains(state.RecentError, "docker") {
		t.Fatal("raw deployment output was persisted to the control plane")
	}
	if len(*calls) < 5 || (*calls)[0] != "GET" || (*calls)[1] != "POST:claim" || (*calls)[2] != "PATCH:validating" || (*calls)[len(*calls)-1] != "PATCH:failed:deployment_failed" {
		t.Fatalf("unexpected provisioning/failure API sequence: %v", *calls)
	}
}

func TestWorkerDoesNotActivateWhenDNSOrReadinessChecksFail(t *testing.T) {
	tests := []struct {
		name        string
		backendAt   string
		failureCode string
	}{
		{name: "dns", backendAt: "dns", failureCode: "dns_failed"},
		{name: "tls", backendAt: "tls", failureCode: "tls_failed"},
		{name: "ready", backendAt: "ready", failureCode: "readiness_failed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			api, state, calls := newTestControlAPI(t, agentRecord{
				AgentID: testAgentID, RequestID: "req-create", Slug: "alpha", Domain: "alpha.cc2.cx",
				DisplayName: "Alpha", OwnerMainUserID: 42, Status: "pending", CurrentStep: "queued",
				UpdatedAt: time.Date(2026, 9, 24, 1, 0, 0, 0, time.UTC),
			})
			backend := newFakeProvisionBackend(t)
			backend.failAt, backend.failure = test.backendAt, errors.New(test.backendAt+" probe failed")
			worker := newWorker(config{}, api, backend)
			if err := worker.process(context.Background(), testAgentID); err == nil {
				t.Fatal("expected readiness failure")
			}
			wantFailedStep := map[string]string{"dns": "domain_check", "tls": "tls_check", "ready": "readiness_check"}[test.backendAt]
			if state.Status != "failed" || state.CurrentStep != wantFailedStep || !state.CanRetry {
				t.Fatalf("failed readiness check did not leave a retryable inactive Agent: %+v", state)
			}
			for _, call := range *calls {
				if call == "POST:activate" {
					t.Fatalf("worker activated an Agent after %s check failed: %v", test.name, *calls)
				}
			}
			wantFailure := "PATCH:failed:" + test.failureCode
			if len(*calls) == 0 || (*calls)[len(*calls)-1] != wantFailure {
				t.Fatalf("failure report = %v, want final call %q", *calls, wantFailure)
			}
		})
	}
}

func TestWorkerWaitsForExplicitAdminRetryEvenWhenFailureIsRetryable(t *testing.T) {
	api, _, calls := newTestControlAPI(t, agentRecord{
		AgentID: testAgentID, RequestID: "req-create", Slug: "alpha", Domain: "alpha.cc2.cx",
		Status: "failed", CurrentStep: "deploying", CanRetry: true,
		UpdatedAt: time.Now().UTC(),
	})
	err := newWorker(config{}, api, newFakeProvisionBackend(t)).process(context.Background(), testAgentID)
	if err == nil || len(*calls) != 1 || (*calls)[0] != "GET" {
		t.Fatalf("retryable failure resumed without an explicit administrator action: err=%v calls=%v", err, *calls)
	}
}

func TestWorkerResumesRetryFromRecordedFailureStep(t *testing.T) {
	api, state, calls := newTestControlAPI(t, agentRecord{
		AgentID: testAgentID, RequestID: "req-create", Slug: "alpha", Domain: "alpha.cc2.cx",
		DisplayName: "Alpha", OwnerMainUserID: 42, Status: "pending", CurrentStep: "domain_check",
		UpdatedAt: time.Date(2026, 9, 24, 1, 0, 0, 0, time.UTC),
	})
	backend := newFakeProvisionBackend(t)
	worker := newWorker(config{}, api, backend)
	if err := worker.process(context.Background(), testAgentID); err != nil {
		t.Fatalf("process() retry error = %v", err)
	}
	if state.Status != "active" || state.CurrentStep != "ready" {
		t.Fatalf("resumed retry did not activate: status=%q step=%q", state.Status, state.CurrentStep)
	}
	if got, want := strings.Join(backend.steps, ","), "dns,tls,ready"; got != want {
		t.Fatalf("retry repeated earlier stages: got %q want %q", got, want)
	}
	if got, want := strings.Join(*calls, ","), "GET,POST:claim,PATCH:tls_check,PATCH:readiness_check,POST:activate"; got != want {
		t.Fatalf("retry control calls = %q want %q", got, want)
	}
}

func TestWorkerResumesTLSRetryWithoutRepeatingEarlierStages(t *testing.T) {
	api, state, calls := newTestControlAPI(t, agentRecord{
		AgentID: testAgentID, RequestID: "req-create", Slug: "alpha", Domain: "alpha.cc2.cx",
		DisplayName: "Alpha", OwnerMainUserID: 42, Status: "pending", CurrentStep: "tls_check",
		UpdatedAt: time.Date(2026, 9, 24, 1, 0, 0, 0, time.UTC),
	})
	backend := newFakeProvisionBackend(t)
	worker := newWorker(config{}, api, backend)
	if err := worker.process(context.Background(), testAgentID); err != nil {
		t.Fatalf("process() TLS retry error = %v", err)
	}
	if state.Status != "active" || state.CurrentStep != "ready" {
		t.Fatalf("resumed TLS retry did not activate: status=%q step=%q", state.Status, state.CurrentStep)
	}
	if got, want := strings.Join(backend.steps, ","), "tls,ready"; got != want {
		t.Fatalf("TLS retry repeated earlier stages: got %q want %q", got, want)
	}
	if got, want := strings.Join(*calls, ","), "GET,POST:claim,PATCH:readiness_check,POST:activate"; got != want {
		t.Fatalf("TLS retry control calls = %q want %q", got, want)
	}
}

func TestWorkerSkipsAgentClaimedByAnotherWorker(t *testing.T) {
	state := agentRecord{
		AgentID: testAgentID, RequestID: "req-create", Slug: "alpha", Domain: "alpha.cc2.cx",
		Status: "pending", CurrentStep: "queued", UpdatedAt: time.Now().UTC(),
	}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/"+testAgentID):
			calls = append(calls, "GET")
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": state})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/"+testAgentID+"/claim"):
			calls = append(calls, "CLAIM")
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 1, "reason": "AGENT_PROVISIONING_LEASE_HELD"})
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	backend := newFakeProvisionBackend(t)
	worker := newWorker(config{}, newControlAPI(config{MainURL: server.URL, RequestTimeout: time.Second}), backend)
	if err := worker.process(context.Background(), testAgentID); err != nil {
		t.Fatalf("process() should treat an existing claim as a no-op: %v", err)
	}
	if strings.Join(calls, ",") != "GET,CLAIM" {
		t.Fatalf("worker made calls after another worker owned the lease: %v", calls)
	}
	if len(backend.steps) != 0 {
		t.Fatalf("worker performed deployment without a lease: %v", backend.steps)
	}
}

type blockingProvisionBackend struct{ entered chan struct{} }

func (b *blockingProvisionBackend) Generate(context.Context, agentRecord) (generatedBundle, error) {
	return generatedBundle{Path: ".", ControlTokenHash: strings.Repeat("1", 64), ModelTokenHash: strings.Repeat("2", 64)}, nil
}
func (b *blockingProvisionBackend) Deploy(ctx context.Context, _ agentRecord, _ string) error {
	select {
	case b.entered <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return ctx.Err()
}
func (*blockingProvisionBackend) PublishNginx(context.Context, agentRecord, string) error { return nil }
func (*blockingProvisionBackend) CheckDNS(context.Context, agentRecord) error             { return nil }
func (*blockingProvisionBackend) CheckTLS(context.Context, agentRecord) error             { return nil }
func (*blockingProvisionBackend) CheckReady(context.Context, agentRecord) error           { return nil }

func TestWorkerCancelsDeploymentWhenLeaseIsLost(t *testing.T) {
	state := agentRecord{
		AgentID: testAgentID, RequestID: "req-create", Slug: "alpha", Domain: "alpha.cc2.cx",
		Status: "pending", CurrentStep: "queued", UpdatedAt: time.Now().UTC(),
	}
	var mu sync.Mutex
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/"+testAgentID):
			calls = append(calls, "GET")
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": state})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/"+testAgentID+"/claim"):
			calls = append(calls, "CLAIM")
			claim := leaseClaim{Agent: state, Token: testLeaseToken, ExpiresAt: time.Now().Add(time.Minute)}
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": claim})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/"+testAgentID+"/runtime-credentials"):
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{}})
		case r.Method == http.MethodPatch && strings.HasSuffix(r.URL.Path, "/progress"):
			var payload struct {
				Step string `json:"step"`
			}
			_ = json.NewDecoder(r.Body).Decode(&payload)
			calls = append(calls, "PROGRESS:"+payload.Step)
			state.Status, state.CurrentStep = "provisioning", payload.Step
			state.UpdatedAt = state.UpdatedAt.Add(time.Second)
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": state})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/lease"):
			calls = append(calls, "RENEW")
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 1, "reason": "AGENT_PROVISIONING_LEASE_LOST"})
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	backend := &blockingProvisionBackend{entered: make(chan struct{}, 1)}
	worker := newWorker(config{}, newControlAPI(config{MainURL: server.URL, RequestTimeout: time.Second}), backend)
	worker.leaseRefreshInterval = 10 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := worker.process(ctx, testAgentID)
	if !errors.Is(err, errAgentLeaseLost) {
		t.Fatalf("process() error = %v, want lease lost", err)
	}
	select {
	case <-backend.entered:
	default:
		t.Fatal("deployment did not start before the lease was lost")
	}
	mu.Lock()
	defer mu.Unlock()
	for _, call := range calls {
		if strings.Contains(call, "failed") || strings.Contains(call, "activate") {
			t.Fatalf("worker continued control-plane mutations after losing the lease: %v", calls)
		}
	}
}

func newFakeProvisionBackend(t *testing.T) *fakeProvisionBackend {
	t.Helper()
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "nginx.conf"), []byte("# managed-by-agentapi-provision-worker agent_id="+testAgentID+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return &fakeProvisionBackend{bundlePath: directory}
}

func TestWriteAtomicDoesNotReusePredictableTemporaryPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agentapi-test.conf")
	predictableTemp := path + ".agentapi-tmp"
	marker := []byte("leave this file untouched")
	if err := os.WriteFile(predictableTemp, marker, 0o600); err != nil {
		t.Fatal(err)
	}

	want := []byte("server_name example.test;\n")
	if err := writeAtomic(path, want, 0o640); err != nil {
		t.Fatalf("writeAtomic() error = %v", err)
	}
	if got, err := os.ReadFile(predictableTemp); err != nil || string(got) != string(marker) {
		t.Fatalf("predictable temporary path changed: got %q, err %v", got, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(want) {
		t.Fatalf("atomic destination = %q, err %v", got, err)
	}
}

func TestBundlePathWithinStateRejectsSymlinkEscape(t *testing.T) {
	stateDir := t.TempDir()
	outsideDir := t.TempDir()
	link := filepath.Join(stateDir, "bundle")
	if err := os.Symlink(outsideDir, link); err != nil {
		t.Skipf("directory symlinks are unavailable: %v", err)
	}
	if _, err := bundlePathWithinState(stateDir, link); err == nil || !strings.Contains(err.Error(), "escaped") {
		t.Fatalf("bundle path symlink escape was accepted: %v", err)
	}
}

func TestBundlePathWithinStateReturnsExistingBundleDirectory(t *testing.T) {
	stateDir := t.TempDir()
	bundleDir := filepath.Join(stateDir, "agents", "agent01")
	if err := os.MkdirAll(bundleDir, 0o700); err != nil {
		t.Fatal(err)
	}
	got, err := bundlePathWithinState(stateDir, bundleDir)
	if err != nil {
		t.Fatalf("bundlePathWithinState() error = %v", err)
	}
	want, err := filepath.EvalSymlinks(bundleDir)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("bundlePathWithinState() = %q, want %q", got, want)
	}
}

func TestConsumeAgentEventsOnlyQueuesAuthoritativeRefreshHints(t *testing.T) {
	var queued []string
	stream := strings.NewReader("retry: 3000\n\nevent: resync\ndata: {}\n\nevent: heartbeat\ndata: now\n\nevent: agent\ndata: {\"agent_id\":\"" + testAgentID + "\"}\n\n")
	err := consumeAgentEvents(stream, func(agentID string) { queued = append(queued, agentID) })
	if err == nil {
		t.Fatal("closed event stream should request reconnect")
	}
	if strings.Join(queued, ",") != ","+testAgentID {
		t.Fatalf("event stream queued payload state instead of refresh hints: %v", queued)
	}
}

func TestControlAPIStreamUsesProvisioningCredential(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-AgentAPI-Provisioning-Credential") != "worker-stream-credential" || r.Header.Get("x-api-key") != "" {
			t.Errorf("event stream did not use provisioning-only credential")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	api := newControlAPI(config{MainURL: server.URL, ProvisioningCredential: "worker-stream-credential", RequestTimeout: time.Second})
	response, err := api.openStream(context.Background())
	if err != nil {
		t.Fatalf("openStream() error = %v", err)
	}
	_ = response.Body.Close()
}

func newTestControlAPI(t *testing.T, initial agentRecord) (*controlAPI, *agentRecord, *[]string) {
	t.Helper()
	state := initial
	calls := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-AgentAPI-Provisioning-Credential") != "test-provisioning-worker-secret" || r.Header.Get("x-api-key") != "" {
			t.Errorf("control request did not use the provisioning-only credential")
		}
		if strings.Contains(r.URL.Path, "/progress") {
			if r.Header.Get("Idempotency-Key") == "" || r.Header.Get("X-Request-ID") == "" {
				t.Errorf("progress request omitted idempotency or request ID")
			}
			if r.Header.Get("X-AgentAPI-Provisioning-Lease") != testLeaseToken {
				t.Errorf("progress request omitted the claimed lease")
			}
		}
		var result any
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/agent-provisioning/agents/"+testAgentID:
			calls = append(calls, "GET")
			result = state
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/agent-provisioning/agents/"+testAgentID+"/retry":
			calls = append(calls, "POST:retry")
			state.Status, state.CanRetry, state.RecentError = "pending", false, ""
			if state.CurrentStep == "failed" {
				state.CurrentStep = "queued"
			}
			state.UpdatedAt = state.UpdatedAt.Add(time.Second)
			result = state
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/agent-provisioning/agents/"+testAgentID+"/claim":
			calls = append(calls, "POST:claim")
			result = leaseClaim{Agent: state, Token: testLeaseToken, ExpiresAt: time.Now().Add(90 * time.Second)}
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/agent-provisioning/agents/"+testAgentID+"/runtime-credentials":
			if r.Header.Get("X-AgentAPI-Provisioning-Lease") != testLeaseToken {
				t.Errorf("runtime credential registration did not use the claimed lease")
			}
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || !isSHA256Hash(payload["control_token_hash"]) || !isSHA256Hash(payload["model_token_hash"]) {
				t.Errorf("worker did not submit only runtime credential hashes")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			calls = append(calls, "POST:runtime-credentials")
			result = map[string]any{}
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/agent-provisioning/agents/"+testAgentID+"/lease":
			if r.Header.Get("X-AgentAPI-Provisioning-Lease") != testLeaseToken {
				t.Errorf("lease renewal did not use the claimed token")
			}
			calls = append(calls, "POST:lease")
			result = map[string]any{}
		case r.Method == http.MethodPatch && r.URL.Path == "/api/v1/agent-provisioning/agents/"+testAgentID+"/progress":
			var payload struct {
				Step        string `json:"step"`
				FailureCode string `json:"failure_code"`
				FailureStep string `json:"failure_step"`
				Retryable   bool   `json:"retryable"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("invalid progress body: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if payload.Step == "failed" {
				calls = append(calls, "PATCH:failed:"+payload.FailureCode)
				state.Status, state.CurrentStep, state.CanRetry, state.RecentError = "failed", payload.FailureStep, payload.Retryable, "agent container deployment failed"
			} else {
				calls = append(calls, "PATCH:"+payload.Step)
				state.Status, state.CurrentStep = "provisioning", payload.Step
				if payload.Step == "readiness_check" {
					state.DomainStatus = "ready"
				}
			}
			state.UpdatedAt = state.UpdatedAt.Add(time.Second)
			result = state
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/agent-provisioning/agents/"+testAgentID+"/activate":
			if r.Header.Get("X-AgentAPI-Provisioning-Lease") != testLeaseToken {
				t.Errorf("activation did not use the claimed lease")
			}
			var payload struct {
				ConfirmAgentID     string `json:"confirm_agent_id"`
				ReadinessConfirmed bool   `json:"readiness_confirmed"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.ConfirmAgentID != testAgentID || !payload.ReadinessConfirmed {
				t.Errorf("worker did not explicitly confirm readiness")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			calls = append(calls, "POST:activate")
			state.Status, state.CurrentStep, state.CanRetry = "active", "ready", false
			state.UpdatedAt = state.UpdatedAt.Add(time.Second)
			result = state
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": result})
	}))
	t.Cleanup(server.Close)
	return newControlAPI(config{MainURL: server.URL, ProvisioningCredential: "test-provisioning-worker-secret", RequestTimeout: time.Second}), &state, &calls
}
