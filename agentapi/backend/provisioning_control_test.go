package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestProvisioningControlGateBlocksNonActiveAndStaleAgents(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("blocked request reached main API: %s", r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	server.cfg.ProvisioningControlEnabled = true
	server.cfg.ProvisioningControlStaleAfter = time.Minute

	for _, test := range []struct {
		name       string
		status     string
		checkedAt  time.Time
		wantStatus int
		wantReason string
	}{
		{name: "pending", status: "pending", checkedAt: time.Now(), wantStatus: http.StatusServiceUnavailable, wantReason: "AGENT_NOT_ACTIVE"},
		{name: "suspended", status: "suspended", checkedAt: time.Now(), wantStatus: http.StatusLocked, wantReason: "AGENT_SUSPENDED"},
		{name: "revoked", status: "revoked", checkedAt: time.Now(), wantStatus: http.StatusGone, wantReason: "AGENT_REVOKED"},
		{name: "unknown", wantStatus: http.StatusServiceUnavailable, wantReason: "AGENT_CONTROL_UNAVAILABLE"},
		{name: "stale", status: "active", checkedAt: time.Now().Add(-2 * time.Minute), wantStatus: http.StatusServiceUnavailable, wantReason: "AGENT_CONTROL_UNAVAILABLE"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server.setProvisioningState(test.status, test.checkedAt)
			response := httptest.NewRecorder()
			server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/settings/public", nil))
			if response.Code != test.wantStatus || !strings.Contains(response.Body.String(), test.wantReason) {
				t.Fatalf("status=%d body=%s, want status=%d reason=%s", response.Code, response.Body.String(), test.wantStatus, test.wantReason)
			}
		})
	}
}

func TestProvisioningControlDoesNotBlockHealthOrReadiness(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	server.cfg.ProvisioningControlEnabled = true
	server.cfg.ProvisioningControlStaleAfter = time.Minute
	server.setProvisioningState("suspended", time.Now())

	for _, path := range []string{"/healthz", "/readyz"} {
		response := httptest.NewRecorder()
		server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("%s returned %d while control status was suspended: %s", path, response.Code, response.Body.String())
		}
	}
}

func TestProvisioningControlDisabledPreservesUnmanagedInstances(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/settings/public", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("unmanaged instance unexpectedly gated: status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestProvisioningControlPollsAuthoritativeStatusWithScopedCredential(t *testing.T) {
	const agentID = "agt_0123456789abcdef0123456789abcdef"
	const credential = "agt_ctl_test-runtime-control-secret-0123456789abcdef"
	var mainStatusMu sync.RWMutex
	mainStatus := "active"
	var calls atomic.Int32
	var invalidCredential atomic.Bool
	updateEvents := make(chan struct{}, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/agent-runtime/agent/stream" {
			if r.Header.Get("X-AgentAPI-Runtime-Control") != credential || r.Header.Get("X-API-Key") != "" || r.Header.Get("Authorization") != "" {
				invalidCredential.Store(true)
			}
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, ok := w.(http.Flusher)
			if !ok {
				t.Error("upstream did not support streaming flush")
				return
			}
			fmt.Fprint(w, "retry: 3000\n\nevent: resync\ndata: {}\n\n")
			flusher.Flush()
			select {
			case <-r.Context().Done():
				return
			case <-updateEvents:
				fmt.Fprintf(w, "event: agent\ndata: %s\n\n", `{"agent_id":"`+agentID+`"}`)
				flusher.Flush()
				<-r.Context().Done()
				return
			}
		}
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/agent-runtime/agent" {
			t.Errorf("unexpected runtime request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("X-AgentAPI-Runtime-Control") != credential || r.Header.Get("X-API-Key") != "" || r.Header.Get("Authorization") != "" {
			invalidCredential.Store(true)
		}
		calls.Add(1)
		mainStatusMu.RLock()
		status := mainStatus
		mainStatusMu.RUnlock()
		_ = json.NewEncoder(w).Encode(envelope(map[string]any{"agent_id": agentID, "status": status}))
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	server.cfg.AgentID = agentID
	server.cfg.RuntimeControlCredential = credential
	server.cfg.ProvisioningControlEnabled = true
	server.cfg.ProvisioningControlStaleAfter = 6 * time.Second
	server.main = NewMainClient(server.cfg)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("provisioning control loop did not stop after cancellation")
		}
	}()
	go func() {
		server.runProvisioningControl(ctx)
		close(done)
	}()

	waitForStatus := func(want string) {
		t.Helper()
		timer := time.NewTimer(3 * time.Second)
		defer timer.Stop()
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			status, available := server.provisioningState(time.Now())
			if available && status == want {
				return
			}
			select {
			case <-ticker.C:
			case <-timer.C:
				t.Fatalf("timed out waiting for authoritative status %q; got %q (available=%v)", want, status, available)
			}
		}
	}
	waitForStatus("active")
	if calls.Load() == 0 || invalidCredential.Load() {
		t.Fatalf("runtime status was not fetched with only the per-Agent credential: calls=%d invalidCredential=%v", calls.Load(), invalidCredential.Load())
	}

	mainStatusMu.Lock()
	mainStatus = "suspended"
	mainStatusMu.Unlock()
	updateEvents <- struct{}{}
	waitForStatus("suspended")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/settings/public", nil))
	if response.Code != http.StatusLocked || !strings.Contains(response.Body.String(), "AGENT_SUSPENDED") {
		t.Fatalf("periodic runtime status did not gate suspended Agent: status=%d body=%s", response.Code, response.Body.String())
	}
}
