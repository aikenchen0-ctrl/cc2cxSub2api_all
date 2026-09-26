package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRuntimeGetProvisioningAgentUsesPerAgentAPIAndValidatesAgentID(t *testing.T) {
	const agentID = "agt_0123456789abcdef0123456789abcdef"
	var gotPath, gotCredential string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotCredential = r.Header.Get("X-AgentAPI-Runtime-Control")
		_ = json.NewEncoder(w).Encode(envelope(map[string]any{"agent_id": agentID, "status": "suspended"}))
	}))
	defer upstream.Close()
	credential := "agt_ctl_test-runtime-control-secret-0123456789abcdef"
	cfg := Config{MainAPIBaseURL: upstream.URL + "/api/v1", RuntimeControlCredential: credential, AgentID: agentID, MainRequestTimeout: time.Second}
	status, err := NewMainClient(cfg).RuntimeGetProvisioningAgent(t.Context(), agentID)
	if err != nil {
		t.Fatal(err)
	}
	if status.AgentID != agentID || status.Status != "suspended" || gotPath != "/api/v1/agent-runtime/agent" || gotCredential != credential {
		t.Fatalf("unexpected provisioning status read: status=%+v path=%q credential=%q", status, gotPath, gotCredential)
	}
	if _, err := NewMainClient(cfg).RuntimeGetProvisioningAgent(t.Context(), "agt_different000000000000000000000000"); err == nil || !strings.Contains(err.Error(), "restricted to its own Agent") {
		t.Fatalf("mismatched main-site Agent ID was accepted: %v", err)
	}
}

func TestMainClientDoesNotForwardCredentialsAcrossRedirects(t *testing.T) {
	const controlCredential = "agt_ctl_test-runtime-control-secret-0123456789abcdef"
	const userToken = "sub2api-user-jwt"
	const ssoTicket = "existing-sub2api-hmac-ticket"
	var redirectedRequests int
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectedRequests++
		if r.Header.Get("X-AgentAPI-Runtime-Control") != "" || r.Header.Get("Authorization") != "" || r.Header.Get("X-AgentAPI-SSO-Ticket") != "" {
			t.Errorf("credential-bearing headers reached redirect target: %v", r.Header)
		}
	}))
	defer target.Close()

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/capture", http.StatusTemporaryRedirect)
	}))
	defer redirector.Close()

	cfg := Config{
		MainAPIBaseURL:           redirector.URL + "/api/v1",
		RuntimeControlCredential: controlCredential,
		AgentID:                  "agt_0123456789abcdef0123456789abcdef",
		MainRequestTimeout:       time.Second,
	}
	if _, err := NewMainClient(cfg).RuntimeGetProvisioningAgent(t.Context(), cfg.AgentID); err == nil {
		t.Fatal("runtime API redirect was followed instead of rejected")
	}
	if err := NewMainClient(cfg).MapRuntimeUser(t.Context(), "43", userToken); err == nil {
		t.Fatal("identity mapping redirect was followed instead of rejected")
	}
	if err := NewMainClient(cfg).MapRuntimeUserWithSSOTicket(t.Context(), "43", ssoTicket); err == nil {
		t.Fatal("SSO identity redirect was followed instead of rejected")
	}
	if redirectedRequests != 0 {
		t.Fatalf("credential-bearing redirect target received %d request(s)", redirectedRequests)
	}
}

func TestMainClientProfileAndPasswordUseOnlyAuthenticatedUserEndpoints(t *testing.T) {
	const accessToken = "main-user-access-token"
	var profileCalls, updateCalls, passwordCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+accessToken {
			t.Errorf("user credential was not scoped to the authenticated account endpoint: %v", r.Header)
		}
		if r.Header.Get("X-AgentAPI-Runtime-Control") != "" || r.Header.Get("Cookie") != "" {
			t.Errorf("unexpected satellite or browser credential forwarded to user endpoint: %v", r.Header)
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/user/profile":
			profileCalls++
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 42, "email": "user@example.com", "username": "User"}))
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/user":
			updateCalls++
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("decode profile update: %v", err)
			}
			if len(payload) != 1 || payload["username"] != "Updated User" {
				t.Errorf("profile proxy accepted more than the username field: %#v", payload)
			}
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 42, "email": "user@example.com", "username": "Updated User"}))
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/user/password":
			passwordCalls++
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("decode password change: %v", err)
			}
			if len(payload) != 2 || payload["old_password"] != "old-password" || payload["new_password"] != "new-password" {
				t.Errorf("password proxy sent unexpected fields: %#v", payload)
			}
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"message": "Password changed successfully"}))
		default:
			t.Errorf("unexpected Sub2API user endpoint: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	client := NewMainClient(Config{MainAPIBaseURL: upstream.URL + "/api/v1", MainRequestTimeout: time.Second})
	profile, err := client.UserProfile(t.Context(), accessToken)
	if err != nil || !strings.Contains(string(profile), `"username":"User"`) {
		t.Fatalf("profile request failed: profile=%s err=%v", profile, err)
	}
	profile, err = client.UpdateUserProfile(t.Context(), accessToken, "Updated User")
	if err != nil || !strings.Contains(string(profile), `"username":"Updated User"`) {
		t.Fatalf("profile update failed: profile=%s err=%v", profile, err)
	}
	if err := client.ChangeUserPassword(t.Context(), accessToken, "old-password", "new-password"); err != nil {
		t.Fatalf("password change failed: %v", err)
	}
	if profileCalls != 1 || updateCalls != 1 || passwordCalls != 1 {
		t.Fatalf("unexpected user endpoint calls: profile=%d update=%d password=%d", profileCalls, updateCalls, passwordCalls)
	}
}

func TestModelRelayDoesNotForwardCredentialsAcrossRedirects(t *testing.T) {
	const appCredential = "agentapi-app-credential"
	const mainUserID = "43"
	var redirectedRequests int
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectedRequests++
		if r.Header.Get("Authorization") != "" || r.Header.Get("X-Sub2API-On-Behalf-Of") != "" {
			t.Errorf("model credentials reached redirect target: %v", r.Header)
		}
	}))
	defer target.Close()

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/capture", http.StatusTemporaryRedirect)
	}))
	defer redirector.Close()

	cfg := Config{
		MainModelBaseURL:   redirector.URL + "/v1",
		AppCredential:      appCredential,
		SatelliteSlug:      "agentapi",
		ModelStreamTimeout: time.Second,
	}
	status, _, _, err := NewMainClient(cfg).RelayModelMethod(t.Context(), http.MethodPost, "/v1/models", nil, nil, mainUserID, "")
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusTemporaryRedirect {
		t.Fatalf("unexpected redirect response status: %d", status)
	}
	if redirectedRequests != 0 {
		t.Fatalf("model redirect target received %d request(s)", redirectedRequests)
	}
}

func TestMapRuntimeUserSendsUserJWTOnlyForIdentityProof(t *testing.T) {
	var gotMethod, gotPath, gotControl, gotAuthorization string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		gotControl = r.Header.Get("X-AgentAPI-Runtime-Control")
		gotAuthorization = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(envelope(map[string]any{"agent_id": "agt_0123456789abcdef0123456789abcdef", "user_id": 43, "status": "mapped"}))
	}))
	defer upstream.Close()
	const controlCredential = "agt_ctl_test-runtime-control-secret-0123456789abcdef"
	const userAccessToken = "sub2api-user-jwt"
	cfg := Config{MainAPIBaseURL: upstream.URL + "/api/v1", RuntimeControlCredential: controlCredential, MainRequestTimeout: time.Second}
	if err := NewMainClient(cfg).MapRuntimeUser(t.Context(), "43", userAccessToken); err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodPost || gotPath != "/api/v1/agent-runtime/users/43/map" || gotControl != controlCredential || gotAuthorization != "Bearer "+userAccessToken {
		t.Fatalf("runtime mapping did not carry both scoped credentials: method=%q path=%q control=%q authorization=%q", gotMethod, gotPath, gotControl, gotAuthorization)
	}
}

func TestMapRuntimeUserWithSSOTicketReusesExistingIdentityTicket(t *testing.T) {
	var gotControl, gotAuthorization, gotTicket string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotControl = r.Header.Get("X-AgentAPI-Runtime-Control")
		gotAuthorization = r.Header.Get("Authorization")
		gotTicket = r.Header.Get("X-AgentAPI-SSO-Ticket")
		_ = json.NewEncoder(w).Encode(envelope(map[string]any{"agent_id": "agt_0123456789abcdef0123456789abcdef", "user_id": 43, "status": "mapped"}))
	}))
	defer upstream.Close()
	const controlCredential = "agt_ctl_test-runtime-control-secret-0123456789abcdef"
	const ticket = "existing-sub2api-hmac-ticket"
	cfg := Config{MainAPIBaseURL: upstream.URL + "/api/v1", RuntimeControlCredential: controlCredential, MainRequestTimeout: time.Second}
	if err := NewMainClient(cfg).MapRuntimeUserWithSSOTicket(t.Context(), "43", ticket); err != nil {
		t.Fatal(err)
	}
	if gotControl != controlCredential || gotAuthorization != "" || gotTicket != ticket {
		t.Fatalf("SSO mapping did not reuse the identity ticket as a server-side proof: control=%q authorization=%q ticket=%q", gotControl, gotAuthorization, gotTicket)
	}
}

func TestRuntimeAgentUpdatesUsesScopedIDOnlyStream(t *testing.T) {
	const credential = "agt_ctl_test-runtime-control-secret-0123456789abcdef"
	var gotPath, gotCredential, gotAuthorization string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotCredential = r.Header.Get("X-AgentAPI-Runtime-Control")
		gotAuthorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: agent\ndata: {\"agent_id\":\"agt_0123456789abcdef0123456789abcdef\"}\n\n")
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	}))
	defer upstream.Close()
	cfg := Config{MainAPIBaseURL: upstream.URL + "/api/v1", RuntimeControlCredential: credential, MainRequestTimeout: time.Second}
	updates, err := NewMainClient(cfg).RuntimeAgentUpdates(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	select {
	case _, open := <-updates:
		if !open {
			t.Fatal("ID-only Agent update stream closed without an event")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Agent runtime event")
	}
	if gotPath != "/api/v1/agent-runtime/agent/stream" || gotCredential != credential || gotAuthorization != "" {
		t.Fatalf("unexpected runtime stream request: path=%q credential=%q authorization=%q", gotPath, gotCredential, gotAuthorization)
	}
}

func TestRuntimeUpdateUserStatusSendsOnlyStatusToScopedEndpoint(t *testing.T) {
	var gotMethod, gotPath, gotCredential string
	var gotPayload map[string]json.RawMessage
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotCredential = r.Method, r.URL.Path, r.Header.Get("X-AgentAPI-Runtime-Control")
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Errorf("decode update payload: %v", err)
		}
		_ = json.NewEncoder(w).Encode(envelope(map[string]any{"agent_id": "agt_0123456789abcdef0123456789abcdef", "user_id": 43, "status": "disabled"}))
	}))
	defer upstream.Close()
	credential := "agt_ctl_test-runtime-control-secret-0123456789abcdef"
	cfg := Config{MainAPIBaseURL: upstream.URL + "/api/v1", RuntimeControlCredential: credential, MainRequestTimeout: time.Second}
	if err := NewMainClient(cfg).RuntimeUpdateUserStatus(t.Context(), "43", "disabled"); err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodPatch || gotPath != "/api/v1/agent-runtime/users/43/status" || gotCredential != credential {
		t.Fatalf("unexpected runtime status update: method=%q path=%q credential=%q", gotMethod, gotPath, gotCredential)
	}
	if len(gotPayload) != 1 || string(gotPayload["status"]) != `"disabled"` {
		t.Fatalf("status update must not send other user fields: %s", mustJSON(gotPayload))
	}
}

func TestRuntimeUpdateUserStatusRejectsUnexpectedResponse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 43, "status": "active"}))
	}))
	defer upstream.Close()
	cfg := Config{MainAPIBaseURL: upstream.URL + "/api/v1", RuntimeControlCredential: "agt_ctl_test-runtime-control-secret-0123456789abcdef", MainRequestTimeout: time.Second}
	if err := NewMainClient(cfg).RuntimeUpdateUserStatus(t.Context(), "43", "disabled"); err == nil || !strings.Contains(err.Error(), "did not confirm") {
		t.Fatalf("unexpected status response was accepted: %v", err)
	}
}

func TestRuntimeUpdateModelAllowlistUsesPerAgentControlAndRequiresConfirmation(t *testing.T) {
	const agentID = "agt_0123456789abcdef0123456789abcdef"
	const credential = "agt_ctl_test-runtime-control-secret-0123456789abcdef"
	var gotMethod, gotPath, gotCredential string
	var gotPayload map[string][]string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotCredential = r.Method, r.URL.Path, r.Header.Get("X-AgentAPI-Runtime-Control")
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Errorf("decode model allowlist update: %v", err)
		}
		_ = json.NewEncoder(w).Encode(envelope(map[string]any{"agent_id": agentID, "enabled": []string{"gpt-image-2", "gpt-5.5"}}))
	}))
	defer upstream.Close()
	cfg := Config{MainAPIBaseURL: upstream.URL + "/api/v1", RuntimeControlCredential: credential, AgentID: agentID, MainRequestTimeout: time.Second}
	models := []string{"gpt-5.5", "gpt-image-2"}
	if err := NewMainClient(cfg).RuntimeUpdateModelAllowlist(t.Context(), models); err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodPut || gotPath != "/api/v1/agent-runtime/model-policy" || gotCredential != credential || len(gotPayload) != 1 || !sameStringSlice(gotPayload["enabled"], models) {
		t.Fatalf("unexpected model scope update: method=%q path=%q credential=%q payload=%v", gotMethod, gotPath, gotCredential, gotPayload)
	}

	wrongAgent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(envelope(map[string]any{"agent_id": "agt_other", "enabled": models}))
	}))
	defer wrongAgent.Close()
	cfg.MainAPIBaseURL = wrongAgent.URL + "/api/v1"
	if err := NewMainClient(cfg).RuntimeUpdateModelAllowlist(t.Context(), models); err == nil || !strings.Contains(err.Error(), "did not confirm") {
		t.Fatalf("scope confirmation for another Agent was accepted: %v", err)
	}
}

func TestRuntimeFindUsageDecodesOwnerScopedUsage(t *testing.T) {
	var gotPath string
	var gotCredential string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.String()
		gotCredential = r.Header.Get("X-AgentAPI-Runtime-Control")
		_ = json.NewEncoder(w).Encode(envelope(map[string]any{
			"items": []map[string]any{{
				"id": 77, "request_id": "req-usage", "model": "gpt-5.5", "total_cost": 1.25, "actual_cost": 1.10,
				"input_tokens": 1000, "output_tokens": 250, "cache_creation_tokens": 12, "cache_read_tokens": 34,
				"input_cost": 0.000000123, "output_cost": 0.000000456, "upstream_model": "provider-gpt-5.5",
				"upstream_response_model": "provider-gpt-5.5-2026-01", "upstream_model_mismatch": true,
				"service_tier": "priority", "reasoning_effort": "high", "inbound_endpoint": "/v1/responses",
				"duration_ms": 900, "first_token_ms": 120, "image_size_breakdown": map[string]int{"1024x1024": 2},
			}},
			"total": 1, "page": 1, "page_size": 20,
		}))
	}))
	defer upstream.Close()
	credential := "agt_ctl_test-runtime-control-secret-0123456789abcdef"
	cfg := Config{MainAPIBaseURL: upstream.URL + "/api/v1", RuntimeControlCredential: credential, MainRequestTimeout: time.Second}
	items, err := NewMainClient(cfg).AdminFindUsage(t.Context(), "req-usage")
	if err != nil {
		t.Fatal(err)
	}
	if gotCredential != credential || gotPath != "/api/v1/agent-runtime/usage?request_id=req-usage" {
		t.Fatalf("unexpected runtime usage request: path=%q credential=%q", gotPath, gotCredential)
	}
	if len(items) != 1 || items[0].ID != "77" || items[0].ActualCents != 110 || items[0].TotalCents != 125 {
		t.Fatalf("unexpected decoded usage: %+v", items)
	}
	usage := items[0]
	if usage.Snapshot.Source != "sub2api_owner_runtime_usage" || usage.Snapshot.InputTokens != 1000 || usage.Snapshot.OutputTokens != 250 || usage.Snapshot.CacheCreationTokens != 12 || usage.Snapshot.CacheReadTokens != 34 {
		t.Fatalf("usage token facts were not decoded: %+v", usage.Snapshot)
	}
	if usage.Snapshot.InputCostNanos != 123 || usage.Snapshot.OutputCostNanos != 456 || usage.Snapshot.UpstreamModel != "provider-gpt-5.5" || usage.Snapshot.UpstreamResponseModel != "provider-gpt-5.5-2026-01" || usage.Snapshot.UpstreamModelMismatch == nil || !*usage.Snapshot.UpstreamModelMismatch {
		t.Fatalf("usage cost/model provenance was not decoded: %+v", usage.Snapshot)
	}
	if usage.Snapshot.ServiceTier != "priority" || usage.Snapshot.ReasoningEffort != "high" || usage.Snapshot.InboundEndpoint != "/v1/responses" || usage.Snapshot.DurationMs != 900 || usage.Snapshot.FirstTokenMs != 120 || usage.Snapshot.ImageSizeBreakdown["1024x1024"] != 2 {
		t.Fatalf("usage request metadata was not decoded: %+v", usage.Snapshot)
	}
}

func TestAdminFindUsagePreservesZeroActualCostInsteadOfUsingStandardCost(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(envelope(map[string]any{
			"items": []map[string]any{{
				"id": 78, "request_id": "req-zero-actual", "model": "free-model", "total_cost": 1.25, "actual_cost": 0,
			}},
		}))
	}))
	defer upstream.Close()

	cfg := Config{MainAPIBaseURL: upstream.URL + "/api/v1", RuntimeControlCredential: "agt_ctl_test-runtime-control-secret-0123456789abcdef", MainRequestTimeout: time.Second}
	items, err := NewMainClient(cfg).AdminFindUsage(t.Context(), "req-zero-actual")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || !items[0].HasActualCost || items[0].ActualCents != 0 || items[0].TotalCents != 125 {
		t.Fatalf("explicit zero actual cost was replaced by standard cost: %+v", items)
	}
}

func TestAdminFindUsageFallsBackOnlyWhenActualCostFieldIsAbsent(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(envelope(map[string]any{
			"items": []map[string]any{{"id": 79, "request_id": "req-legacy-dto", "total_cost": 0.25}},
		}))
	}))
	defer upstream.Close()

	cfg := Config{MainAPIBaseURL: upstream.URL + "/api/v1", RuntimeControlCredential: "agt_ctl_test-runtime-control-secret-0123456789abcdef", MainRequestTimeout: time.Second}
	items, err := NewMainClient(cfg).AdminFindUsage(t.Context(), "req-legacy-dto")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].HasActualCost || items[0].ActualCents != 25 {
		t.Fatalf("legacy DTO total_cost fallback was not applied: %+v", items)
	}
}

func TestRelayModelPreservesMultipartContentTypeAndSessionIdentity(t *testing.T) {
	var gotContentType, gotAuthorization, gotOwner, gotSatellite, gotRequestID string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		gotAuthorization = r.Header.Get("Authorization")
		gotOwner = r.Header.Get("X-Sub2API-On-Behalf-Of")
		gotSatellite = r.Header.Get("X-Sub2API-Satellite")
		gotRequestID = r.Header.Get("X-Request-ID")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	cfg := Config{
		MainModelBaseURL:   upstream.URL + "/v1",
		AppCredential:      "app-secret",
		OwnerMainUserID:    "owner-1",
		SatelliteSlug:      "agentapi",
		MainRequestTimeout: time.Second,
	}
	_, _, body, err := NewMainClient(cfg).RelayModelMethodWithContentType(
		t.Context(), http.MethodPost, "/v1/images/edits", nil,
		[]byte("multipart-body"), "multipart/form-data; boundary=test-boundary",
		"43", "req-multipart",
	)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"ok":true}` {
		t.Fatalf("unexpected upstream body: %s", body)
	}
	if gotContentType != "multipart/form-data; boundary=test-boundary" {
		t.Fatalf("content type was not preserved: %q", gotContentType)
	}
	if gotAuthorization != "Bearer app-secret" || gotOwner != "43" || gotSatellite != "agentapi" || gotRequestID != "req-multipart" {
		t.Fatalf("unexpected relay headers: authorization=%q on_behalf_of=%q satellite=%q request_id=%q", gotAuthorization, gotOwner, gotSatellite, gotRequestID)
	}
}

func TestRelayModelDoesNotFallBackToOwnerWhenSessionIdentityIsMissing(t *testing.T) {
	var upstreamCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamCalls++
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	cfg := Config{
		MainModelBaseURL:   upstream.URL + "/v1",
		AppCredential:      "app-secret",
		OwnerMainUserID:    "42",
		SatelliteSlug:      "agentapi",
		MainRequestTimeout: time.Second,
	}
	_, _, _, err := NewMainClient(cfg).RelayModelMethod(
		t.Context(), http.MethodPost, "/v1/chat/completions", nil, []byte(`{"model":"gpt-5.5"}`), "", "req-no-session-user",
	)
	var apiErr *MainAPIError
	if !errors.As(err, &apiErr) || apiErr.Code != "MAIN_USER_MISSING" || upstreamCalls != 0 {
		t.Fatalf("missing Session identity was not rejected before relay: err=%v upstream_calls=%d", err, upstreamCalls)
	}
}
