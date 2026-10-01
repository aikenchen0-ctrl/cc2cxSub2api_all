package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func makeSSOTicket(t *testing.T, secret string, payload map[string]any) string {
	t.Helper()
	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(encodedPayload)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(encoded))
	return encoded + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func makeSharedAgentSSOTicket(t *testing.T, server *Server, subject, agentID, role, agentName, jti string) string {
	t.Helper()
	now := time.Now().UTC()
	return makeSSOTicket(t, server.cfg.SSOSecret, map[string]any{
		"iss": "sub2api", "aud": server.cfg.SSOAudience, "sub": subject,
		"email": subject + "@example.com", "displayName": "User " + subject,
		"agent_id": agentID, "agent_role": role, "agent_name": agentName,
		"iat": now.Unix(), "exp": now.Add(90 * time.Second).Unix(), "jti": jti, "next": "/dashboard",
	})
}

func testServer(t *testing.T, upstream *httptest.Server) *Server {
	t.Helper()
	cfg := Config{
		Addr:                ":0",
		DatabasePath:        ":memory:",
		MainAPIBaseURL:      upstream.URL + "/api/v1",
		MainAdminAPIKey:     "test-main-admin-key",
		MainModelBaseURL:    upstream.URL + "/v1",
		AppCredential:       "app-secret",
		SatelliteSlug:       "agentapi",
		SSOSecret:           "test-sso-secret-0123456789abcdef",
		SSOAudience:         "agentapi",
		SessionSecret:       "session-secret",
		CookieName:          "agentapi_session",
		AgentID:             "agent-test",
		AgentName:           "Agent Test",
		SiteName:            "Agent Test",
		BillingMode:         "owner_upstream",
		OwnerMainUserID:     "42",
		MaxRequestCostCents: 100,
		MainRequestTimeout:  0,
	}
	// http.Client treats a zero timeout as no timeout; use the normal value for
	// the production client path.
	cfg.MainRequestTimeout = 10 * time.Second
	store, err := OpenStore(":memory:", cfg.SessionSecret)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAgent(cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertUser(cfg.AgentID, "42", "u@example.com", "User"); err != nil {
		t.Fatal(err)
	}
	s := &Server{cfg: cfg, store: store, main: NewMainClient(cfg), webDir: ""}
	t.Cleanup(func() { _ = store.Close() })
	return s
}

func setTenantBillingModeForTest(t *testing.T, s *Server, mode string) {
	t.Helper()
	if _, err := s.store.db.Exec(`UPDATE agent_config SET billing_mode=? WHERE agent_id=?`, mode, s.cfg.AgentID); err != nil {
		t.Fatalf("set persisted tenant billing mode: %v", err)
	}
}

func TestHealthDoesNotExposeProcessTenant(t *testing.T) {
	upstream := httptest.NewServer(http.NotFoundHandler())
	defer upstream.Close()
	server := testServer(t, upstream)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if recorder.Code != http.StatusOK || strings.Contains(recorder.Body.String(), "agent_id") {
		t.Fatalf("health response leaked process tenant: %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestSharedAgentSSOCreatesTenantAndOwnerSession(t *testing.T) {
	upstream := httptest.NewServer(http.NotFoundHandler())
	defer upstream.Close()
	server := testServer(t, upstream)
	agentID := "agt_0123456789abcdef0123456789abcdef"
	raw := makeSharedAgentSSOTicket(t, server, "77", agentID, tenantRoleOwner, "Alice Station", "shared-create")

	request := httptest.NewRequest(http.MethodGet, "http://shared.example/api/auth/sso/callback?ticket="+url.QueryEscape(raw), nil)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusFound || response.Header().Get("Location") != "/dashboard" {
		t.Fatalf("shared SSO status=%d location=%q body=%s", response.Code, response.Header().Get("Location"), response.Body.String())
	}
	agent, err := server.store.Agent(agentID)
	if err != nil || agent.OwnerMainUserID != "77" || agent.BillingMode != "user_upstream" || agent.Status != "active" || agent.SiteName != "Alice Station" {
		t.Fatalf("unexpected shared tenant: agent=%+v err=%v", agent, err)
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly {
		t.Fatalf("unexpected shared SSO cookie: %#v", cookies)
	}
	session, err := server.store.LoadSession(cookies[0].Value)
	if err != nil || session.AgentID != agentID || session.MainUserID != "77" || session.Role != tenantRoleOwner {
		t.Fatalf("unexpected shared tenant session: session=%+v err=%v", session, err)
	}
}

func TestSharedAgentSSOIgnoresLegacyDomainClaimOnSharedHost(t *testing.T) {
	upstream := httptest.NewServer(http.NotFoundHandler())
	defer upstream.Close()
	server := testServer(t, upstream)
	server.cfg.SharedHosts = []string{"shared.example"}

	legacy := server.cfg
	legacy.AgentID = "agt_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	legacy.AgentDomain = "shared.example"
	legacy.OwnerMainUserID = "10"
	legacy.BillingMode = "user_upstream"
	if err := server.store.UpsertAgent(legacy); err != nil {
		t.Fatal(err)
	}

	newAgentID := "agt_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	raw := makeSharedAgentSSOTicket(t, server, "77", newAgentID, tenantRoleOwner, "Shared Station", "shared-host-legacy-domain")
	request := httptest.NewRequest(http.MethodGet, "http://shared.example/api/auth/sso/callback?ticket="+url.QueryEscape(raw), nil)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusFound || response.Header().Get("Location") != "/dashboard" {
		t.Fatalf("shared SSO was captured by legacy domain mapping: status=%d body=%s", response.Code, response.Body.String())
	}
	agent, err := server.store.Agent(newAgentID)
	if err != nil || agent.OwnerMainUserID != "77" {
		t.Fatalf("shared tenant was not created from signed claims: agent=%+v err=%v", agent, err)
	}
}

func TestSharedAgentSSONeverReassignsOwner(t *testing.T) {
	upstream := httptest.NewServer(http.NotFoundHandler())
	defer upstream.Close()
	server := testServer(t, upstream)
	agentID := "agt_11111111111111111111111111111111"
	first := makeSharedAgentSSOTicket(t, server, "77", agentID, tenantRoleOwner, "Owner Station", "owner-first")
	firstRequest := httptest.NewRequest(http.MethodGet, "http://shared.example/api/auth/sso/callback?ticket="+url.QueryEscape(first), nil)
	firstResponse := httptest.NewRecorder()
	server.ServeHTTP(firstResponse, firstRequest)
	if firstResponse.Code != http.StatusFound {
		t.Fatalf("initial owner SSO failed: status=%d body=%s", firstResponse.Code, firstResponse.Body.String())
	}

	second := makeSharedAgentSSOTicket(t, server, "88", agentID, tenantRoleOwner, "Hijacked", "owner-second")
	secondRequest := httptest.NewRequest(http.MethodGet, "http://shared.example/api/auth/sso/callback?ticket="+url.QueryEscape(second), nil)
	secondResponse := httptest.NewRecorder()
	server.ServeHTTP(secondResponse, secondRequest)
	if secondResponse.Code != http.StatusForbidden || !strings.Contains(secondResponse.Body.String(), "SSO_TENANT_FORBIDDEN") {
		t.Fatalf("owner reassignment was not rejected: status=%d body=%s", secondResponse.Code, secondResponse.Body.String())
	}
	agent, err := server.store.Agent(agentID)
	if err != nil || agent.OwnerMainUserID != "77" || agent.SiteName != "Owner Station" {
		t.Fatalf("tenant owner or branding changed: agent=%+v err=%v", agent, err)
	}
}

func TestSharedAgentSSORejectsCrossTenantHost(t *testing.T) {
	upstream := httptest.NewServer(http.NotFoundHandler())
	defer upstream.Close()
	server := testServer(t, upstream)
	other := server.cfg
	other.AgentID = "agt_22222222222222222222222222222222"
	other.AgentDomain = "other.example"
	other.OwnerMainUserID = "22"
	other.BillingMode = "user_upstream"
	if err := server.store.UpsertAgent(other); err != nil {
		t.Fatal(err)
	}
	raw := makeSharedAgentSSOTicket(t, server, "77", "agt_33333333333333333333333333333333", tenantRoleOwner, "Shared", "host-conflict")
	request := httptest.NewRequest(http.MethodGet, "http://other.example/api/auth/sso/callback?ticket="+url.QueryEscape(raw), nil)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "SSO_TENANT_FORBIDDEN") {
		t.Fatalf("cross-tenant host was not rejected: status=%d body=%s", response.Code, response.Body.String())
	}
	if _, err := server.store.Agent("agt_33333333333333333333333333333333"); !errors.Is(err, errNotFound) {
		t.Fatalf("cross-host callback created a tenant: %v", err)
	}
}

func TestVerifySSOTicketRejectsMalformedSharedTenantClaims(t *testing.T) {
	secret := "test-sso-secret-0123456789abcdef"
	now := time.Now().UTC()
	base := map[string]any{
		"iss": "sub2api", "aud": "agentapi", "sub": "77", "iat": now.Unix(),
		"exp": now.Add(90 * time.Second).Unix(), "jti": "claim-validation",
	}
	for name, values := range map[string]map[string]any{
		"role without id": {"agent_role": tenantRoleOwner},
		"invalid id":      {"agent_id": "agent-77", "agent_role": tenantRoleOwner},
		"invalid role":    {"agent_id": "agt_44444444444444444444444444444444", "agent_role": "admin"},
		"control in name": {"agent_id": "agt_55555555555555555555555555555555", "agent_role": tenantRoleOwner, "agent_name": "bad\nname"},
	} {
		t.Run(name, func(t *testing.T) {
			payload := make(map[string]any, len(base)+len(values))
			for key, value := range base {
				payload[key] = value
			}
			for key, value := range values {
				payload[key] = value
			}
			if _, err := verifySSOTicket(makeSSOTicket(t, secret, payload), secret, "agentapi", now); !errors.Is(err, errInvalidSSOTicket) {
				t.Fatalf("malformed tenant claim was accepted: %v", err)
			}
		})
	}
}

func enableTestPayment(t *testing.T, server *Server, secret string) PaymentConfig {
	t.Helper()
	updated, err := server.store.UpdatePaymentConfig(server.cfg.AgentID, PaymentConfig{
		Enabled: true, Provider: "manual", Currency: "CNY", WebhookSecret: secret,
		MinAmountCents: 100, MaxAmountCents: 100000, OrderTTLSeconds: int64((30 * time.Minute) / time.Second),
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	return updated
}

func envelope(data any) map[string]any {
	return map[string]any{"code": 0, "message": "success", "data": data}
}

func TestLoginUsesHttpOnlyAgentSession(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/login" {
			t.Fatalf("unexpected upstream path: %s", r.URL.Path)
		}
		w.Header().Set("Set-Cookie", "main_secret=must-not-leak")
		_ = json.NewEncoder(w).Encode(envelope(map[string]any{
			"access_token": "main-access-secret", "refresh_token": "main-refresh-secret", "expires_in": 3600,
			"token_type": "Bearer", "user": map[string]any{"id": 42, "email": "u@example.com", "role": "user"},
		}))
	}))
	defer upstream.Close()
	server := testServer(t, upstream)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"u@example.com","password":"password"}`))
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "main-access-secret") || strings.Contains(rec.Body.String(), "main-refresh-secret") {
		t.Fatalf("main credentials leaked: %s", rec.Body.String())
	}
	cookie := rec.Result().Cookies()
	if len(cookie) != 1 || !cookie[0].HttpOnly || cookie[0].Name != "agentapi_session" {
		t.Fatalf("unexpected session cookie: %#v", cookie)
	}

	// The upstream test server only implements login; /auth/me is not called by
	// this assertion, proving login itself does not expose a token.
}

func TestAgentContextReturnsCurrentUsersAvailableAndFrozenMainBalance(t *testing.T) {
	var gotHeaders http.Header
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/sub2api/balance" {
			http.NotFound(w, r)
			return
		}
		gotHeaders = r.Header.Clone()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "sub2api.user_balance", "balance": 12.34, "frozen_balance": 2.5,
		})
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	server.main = NewMainClient(server.cfg)
	sessionID, err := server.store.CreateSession("42", []byte(`{"id":"42","email":"u@example.com"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agent/context", nil)
	req.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("context status=%d body=%s", rec.Code, rec.Body.String())
	}
	if gotHeaders.Get("Authorization") != "Bearer app-secret" || gotHeaders.Get("X-Sub2API-On-Behalf-Of") != "42" || gotHeaders.Get("X-Sub2API-Satellite") != "agentapi" {
		t.Fatalf("context balance lookup did not use the satellite OBO contract: %v", gotHeaders)
	}
	var response struct {
		Data struct {
			User struct {
				BalanceCents       int64 `json:"balance_cents"`
				FrozenBalanceCents int64 `json:"frozen_balance_cents"`
			} `json:"user"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Data.User.BalanceCents != 1234 || response.Data.User.FrozenBalanceCents != 250 {
		t.Fatalf("unexpected context balance facts: %+v", response.Data.User)
	}
	for _, secret := range []string{"app-secret", "test-main-admin-key"} {
		if strings.Contains(rec.Body.String(), secret) {
			t.Fatalf("server credential leaked in context response: %s", rec.Body.String())
		}
	}
}

func TestAgentProfileUsesAuthenticatedMainProfileAndFiltersAdminFields(t *testing.T) {
	var profileCalls, updateCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer private-main-user-token" {
			t.Errorf("main user token was not used for the profile request: %v", r.Header)
		}
		if r.Header.Get("Cookie") != "" {
			t.Errorf("browser cookie was forwarded upstream: %v", r.Header)
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/auth/me":
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 42, "email": "profile@example.com", "username": "Current Name"}))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/user/profile":
			profileCalls++
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{
				"id": 42, "email": "profile@example.com", "username": "Current Name",
				"role": "admin", "is_admin": true, "balance": 987654, "access_token": "must-not-leak",
				"identities": []string{"private-identity"},
			}))
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/user":
			updateCalls++
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("decode update profile: %v", err)
			}
			if len(payload) != 1 || payload["username"] != "Changed Name" {
				t.Errorf("AgentAPI forwarded a profile field outside its allowlist: %#v", payload)
			}
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{
				"id": 42, "email": "profile@example.com", "username": "Changed Name",
				"role": "admin", "balance": 987654, "access_token": "must-not-leak",
			}))
		default:
			t.Errorf("unexpected Sub2API request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	sessionID, err := server.store.CreateSession("42", []byte(`{"id":"42","email":"stale@example.com"}`), "private-main-user-token", "refresh-token", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	cookie := &http.Cookie{Name: server.cfg.CookieName, Value: sessionID}

	get := httptest.NewRequest(http.MethodGet, "http://agent.local/api/v1/agent/profile", nil)
	get.AddCookie(cookie)
	getResponse := httptest.NewRecorder()
	server.ServeHTTP(getResponse, get)
	if getResponse.Code != http.StatusOK {
		t.Fatalf("profile status=%d body=%s", getResponse.Code, getResponse.Body.String())
	}
	if getResponse.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("profile response is cacheable: %v", getResponse.Header())
	}
	for _, privateField := range []string{"must-not-leak", "987654", "\"role\"", "\"is_admin\"", "identities"} {
		if strings.Contains(getResponse.Body.String(), privateField) {
			t.Fatalf("main-site profile field %q leaked to AgentAPI browser: %s", privateField, getResponse.Body.String())
		}
	}
	var profileResponse struct {
		Data struct {
			ID       string `json:"id"`
			Email    string `json:"email"`
			Username string `json:"username"`
			CanEdit  bool   `json:"can_edit"`
		} `json:"data"`
	}
	if err := json.Unmarshal(getResponse.Body.Bytes(), &profileResponse); err != nil {
		t.Fatal(err)
	}
	if profileResponse.Data.ID != "42" || profileResponse.Data.Email != "profile@example.com" || profileResponse.Data.Username != "Current Name" || !profileResponse.Data.CanEdit {
		t.Fatalf("unexpected profile response: %+v", profileResponse.Data)
	}
	crossOrigin := httptest.NewRequest(http.MethodPut, "http://agent.local/api/v1/agent/profile", strings.NewReader(`{"username":"Cross-origin"}`))
	crossOrigin.Header.Set("Content-Type", "application/json")
	crossOrigin.Header.Set("Origin", "https://attacker.example")
	crossOrigin.AddCookie(cookie)
	crossOriginResponse := httptest.NewRecorder()
	server.ServeHTTP(crossOriginResponse, crossOrigin)
	if crossOriginResponse.Code != http.StatusForbidden || updateCalls != 0 {
		t.Fatalf("cross-origin profile update was not rejected before upstream: status=%d calls=%d body=%s", crossOriginResponse.Code, updateCalls, crossOriginResponse.Body.String())
	}

	update := httptest.NewRequest(http.MethodPut, "http://agent.local/api/v1/agent/profile", strings.NewReader(`{"username":"Changed Name","role":"admin","balance":1}`))
	update.Header.Set("Content-Type", "application/json")
	update.Header.Set("Origin", "http://agent.local")
	update.AddCookie(cookie)
	updateResponse := httptest.NewRecorder()
	server.ServeHTTP(updateResponse, update)
	if updateResponse.Code != http.StatusOK || strings.Contains(updateResponse.Body.String(), "must-not-leak") {
		t.Fatalf("profile update status=%d body=%s", updateResponse.Code, updateResponse.Body.String())
	}
	localUser, err := server.store.User(server.cfg.AgentID, "42")
	if err != nil || localUser.DisplayName != "Changed Name" {
		t.Fatalf("AgentAPI user mapping did not reflect the updated username: user=%+v err=%v", localUser, err)
	}
	if profileCalls != 1 || updateCalls != 1 {
		t.Fatalf("unexpected main profile calls: get=%d update=%d", profileCalls, updateCalls)
	}
}

func TestAuthRefreshRejectsUnknownOrDifferentMainIdentity(t *testing.T) {
	tests := []struct {
		name             string
		accessToken      string
		refreshUser      map[string]any
		wantCurrentCalls int
		wantStatus       int
		wantReason       string
	}{
		{
			name:             "different user in refresh response",
			accessToken:      "rotated-access-token",
			refreshUser:      map[string]any{"id": 99, "email": "other-user@example.com"},
			wantCurrentCalls: 0,
			wantStatus:       http.StatusUnauthorized,
			wantReason:       "SESSION_IDENTITY_MISMATCH",
		},
		{
			name:             "identity absent from refresh response and different current user",
			accessToken:      "rotated-access-token",
			refreshUser:      map[string]any{"email": "unknown-user@example.com"},
			wantCurrentCalls: 1,
			wantStatus:       http.StatusUnauthorized,
			wantReason:       "SESSION_IDENTITY_MISMATCH",
		},
		{
			name:             "refresh response missing access token",
			accessToken:      "",
			refreshUser:      map[string]any{"id": 42, "email": "u@example.com"},
			wantCurrentCalls: 0,
			wantStatus:       http.StatusBadGateway,
			wantReason:       "UPSTREAM_AUTH_INVALID",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var currentCalls int
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodPost && r.URL.Path == "/api/v1/auth/refresh":
					var payload map[string]any
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						t.Errorf("decode refresh request: %v", err)
					}
					if payload["refresh_token"] != "bound-refresh-token" {
						t.Errorf("unexpected refresh token: %#v", payload["refresh_token"])
					}
					result := map[string]any{
						"access_token":  test.accessToken,
						"refresh_token": "rotated-refresh-token",
						"expires_in":    3600,
						"user":          test.refreshUser,
					}
					_ = json.NewEncoder(w).Encode(envelope(result))
				case r.Method == http.MethodGet && r.URL.Path == "/api/v1/auth/me":
					currentCalls++
					if r.Header.Get("Authorization") != "Bearer rotated-access-token" {
						t.Errorf("current-user lookup did not use the rotated token: %v", r.Header)
					}
					_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 99, "email": "other-user@example.com"}))
				default:
					t.Errorf("unexpected Sub2API request: %s %s", r.Method, r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer upstream.Close()
			server := testServer(t, upstream)
			sessionID, err := server.store.CreateSession("42", []byte(`{"id":"42","email":"u@example.com"}`), "expired-access-token", "bound-refresh-token", time.Now().Add(sessionTTL))
			if err != nil {
				t.Fatal(err)
			}

			request := httptest.NewRequest(http.MethodPost, "http://agent.local/api/v1/auth/refresh", nil)
			request.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
			response := httptest.NewRecorder()
			server.ServeHTTP(response, request)
			if response.Code != test.wantStatus || !strings.Contains(response.Body.String(), test.wantReason) {
				t.Fatalf("refresh accepted an unverified identity: status=%d body=%s", response.Code, response.Body.String())
			}
			if strings.Contains(response.Body.String(), "other-user@example.com") || strings.Contains(response.Body.String(), "rotated-access-token") {
				t.Fatalf("refreshed identity or credentials leaked to the browser: %s", response.Body.String())
			}
			if _, err := server.store.LoadSession(sessionID); err == nil {
				t.Fatal("mismatched refresh left the local session active")
			}
			cookies := response.Result().Cookies()
			if len(cookies) != 1 || cookies[0].Name != server.cfg.CookieName || cookies[0].MaxAge >= 0 {
				t.Fatalf("mismatched refresh did not clear the session cookie: %#v", cookies)
			}
			if currentCalls != test.wantCurrentCalls {
				t.Fatalf("unexpected current-user lookups: got=%d want=%d", currentCalls, test.wantCurrentCalls)
			}
		})
	}
}

func TestCurrentSessionAutoRefreshRejectsIdentityWithoutMatchingUserID(t *testing.T) {
	var currentCalls, profileCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/auth/me":
			currentCalls++
			if r.Header.Get("Authorization") == "Bearer expired-access-token" {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]any{"code": http.StatusUnauthorized, "message": "expired"})
				return
			}
			if r.Header.Get("Authorization") != "Bearer rotated-access-token" {
				t.Errorf("current-user lookup used unexpected token: %v", r.Header)
			}
			// A successful response without an ID cannot prove that it still
			// belongs to the AgentAPI session being refreshed.
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"email": "unverified@example.com"}))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/auth/refresh":
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{
				"access_token": "rotated-access-token", "refresh_token": "rotated-refresh-token",
				"user": map[string]any{"id": 42, "email": "u@example.com"},
			}))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/user/profile":
			profileCalls++
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 42, "email": "u@example.com"}))
		default:
			t.Errorf("unexpected Sub2API request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	sessionID, err := server.store.CreateSession("42", []byte(`{"id":"42","email":"u@example.com"}`), "expired-access-token", "bound-refresh-token", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "http://agent.local/api/v1/agent/profile", nil)
	request.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "SESSION_IDENTITY_MISMATCH") {
		t.Fatalf("automatic refresh accepted an identity without a matching user ID: status=%d body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "unverified@example.com") || profileCalls != 0 {
		t.Fatalf("unverified profile data reached the AgentAPI browser: profile_calls=%d body=%s", profileCalls, response.Body.String())
	}
	session, err := server.store.LoadSession(sessionID)
	if err != nil {
		t.Fatalf("identity mismatch unexpectedly modified/deleted the stored session: %v", err)
	}
	if session.AccessToken != "expired-access-token" || session.RefreshToken != "bound-refresh-token" {
		t.Fatalf("identity mismatch replaced the original session credentials: access=%q refresh=%q", session.AccessToken, session.RefreshToken)
	}
	if currentCalls != 2 {
		t.Fatalf("unexpected current-user lookup count: got=%d want=2", currentCalls)
	}
}

func TestAgentSSOProfileIsReadOnlyAndPasswordChangeRevokesSession(t *testing.T) {
	var upstreamCalls, passwordCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		if r.Header.Get("Authorization") != "Bearer password-change-token" || r.Header.Get("Cookie") != "" {
			t.Errorf("unexpected credentials on password endpoint: %v", r.Header)
		}
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/auth/me" {
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 42, "email": "u@example.com"}))
			return
		}
		if r.Method != http.MethodPut || r.URL.Path != "/api/v1/user/password" {
			t.Errorf("identity-only SSO session reached an account-edit endpoint: %s %s", r.Method, r.URL.Path)
		}
		passwordCalls++
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode password change: %v", err)
		}
		if len(payload) != 2 || payload["old_password"] != "old-password" || payload["new_password"] != "new-password" {
			t.Errorf("unexpected password payload: %#v", payload)
		}
		_ = json.NewEncoder(w).Encode(envelope(map[string]any{"message": "Password changed"}))
	}))
	defer upstream.Close()
	server := testServer(t, upstream)

	identitySession, err := server.store.CreateSession("42", []byte(`{"id":"42","email":"u@example.com","username":"Ticket Name"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	identityCookie := &http.Cookie{Name: server.cfg.CookieName, Value: identitySession}
	profile := httptest.NewRequest(http.MethodGet, "http://agent.local/api/v1/agent/profile", nil)
	profile.AddCookie(identityCookie)
	profileResponse := httptest.NewRecorder()
	server.ServeHTTP(profileResponse, profile)
	if profileResponse.Code != http.StatusOK || !strings.Contains(profileResponse.Body.String(), `"can_edit":false`) || !strings.Contains(profileResponse.Body.String(), "Ticket Name") {
		t.Fatalf("SSO identity profile was not returned read-only: status=%d body=%s", profileResponse.Code, profileResponse.Body.String())
	}
	blocked := httptest.NewRequest(http.MethodPut, "http://agent.local/api/v1/agent/password", strings.NewReader(`{"old_password":"old-password","new_password":"new-password"}`))
	blocked.Header.Set("Origin", "http://agent.local")
	blocked.AddCookie(identityCookie)
	blockedResponse := httptest.NewRecorder()
	server.ServeHTTP(blockedResponse, blocked)
	if blockedResponse.Code != http.StatusForbidden || upstreamCalls != 0 {
		t.Fatalf("identity-only SSO session changed password: status=%d calls=%d body=%s", blockedResponse.Code, upstreamCalls, blockedResponse.Body.String())
	}

	authenticatedSession, err := server.store.CreateSession("42", []byte(`{"id":"42","email":"u@example.com"}`), "password-change-token", "password-change-refresh", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	change := httptest.NewRequest(http.MethodPut, "http://agent.local/api/v1/agent/password", strings.NewReader(`{"old_password":"old-password","new_password":"new-password"}`))
	change.Header.Set("Content-Type", "application/json")
	change.Header.Set("Origin", "http://agent.local")
	change.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: authenticatedSession})
	changeResponse := httptest.NewRecorder()
	server.ServeHTTP(changeResponse, change)
	if changeResponse.Code != http.StatusOK || !strings.Contains(strings.ToLower(changeResponse.Body.String()), "sign in again") {
		t.Fatalf("password change status=%d body=%s", changeResponse.Code, changeResponse.Body.String())
	}
	if changeResponse.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("password change response is cacheable: %v", changeResponse.Header())
	}
	if strings.Contains(changeResponse.Body.String(), "old-password") || strings.Contains(changeResponse.Body.String(), "new-password") {
		t.Fatalf("password value leaked in response: %s", changeResponse.Body.String())
	}
	if session, err := server.store.LoadSession(authenticatedSession); err == nil || session.ID != "" {
		t.Fatalf("successful password change retained the local session: session=%+v err=%v", session, err)
	}
	cleared := changeResponse.Result().Cookies()
	if len(cleared) != 1 || cleared[0].Name != server.cfg.CookieName || cleared[0].MaxAge >= 0 {
		t.Fatalf("successful password change did not clear the AgentAPI cookie: %#v", cleared)
	}
	if upstreamCalls != 2 || passwordCalls != 1 {
		t.Fatalf("unexpected account-edit calls for SSO+password flow: total=%d password=%d", upstreamCalls, passwordCalls)
	}
}

func TestLoginRejectsMappedUserDisabledOnThisAgent(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/login" {
			t.Errorf("unexpected upstream path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(envelope(map[string]any{
			"access_token": "main-access", "refresh_token": "main-refresh", "expires_in": 3600,
			"user": map[string]any{"id": 43, "email": "mapped@example.com", "role": "user"},
		}))
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	if _, err := server.store.UpsertUser(server.cfg.AgentID, "43", "mapped@example.com", "Mapped"); err != nil {
		t.Fatal(err)
	}
	if _, err := server.store.SetUserStatus(server.cfg.AgentID, "43", "disabled"); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"mapped@example.com","password":"password"}`))
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "AGENT_USER_DISABLED") || len(rec.Result().Cookies()) != 0 {
		t.Fatalf("disabled mapped user login status=%d cookies=%v body=%s", rec.Code, rec.Result().Cookies(), rec.Body.String())
	}
}

func TestFailedMainRegistrationDoesNotCreateLocalAgentMapping(t *testing.T) {
	var registerCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/admin/users":
			registerCalls.Add(1)
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "REGISTRATION_REJECTED", "message": "registration rejected"})
		default:
			t.Errorf("unexpected Sub2API request: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	server := testServer(t, upstream)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(`{"email":"new@example.com","password":"password"}`))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code == http.StatusOK || len(response.Result().Cookies()) != 0 {
		t.Fatalf("failed main-site registration unexpectedly created a session: status=%d cookies=%v body=%s", response.Code, response.Result().Cookies(), response.Body.String())
	}
	if _, err := server.store.User(server.cfg.AgentID, "43"); err != errNotFound {
		t.Fatalf("failed main-site registration left a local Agent mapping: user err=%v", err)
	}
	if registerCalls.Load() != 1 {
		t.Fatalf("unexpected registration calls: %d", registerCalls.Load())
	}
}

func TestRegistrationCreatesLocalMappingWithoutLegacyRuntimeCall(t *testing.T) {
	var registerCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/admin/users":
			registerCalls.Add(1)
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 43, "email": "new@example.com", "role": "user"}))
		case "/api/v1/auth/login":
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{
				"access_token": "register-access", "refresh_token": "register-refresh", "expires_in": 3600,
				"user": map[string]any{"id": 43, "email": "new@example.com", "role": "user"},
			}))
		default:
			t.Errorf("unexpected Sub2API request: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	server := testServer(t, upstream)
	registerRequest := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(`{"email":"new@example.com","password":"password"}`))
	registerResponse := httptest.NewRecorder()
	server.ServeHTTP(registerResponse, registerRequest)
	if registerResponse.Code != http.StatusOK || len(registerResponse.Result().Cookies()) != 1 {
		t.Fatalf("registration did not create a shared-site session: status=%d cookies=%v body=%s", registerResponse.Code, registerResponse.Result().Cookies(), registerResponse.Body.String())
	}
	if user, err := server.store.User(server.cfg.AgentID, "43"); err != nil || user.Status != "active" {
		t.Fatalf("local tenant mapping was not created: user=%+v err=%v", user, err)
	}
	if registerCalls.Load() != 1 {
		t.Fatalf("registration called main site %d times", registerCalls.Load())
	}
}

func TestSSOCallbackCreatesLocalSessionAndConsumesTicketOnce(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("SSO session should not call the main auth API: %s", r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	server.cfg.SSOSecret = strings.Repeat("s", 32)
	server.cfg.SSOAudience = "agentapi"
	now := time.Now().UTC().Unix()
	ticket := makeSSOTicket(t, server.cfg.SSOSecret, map[string]any{
		"iss": "sub2api", "aud": "agentapi", "sub": "42", "jti": "sso-once",
		"iat": now, "exp": now + 60, "email": "u@example.com", "username": "SSO user", "next": "/dashboard",
	})
	request := httptest.NewRequest(http.MethodGet, "/api/auth/sso/callback?ticket="+url.QueryEscape(ticket), nil)
	first := httptest.NewRecorder()
	server.ServeHTTP(first, request)
	if first.Code != http.StatusFound || first.Header().Get("Location") != "/dashboard" {
		t.Fatalf("SSO callback status=%d headers=%v body=%s", first.Code, first.Header(), first.Body.String())
	}
	cookies := first.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].MaxAge != int(sessionTTL.Seconds()) {
		t.Fatalf("unexpected SSO session cookie: %#v", cookies)
	}
	me := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	me.AddCookie(cookies[0])
	meResponse := httptest.NewRecorder()
	server.ServeHTTP(meResponse, me)
	if meResponse.Code != http.StatusOK || strings.Contains(meResponse.Body.String(), "access_token") {
		t.Fatalf("SSO auth/me leaked credentials or failed: status=%d body=%s", meResponse.Code, meResponse.Body.String())
	}
	replay := httptest.NewRecorder()
	server.ServeHTTP(replay, httptest.NewRequest(http.MethodGet, "/api/auth/sso/callback?ticket="+url.QueryEscape(ticket), nil))
	if replay.Code != http.StatusConflict || !strings.Contains(replay.Body.String(), "SSO_TICKET_REPLAYED") {
		t.Fatalf("SSO replay status=%d body=%s", replay.Code, replay.Body.String())
	}
}

func TestMainSiteLoginRedirectUsesRegisteredSatelliteStart(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("main-site login redirect must not call Sub2API server-side: %s", r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	server.cfg.PublicMainURL = upstream.URL

	request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/main-site/login?next="+url.QueryEscape("/usage?page=2"), nil)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusFound {
		t.Fatalf("main-site login status=%d body=%s", response.Code, response.Body.String())
	}
	target, err := url.Parse(response.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse redirect: %v", err)
	}
	if target.Scheme+"://"+target.Host != upstream.URL || target.Path != "/api/v1/auth/integrations/agentapi/start" || target.Query().Get("next") != "/usage?page=2" {
		t.Fatalf("unexpected main-site SSO redirect: %s", target.String())
	}
	if target.Query().Has("key") || target.Query().Has("token") || strings.Contains(target.RawQuery, "credential") {
		t.Fatalf("main-site SSO redirect leaked credentials: %s", target.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("missing privacy headers: %#v", response.Header())
	}
}

func TestMainSiteLoginRejectsUnsafeOrMissingPublicOrigin(t *testing.T) {
	upstream := httptest.NewServer(http.NotFoundHandler())
	defer upstream.Close()
	server := testServer(t, upstream)
	for _, publicURL := range []string{"", "http://remote.example", "https://user:pass@main.example", "https://main.example/path", "javascript:alert(1)"} {
		server.cfg.PublicMainURL = publicURL
		response := httptest.NewRecorder()
		server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/auth/main-site/login?next=//evil.example", nil))
		if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "MAIN_SITE_LOGIN_UNAVAILABLE") {
			t.Fatalf("unsafe public URL %q status=%d body=%s", publicURL, response.Code, response.Body.String())
		}
	}
}

func TestReadyEndpointChecksAgentAndModelCredential(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)

	ready := httptest.NewRecorder()
	server.ServeHTTP(ready, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if ready.Code != http.StatusOK {
		t.Fatalf("shared runtime ready status=%d body=%s", ready.Code, ready.Body.String())
	}
	server.cfg.AppCredential = ""
	notReady := httptest.NewRecorder()
	server.ServeHTTP(notReady, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if notReady.Code != http.StatusServiceUnavailable {
		t.Fatalf("missing credential ready status=%d body=%s", notReady.Code, notReady.Body.String())
	}
	server.cfg.AppCredential = "app-secret"
	server.cfg.SSOSecret = "short"
	weakSSO := httptest.NewRecorder()
	server.ServeHTTP(weakSSO, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if weakSSO.Code != http.StatusServiceUnavailable || !strings.Contains(weakSSO.Body.String(), "SSO_SECRET_WEAK") {
		t.Fatalf("weak SSO secret ready status=%d body=%s", weakSSO.Code, weakSSO.Body.String())
	}
}

func TestHealthAndReadinessProbesDoNotContactSub2API(t *testing.T) {
	var upstreamRequests atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamRequests.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)

	for _, path := range []string{"/healthz", "/readyz"} {
		t.Run(strings.TrimPrefix(path, "/"), func(t *testing.T) {
			response := httptest.NewRecorder()
			server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
			if response.Code != http.StatusOK {
				t.Fatalf("%s returned %d: %s", path, response.Code, response.Body.String())
			}
		})
	}
	if calls := upstreamRequests.Load(); calls != 0 {
		t.Fatalf("health/readiness probes contacted Sub2API %d times", calls)
	}
}

func TestHealthProbeRequiresLocalPersistence(t *testing.T) {
	upstream := httptest.NewServer(http.NotFoundHandler())
	defer upstream.Close()
	server := testServer(t, upstream)

	if err := server.store.Close(); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "STORE_UNAVAILABLE") {
		t.Fatalf("closed store health status=%d body=%s", response.Code, response.Body.String())
	}

	method := httptest.NewRecorder()
	server.ServeHTTP(method, httptest.NewRequest(http.MethodPost, "/healthz", nil))
	if method.Code != http.StatusMethodNotAllowed {
		t.Fatalf("health POST status=%d body=%s", method.Code, method.Body.String())
	}
}

func TestModelRelayAddsSatelliteHeadersAndNeverCopiesCookies(t *testing.T) {
	var gotHeaders http.Header
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/auth/login" {
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{
				"access_token": "main-access", "refresh_token": "main-refresh", "expires_in": 3600,
				"user": map[string]any{"id": 42, "email": "u@example.com", "role": "user"},
			}))
			return
		}
		if r.URL.Path == "/v1/sub2api/balance" {
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 42, "email": "u@example.com", "balance": 10.0}))
			return
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected model path: %s", r.URL.Path)
		}
		gotHeaders = r.Header.Clone()
		w.Header().Set("Set-Cookie", "upstream=secret")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"completion-1"}`)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	if err := server.store.CreditAgent(server.cfg.AgentID, 1000, "seed", "seed", "test"); err != nil {
		t.Fatal(err)
	}
	if err := server.store.Allocate(server.cfg.AgentID, "42", 500, "allocate", "order", "test"); err != nil {
		t.Fatal(err)
	}

	login := httptest.NewRecorder()
	server.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"u@example.com","password":"password"}`)))
	if login.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", login.Code, login.Body.String())
	}
	cookie := login.Result().Cookies()[0]
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.5","messages":[]}`))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || strings.Contains(strings.ToLower(rec.Header().Get("Set-Cookie")), "upstream") {
		t.Fatalf("relay status=%d headers=%v body=%s", rec.Code, rec.Header(), rec.Body.String())
	}
	if gotHeaders.Get("Authorization") != "Bearer app-secret" || gotHeaders.Get("X-Sub2API-On-Behalf-Of") != "42" || gotHeaders.Get("X-Sub2API-Satellite") != "agentapi" {
		t.Fatalf("missing relay headers: %v", gotHeaders)
	}
	user, err := server.store.User(server.cfg.AgentID, "42")
	if err != nil {
		t.Fatal(err)
	}
	if user.BalanceCents != 500 {
		t.Fatalf("direct Sub2API billing changed the legacy local wallet: %+v", user)
	}
}

func TestModelsEndpointExposesOnlyTheAgentPublicCatalog(t *testing.T) {
	upstreamCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		t.Fatalf("models listing must not call upstream: %s", r.URL.Path)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	sessionID, err := server.store.CreateSession("42", []byte(`{"id":"42","email":"u@example.com"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("models status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Object string `json:"object"`
		Data   []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Object != "list" || len(payload.Data) != len(publicModelCatalog) {
		t.Fatalf("unexpected public model response: object=%q count=%d", payload.Object, len(payload.Data))
	}
	for _, item := range payload.Data {
		if _, ok := publicModelNames[item.ID]; !ok {
			t.Fatalf("private or unknown model leaked: %q", item.ID)
		}
	}
	if upstreamCalls != 0 {
		t.Fatalf("unexpected upstream calls: %d", upstreamCalls)
	}
}

func TestRechargeWebhookRequiresSignatureAndAllocatesVerifiedOrder(t *testing.T) {
	adminReads := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/sub2api/balance" {
			t.Fatalf("unexpected upstream path: %s", r.URL.Path)
		}
		adminReads++
		_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 42, "email": "u@example.com", "balance": 100.0}))
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	payment := enableTestPayment(t, server, "payment-secret")

	sessionID, err := server.store.CreateSession("42", []byte(`{"id":"42","email":"u@example.com"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	create := httptest.NewRequest(http.MethodPost, "/api/v1/agent/recharge/orders", strings.NewReader(`{"amount":"1.00"}`))
	create.Header.Set("Idempotency-Key", "recharge-test-1")
	create.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	created := httptest.NewRecorder()
	server.ServeHTTP(created, create)
	if created.Code != http.StatusCreated {
		t.Fatalf("create order status=%d body=%s", created.Code, created.Body.String())
	}
	var createdEnvelope struct {
		Data struct {
			Order RechargeOrder `json:"order"`
		} `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &createdEnvelope); err != nil {
		t.Fatal(err)
	}
	order := createdEnvelope.Data.Order
	if order.OrderNo == "" || order.Status != "pending" || order.AmountCents != 100 {
		t.Fatalf("unexpected order: %+v", order)
	}

	body := []byte(fmt.Sprintf(`{"order_no":%q,"status":"paid","amount_cents":100,"currency":"CNY","provider_trade_no":"trade-1"}`, order.OrderNo))
	bad := httptest.NewRequest(http.MethodPost, "/api/v1/payments/webhook", bytes.NewReader(body))
	bad.Header.Set("X-Agent-Payment-Event-ID", "evt-bad")
	bad.Header.Set("X-Agent-Payment-Signature", "sha256=00")
	badResponse := httptest.NewRecorder()
	server.ServeHTTP(badResponse, bad)
	if badResponse.Code != http.StatusUnauthorized {
		t.Fatalf("bad signature status=%d body=%s", badResponse.Code, badResponse.Body.String())
	}

	mac := hmac.New(sha256.New, []byte(payment.WebhookSecret))
	_, _ = mac.Write(body)
	valid := httptest.NewRequest(http.MethodPost, "/api/v1/payments/webhook", bytes.NewReader(body))
	valid.Header.Set("X-Agent-Payment-Event-ID", "evt-1")
	valid.Header.Set("X-Agent-Payment-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	validResponse := httptest.NewRecorder()
	server.ServeHTTP(validResponse, valid)
	if validResponse.Code != http.StatusOK {
		t.Fatalf("valid webhook status=%d body=%s", validResponse.Code, validResponse.Body.String())
	}
	allocated, err := server.store.RechargeOrder(server.cfg.AgentID, order.OrderNo)
	if err != nil {
		t.Fatal(err)
	}
	if allocated.Status != "allocated" || adminReads == 0 {
		t.Fatalf("order was not allocated after owner sync: %+v reads=%d", allocated, adminReads)
	}
	user, err := server.store.User(server.cfg.AgentID, "42")
	if err != nil {
		t.Fatal(err)
	}
	if user.BalanceCents != 100 {
		t.Fatalf("verified recharge did not credit user sub-balance: %+v", user)
	}

	duplicate := httptest.NewRequest(http.MethodPost, "/api/v1/payments/webhook", bytes.NewReader(body))
	duplicate.Header.Set("X-Agent-Payment-Event-ID", "evt-1")
	duplicate.Header.Set("X-Agent-Payment-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	duplicateResponse := httptest.NewRecorder()
	server.ServeHTTP(duplicateResponse, duplicate)
	if duplicateResponse.Code != http.StatusOK || !strings.Contains(duplicateResponse.Body.String(), `"duplicate":true`) {
		t.Fatalf("duplicate webhook was not idempotent: status=%d body=%s", duplicateResponse.Code, duplicateResponse.Body.String())
	}
	replayedUser, err := server.store.User(server.cfg.AgentID, "42")
	if err != nil {
		t.Fatal(err)
	}
	if replayedUser.BalanceCents != 100 {
		t.Fatalf("duplicate webhook credited the user twice: %+v", replayedUser)
	}
}

func TestRechargeWebhookRejectsAnotherInstancesMerchant(t *testing.T) {
	upstreamCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	payment := enableTestPayment(t, server, "merchant-isolation-secret")
	payment.MerchantID = "merchant-a"
	updated, err := server.store.UpdatePaymentConfig(server.cfg.AgentID, payment, false)
	if err != nil {
		t.Fatal(err)
	}
	payment = updated

	if _, err := server.store.UpsertUser(server.cfg.AgentID, "43", "payer@example.com", "Payer"); err != nil {
		t.Fatal(err)
	}
	order, _, err := server.store.CreateRechargeOrder(server.cfg.AgentID, "43", 100, "CNY", payment.Provider, "", "merchant-isolation-order", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(fmt.Sprintf(`{"order_no":%q,"status":"paid","amount_cents":100,"currency":"CNY","merchant_id":"merchant-b","provider_trade_no":"trade-other-merchant"}`, order.OrderNo))
	mac := hmac.New(sha256.New, []byte(payment.WebhookSecret))
	_, _ = mac.Write(body)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/payments/webhook", bytes.NewReader(body))
	request.Header.Set("X-Agent-Payment-Event-ID", "evt-other-merchant")
	request.Header.Set("X-Agent-Payment-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "PAYMENT_MERCHANT_MISMATCH") {
		t.Fatalf("merchant mismatch status=%d body=%s", response.Code, response.Body.String())
	}
	stored, err := server.store.RechargeOrder(server.cfg.AgentID, order.OrderNo)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != "pending" || upstreamCalls != 0 {
		t.Fatalf("wrong merchant changed payment state: order=%+v upstream_calls=%d", stored, upstreamCalls)
	}
}

func TestPaidRechargeWaitsForOwnerCreditAndAdminCanRetryAllocation(t *testing.T) {
	adminReads := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/sub2api/balance" {
			t.Fatalf("unexpected upstream path: %s", r.URL.Path)
		}
		adminReads++
		balance := 0.0
		if adminReads > 1 {
			balance = 100.0
		}
		_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 42, "email": "u@example.com", "balance": balance}))
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	payment := enableTestPayment(t, server, "payment-secret")
	sessionID, err := server.store.CreateSession("42", []byte(`{"id":"42","email":"u@example.com"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	create := httptest.NewRequest(http.MethodPost, "/api/v1/agent/recharge/orders", strings.NewReader(`{"amount":"1.00"}`))
	create.Header.Set("Idempotency-Key", "recharge-pending-1")
	create.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	created := httptest.NewRecorder()
	server.ServeHTTP(created, create)
	if created.Code != http.StatusCreated {
		t.Fatalf("create order status=%d body=%s", created.Code, created.Body.String())
	}
	var createdEnvelope struct {
		Data struct {
			Order RechargeOrder `json:"order"`
		} `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &createdEnvelope); err != nil {
		t.Fatal(err)
	}
	order := createdEnvelope.Data.Order
	body := []byte(fmt.Sprintf(`{"order_no":%q,"status":"paid","amount_cents":100,"currency":"CNY","provider_trade_no":"trade-pending"}`, order.OrderNo))
	mac := hmac.New(sha256.New, []byte(payment.WebhookSecret))
	_, _ = mac.Write(body)
	webhook := httptest.NewRequest(http.MethodPost, "/api/v1/payments/webhook", bytes.NewReader(body))
	webhook.Header.Set("X-Agent-Payment-Event-ID", "evt-pending")
	webhook.Header.Set("X-Agent-Payment-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, webhook)
	if response.Code != http.StatusAccepted {
		t.Fatalf("pending webhook status=%d body=%s", response.Code, response.Body.String())
	}
	pending, err := server.store.RechargeOrder(server.cfg.AgentID, order.OrderNo)
	if err != nil || pending.Status != "paid_pending_allocation" {
		t.Fatalf("expected durable pending allocation order: %+v err=%v", pending, err)
	}

	retry := httptest.NewRequest(http.MethodPost, "/api/v1/agent/admin/recharge/allocate", strings.NewReader(fmt.Sprintf(`{"order_no":%q}`, order.OrderNo)))
	retry.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	retried := httptest.NewRecorder()
	server.ServeHTTP(retried, retry)
	if retried.Code != http.StatusOK {
		t.Fatalf("admin allocation status=%d body=%s", retried.Code, retried.Body.String())
	}
	allocated, err := server.store.RechargeOrder(server.cfg.AgentID, order.OrderNo)
	if err != nil || allocated.Status != "allocated" {
		t.Fatalf("admin retry did not allocate order: %+v err=%v", allocated, err)
	}
}

func TestAgentAPIKeyCanRelayWithoutCookieAndIsNotForwarded(t *testing.T) {
	var gotHeaders http.Header
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/login":
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{
				"access_token": "main-access", "refresh_token": "main-refresh", "expires_in": 3600,
				"user": map[string]any{"id": 42, "email": "u@example.com", "role": "user"},
			}))
		case "/v1/sub2api/balance":
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 42, "email": "u@example.com", "balance": 10.0}))
		case "/v1/chat/completions":
			gotHeaders = r.Header.Clone()
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":"completion-key"}`)
		default:
			t.Errorf("unexpected upstream path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	if err := server.store.CreditAgent(server.cfg.AgentID, 1000, "seed", "seed", "test"); err != nil {
		t.Fatal(err)
	}
	if err := server.store.Allocate(server.cfg.AgentID, "42", 500, "allocate", "order", "test"); err != nil {
		t.Fatal(err)
	}

	login := httptest.NewRecorder()
	server.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"u@example.com","password":"password"}`)))
	if login.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", login.Code, login.Body.String())
	}
	cookie := login.Result().Cookies()[0]
	create := httptest.NewRequest(http.MethodPost, "/api/v1/api-keys", strings.NewReader(`{"name":"CLI"}`))
	create.AddCookie(cookie)
	created := httptest.NewRecorder()
	server.ServeHTTP(created, create)
	if created.Code != http.StatusCreated {
		t.Fatalf("create key status=%d body=%s", created.Code, created.Body.String())
	}
	var createdEnvelope struct {
		Data struct {
			Key string `json:"key"`
		} `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &createdEnvelope); err != nil {
		t.Fatal(err)
	}
	if createdEnvelope.Data.Key == "" {
		t.Fatalf("one-time key missing: %s", created.Body.String())
	}

	relay := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.5","messages":[]}`))
	relay.Header.Set("Authorization", "Bearer "+createdEnvelope.Data.Key)
	relayResponse := httptest.NewRecorder()
	server.ServeHTTP(relayResponse, relay)
	if relayResponse.Code != http.StatusOK {
		t.Fatalf("key relay status=%d body=%s", relayResponse.Code, relayResponse.Body.String())
	}
	if gotHeaders.Get("Authorization") != "Bearer app-secret" || gotHeaders.Get("X-Sub2API-On-Behalf-Of") != "42" || gotHeaders.Get("X-Sub2API-Satellite") != "agentapi" {
		t.Fatalf("unexpected upstream identity headers: %v", gotHeaders)
	}
	if strings.Contains(gotHeaders.Get("Authorization"), createdEnvelope.Data.Key) {
		t.Fatalf("local AgentAPI key leaked upstream: %v", gotHeaders)
	}
	user, err := server.store.User(server.cfg.AgentID, "42")
	if err != nil {
		t.Fatal(err)
	}
	if user.BalanceCents != 500 {
		t.Fatalf("AgentAPI key relay changed the legacy local wallet: %+v", user)
	}
}

func TestModelRelayUsesMappedUserAsBillingIdentity(t *testing.T) {
	var modelHeader http.Header
	adminReads := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/sub2api/balance":
			if r.Header.Get("Authorization") != "Bearer app-secret" || r.Header.Get("X-Sub2API-On-Behalf-Of") != "42" || r.Header.Get("X-Sub2API-Satellite") != "agentapi" {
				t.Errorf("ordinary user balance headers = %v", r.Header)
			}
			adminReads++
			balance := 10.0
			if adminReads > 1 {
				balance = 9.0
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"object": "sub2api.user_balance", "balance": balance})
		case "/v1/chat/completions":
			modelHeader = r.Header.Clone()
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":"owner-billed"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	server.cfg.OwnerMainUserID = "99"
	server.main = NewMainClient(server.cfg)
	if err := server.store.CreditAgent(server.cfg.AgentID, 1000, "seed-owner-test", "seed", "test"); err != nil {
		t.Fatal(err)
	}
	if err := server.store.Allocate(server.cfg.AgentID, "42", 500, "allocate-owner-test", "order", "test"); err != nil {
		t.Fatal(err)
	}
	_, key, err := server.store.CreateAPIKey(server.cfg.AgentID, "42", "owner billing")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.5","messages":[]}`))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("X-Request-ID", "req-owner-billing")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("relay status=%d body=%s", response.Code, response.Body.String())
	}
	if modelHeader.Get("X-Sub2API-On-Behalf-Of") != "42" {
		t.Fatalf("upstream did not receive the mapped user billing identity: %v", modelHeader)
	}
	user, err := server.store.User(server.cfg.AgentID, "42")
	if err != nil {
		t.Fatal(err)
	}
	if user.BalanceCents != 500 {
		t.Fatalf("mapped user billing changed the legacy local wallet: %+v", user)
	}
	usage, err := server.store.Usage(server.cfg.AgentID, "42", 10)
	if err != nil || len(usage) != 1 || usage[0].SettlementStatus != "confirmed" || usage[0].ActualCents != 100 {
		t.Fatalf("unexpected user settlement: %+v err=%v", usage, err)
	}
	settlement, err := server.store.Settlement(server.cfg.AgentID, "req-owner-billing")
	if err != nil || settlement.ProxyMainUserID != "42" || settlement.BillingMainUserID != "42" {
		t.Fatalf("mapped user was not used as the billing identity: settlement=%+v err=%v", settlement, err)
	}
}

func TestModelRelayRejectsInsufficientMappedUserBalanceBeforeUpstream(t *testing.T) {
	var modelCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/sub2api/balance":
			if r.Header.Get("X-Sub2API-On-Behalf-Of") != "42" {
				t.Errorf("balance lookup used the wrong user identity: %v", r.Header)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"object": "sub2api.user_balance", "balance": 0})
		case "/v1/chat/completions":
			modelCalls.Add(1)
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	setTenantBillingModeForTest(t, server, "user_upstream")
	if _, err := server.store.db.Exec(`UPDATE agent_config SET billing_mode='user_upstream' WHERE agent_id=?`, server.cfg.AgentID); err != nil {
		t.Fatal(err)
	}
	server.main = NewMainClient(server.cfg)
	_, key, err := server.store.CreateAPIKey(server.cfg.AgentID, "42", "insufficient balance")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.5","messages":[]}`))
	req.Header.Set("Authorization", "Bearer "+key)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusPaymentRequired || !strings.Contains(rec.Body.String(), "MAIN_USER_BALANCE_INSUFFICIENT") {
		t.Fatalf("insufficient user balance status=%d body=%s", rec.Code, rec.Body.String())
	}
	if modelCalls.Load() != 0 {
		t.Fatalf("insufficient balance reached the model upstream %d times", modelCalls.Load())
	}
}

func TestDirectUserBillingDisablesLegacyRechargeAndAllocationEndpoints(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("disabled local funding endpoint contacted Sub2API: %s", r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	setTenantBillingModeForTest(t, server, "user_upstream")
	if _, err := server.store.db.Exec(`UPDATE agent_config SET billing_mode='user_upstream' WHERE agent_id=?`, server.cfg.AgentID); err != nil {
		t.Fatal(err)
	}
	server.cfg.PublicMainURL = "https://main.example.com"
	ownerSession, err := server.store.CreateSession("42", []byte(`{"id":"42","role":"admin"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}

	userList := httptest.NewRequest(http.MethodGet, "/api/v1/agent/recharge/orders", nil)
	userList.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: ownerSession})
	userListResponse := httptest.NewRecorder()
	server.ServeHTTP(userListResponse, userList)
	if userListResponse.Code != http.StatusOK || !strings.Contains(userListResponse.Body.String(), `"recharge_url":"https://main.example.com/purchase"`) || !strings.Contains(userListResponse.Body.String(), `"enabled":false`) {
		t.Fatalf("direct billing recharge discovery status=%d body=%s", userListResponse.Code, userListResponse.Body.String())
	}

	tests := []struct {
		method string
		path   string
		body   string
		code   string
	}{
		{http.MethodPost, "/api/v1/agent/recharge/orders", `{"amount":"10.00"}`, "LOCAL_RECHARGE_DISABLED"},
		{http.MethodPost, "/api/v1/agent/admin/wallet/sync", `{}`, "OWNER_WALLET_DISABLED"},
		{http.MethodGet, "/api/v1/agent/admin/payment-config", ``, "LOCAL_RECHARGE_DISABLED"},
	}
	for _, test := range tests {
		req := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
		req.Host = "agent.example.com"
		req.Header.Set("Origin", "https://agent.example.com")
		req.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: ownerSession})
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		if rec.Code != http.StatusGone || !strings.Contains(rec.Body.String(), test.code) {
			t.Fatalf("%s %s status=%d body=%s", test.method, test.path, rec.Code, rec.Body.String())
		}
	}

	settings := httptest.NewRecorder()
	server.ServeHTTP(settings, httptest.NewRequest(http.MethodGet, "/api/v1/settings/public", nil))
	if settings.Code != http.StatusOK ||
		!strings.Contains(settings.Body.String(), `"payment_enabled":false`) ||
		!strings.Contains(settings.Body.String(), `"recharge_url":"https://main.example.com/purchase"`) ||
		!strings.Contains(settings.Body.String(), `"available_channels_enabled":false`) ||
		!strings.Contains(settings.Body.String(), `"subscription_enabled":false`) ||
		!strings.Contains(settings.Body.String(), `"model_plaza_enabled":false`) ||
		!strings.Contains(settings.Body.String(), `"plugin_management_enabled":false`) {
		t.Fatalf("direct billing public settings status=%d body=%s", settings.Code, settings.Body.String())
	}
}

func TestModelRelayUsesRequestUsageInsteadOfSharedOwnerBalanceDelta(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/sub2api/balance":
			// The owner balance includes an unrelated external charge after the
			// request. It must not become this proxy user's usage.
			_ = json.NewEncoder(w).Encode(map[string]any{"object": "sub2api.user_balance", "balance": 9.0})
		case "/v1/sub2api/usage":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"items": []map[string]any{{
				"id": 700, "request_id": "usage-authoritative", "model": "gpt-5.5", "actual_cost": 0.25, "total_cost": 0.31,
				"input_tokens": 1000, "output_tokens": 25, "input_cost": 0.000001234, "upstream_model": "provider-gpt-5.5",
				"ip_address": "192.0.2.123", "user": map[string]any{"email": "private@example.com"},
			}}}})
		case "/v1/chat/completions":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "ok"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	server.cfg.OwnerMainUserID = "99"
	server.cfg.MainUsageAPI = true
	server.main = NewMainClient(server.cfg)
	if err := server.store.CreditAgent(server.cfg.AgentID, 1000, "seed-authoritative", "seed", "test"); err != nil {
		t.Fatal(err)
	}
	if err := server.store.Allocate(server.cfg.AgentID, "42", 500, "allocate-authoritative", "order", "test"); err != nil {
		t.Fatal(err)
	}
	_, key, err := server.store.CreateAPIKey(server.cfg.AgentID, "42", "authoritative")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.5","messages":[]}`))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Idempotency-Key", "usage-authoritative")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("relay status=%d body=%s", rec.Code, rec.Body.String())
	}
	usage, err := server.store.Usage(server.cfg.AgentID, "42", 10)
	if err != nil || len(usage) != 1 || usage[0].ActualCents != 25 || usage[0].RequestID != "usage-authoritative" || usage[0].UsageID != "700" {
		t.Fatalf("authoritative usage was not applied: %+v err=%v", usage, err)
	}
	if usage[0].Model != "gpt-5.5" || usage[0].Source != "sub2api_user_usage" || usage[0].InputTokens != 1000 || usage[0].OutputTokens != 25 || usage[0].InputCostNanos != 1234 || usage[0].TotalCostNanos != 310_000_000 || usage[0].ActualCostNanos != 250_000_000 || usage[0].UpstreamModel != "provider-gpt-5.5" {
		t.Fatalf("authoritative usage details were not stored: %+v", usage[0])
	}
	encodedUsage, err := json.Marshal(usage[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encodedUsage), "192.0.2.123") || strings.Contains(string(encodedUsage), "private@example.com") || strings.Contains(string(encodedUsage), "ip_address") {
		t.Fatalf("admin-only usage fields leaked into the AgentAPI view: %s", encodedUsage)
	}
	if !strings.Contains(string(encodedUsage), `"usage_source":"sub2api_user_usage"`) || !strings.Contains(string(encodedUsage), `"input_tokens":1000`) || strings.Contains(string(encodedUsage), `"MainUsageSnapshot"`) {
		t.Fatalf("main usage snapshot was not flattened into the user API DTO: %s", encodedUsage)
	}
}

func TestSettlementReconcileStoresMainUsageSnapshot(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/sub2api/usage" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"items": []map[string]any{{
			"id": 701, "request_id": "pending-usage", "model": "claude-sonnet", "actual_cost": 0.12, "total_cost": 0.15,
			"input_tokens": 80, "output_tokens": 20, "cache_read_tokens": 10, "cache_read_cost": 0.000000019,
		}}}})
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	server.cfg.MainUsageAPI = true
	server.main = NewMainClient(server.cfg)
	if err := server.store.CreditAgent(server.cfg.AgentID, 500, "seed-reconcile-snapshot", "seed", "test"); err != nil {
		t.Fatal(err)
	}
	if err := server.store.Allocate(server.cfg.AgentID, "42", 200, "allocate-reconcile-snapshot", "order", "test"); err != nil {
		t.Fatal(err)
	}
	if _, created, err := server.store.PrepareSettlement(server.cfg.AgentID, "42", "42", "pending-usage", "pending-usage", 100); err != nil || !created {
		t.Fatalf("prepare pending usage: created=%v err=%v", created, err)
	}
	results, err := server.reconcileTenantSettlements(t.Context(), server.cfg.AgentID, "pending-usage")
	if err != nil || len(results) != 1 || results[0].Status != "confirmed" {
		t.Fatalf("reconcile result=%+v err=%v", results, err)
	}
	usage, err := server.store.Usage(server.cfg.AgentID, "42", 10)
	if err != nil || len(usage) != 1 {
		t.Fatalf("read reconciled usage: %+v err=%v", usage, err)
	}
	item := usage[0]
	if item.Source != "sub2api_user_usage" || item.UsageID != "701" || item.ActualCents != 12 || item.Model != "claude-sonnet" || item.InputTokens != 80 || item.OutputTokens != 20 || item.CacheReadTokens != 10 || item.CacheReadCostNanos != 19 {
		t.Fatalf("reconciler lost authoritative usage fields: %+v", item)
	}
}

func TestForbiddenAgentManagementModulesAreNotExposed(t *testing.T) {
	upstreamCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		http.Error(w, "forbidden module reached upstream", http.StatusInternalServerError)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	sessionID, err := server.store.CreateSession("42", []byte(`{"id":"42","role":"admin"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/agent/users"},
		{http.MethodGet, "/api/v1/agent/admin/groups"},
		{http.MethodGet, "/api/v1/agent/admin/accounts"},
		{http.MethodPatch, "/api/v1/agent/admin/users/43/status"},
		{http.MethodPost, "/api/v1/agent/admin/users/43/allocate"},
		{http.MethodGet, "/api/v1/agent/admin/agents"},
		{http.MethodGet, "/api/v1/agent/admin/proxies"},
		{http.MethodGet, "/api/v1/agent/admin/agent-provisioning"},
		{http.MethodGet, "/api/v1/agent/admin/ops"},
		{http.MethodGet, "/api/v1/agent/admin/promo-codes"},
		{http.MethodGet, "/api/v1/agent/admin/redeem"},
		{http.MethodGet, "/api/v1/agent/admin/redeem-codes"},
		{http.MethodGet, "/api/v1/agent/admin/plugins"},
		{http.MethodGet, "/api/v1/agent/admin/audit"},
		{http.MethodGet, "/api/v1/agent/admin/audit-logs"},
		{http.MethodGet, "/api/v1/agent/admin/audit-events"},
		{http.MethodGet, "/api/v1/agent/admin/risk-control"},
		{http.MethodGet, "/api/v1/agent/admin/prompt-audit"},
		{http.MethodGet, "/api/v1/agent/admin/security-audit"},
		{http.MethodGet, "/api/v1/agent/admin/upstream-audit"},
		{http.MethodGet, "/api/v1/agent/admin/upstream-verification"},
		{http.MethodGet, "/api/v1/agent/admin/channels"},
		{http.MethodGet, "/api/v1/agent/admin/subscriptions"},
		{http.MethodGet, "/api/v1/agent/admin/payment/plans"},
		{http.MethodPut, "/api/v1/agent/admin/payment/plans/7"},
		{http.MethodGet, "/api/v1/agent/admin/satellite-billing"},
		{http.MethodGet, "/api/v1/agent/admin/content"},
		{http.MethodGet, "/api/v1/agent/admin/content-pages"},
		{http.MethodGet, "/api/v1/agent/content-pages"},
		{http.MethodGet, "/api/v1/agent/content/legal/terms"},
		{http.MethodGet, "/api/v1/agent/admin/backup"},
		{http.MethodGet, "/api/v1/agent/admin/backups"},
		{http.MethodGet, "/api/v1/agent/admin/model-policy"},
		{http.MethodPut, "/api/v1/agent/admin/model-policy"},
		{http.MethodGet, "/api/v1/agent/tasks"},
	}
	for _, test := range tests {
		for _, authenticated := range []bool{false, true} {
			req := httptest.NewRequest(test.method, test.path, nil)
			if authenticated {
				req.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
			}
			rec := httptest.NewRecorder()
			server.ServeHTTP(rec, req)
			if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "NOT_FOUND") {
				t.Fatalf("%s %s authenticated=%t status=%d body=%s", test.method, test.path, authenticated, rec.Code, rec.Body.String())
			}
		}
	}
	if upstreamCalls != 0 {
		t.Fatalf("forbidden management modules contacted upstream %d times", upstreamCalls)
	}
}
func TestAgentUsageEndpointPaginatesAndKeepsUserScope(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("usage history must come from AgentAPI's local settlement ledger, not a new upstream read: %s", r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	if _, err := server.store.UpsertUser(server.cfg.AgentID, "43", "other@example.com", "Other"); err != nil {
		t.Fatal(err)
	}
	if err := server.store.CreditAgent(server.cfg.AgentID, 500, "seed-usage-pages", "seed", "test"); err != nil {
		t.Fatal(err)
	}
	for _, userID := range []string{"42", "43"} {
		if err := server.store.Allocate(server.cfg.AgentID, userID, 100, "allocate-usage-pages-"+userID, "order-"+userID, "test"); err != nil {
			t.Fatal(err)
		}
	}
	for _, record := range []struct{ userID, requestID string }{
		{"42", "owner-first"}, {"42", "owner-second"}, {"43", "user-third"},
	} {
		if _, created, err := server.store.PrepareSettlement(server.cfg.AgentID, record.userID, "42", record.requestID, record.requestID, 10); err != nil || !created {
			t.Fatalf("prepare %s: created=%v err=%v", record.requestID, created, err)
		}
	}
	adminSession, err := server.store.CreateSession("42", []byte(`{"id":"42"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	userSession, err := server.store.CreateSession("43", []byte(`{"id":"43"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}

	adminRequest := httptest.NewRequest(http.MethodGet, "/api/v1/agent/usage?page=2&page_size=1", nil)
	adminRequest.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: adminSession})
	adminResponse := httptest.NewRecorder()
	server.ServeHTTP(adminResponse, adminRequest)
	if adminResponse.Code != http.StatusOK {
		t.Fatalf("admin usage status=%d body=%s", adminResponse.Code, adminResponse.Body.String())
	}
	var adminPayload struct {
		Data struct {
			Items    []AgentUsageView `json:"items"`
			Total    int              `json:"total"`
			Page     int              `json:"page"`
			PageSize int              `json:"page_size"`
		} `json:"data"`
	}
	if err := json.Unmarshal(adminResponse.Body.Bytes(), &adminPayload); err != nil {
		t.Fatal(err)
	}
	if len(adminPayload.Data.Items) != 1 || adminPayload.Data.Items[0].RequestID != "owner-second" || adminPayload.Data.Total != 3 || adminPayload.Data.Page != 2 || adminPayload.Data.PageSize != 1 {
		t.Fatalf("unexpected paginated admin usage: %+v", adminPayload.Data)
	}

	userRequest := httptest.NewRequest(http.MethodGet, "/api/v1/agent/usage?page=1&page_size=25", nil)
	userRequest.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: userSession})
	userResponse := httptest.NewRecorder()
	server.ServeHTTP(userResponse, userRequest)
	var userPayload struct {
		Data struct {
			Items []AgentUsageView `json:"items"`
			Total int              `json:"total"`
		} `json:"data"`
	}
	if userResponse.Code != http.StatusOK || json.Unmarshal(userResponse.Body.Bytes(), &userPayload) != nil || userPayload.Data.Total != 1 || len(userPayload.Data.Items) != 1 || userPayload.Data.Items[0].RequestID != "user-third" {
		t.Fatalf("user usage page escaped user scope: status=%d body=%s data=%+v", userResponse.Code, userResponse.Body.String(), userPayload.Data)
	}
	for _, check := range []struct {
		query  string
		status int
		total  int
	}{
		{"?request_id=owner-first", http.StatusOK, 0},
		{"?request_id=user-third", http.StatusOK, 1},
		{"?model=not-present", http.StatusOK, 0},
		{"?start_time=invalid", http.StatusBadRequest, 0},
		{"?start_time=2026-09-29T00:00:00Z&end_time=2026-09-28T00:00:00Z", http.StatusBadRequest, 0},
	} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/agent/usage"+check.query, nil)
		req.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: userSession})
		res := httptest.NewRecorder()
		server.ServeHTTP(res, req)
		if res.Code != check.status {
			t.Fatalf("filter %s status=%d body=%s", check.query, res.Code, res.Body.String())
		}
		if check.status == http.StatusOK {
			if err := json.Unmarshal(res.Body.Bytes(), &userPayload); err != nil || userPayload.Data.Total != check.total {
				t.Fatalf("filtered total: %s", res.Body.String())
			}
		} else if !strings.Contains(res.Body.String(), "INVALID_FILTER") {
			t.Fatalf("unexpected error: %s", res.Body.String())
		}
	}

	badRequest := httptest.NewRequest(http.MethodGet, "/api/v1/agent/usage?page_size=201", nil)
	badRequest.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: adminSession})
	badResponse := httptest.NewRecorder()
	server.ServeHTTP(badResponse, badRequest)
	if badResponse.Code != http.StatusBadRequest || !strings.Contains(badResponse.Body.String(), "INVALID_PAGINATION") {
		t.Fatalf("invalid page size status=%d body=%s", badResponse.Code, badResponse.Body.String())
	}
}

func TestStreamingModelRelayForwardsChunksAndSettlesAfterEOF(t *testing.T) {
	adminReads := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/sub2api/balance":
			adminReads++
			balance := 10.0
			if adminReads > 1 {
				balance = 9.0
			}
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 42, "balance": balance}))
		case "/v1/chat/completions":
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, _ := w.(http.Flusher)
			_, _ = io.WriteString(w, "data: one\n\n")
			if flusher != nil {
				flusher.Flush()
			}
			_, _ = io.WriteString(w, "data: [DONE]\n\n")
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	if err := server.store.CreditAgent(server.cfg.AgentID, 1000, "seed-stream", "seed", "test"); err != nil {
		t.Fatal(err)
	}
	if err := server.store.Allocate(server.cfg.AgentID, "42", 500, "allocate-stream", "order", "test"); err != nil {
		t.Fatal(err)
	}
	_, key, err := server.store.CreateAPIKey(server.cfg.AgentID, "42", "stream")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.5","stream":true,"messages":[]}`))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "text/event-stream")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, req)
	if response.Code != http.StatusOK || response.Body.String() != "data: one\n\ndata: [DONE]\n\n" {
		t.Fatalf("stream relay status=%d body=%q", response.Code, response.Body.String())
	}
	usage, err := server.store.Usage(server.cfg.AgentID, "42", 10)
	if err != nil || len(usage) != 1 || usage[0].SettlementStatus != "confirmed" || usage[0].ActualCents != 100 {
		t.Fatalf("stream settlement=%+v err=%v", usage, err)
	}
}

func TestVideoTaskPollIsUserScopedAndSettlesOriginalRequest(t *testing.T) {
	var usageVisible atomic.Bool
	videoPolls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/sub2api/balance":
			_ = json.NewEncoder(w).Encode(map[string]any{"object": "sub2api.user_balance", "balance": 10.0})
		case "/v1/sub2api/usage":
			items := []map[string]any{}
			if usageVisible.Load() {
				items = append(items, map[string]any{"id": 702, "request_id": "video-request-1", "actual_cost": 1.0, "total_cost": 1.0})
			}
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"items": items}))
		case "/v1/videos":
			_, _ = io.WriteString(w, `{"request_id":"video-task-1","status":"queued"}`)
		case "/v1/videos/video-task-1":
			videoPolls++
			usageVisible.Store(true)
			_, _ = io.WriteString(w, `{"id":"video-task-1","status":"completed"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	server.cfg.MainUsageAPI = true
	server.main = NewMainClient(server.cfg)
	if err := server.store.CreditAgent(server.cfg.AgentID, 1000, "seed-video", "seed", "test"); err != nil {
		t.Fatal(err)
	}
	if err := server.store.Allocate(server.cfg.AgentID, "42", 500, "allocate-video", "order", "test"); err != nil {
		t.Fatal(err)
	}
	_, key, err := server.store.CreateAPIKey(server.cfg.AgentID, "42", "video")
	if err != nil {
		t.Fatal(err)
	}
	create := httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{"model":"grok-imagine-video-1.5"}`))
	create.Header.Set("Authorization", "Bearer "+key)
	create.Header.Set("Idempotency-Key", "video-request-1")
	created := httptest.NewRecorder()
	server.ServeHTTP(created, create)
	if created.Code != http.StatusOK {
		t.Fatalf("video create status=%d body=%s", created.Code, created.Body.String())
	}
	if _, err := server.store.UpsertUser(server.cfg.AgentID, "43", "other@example.com", "Other"); err != nil {
		t.Fatal(err)
	}
	_, otherKey, err := server.store.CreateAPIKey(server.cfg.AgentID, "43", "other")
	if err != nil {
		t.Fatal(err)
	}
	deniedRequest := httptest.NewRequest(http.MethodGet, "/v1/videos/video-task-1", nil)
	deniedRequest.Header.Set("Authorization", "Bearer "+otherKey)
	denied := httptest.NewRecorder()
	server.ServeHTTP(denied, deniedRequest)
	if denied.Code != http.StatusNotFound || videoPolls != 0 {
		t.Fatalf("foreign video poll was not isolated: status=%d polls=%d body=%s", denied.Code, videoPolls, denied.Body.String())
	}
	poll := httptest.NewRequest(http.MethodGet, "/v1/videos/video-task-1", nil)
	poll.Header.Set("Authorization", "Bearer "+key)
	polled := httptest.NewRecorder()
	server.ServeHTTP(polled, poll)
	if polled.Code != http.StatusOK || videoPolls != 1 {
		t.Fatalf("video poll status=%d polls=%d body=%s", polled.Code, videoPolls, polled.Body.String())
	}
	usage, err := server.store.Usage(server.cfg.AgentID, "42", 10)
	if err != nil || len(usage) != 1 || usage[0].RequestID != "video-request-1" || usage[0].SettlementStatus != "confirmed" || usage[0].ActualCents != 100 {
		t.Fatalf("unexpected video settlement: %+v err=%v", usage, err)
	}
	task, err := server.store.VideoTask(server.cfg.AgentID, "video-task-1")
	if err != nil || task.Status != "completed" || task.RequestID != "video-request-1" {
		t.Fatalf("unexpected video task mapping: %+v err=%v", task, err)
	}
}

func TestAsyncImageTaskPollIsUserScopedAndSettlesOriginalRequest(t *testing.T) {
	var usageVisible atomic.Bool
	var imageCreates atomic.Int32
	imagePolls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/sub2api/balance":
			_ = json.NewEncoder(w).Encode(map[string]any{"object": "sub2api.user_balance", "balance": 10.0})
		case "/v1/sub2api/usage":
			items := []map[string]any{}
			if usageVisible.Load() {
				items = append(items, map[string]any{"id": 701, "request_id": "image-request-1", "actual_cost": 0.25, "total_cost": 0.25})
			}
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"items": items}))
		case "/v1/images/generations/async":
			imageCreates.Add(1)
			if r.Header.Get("X-Request-ID") != "image-request-1" || r.Header.Get("X-Sub2API-On-Behalf-Of") != "42" || r.Header.Get("X-Sub2API-Satellite") != "agentapi" {
				t.Errorf("async image request identity headers = %v", r.Header)
			}
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `{"id":"imgtask_001","task_id":"imgtask_001","status":"processing"}`)
		case "/v1/images/tasks/imgtask_001":
			imagePolls++
			usageVisible.Store(true)
			_, _ = io.WriteString(w, `{"id":"imgtask_001","task_id":"imgtask_001","status":"completed","result":{"data":[{"url":"https://example.test/image.png"}]}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	server.cfg.OwnerMainUserID = "42"
	server.cfg.MainUsageAPI = true
	server.main = NewMainClient(server.cfg)
	if err := server.store.CreditAgent(server.cfg.AgentID, 1000, "seed-async-image", "seed", "test"); err != nil {
		t.Fatal(err)
	}
	if err := server.store.Allocate(server.cfg.AgentID, "42", 500, "allocate-async-image", "order", "test"); err != nil {
		t.Fatal(err)
	}
	_, ownerKey, err := server.store.CreateAPIKey(server.cfg.AgentID, "42", "async image")
	if err != nil {
		t.Fatal(err)
	}
	create := httptest.NewRequest(http.MethodPost, "/v1/images/generations/async", strings.NewReader(`{"model":"gpt-image-2","prompt":"a quiet lake"}`))
	create.Header.Set("Authorization", "Bearer "+ownerKey)
	create.Header.Set("Idempotency-Key", "image-request-1")
	created := httptest.NewRecorder()
	server.ServeHTTP(created, create)
	if created.Code != http.StatusAccepted {
		t.Fatalf("async image create status=%d body=%s", created.Code, created.Body.String())
	}
	settlement, err := server.store.Settlement(server.cfg.AgentID, "image-request-1")
	if err != nil || settlement.Status != "pending" {
		t.Fatalf("async image reservation must remain pending until task completion: settlement=%+v err=%v", settlement, err)
	}
	replay := httptest.NewRequest(http.MethodPost, "/v1/images/generations/async", strings.NewReader(`{"model":"gpt-image-2","prompt":"a quiet lake"}`))
	replay.Header.Set("Authorization", "Bearer "+ownerKey)
	replay.Header.Set("Idempotency-Key", "image-request-1")
	replayResponse := httptest.NewRecorder()
	server.ServeHTTP(replayResponse, replay)
	if replayResponse.Code != http.StatusConflict || imageCreates.Load() != 1 {
		t.Fatalf("duplicate image request was forwarded: status=%d creates=%d body=%s", replayResponse.Code, imageCreates.Load(), replayResponse.Body.String())
	}

	if _, err := server.store.UpsertUser(server.cfg.AgentID, "43", "other@example.com", "Other"); err != nil {
		t.Fatal(err)
	}
	_, otherKey, err := server.store.CreateAPIKey(server.cfg.AgentID, "43", "other")
	if err != nil {
		t.Fatal(err)
	}
	deniedRequest := httptest.NewRequest(http.MethodGet, "/v1/images/tasks/imgtask_001", nil)
	deniedRequest.Header.Set("Authorization", "Bearer "+otherKey)
	denied := httptest.NewRecorder()
	server.ServeHTTP(denied, deniedRequest)
	if denied.Code != http.StatusNotFound || imagePolls != 0 {
		t.Fatalf("foreign image task poll was not isolated: status=%d polls=%d body=%s", denied.Code, imagePolls, denied.Body.String())
	}

	poll := httptest.NewRequest(http.MethodGet, "/v1/images/tasks/imgtask_001", nil)
	poll.Header.Set("Authorization", "Bearer "+ownerKey)
	polled := httptest.NewRecorder()
	server.ServeHTTP(polled, poll)
	if polled.Code != http.StatusOK || imagePolls != 1 || !strings.Contains(polled.Body.String(), "image.png") {
		t.Fatalf("image task poll status=%d polls=%d body=%s", polled.Code, imagePolls, polled.Body.String())
	}
	usage, err := server.store.Usage(server.cfg.AgentID, "42", 10)
	if err != nil || len(usage) != 1 || usage[0].RequestID != "image-request-1" || usage[0].UsageID != "701" || usage[0].SettlementStatus != "confirmed" || usage[0].ActualCents != 25 {
		t.Fatalf("async image did not settle the original reservation from main usage: %+v err=%v", usage, err)
	}
	if task, err := server.store.ImageTask(server.cfg.AgentID, "imgtask_001"); err != nil || task.MainUserID != "42" || task.Status != "completed" || task.RequestID != "image-request-1" {
		t.Fatalf("unexpected image task mapping: %+v err=%v", task, err)
	}
}

func TestAsyncImageTrackingFailuresRemainPendingAndDoNotRetryCreate(t *testing.T) {
	tests := []struct {
		name          string
		taskID        string
		body          string
		seedCollision bool
		wantStatus    int
		wantReason    string
	}{
		{
			name:       "missing task id",
			body:       `{"status":"processing"}`,
			wantStatus: http.StatusBadGateway,
			wantReason: "IMAGE_TASK_ID_MISSING",
		},
		{
			name:          "task mapping conflict",
			taskID:        "imgtask_collision",
			body:          `{"id":"imgtask_collision","task_id":"imgtask_collision","status":"processing"}`,
			seedCollision: true,
			wantStatus:    http.StatusServiceUnavailable,
			wantReason:    "IMAGE_TASK_TRACKING_FAILED",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var creates atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/sub2api/balance":
					_ = json.NewEncoder(w).Encode(map[string]any{"object": "sub2api.user_balance", "balance": 10.0})
				case "/v1/images/generations/async":
					creates.Add(1)
					w.WriteHeader(http.StatusAccepted)
					_, _ = io.WriteString(w, test.body)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer upstream.Close()
			server := testServer(t, upstream)
			server.cfg.OwnerMainUserID = "99"
			server.main = NewMainClient(server.cfg)
			if test.seedCollision {
				if _, err := server.store.UpsertUser(server.cfg.AgentID, "43", "other@example.com", "Other"); err != nil {
					t.Fatal(err)
				}
				if _, err := server.store.RecordImageTask(server.cfg.AgentID, "43", test.taskID, "other-image-request", "processing"); err != nil {
					t.Fatal(err)
				}
			}
			if err := server.store.CreditAgent(server.cfg.AgentID, 1000, "seed-async-image-tracking", "seed", "test"); err != nil {
				t.Fatal(err)
			}
			if err := server.store.Allocate(server.cfg.AgentID, "42", 500, "allocate-async-image-tracking", "order", "test"); err != nil {
				t.Fatal(err)
			}
			_, key, err := server.store.CreateAPIKey(server.cfg.AgentID, "42", "image tracking")
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, "/v1/images/generations/async", strings.NewReader(`{"model":"gpt-image-2","prompt":"track me"}`))
			request.Header.Set("Authorization", "Bearer "+key)
			request.Header.Set("Idempotency-Key", "image-tracking-request")
			response := httptest.NewRecorder()
			server.ServeHTTP(response, request)
			if response.Code != test.wantStatus || !strings.Contains(response.Body.String(), test.wantReason) {
				t.Fatalf("tracking failure status=%d body=%s, want status=%d reason=%s", response.Code, response.Body.String(), test.wantStatus, test.wantReason)
			}
			settlement, err := server.store.Settlement(server.cfg.AgentID, "image-tracking-request")
			if err != nil || settlement.Status != "pending" {
				t.Fatalf("untracked accepted task must remain pending: settlement=%+v err=%v", settlement, err)
			}

			replay := httptest.NewRequest(http.MethodPost, "/v1/images/generations/async", strings.NewReader(`{"model":"gpt-image-2","prompt":"track me"}`))
			replay.Header.Set("Authorization", "Bearer "+key)
			replay.Header.Set("Idempotency-Key", "image-tracking-request")
			replayResponse := httptest.NewRecorder()
			server.ServeHTTP(replayResponse, replay)
			if replayResponse.Code != http.StatusConflict || creates.Load() != 1 {
				t.Fatalf("retry must not repeat async image creation: status=%d creates=%d body=%s", replayResponse.Code, creates.Load(), replayResponse.Body.String())
			}
		})
	}
}

func TestFailedAsyncImagePollWaitsForAuthoritativeUsage(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/sub2api/balance":
			_ = json.NewEncoder(w).Encode(map[string]any{"object": "sub2api.user_balance", "balance": 10.0})
		case "/v1/sub2api/usage":
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"items": []map[string]any{}}))
		case "/v1/images/edits/async":
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `{"id":"imgtask_failed","task_id":"imgtask_failed","status":"processing"}`)
		case "/v1/images/tasks/imgtask_failed":
			_, _ = io.WriteString(w, `{"id":"imgtask_failed","task_id":"imgtask_failed","status":"failed"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	server.cfg.OwnerMainUserID = "42"
	server.cfg.MainUsageAPI = true
	server.main = NewMainClient(server.cfg)
	if err := server.store.CreditAgent(server.cfg.AgentID, 1000, "seed-failed-image", "seed", "test"); err != nil {
		t.Fatal(err)
	}
	if err := server.store.Allocate(server.cfg.AgentID, "42", 500, "allocate-failed-image", "order", "test"); err != nil {
		t.Fatal(err)
	}
	_, key, err := server.store.CreateAPIKey(server.cfg.AgentID, "42", "failed image")
	if err != nil {
		t.Fatal(err)
	}
	create := httptest.NewRequest(http.MethodPost, "/v1/images/edits/async", strings.NewReader(`{"model":"gpt-image-2","prompt":"edit"}`))
	create.Header.Set("Authorization", "Bearer "+key)
	create.Header.Set("Idempotency-Key", "failed-image-request")
	created := httptest.NewRecorder()
	server.ServeHTTP(created, create)
	if created.Code != http.StatusAccepted {
		t.Fatalf("async image create status=%d body=%s", created.Code, created.Body.String())
	}
	poll := httptest.NewRequest(http.MethodGet, "/v1/images/tasks/imgtask_failed", nil)
	poll.Header.Set("Authorization", "Bearer "+key)
	polled := httptest.NewRecorder()
	server.ServeHTTP(polled, poll)
	if polled.Code != http.StatusOK {
		t.Fatalf("failed image poll status=%d body=%s", polled.Code, polled.Body.String())
	}
	usage, err := server.store.Usage(server.cfg.AgentID, "42", 10)
	if err != nil || len(usage) != 1 || usage[0].SettlementStatus != "pending" || usage[0].ActualCents != 0 {
		t.Fatalf("missing main usage must remain unconfirmed: %+v err=%v", usage, err)
	}
}

func TestFailedAsyncPollKeepsReservationWhenUsageAPIIsUnavailable(t *testing.T) {
	tests := []struct {
		name       string
		createPath string
		taskPath   string
		createBody string
		pollBody   string
	}{
		{
			name:       "video",
			createPath: "/v1/videos",
			taskPath:   "/v1/videos/video-legacy-failed",
			createBody: `{"id":"video-legacy-failed","status":"processing"}`,
			pollBody:   `{"id":"video-legacy-failed","status":"failed"}`,
		},
		{
			name:       "image",
			createPath: "/v1/images/generations/async",
			taskPath:   "/v1/images/tasks/image-legacy-failed",
			createBody: `{"id":"image-legacy-failed","task_id":"image-legacy-failed","status":"processing"}`,
			pollBody:   `{"id":"image-legacy-failed","task_id":"image-legacy-failed","status":"failed"}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/sub2api/balance":
					_ = json.NewEncoder(w).Encode(map[string]any{"object": "sub2api.user_balance", "balance": 10.0})
				case test.createPath:
					w.WriteHeader(http.StatusAccepted)
					_, _ = io.WriteString(w, test.createBody)
				case test.taskPath:
					_, _ = io.WriteString(w, test.pollBody)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer upstream.Close()
			server := testServer(t, upstream)
			server.cfg.OwnerMainUserID = "99"
			server.cfg.MainUsageAPI = false
			server.main = NewMainClient(server.cfg)
			if err := server.store.CreditAgent(server.cfg.AgentID, 1000, "seed-legacy-async-failure", "seed", "test"); err != nil {
				t.Fatal(err)
			}
			if err := server.store.Allocate(server.cfg.AgentID, "42", 500, "allocate-legacy-async-failure", "order", "test"); err != nil {
				t.Fatal(err)
			}
			_, key, err := server.store.CreateAPIKey(server.cfg.AgentID, "42", "legacy async failure")
			if err != nil {
				t.Fatal(err)
			}
			requestID := "legacy-" + test.name + "-failed-request"
			create := httptest.NewRequest(http.MethodPost, test.createPath, strings.NewReader(`{"model":"gpt-image-2","prompt":"test"}`))
			create.Header.Set("Authorization", "Bearer "+key)
			create.Header.Set("Content-Type", "application/json")
			create.Header.Set("Idempotency-Key", requestID)
			created := httptest.NewRecorder()
			server.ServeHTTP(created, create)
			if created.Code != http.StatusAccepted {
				t.Fatalf("async create status=%d body=%s", created.Code, created.Body.String())
			}
			poll := httptest.NewRequest(http.MethodGet, test.taskPath, nil)
			poll.Header.Set("Authorization", "Bearer "+key)
			polled := httptest.NewRecorder()
			server.ServeHTTP(polled, poll)
			if polled.Code != http.StatusOK {
				t.Fatalf("failed task poll status=%d body=%s", polled.Code, polled.Body.String())
			}
			settlement, err := server.store.Settlement(server.cfg.AgentID, requestID)
			if err != nil || settlement.Status != "pending" {
				t.Fatalf("unavailable authoritative usage must not release a failed async task: settlement=%+v err=%v", settlement, err)
			}
		})
	}
}

func TestStaleVideoTaskReconcilerWaitsForFailedTaskUsage(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/videos/video-stale":
			_, _ = io.WriteString(w, `{"id":"video-stale","status":"failed"}`)
		case "/v1/sub2api/usage":
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"items": []map[string]any{}}))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	server.cfg.OwnerMainUserID = "99"
	server.cfg.VideoTaskReconcileAge = time.Minute
	server.cfg.MainUsageAPI = true
	server.main = NewMainClient(server.cfg)
	if err := server.store.CreditAgent(server.cfg.AgentID, 1000, "seed-stale-video", "seed", "test"); err != nil {
		t.Fatal(err)
	}
	if err := server.store.Allocate(server.cfg.AgentID, "42", 500, "allocate-stale-video", "order", "test"); err != nil {
		t.Fatal(err)
	}
	old := time.Now().UTC().Add(-2 * time.Hour)
	server.store.clock = func() time.Time { return old }
	if _, _, err := server.store.PrepareSettlement(server.cfg.AgentID, "42", "99", "stale-video-request", "stale-video-request", 100); err != nil {
		t.Fatal(err)
	}
	if _, err := server.store.RecordVideoTask(server.cfg.AgentID, "42", "video-stale", "stale-video-request", "processing"); err != nil {
		t.Fatal(err)
	}
	server.store.clock = time.Now
	server.reconcileStaleVideoTasksForTenant(context.Background(), server.cfg.AgentID)
	usage, err := server.store.Usage(server.cfg.AgentID, "42", 10)
	if err != nil || len(usage) != 1 || usage[0].SettlementStatus != "pending" {
		t.Fatalf("stale failed video must await main usage: %+v err=%v", usage, err)
	}
	task, err := server.store.VideoTask(server.cfg.AgentID, "video-stale")
	if err != nil || task.Status != "failed" {
		t.Fatalf("stale task status was not updated: %+v err=%v", task, err)
	}
}

func TestStaleFailedVideoTaskSettlesKnownMainUsageInsteadOfRefunding(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/videos/video-partial":
			_, _ = io.WriteString(w, `{"id":"video-partial","status":"failed"}`)
		case "/v1/sub2api/usage":
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"items": []map[string]any{{
				"id": 704, "request_id": "stale-video-partial-request", "actual_cost": 0.25, "total_cost": 0.25,
			}}}))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	server.cfg.OwnerMainUserID = "99"
	server.cfg.VideoTaskReconcileAge = time.Minute
	server.cfg.MainUsageAPI = true
	server.main = NewMainClient(server.cfg)
	if err := server.store.CreditAgent(server.cfg.AgentID, 1000, "seed-stale-video-partial", "seed", "test"); err != nil {
		t.Fatal(err)
	}
	if err := server.store.Allocate(server.cfg.AgentID, "42", 500, "allocate-stale-video-partial", "order", "test"); err != nil {
		t.Fatal(err)
	}
	old := time.Now().UTC().Add(-2 * time.Hour)
	server.store.clock = func() time.Time { return old }
	if _, _, err := server.store.PrepareSettlement(server.cfg.AgentID, "42", "99", "stale-video-partial-request", "stale-video-partial-request", 100); err != nil {
		t.Fatal(err)
	}
	if _, err := server.store.RecordVideoTask(server.cfg.AgentID, "42", "video-partial", "stale-video-partial-request", "processing"); err != nil {
		t.Fatal(err)
	}
	server.store.clock = time.Now
	server.reconcileStaleVideoTasksForTenant(context.Background(), server.cfg.AgentID)
	usage, err := server.store.Usage(server.cfg.AgentID, "42", 10)
	if err != nil || len(usage) != 1 || usage[0].SettlementStatus != "confirmed" || usage[0].UsageID != "704" || usage[0].ActualCents != 25 {
		t.Fatalf("known main-site usage was not settled for failed video task: %+v err=%v", usage, err)
	}
}

func TestStaleFailedVideoTaskStaysPendingWhenUsageLookupFails(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/videos/video-uncertain":
			_, _ = io.WriteString(w, `{"id":"video-uncertain","status":"failed"}`)
		case "/v1/sub2api/usage":
			http.Error(w, `{"code":"TEMPORARY_FAILURE"}`, http.StatusServiceUnavailable)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	server.cfg.OwnerMainUserID = "99"
	server.cfg.VideoTaskReconcileAge = time.Minute
	server.cfg.MainUsageAPI = true
	server.main = NewMainClient(server.cfg)
	if err := server.store.CreditAgent(server.cfg.AgentID, 1000, "seed-stale-video-uncertain", "seed", "test"); err != nil {
		t.Fatal(err)
	}
	if err := server.store.Allocate(server.cfg.AgentID, "42", 500, "allocate-stale-video-uncertain", "order", "test"); err != nil {
		t.Fatal(err)
	}
	old := time.Now().UTC().Add(-2 * time.Hour)
	server.store.clock = func() time.Time { return old }
	if _, _, err := server.store.PrepareSettlement(server.cfg.AgentID, "42", "99", "stale-video-uncertain-request", "stale-video-uncertain-request", 100); err != nil {
		t.Fatal(err)
	}
	if _, err := server.store.RecordVideoTask(server.cfg.AgentID, "42", "video-uncertain", "stale-video-uncertain-request", "processing"); err != nil {
		t.Fatal(err)
	}
	server.store.clock = time.Now
	server.reconcileStaleVideoTasksForTenant(context.Background(), server.cfg.AgentID)
	usage, err := server.store.Usage(server.cfg.AgentID, "42", 10)
	if err != nil || len(usage) != 1 || usage[0].SettlementStatus != "pending" {
		t.Fatalf("failed usage lookup must keep failed video reservation pending: %+v err=%v", usage, err)
	}
}

func TestStaleImageTaskReconcilerWaitsForFailedTaskUsage(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/sub2api/usage" {
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"items": []map[string]any{}}))
			return
		}
		if r.URL.Path == "/v1/images/tasks/image-stale" {
			_, _ = io.WriteString(w, `{"id":"image-stale","task_id":"image-stale","status":"failed"}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	server.cfg.OwnerMainUserID = "99"
	server.cfg.ImageTaskReconcileAge = time.Minute
	server.cfg.MainUsageAPI = true
	server.main = NewMainClient(server.cfg)
	if err := server.store.CreditAgent(server.cfg.AgentID, 1000, "seed-stale-image", "seed", "test"); err != nil {
		t.Fatal(err)
	}
	if err := server.store.Allocate(server.cfg.AgentID, "42", 500, "allocate-stale-image", "order", "test"); err != nil {
		t.Fatal(err)
	}
	old := time.Now().UTC().Add(-2 * time.Hour)
	server.store.clock = func() time.Time { return old }
	if _, _, err := server.store.PrepareSettlement(server.cfg.AgentID, "42", "99", "stale-image-request", "stale-image-request", 100); err != nil {
		t.Fatal(err)
	}
	if _, err := server.store.RecordImageTask(server.cfg.AgentID, "42", "image-stale", "stale-image-request", "processing"); err != nil {
		t.Fatal(err)
	}
	server.store.clock = time.Now
	server.reconcileStaleImageTasksForTenant(context.Background(), server.cfg.AgentID)
	usage, err := server.store.Usage(server.cfg.AgentID, "42", 10)
	if err != nil || len(usage) != 1 || usage[0].SettlementStatus != "pending" {
		t.Fatalf("stale failed image must await main usage: %+v err=%v", usage, err)
	}
	task, err := server.store.ImageTask(server.cfg.AgentID, "image-stale")
	if err != nil || task.Status != "failed" {
		t.Fatalf("stale image task status was not updated: %+v err=%v", task, err)
	}
}

func TestStaleFailedImageTaskSettlesKnownMainUsageInsteadOfRefunding(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/images/tasks/image-partial":
			_, _ = io.WriteString(w, `{"id":"image-partial","task_id":"image-partial","status":"failed"}`)
		case "/v1/sub2api/usage":
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"items": []map[string]any{{
				"id": 703, "request_id": "stale-image-partial-request", "actual_cost": 0.25, "total_cost": 0.25,
			}}}))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	server.cfg.OwnerMainUserID = "99"
	server.cfg.ImageTaskReconcileAge = time.Minute
	server.cfg.MainUsageAPI = true
	server.main = NewMainClient(server.cfg)
	if err := server.store.CreditAgent(server.cfg.AgentID, 1000, "seed-stale-image-partial", "seed", "test"); err != nil {
		t.Fatal(err)
	}
	if err := server.store.Allocate(server.cfg.AgentID, "42", 500, "allocate-stale-image-partial", "order", "test"); err != nil {
		t.Fatal(err)
	}
	old := time.Now().UTC().Add(-2 * time.Hour)
	server.store.clock = func() time.Time { return old }
	if _, _, err := server.store.PrepareSettlement(server.cfg.AgentID, "42", "99", "stale-image-partial-request", "stale-image-partial-request", 100); err != nil {
		t.Fatal(err)
	}
	if _, err := server.store.RecordImageTask(server.cfg.AgentID, "42", "image-partial", "stale-image-partial-request", "processing"); err != nil {
		t.Fatal(err)
	}
	server.store.clock = time.Now
	server.reconcileStaleImageTasksForTenant(context.Background(), server.cfg.AgentID)
	usage, err := server.store.Usage(server.cfg.AgentID, "42", 10)
	if err != nil || len(usage) != 1 || usage[0].SettlementStatus != "confirmed" || usage[0].UsageID != "703" || usage[0].ActualCents != 25 {
		t.Fatalf("known main-site usage was not settled for failed image task: %+v err=%v", usage, err)
	}
}

func TestUncertainRelayStaysPendingInsteadOfRefunding(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/sub2api/balance" {
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 42, "balance": 10.0}))
			return
		}
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	if err := server.store.CreditAgent(server.cfg.AgentID, 1000, "seed-pending", "seed", "test"); err != nil {
		t.Fatal(err)
	}
	if err := server.store.Allocate(server.cfg.AgentID, "42", 500, "allocate-pending", "order", "test"); err != nil {
		t.Fatal(err)
	}
	_, key, err := server.store.CreateAPIKey(server.cfg.AgentID, "42", "pending")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.5","messages":[]}`))
	req.Header.Set("Authorization", "Bearer "+key)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, req)
	if response.Code != http.StatusBadGateway {
		t.Fatalf("relay status=%d body=%s", response.Code, response.Body.String())
	}
	user, err := server.store.User(server.cfg.AgentID, "42")
	if err != nil {
		t.Fatal(err)
	}
	if user.BalanceCents != 500 {
		t.Fatalf("uncertain direct charge changed the legacy local wallet: %+v", user)
	}
	usage, err := server.store.Usage(server.cfg.AgentID, "42", 10)
	if err != nil || len(usage) != 1 || usage[0].SettlementStatus != "pending" {
		t.Fatalf("unexpected pending settlement: %+v err=%v", usage, err)
	}
}

func TestDuplicateModelRequestIsNotForwardedOrChargedTwice(t *testing.T) {
	modelCalls := 0
	adminReads := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/sub2api/balance":
			adminReads++
			balance := 10.0
			if adminReads > 1 {
				balance = 9.0
			}
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 42, "balance": balance}))
		case "/v1/chat/completions":
			modelCalls++
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":"idempotent"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	if err := server.store.CreditAgent(server.cfg.AgentID, 1000, "seed-idempotent", "seed", "test"); err != nil {
		t.Fatal(err)
	}
	if err := server.store.Allocate(server.cfg.AgentID, "42", 500, "allocate-idempotent", "order", "test"); err != nil {
		t.Fatal(err)
	}
	_, key, err := server.store.CreateAPIKey(server.cfg.AgentID, "42", "idempotent")
	if err != nil {
		t.Fatal(err)
	}
	newRequest := func() *http.Request {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.5","messages":[]}`))
		req.Header.Set("Authorization", "Bearer "+key)
		req.Header.Set("Idempotency-Key", "same-model-request")
		return req
	}
	first := httptest.NewRecorder()
	server.ServeHTTP(first, newRequest())
	if first.Code != http.StatusOK {
		t.Fatalf("first relay status=%d body=%s", first.Code, first.Body.String())
	}
	second := httptest.NewRecorder()
	server.ServeHTTP(second, newRequest())
	if second.Code != http.StatusConflict || !strings.Contains(second.Body.String(), "IDEMPOTENCY_REPLAY") {
		t.Fatalf("duplicate relay status=%d body=%s", second.Code, second.Body.String())
	}
	if modelCalls != 1 {
		t.Fatalf("duplicate request reached upstream %d times", modelCalls)
	}
	user, err := server.store.User(server.cfg.AgentID, "42")
	if err != nil {
		t.Fatal(err)
	}
	if user.BalanceCents != 500 {
		t.Fatalf("duplicate direct request changed the legacy local wallet: %+v", user)
	}
}

func TestAgentAPIRejectsForeignCredentialedOriginAndMainKeyProxy(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected upstream request: %s", r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)

	options := httptest.NewRequest(http.MethodOptions, "/api/v1/api-keys", nil)
	options.Host = "agent.example.com"
	options.Header.Set("Origin", "https://evil.example.com")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, options)
	if response.Code != http.StatusForbidden {
		t.Fatalf("foreign preflight status=%d body=%s", response.Code, response.Body.String())
	}

	mainKeys := httptest.NewRequest(http.MethodGet, "/api/v1/keys", nil)
	mainKeys.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: "not-a-real-session"})
	mainResponse := httptest.NewRecorder()
	server.ServeHTTP(mainResponse, mainKeys)
	if mainResponse.Code != http.StatusForbidden {
		t.Fatalf("main key proxy status=%d body=%s", mainResponse.Code, mainResponse.Body.String())
	}
	ownerSession, err := server.store.CreateSession("42", []byte(`{"id":"42"}`), "private-main-access", "private-main-refresh", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodDelete} {
		for _, route := range []string{"/api/v1/admin/users", "/api/v1/keys", "/api/v1/subscriptions", "/api/v1/payment/orders"} {
			probe := httptest.NewRecorder()
			req := httptest.NewRequest(method, route, strings.NewReader(`{}`))
			req.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: ownerSession})
			server.ServeHTTP(probe, req)
			if probe.Code != http.StatusForbidden || !strings.Contains(probe.Body.String(), "AGENT_ROUTE_FORBIDDEN") {
				t.Fatalf("authenticated owner main route %s %s: %d %s", method, route, probe.Code, probe.Body.String())
			}
		}
	}
	for _, route := range []string{"/api/v1/users", "/api/v1/balance", "/api/v1/payment/orders"} {
		probe := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, route, nil)
		req.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: "not-a-real-session"})
		server.ServeHTTP(probe, req)
		if probe.Code != http.StatusForbidden || !strings.Contains(probe.Body.String(), "AGENT_ROUTE_FORBIDDEN") {
			t.Fatalf("main route %s was not rejected: status=%d body=%s", route, probe.Code, probe.Body.String())
		}
	}
	for _, route := range []string{"/v1/admin/users", "/v1/private/completions"} {
		probe := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, route, nil)
		req.Header.Set("Authorization", "Bearer sk-not-used")
		server.ServeHTTP(probe, req)
		if probe.Code != http.StatusForbidden || !strings.Contains(probe.Body.String(), "MODEL_ROUTE_FORBIDDEN") {
			t.Fatalf("model route %s was not rejected: status=%d body=%s", route, probe.Code, probe.Body.String())
		}
	}
	unknownAsync := httptest.NewRecorder()
	unknownAsyncRequest := httptest.NewRequest(http.MethodPost, "/v1/images/generations/async/extra", strings.NewReader(`{"model":"gpt-image-2"}`))
	unknownAsyncRequest.Header.Set("Authorization", "Bearer sk-not-used")
	server.ServeHTTP(unknownAsync, unknownAsyncRequest)
	if unknownAsync.Code != http.StatusForbidden || !strings.Contains(unknownAsync.Body.String(), "MODEL_ROUTE_FORBIDDEN") {
		t.Fatalf("unknown async image route was not rejected: status=%d body=%s", unknownAsync.Code, unknownAsync.Body.String())
	}
}

func TestAgentAdminCanPersistBrandingWithoutExposingOwnerFields(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("branding update must not call upstream: %s", r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	sessionID, err := server.store.CreateSession("42", []byte(`{"id":"42","email":"u@example.com"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPut, "/api/v1/agent/admin/branding", strings.NewReader(`{"name":"Brand Agent","site_name":"Brand Site","site_logo":"https://cdn.example.com/logo.svg","doc_url":"https://docs.brand.example.com/start","contact_info":"support@brand.example.com"}`))
	req.Host = "agent.example.com"
	req.Header.Set("Origin", "https://agent.example.com")
	req.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "owner_main_user_id") {
		t.Fatalf("branding update status=%d body=%s", rec.Code, rec.Body.String())
	}
	agent, err := server.store.Agent(server.cfg.AgentID)
	if err != nil {
		t.Fatal(err)
	}
	if agent.Name != "Brand Agent" || agent.SiteName != "Brand Site" || agent.SiteLogo != "https://cdn.example.com/logo.svg" || agent.DocURL != "https://docs.brand.example.com/start" || agent.ContactInfo != "support@brand.example.com" {
		t.Fatalf("branding was not persisted: %+v", agent)
	}
	settings := httptest.NewRecorder()
	server.ServeHTTP(settings, httptest.NewRequest(http.MethodGet, "/api/v1/settings/public", nil))
	if settings.Code != http.StatusOK || !strings.Contains(settings.Body.String(), "Brand Site") || !strings.Contains(settings.Body.String(), "cdn.example.com/logo.svg") || !strings.Contains(settings.Body.String(), `"doc_url":"https://docs.brand.example.com/start"`) || !strings.Contains(settings.Body.String(), `"contact_info":"support@brand.example.com"`) {
		t.Fatalf("public branding did not reflect persisted value: status=%d body=%s", settings.Code, settings.Body.String())
	}
	bad := httptest.NewRequest(http.MethodPut, "/api/v1/agent/admin/branding", strings.NewReader(`{"site_logo":"javascript:alert(1)"}`))
	bad.Host = "agent.example.com"
	bad.Header.Set("Origin", "https://agent.example.com")
	bad.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	badResponse := httptest.NewRecorder()
	server.ServeHTTP(badResponse, bad)
	if badResponse.Code != http.StatusBadRequest || !strings.Contains(badResponse.Body.String(), "INVALID_BRANDING") {
		t.Fatalf("unsafe branding URL status=%d body=%s", badResponse.Code, badResponse.Body.String())
	}
	badDoc := httptest.NewRequest(http.MethodPut, "/api/v1/agent/admin/branding", strings.NewReader(`{"doc_url":"javascript:alert(1)"}`))
	badDoc.Host = "agent.example.com"
	badDoc.Header.Set("Origin", "https://agent.example.com")
	badDoc.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	badDocResponse := httptest.NewRecorder()
	server.ServeHTTP(badDocResponse, badDoc)
	if badDocResponse.Code != http.StatusBadRequest || !strings.Contains(badDocResponse.Body.String(), "INVALID_BRANDING") {
		t.Fatalf("unsafe documentation URL status=%d body=%s", badDocResponse.Code, badDocResponse.Body.String())
	}
	badContact := httptest.NewRequest(http.MethodPut, "/api/v1/agent/admin/branding", strings.NewReader("{\"contact_info\":\"support@example.com\\nforged\"}"))
	badContact.Host = "agent.example.com"
	badContact.Header.Set("Origin", "https://agent.example.com")
	badContact.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	badContactResponse := httptest.NewRecorder()
	server.ServeHTTP(badContactResponse, badContact)
	if badContactResponse.Code != http.StatusBadRequest || !strings.Contains(badContactResponse.Body.String(), "INVALID_BRANDING") {
		t.Fatalf("unsafe contact info status=%d body=%s", badContactResponse.Code, badContactResponse.Body.String())
	}
}

func TestAgentAdminPaymentConfigRetainsAndClearsEncryptedSecret(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("payment config must not call upstream: %s", r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	sessionID, err := server.store.CreateSession("42", []byte(`{"id":"42","email":"owner@example.com"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	cookie := &http.Cookie{Name: server.cfg.CookieName, Value: sessionID}

	update := func(origin, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPut, "https://agent.example.com/api/v1/agent/admin/payment-config", strings.NewReader(body))
		req.Header.Set("Origin", origin)
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		return rec
	}

	configured := update("https://agent.example.com", `{"enabled":true,"provider":"stripe-cn","currency":"CNY","merchant_id":"merchant-a","webhook_secret":"top-secret","min_amount_cents":100,"max_amount_cents":100000,"order_ttl_seconds":900,"checkout_url_template":"https://pay.example/checkout?order={order_no}&merchant={merchant_id}"}`)
	if configured.Code != http.StatusOK || !strings.Contains(configured.Body.String(), `"webhook_secret_configured":true`) || strings.Contains(configured.Body.String(), "top-secret") {
		t.Fatalf("configure payment status=%d body=%s", configured.Code, configured.Body.String())
	}

	retained := update("https://agent.example.com", `{"enabled":true,"provider":"stripe-cn","currency":"CNY","merchant_id":"merchant-b","min_amount_cents":200,"max_amount_cents":200000,"order_ttl_seconds":1200,"checkout_url_template":"https://pay.example/checkout?order={order_no}&merchant={merchant_id}"}`)
	if retained.Code != http.StatusOK || strings.Contains(retained.Body.String(), "top-secret") {
		t.Fatalf("retain payment secret status=%d body=%s", retained.Code, retained.Body.String())
	}
	stored, err := server.store.PaymentConfig(server.cfg.AgentID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.WebhookSecret != "top-secret" || stored.MerchantID != "merchant-b" {
		t.Fatalf("payment secret was not retained safely: %+v", stored)
	}

	foreign := update("https://attacker.example", `{"enabled":false,"provider":"manual","currency":"CNY","merchant_id":"","clear_webhook_secret":true,"min_amount_cents":100,"max_amount_cents":100000,"order_ttl_seconds":900,"checkout_url_template":""}`)
	if foreign.Code != http.StatusForbidden || !strings.Contains(foreign.Body.String(), "CSRF_ORIGIN_REJECTED") {
		t.Fatalf("foreign payment update status=%d body=%s", foreign.Code, foreign.Body.String())
	}

	cleared := update("https://agent.example.com", `{"enabled":false,"provider":"manual","currency":"CNY","merchant_id":"","clear_webhook_secret":true,"min_amount_cents":100,"max_amount_cents":100000,"order_ttl_seconds":900,"checkout_url_template":""}`)
	if cleared.Code != http.StatusOK || !strings.Contains(cleared.Body.String(), `"webhook_secret_configured":false`) {
		t.Fatalf("clear payment secret status=%d body=%s", cleared.Code, cleared.Body.String())
	}
	stored, err = server.store.PaymentConfig(server.cfg.AgentID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.WebhookSecret != "" || stored.WebhookSecretSet {
		t.Fatalf("payment secret was not cleared: %+v", stored)
	}
}

func TestAgentRejectsUnknownHostButKeepsHealthProbeAvailable(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unknown host must be rejected before upstream access: %s", r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	server.cfg.AgentDomain = "agent.example.com"
	for _, path := range []string{
		"/",
		"/api/auth/sso/callback?ticket=ignored",
		"/api/v1/settings/public",
		"/api/v1/payments/webhook",
		"/api/v1/agent/context",
		"/v1/usage",
		"/assets/app.js",
	} {
		t.Run(path, func(t *testing.T) {
			unknown := httptest.NewRequest(http.MethodGet, path, nil)
			unknown.Host = "other.example.com"
			unknown.Header.Set("X-Forwarded-Host", "agent.example.com")
			unknown.Header.Set("Forwarded", "host=agent.example.com;proto=https")
			unknownResponse := httptest.NewRecorder()
			server.ServeHTTP(unknownResponse, unknown)
			if unknownResponse.Code != http.StatusMisdirectedRequest || !strings.Contains(unknownResponse.Body.String(), "HOST_NOT_ALLOWED") {
				t.Fatalf("unknown host path=%s status=%d body=%s", path, unknownResponse.Code, unknownResponse.Body.String())
			}
		})
	}
	valid := httptest.NewRequest(http.MethodGet, "/api/v1/settings/public", nil)
	valid.Host = "agent.example.com:443"
	validResponse := httptest.NewRecorder()
	server.ServeHTTP(validResponse, valid)
	if validResponse.Code != http.StatusOK {
		t.Fatalf("configured host with port status=%d body=%s", validResponse.Code, validResponse.Body.String())
	}
	health := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	health.Host = "other.example.com"
	healthResponse := httptest.NewRecorder()
	server.ServeHTTP(healthResponse, health)
	if healthResponse.Code != http.StatusOK {
		t.Fatalf("health probe on internal host status=%d body=%s", healthResponse.Code, healthResponse.Body.String())
	}
	ready := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	ready.Host = "other.example.com"
	readyResponse := httptest.NewRecorder()
	server.ServeHTTP(readyResponse, ready)
	if readyResponse.Code != http.StatusOK {
		t.Fatalf("readiness probe on internal host status=%d body=%s", readyResponse.Code, readyResponse.Body.String())
	}
}

func TestSharedRuntimeAllowsOnlyConfiguredOrPersistedHosts(t *testing.T) {
	upstream := httptest.NewServer(http.NotFoundHandler())
	defer upstream.Close()
	server := testServer(t, upstream)
	server.cfg.AgentID = ""
	server.cfg.AgentDomain = ""
	server.cfg.SharedHosts = []string{"shared.example.com"}

	custom := tenantTestConfig("custom-agent", "custom-owner", "Custom")
	custom.AgentDomain = "custom.example.com"
	if err := server.store.UpsertAgent(custom); err != nil {
		t.Fatal(err)
	}
	for host, want := range map[string]bool{
		"shared.example.com":         true,
		"SHARED.EXAMPLE.COM:443":     true,
		"custom.example.com":         true,
		"unknown.example.com":        false,
		"https://shared.example.com": false,
	} {
		if got := server.allowedHost(host); got != want {
			t.Errorf("allowedHost(%q)=%v, want %v", host, got, want)
		}
	}

	unknown := httptest.NewRequest(http.MethodGet, "/api/v1/settings/public", nil)
	unknown.Host = "unknown.example.com"
	response := httptest.NewRecorder()
	server.ServeHTTP(response, unknown)
	if response.Code != http.StatusMisdirectedRequest || !strings.Contains(response.Body.String(), "HOST_NOT_ALLOWED") {
		t.Fatalf("unknown shared host status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestModelRelayRejectsNonPublicModelBeforeCharging(t *testing.T) {
	called := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	if err := server.store.CreditAgent(server.cfg.AgentID, 1000, "seed", "seed", "test"); err != nil {
		t.Fatal(err)
	}
	if err := server.store.Allocate(server.cfg.AgentID, "42", 500, "allocate", "order", "test"); err != nil {
		t.Fatal(err)
	}
	keyView, key, err := server.store.CreateAPIKey(server.cfg.AgentID, "42", "test")
	if err != nil || keyView.ID == 0 {
		t.Fatalf("create key: view=%+v err=%v", keyView, err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"private-model","messages":[]}`))
	req.Header.Set("Authorization", "Bearer "+key)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, req)
	if response.Code != http.StatusBadRequest || called {
		t.Fatalf("non-public model status=%d called=%v body=%s", response.Code, called, response.Body.String())
	}
	user, err := server.store.User(server.cfg.AgentID, "42")
	if err != nil {
		t.Fatal(err)
	}
	if user.BalanceCents != 500 {
		t.Fatalf("model validation charged wallet: %+v", user)
	}
}

func TestMultipartModelValidationUsesThePublicCatalog(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("model", "private-model"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := validatePublicModel(writer.FormDataContentType(), body.Bytes()); err == nil {
		t.Fatal("private multipart model was accepted")
	}

	body.Reset()
	writer = multipart.NewWriter(&body)
	if err := writer.WriteField("model", "gpt-image-2"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := validatePublicModel(writer.FormDataContentType(), body.Bytes()); err != nil {
		t.Fatalf("public multipart model was rejected: %v", err)
	}
}

func TestTenantAnnouncementsAreLocalIsolatedAndUserScoped(t *testing.T) {
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		http.Error(w, "announcement endpoints must not reach Sub2API", http.StatusInternalServerError)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	if _, err := server.store.UpsertUser(server.cfg.AgentID, "43", "member@example.com", "Member"); err != nil {
		t.Fatal(err)
	}
	ownerSession, err := server.store.CreateSession("42", []byte(`{"id":"42"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	memberSession, err := server.store.CreateSession("43", []byte(`{"id":"43"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	ownerCookie := &http.Cookie{Name: server.cfg.CookieName, Value: ownerSession}
	memberCookie := &http.Cookie{Name: server.cfg.CookieName, Value: memberSession}

	create := httptest.NewRequest(http.MethodPost, "http://agent.local/api/v1/agent/admin/announcements", strings.NewReader(`{"title":"维护通知","content":"今晚进行租户维护。","status":"active","notify_mode":"popup"}`))
	create.Header.Set("Content-Type", "application/json")
	create.Header.Set("Origin", "http://agent.local")
	create.AddCookie(ownerCookie)
	createResponse := httptest.NewRecorder()
	server.ServeHTTP(createResponse, create)
	if createResponse.Code != http.StatusCreated || !strings.Contains(createResponse.Body.String(), "维护通知") {
		t.Fatalf("create announcement status=%d body=%s", createResponse.Code, createResponse.Body.String())
	}

	forbidden := httptest.NewRequest(http.MethodPost, "http://agent.local/api/v1/agent/admin/announcements", strings.NewReader(`{"title":"越权","content":"禁止","status":"active","notify_mode":"silent"}`))
	forbidden.Header.Set("Origin", "http://agent.local")
	forbidden.AddCookie(memberCookie)
	forbiddenResponse := httptest.NewRecorder()
	server.ServeHTTP(forbiddenResponse, forbidden)
	if forbiddenResponse.Code != http.StatusForbidden {
		t.Fatalf("member managed announcements status=%d body=%s", forbiddenResponse.Code, forbiddenResponse.Body.String())
	}

	list := httptest.NewRequest(http.MethodGet, "http://agent.local/api/v1/agent/announcements", nil)
	list.AddCookie(memberCookie)
	listResponse := httptest.NewRecorder()
	server.ServeHTTP(listResponse, list)
	if listResponse.Code != http.StatusOK || !strings.Contains(listResponse.Body.String(), `"unread":1`) || !strings.Contains(listResponse.Body.String(), "维护通知") {
		t.Fatalf("member announcement list status=%d body=%s", listResponse.Code, listResponse.Body.String())
	}
	if listResponse.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("announcement list is cacheable: %v", listResponse.Header())
	}

	read := httptest.NewRequest(http.MethodPost, "http://agent.local/api/v1/agent/announcements/1/read", strings.NewReader(`{}`))
	read.Header.Set("Origin", "http://agent.local")
	read.AddCookie(memberCookie)
	readResponse := httptest.NewRecorder()
	server.ServeHTTP(readResponse, read)
	if readResponse.Code != http.StatusOK {
		t.Fatalf("mark announcement read status=%d body=%s", readResponse.Code, readResponse.Body.String())
	}

	ownerList := httptest.NewRequest(http.MethodGet, "http://agent.local/api/v1/agent/announcements", nil)
	ownerList.AddCookie(ownerCookie)
	ownerListResponse := httptest.NewRecorder()
	server.ServeHTTP(ownerListResponse, ownerList)
	if !strings.Contains(ownerListResponse.Body.String(), `"unread":1`) {
		t.Fatalf("member read state leaked to owner: %s", ownerListResponse.Body.String())
	}
	memberList := httptest.NewRequest(http.MethodGet, "http://agent.local/api/v1/agent/announcements", nil)
	memberList.AddCookie(memberCookie)
	memberListResponse := httptest.NewRecorder()
	server.ServeHTTP(memberListResponse, memberList)
	if !strings.Contains(memberListResponse.Body.String(), `"unread":0`) || !strings.Contains(memberListResponse.Body.String(), `"read_at"`) {
		t.Fatalf("member read state was not persisted: %s", memberListResponse.Body.String())
	}

	crossOriginDelete := httptest.NewRequest(http.MethodDelete, "http://agent.local/api/v1/agent/admin/announcements/1", nil)
	crossOriginDelete.Header.Set("Origin", "https://attacker.example")
	crossOriginDelete.AddCookie(ownerCookie)
	crossOriginResponse := httptest.NewRecorder()
	server.ServeHTTP(crossOriginResponse, crossOriginDelete)
	if crossOriginResponse.Code != http.StatusForbidden {
		t.Fatalf("cross-origin delete status=%d body=%s", crossOriginResponse.Code, crossOriginResponse.Body.String())
	}
	if upstreamCalls.Load() != 0 {
		t.Fatalf("tenant announcements unexpectedly called Sub2API %d times", upstreamCalls.Load())
	}
}
