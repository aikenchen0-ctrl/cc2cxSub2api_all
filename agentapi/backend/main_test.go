package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const testRuntimeControlCredential = "agt_ctl_test-runtime-control-secret-0123456789abcdef"

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

func testServer(t *testing.T, upstream *httptest.Server) *Server {
	t.Helper()
	cfg := Config{
		Addr:                     ":0",
		DatabasePath:             ":memory:",
		MainAPIBaseURL:           upstream.URL + "/api/v1",
		MainModelBaseURL:         upstream.URL + "/v1",
		RuntimeControlCredential: testRuntimeControlCredential,
		AppCredential:            "app-secret",
		SatelliteSlug:            "agentapi",
		SessionSecret:            "session-secret",
		CookieName:               "agentapi_session",
		AgentID:                  "agent-test",
		AgentName:                "Agent Test",
		SiteName:                 "Agent Test",
		OwnerMainUserID:          "42",
		MaxRequestCostCents:      100,
		MainRequestTimeout:       0,
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
	s := &Server{cfg: cfg, store: store, main: NewMainClient(cfg), webDir: "", settleByUser: map[string]*sync.Mutex{}}
	t.Cleanup(func() { _ = store.Close() })
	return s
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
	var mapCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/register":
			registerCalls.Add(1)
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "REGISTRATION_REJECTED", "message": "registration rejected"})
		case "/api/v1/agent-runtime/users/43/map":
			mapCalls.Add(1)
			http.Error(w, "mapping must not run after registration failure", http.StatusInternalServerError)
		default:
			t.Errorf("unexpected Sub2API request: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	server := testServer(t, upstream)
	server.cfg.ProvisioningControlEnabled = true
	server.setProvisioningState("active", time.Now().UTC())
	server.main = NewMainClient(server.cfg)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(`{"email":"new@example.com","password":"password"}`))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code == http.StatusOK || len(response.Result().Cookies()) != 0 {
		t.Fatalf("failed main-site registration unexpectedly created a session: status=%d cookies=%v body=%s", response.Code, response.Result().Cookies(), response.Body.String())
	}
	if _, err := server.store.User(server.cfg.AgentID, "43"); err != errNotFound {
		t.Fatalf("failed main-site registration left a local Agent mapping: user err=%v", err)
	}
	if registerCalls.Load() != 1 || mapCalls.Load() != 0 {
		t.Fatalf("unexpected registration/mapping calls: register=%d map=%d", registerCalls.Load(), mapCalls.Load())
	}
}

func TestRegistrationMappingFailureCanRetryThroughLoginWithoutDuplicateMainUser(t *testing.T) {
	var registerCalls atomic.Int32
	var loginCalls atomic.Int32
	var mapCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/register":
			registerCalls.Add(1)
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{
				"access_token": "register-access", "refresh_token": "register-refresh", "expires_in": 3600,
				"user": map[string]any{"id": 43, "email": "new@example.com", "role": "user"},
			}))
		case "/api/v1/auth/login":
			loginCalls.Add(1)
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{
				"access_token": "login-access", "refresh_token": "login-refresh", "expires_in": 3600,
				"user": map[string]any{"id": 43, "email": "new@example.com", "role": "user"},
			}))
		case "/api/v1/agent-runtime/users/43/map":
			if r.Header.Get("X-AgentAPI-Runtime-Control") != testRuntimeControlCredential || !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
				t.Errorf("runtime mapping did not receive its scoped credentials: %v", r.Header)
			}
			if mapCalls.Add(1) == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				_ = json.NewEncoder(w).Encode(map[string]any{"code": "TEMPORARY_FAILURE", "message": "try again"})
				return
			}
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"agent_id": "agent-test", "user_id": 43, "status": "mapped"}))
		default:
			t.Errorf("unexpected Sub2API request: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	server := testServer(t, upstream)
	server.cfg.ProvisioningControlEnabled = true
	server.setProvisioningState("active", time.Now().UTC())
	server.main = NewMainClient(server.cfg)
	registerRequest := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(`{"email":"new@example.com","password":"password"}`))
	registerResponse := httptest.NewRecorder()
	server.ServeHTTP(registerResponse, registerRequest)
	if registerResponse.Code == http.StatusOK || len(registerResponse.Result().Cookies()) != 0 {
		t.Fatalf("registration with failed mapping unexpectedly created a session: status=%d cookies=%v", registerResponse.Code, registerResponse.Result().Cookies())
	}
	if user, err := server.store.User(server.cfg.AgentID, "43"); err != nil || user.Status != "active" {
		t.Fatalf("retryable local mapping was not retained: user=%+v err=%v", user, err)
	}

	loginRequest := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"new@example.com","password":"password"}`))
	loginResponse := httptest.NewRecorder()
	server.ServeHTTP(loginResponse, loginRequest)
	if loginResponse.Code != http.StatusOK || len(loginResponse.Result().Cookies()) != 1 {
		t.Fatalf("login retry did not recover the mapping: status=%d cookies=%v body=%s", loginResponse.Code, loginResponse.Result().Cookies(), loginResponse.Body.String())
	}
	if registerCalls.Load() != 1 || loginCalls.Load() != 1 || mapCalls.Load() != 2 {
		t.Fatalf("mapping retry duplicated main-site account creation: register=%d login=%d map=%d", registerCalls.Load(), loginCalls.Load(), mapCalls.Load())
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

func TestReadyEndpointChecksAgentAndModelCredential(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	server.cfg.RuntimeControlCredential = ""

	ready := httptest.NewRecorder()
	server.ServeHTTP(ready, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if ready.Code != http.StatusOK {
		t.Fatalf("standalone ready status=%d body=%s", ready.Code, ready.Body.String())
	}
	server.cfg.ProvisioningControlEnabled = true
	controlNotReady := httptest.NewRecorder()
	server.ServeHTTP(controlNotReady, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if controlNotReady.Code != http.StatusServiceUnavailable || !strings.Contains(controlNotReady.Body.String(), "AGENT_RUNTIME_CONTROL_CREDENTIAL_MISSING") {
		t.Fatalf("managed agent without control credential status=%d body=%s", controlNotReady.Code, controlNotReady.Body.String())
	}
	server.cfg.ProvisioningControlEnabled = false
	server.cfg.AppCredential = ""
	notReady := httptest.NewRecorder()
	server.ServeHTTP(notReady, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if notReady.Code != http.StatusServiceUnavailable {
		t.Fatalf("missing credential ready status=%d body=%s", notReady.Code, notReady.Body.String())
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
		if r.URL.Path == "/api/v1/agent-runtime/owner" {
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
	if user.BalanceCents != 400 {
		t.Fatalf("reservation was not charged: %+v", user)
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

func TestAgentAdminModelPolicyFiltersDiscoveryAndBlocksDisabledRequests(t *testing.T) {
	upstreamCalls := 0
	var syncedModels []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/agent-runtime/model-policy" {
			var payload struct {
				Enabled []string `json:"enabled"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("decode main model policy request: %v", err)
			}
			syncedModels = append([]string(nil), payload.Enabled...)
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"agent_id": "agent-test", "enabled": payload.Enabled}))
			return
		}
		upstreamCalls++
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	sessionID, err := server.store.CreateSession("42", []byte(`{"id":"42","email":"owner@example.com","role":"admin"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}

	update := httptest.NewRequest(http.MethodPut, "http://agent.local/api/v1/agent/admin/model-policy", strings.NewReader(`{"enabled":["gpt-5.5","gpt-image-2"]}`))
	update.Header.Set("Content-Type", "application/json")
	update.Header.Set("Origin", "http://agent.local")
	update.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	updateResponse := httptest.NewRecorder()
	server.ServeHTTP(updateResponse, update)
	if updateResponse.Code != http.StatusOK {
		t.Fatalf("model policy update status=%d body=%s", updateResponse.Code, updateResponse.Body.String())
	}
	if len(syncedModels) != 2 || syncedModels[0] != "gpt-5.5" || syncedModels[1] != "gpt-image-2" {
		t.Fatalf("Agent model policy was not synchronized to Sub2API: %v", syncedModels)
	}
	if _, err := server.store.UpsertUser(server.cfg.AgentID, "99", "member@example.com", "Member"); err != nil {
		t.Fatal(err)
	}
	nonOwnerSession, err := server.store.CreateSession("99", []byte(`{"id":"99","email":"member@example.com","role":"admin"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	nonOwnerUpdate := httptest.NewRequest(http.MethodPut, "http://agent.local/api/v1/agent/admin/model-policy", strings.NewReader(`{"enabled":["gpt-image-1"]}`))
	nonOwnerUpdate.Header.Set("Content-Type", "application/json")
	nonOwnerUpdate.Header.Set("Origin", "http://agent.local")
	nonOwnerUpdate.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: nonOwnerSession})
	nonOwnerResponse := httptest.NewRecorder()
	server.ServeHTTP(nonOwnerResponse, nonOwnerUpdate)
	if nonOwnerResponse.Code != http.StatusForbidden {
		t.Fatalf("non-owner model policy update status=%d body=%s, want forbidden", nonOwnerResponse.Code, nonOwnerResponse.Body.String())
	}

	modelsRequest := httptest.NewRequest(http.MethodGet, "http://agent.local/v1/models", nil)
	modelsRequest.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	modelsResponse := httptest.NewRecorder()
	server.ServeHTTP(modelsResponse, modelsRequest)
	var models struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(modelsResponse.Body.Bytes(), &models); err != nil {
		t.Fatal(err)
	}
	if modelsResponse.Code != http.StatusOK || len(models.Data) != 2 || models.Data[0].ID != "gpt-5.5" || models.Data[1].ID != "gpt-image-2" {
		t.Fatalf("unexpected filtered models response: status=%d body=%s", modelsResponse.Code, modelsResponse.Body.String())
	}

	for _, test := range []struct {
		body, reason string
	}{
		{body: `{"model":"grok-imagine-video-1.5"}`, reason: "MODEL_NOT_ALLOWED"},
		{body: `{"messages":[]}`, reason: "MODEL_REQUIRED"},
	} {
		request := httptest.NewRequest(http.MethodPost, "http://agent.local/v1/chat/completions", strings.NewReader(test.body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Origin", "http://agent.local")
		request.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), test.reason) {
			t.Fatalf("model request %s returned %d %s, want %s", test.body, response.Code, response.Body.String(), test.reason)
		}
	}

	var multipartBody bytes.Buffer
	multipartWriter := multipart.NewWriter(&multipartBody)
	if err := multipartWriter.WriteField("model", "gpt-image-1"); err != nil {
		t.Fatal(err)
	}
	if err := multipartWriter.Close(); err != nil {
		t.Fatal(err)
	}
	imageRequest := httptest.NewRequest(http.MethodPost, "http://agent.local/v1/images/generations", &multipartBody)
	imageRequest.Header.Set("Content-Type", multipartWriter.FormDataContentType())
	imageRequest.Header.Set("Origin", "http://agent.local")
	imageRequest.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	imageResponse := httptest.NewRecorder()
	server.ServeHTTP(imageResponse, imageRequest)
	if imageResponse.Code != http.StatusBadRequest || !strings.Contains(imageResponse.Body.String(), "MODEL_NOT_ALLOWED") {
		t.Fatalf("disabled multipart image model returned %d %s", imageResponse.Code, imageResponse.Body.String())
	}
	var duplicateModelBody bytes.Buffer
	duplicateModelWriter := multipart.NewWriter(&duplicateModelBody)
	if err := duplicateModelWriter.WriteField("model", "gpt-5.5"); err != nil {
		t.Fatal(err)
	}
	if err := duplicateModelWriter.WriteField("model", "gpt-image-1"); err != nil {
		t.Fatal(err)
	}
	if err := duplicateModelWriter.Close(); err != nil {
		t.Fatal(err)
	}
	duplicateModelRequest := httptest.NewRequest(http.MethodPost, "http://agent.local/v1/images/generations", &duplicateModelBody)
	duplicateModelRequest.Header.Set("Content-Type", duplicateModelWriter.FormDataContentType())
	duplicateModelRequest.Header.Set("Origin", "http://agent.local")
	duplicateModelRequest.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	duplicateModelResponse := httptest.NewRecorder()
	server.ServeHTTP(duplicateModelResponse, duplicateModelRequest)
	if duplicateModelResponse.Code != http.StatusBadRequest || !strings.Contains(duplicateModelResponse.Body.String(), "MODEL_NOT_ALLOWED") {
		t.Fatalf("duplicate multipart model fields returned %d %s", duplicateModelResponse.Code, duplicateModelResponse.Body.String())
	}
	if upstreamCalls != 0 {
		t.Fatalf("disabled or underspecified model reached Sub2API: calls=%d", upstreamCalls)
	}
}

func TestAgentModelPolicyRemainsUnchangedWhenMainSiteScopeCannotBeConfirmed(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	mainControl := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer mainControl.Close()
	server.cfg.MainAPIBaseURL = mainControl.URL + "/api/v1"
	server.main = NewMainClient(server.cfg)
	sessionID, err := server.store.CreateSession("42", []byte(`{"id":"42","email":"owner@example.com","role":"admin"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPut, "http://agent.local/api/v1/agent/admin/model-policy", strings.NewReader(`{"enabled":["gpt-5.5"]}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://agent.local")
	request.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("unconfirmed model scope status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	policy, err := server.store.AgentModelPolicy(server.cfg.AgentID)
	if err != nil {
		t.Fatal(err)
	}
	if policy.Customized || len(policy.Enabled) != len(publicModelCatalog) {
		t.Fatalf("local policy changed despite failed main-site confirmation: %+v", policy)
	}
}

func TestConcurrentModelPolicyUpdateFailureCannotRollbackConfirmedNewerPolicy(t *testing.T) {
	firstSyncEntered := make(chan struct{}, 1)
	secondSyncEntered := make(chan struct{}, 1)
	releaseFirstSync := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(releaseFirstSync) })
	var remoteMu sync.Mutex
	var remoteEnabled []string

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agent-runtime/model-policy" {
			t.Errorf("unexpected upstream path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		var payload struct {
			Enabled []string `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode model policy request: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		switch {
		case sameStringSlice(payload.Enabled, []string{"gpt-5.5"}):
			firstSyncEntered <- struct{}{}
			<-releaseFirstSync
			http.Error(w, "temporary main-site failure", http.StatusServiceUnavailable)
		case sameStringSlice(payload.Enabled, []string{"gpt-image-2"}):
			secondSyncEntered <- struct{}{}
			remoteMu.Lock()
			remoteEnabled = append([]string(nil), payload.Enabled...)
			remoteMu.Unlock()
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{
				"agent_id": "agent-test", "enabled": payload.Enabled,
			}))
		default:
			t.Errorf("unexpected model policy: %v", payload.Enabled)
			http.Error(w, "unexpected policy", http.StatusBadRequest)
		}
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	server.cfg.ProvisioningControlEnabled = true
	server.main = NewMainClient(server.cfg)
	server.setProvisioningState("active", time.Now().UTC())
	sessionID, err := server.store.CreateSession("42", []byte(`{"id":"42","email":"owner@example.com"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	update := func(models string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPut, "http://agent.local/api/v1/agent/admin/model-policy", strings.NewReader(models))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Origin", "http://agent.local")
		request.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)
		return response
	}

	firstDone := make(chan *httptest.ResponseRecorder, 1)
	go func() { firstDone <- update(`{"enabled":["gpt-5.5"]}`) }()
	select {
	case <-firstSyncEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("first model policy update did not reach Sub2API")
	}

	secondStarted := make(chan struct{})
	secondDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		close(secondStarted)
		secondDone <- update(`{"enabled":["gpt-image-2"]}`)
	}()
	<-secondStarted
	select {
	case <-secondSyncEntered:
		t.Fatal("second model policy reached Sub2API before the first update and rollback completed")
	case <-time.After(250 * time.Millisecond):
	}

	releaseOnce.Do(func() { close(releaseFirstSync) })
	firstResponse := <-firstDone
	secondResponse := <-secondDone
	if firstResponse.Code != http.StatusBadGateway {
		t.Fatalf("first update status=%d body=%s, want failed main-site confirmation", firstResponse.Code, firstResponse.Body.String())
	}
	if secondResponse.Code != http.StatusOK {
		t.Fatalf("second update status=%d body=%s", secondResponse.Code, secondResponse.Body.String())
	}
	policy, err := server.store.AgentModelPolicy(server.cfg.AgentID)
	if err != nil {
		t.Fatal(err)
	}
	if !policy.Customized || !sameStringSlice(policy.Enabled, []string{"gpt-image-2"}) {
		t.Fatalf("local model policy does not match the latest confirmed update: %+v", policy)
	}
	remoteMu.Lock()
	defer remoteMu.Unlock()
	if !sameStringSlice(remoteEnabled, []string{"gpt-image-2"}) {
		t.Fatalf("unexpected final Sub2API model policy: %v", remoteEnabled)
	}
}

func TestRechargeWebhookRequiresSignatureAndAllocatesVerifiedOrder(t *testing.T) {
	adminReads := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agent-runtime/owner" {
			t.Fatalf("unexpected upstream path: %s", r.URL.Path)
		}
		adminReads++
		_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 42, "email": "u@example.com", "balance": 100.0}))
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	server.cfg.PaymentEnabled = true
	server.cfg.PaymentProvider = "manual"
	server.cfg.PaymentCurrency = "CNY"
	server.cfg.PaymentWebhookSecret = "payment-secret"
	server.cfg.PaymentMinCents = 100
	server.cfg.PaymentMaxCents = 100000
	server.cfg.PaymentOrderTTL = 30 * time.Minute

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

	mac := hmac.New(sha256.New, []byte(server.cfg.PaymentWebhookSecret))
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
}

func TestPaidRechargeWaitsForOwnerCreditAndAdminCanRetryAllocation(t *testing.T) {
	adminReads := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agent-runtime/owner" {
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
	server.cfg.PaymentEnabled = true
	server.cfg.PaymentProvider = "manual"
	server.cfg.PaymentCurrency = "CNY"
	server.cfg.PaymentWebhookSecret = "payment-secret"
	server.cfg.PaymentMinCents = 100
	server.cfg.PaymentMaxCents = 100000
	server.cfg.PaymentOrderTTL = 30 * time.Minute
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
	mac := hmac.New(sha256.New, []byte(server.cfg.PaymentWebhookSecret))
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
		case "/api/v1/agent-runtime/owner":
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
	if user.BalanceCents != 400 {
		t.Fatalf("key relay did not charge local wallet: %+v", user)
	}
}

func TestModelRelayUsesSessionIdentityAndKeepsOwnerBilling(t *testing.T) {
	var modelHeader http.Header
	adminReads := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/agent-runtime/owner":
			adminReads++
			balance := 10.0
			if adminReads > 1 {
				balance = 9.0
			}
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 99, "email": "owner@example.com", "balance": balance}))
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
		t.Fatalf("upstream did not receive the mapped session identity: %v", modelHeader)
	}
	user, err := server.store.User(server.cfg.AgentID, "42")
	if err != nil {
		t.Fatal(err)
	}
	if user.BalanceCents != 400 {
		t.Fatalf("proxy user's local reservation was not charged: %+v", user)
	}
	usage, err := server.store.Usage(server.cfg.AgentID, "42", 10)
	if err != nil || len(usage) != 1 || usage[0].SettlementStatus != "confirmed" || usage[0].ActualCents != 100 {
		t.Fatalf("unexpected owner settlement: %+v err=%v", usage, err)
	}
	settlement, err := server.store.Settlement(server.cfg.AgentID, "req-owner-billing")
	if err != nil || settlement.ProxyMainUserID != "42" || settlement.BillingMainUserID != "99" {
		t.Fatalf("session and billing identities were not kept distinct: settlement=%+v err=%v", settlement, err)
	}
}

func TestModelRelayUsesRequestUsageInsteadOfSharedOwnerBalanceDelta(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/agent-runtime/owner":
			// The owner balance includes an unrelated external charge after the
			// request. It must not become this proxy user's usage.
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 99, "balance": 9.0}))
		case "/api/v1/agent-runtime/usage":
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"items": []map[string]any{{
				"id": 700, "request_id": "usage-authoritative", "model": "gpt-5.5", "actual_cost": 0.25, "total_cost": 0.31,
				"input_tokens": 1000, "output_tokens": 25, "input_cost": 0.000001234, "upstream_model": "provider-gpt-5.5",
				"ip_address": "192.0.2.123", "user": map[string]any{"email": "private@example.com"},
			}}}))
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
	if usage[0].Model != "gpt-5.5" || usage[0].Source != "sub2api_owner_runtime_usage" || usage[0].InputTokens != 1000 || usage[0].OutputTokens != 25 || usage[0].InputCostNanos != 1234 || usage[0].TotalCostNanos != 310_000_000 || usage[0].ActualCostNanos != 250_000_000 || usage[0].UpstreamModel != "provider-gpt-5.5" {
		t.Fatalf("authoritative usage details were not stored: %+v", usage[0])
	}
	encodedUsage, err := json.Marshal(usage[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encodedUsage), "192.0.2.123") || strings.Contains(string(encodedUsage), "private@example.com") || strings.Contains(string(encodedUsage), "ip_address") {
		t.Fatalf("admin-only usage fields leaked into the AgentAPI view: %s", encodedUsage)
	}
	if !strings.Contains(string(encodedUsage), `"usage_source":"sub2api_owner_runtime_usage"`) || !strings.Contains(string(encodedUsage), `"input_tokens":1000`) || strings.Contains(string(encodedUsage), `"MainUsageSnapshot"`) {
		t.Fatalf("main usage snapshot was not flattened into the user API DTO: %s", encodedUsage)
	}
}

func TestSettlementReconcileStoresMainUsageSnapshot(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agent-runtime/usage" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(envelope(map[string]any{"items": []map[string]any{{
			"id": 701, "request_id": "pending-usage", "model": "claude-sonnet", "actual_cost": 0.12, "total_cost": 0.15,
			"input_tokens": 80, "output_tokens": 20, "cache_read_tokens": 10, "cache_read_cost": 0.000000019,
		}}}))
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
	results, err := server.reconcileSettlements(t.Context(), "pending-usage")
	if err != nil || len(results) != 1 || results[0].Status != "confirmed" {
		t.Fatalf("reconcile result=%+v err=%v", results, err)
	}
	usage, err := server.store.Usage(server.cfg.AgentID, "42", 10)
	if err != nil || len(usage) != 1 {
		t.Fatalf("read reconciled usage: %+v err=%v", usage, err)
	}
	item := usage[0]
	if item.Source != "sub2api_owner_runtime_usage" || item.UsageID != "701" || item.ActualCents != 12 || item.Model != "claude-sonnet" || item.InputTokens != 80 || item.OutputTokens != 20 || item.CacheReadTokens != 10 || item.CacheReadCostNanos != 19 {
		t.Fatalf("reconciler lost authoritative usage fields: %+v", item)
	}
}

func TestAgentUsersEndpointPaginatesAndRestrictsNonAdminToSelf(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("mapped user list must come from the local Agent store: %s", r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	for _, id := range []string{"43", "44", "45"} {
		if _, err := server.store.UpsertUser(server.cfg.AgentID, id, id+"@example.com", "User "+id); err != nil {
			t.Fatal(err)
		}
	}
	adminSession, err := server.store.CreateSession("42", []byte(`{"id":"42","role":"admin"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	userSession, err := server.store.CreateSession("43", []byte(`{"id":"43"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}

	adminRequest := httptest.NewRequest(http.MethodGet, "/api/v1/agent/users?page=2&page_size=2", nil)
	adminRequest.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: adminSession})
	adminResponse := httptest.NewRecorder()
	server.ServeHTTP(adminResponse, adminRequest)
	if adminResponse.Code != http.StatusOK {
		t.Fatalf("admin users page status=%d body=%s", adminResponse.Code, adminResponse.Body.String())
	}
	var adminPayload struct {
		Data struct {
			Items    []AgentUserView `json:"items"`
			Total    int             `json:"total"`
			Page     int             `json:"page"`
			PageSize int             `json:"page_size"`
		} `json:"data"`
	}
	if err := json.Unmarshal(adminResponse.Body.Bytes(), &adminPayload); err != nil {
		t.Fatal(err)
	}
	if adminPayload.Data.Total != 4 || adminPayload.Data.Page != 2 || adminPayload.Data.PageSize != 2 || len(adminPayload.Data.Items) != 2 || adminPayload.Data.Items[0].MainUserID != "43" || adminPayload.Data.Items[1].MainUserID != "42" {
		t.Fatalf("unexpected admin user page: %+v", adminPayload.Data)
	}
	adminRouteRequest := httptest.NewRequest(http.MethodGet, "/api/v1/agent/admin/users?page=2&page_size=2", nil)
	adminRouteRequest.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: adminSession})
	adminRouteResponse := httptest.NewRecorder()
	server.ServeHTTP(adminRouteResponse, adminRouteRequest)
	if adminRouteResponse.Code != http.StatusOK || !strings.Contains(adminRouteResponse.Body.String(), `"page":2`) || !strings.Contains(adminRouteResponse.Body.String(), `"total":4`) {
		t.Fatalf("admin compatibility user page status=%d body=%s", adminRouteResponse.Code, adminRouteResponse.Body.String())
	}
	adminSearchRequest := httptest.NewRequest(http.MethodGet, "/api/v1/agent/users?q=44", nil)
	adminSearchRequest.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: adminSession})
	adminSearchResponse := httptest.NewRecorder()
	server.ServeHTTP(adminSearchResponse, adminSearchRequest)
	var searchPayload struct {
		Data struct {
			Items []AgentUserView `json:"items"`
			Total int             `json:"total"`
		} `json:"data"`
	}
	if adminSearchResponse.Code != http.StatusOK || json.Unmarshal(adminSearchResponse.Body.Bytes(), &searchPayload) != nil || searchPayload.Data.Total != 1 || len(searchPayload.Data.Items) != 1 || searchPayload.Data.Items[0].MainUserID != "44" {
		t.Fatalf("admin user search failed: status=%d body=%s data=%+v", adminSearchResponse.Code, adminSearchResponse.Body.String(), searchPayload.Data)
	}

	userRequest := httptest.NewRequest(http.MethodGet, "/api/v1/agent/users?page=2&page_size=2&q=44", nil)
	userRequest.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: userSession})
	userResponse := httptest.NewRecorder()
	server.ServeHTTP(userResponse, userRequest)
	var userPayload struct {
		Data struct {
			Items []AgentUserView `json:"items"`
			Total int             `json:"total"`
			Page  int             `json:"page"`
		} `json:"data"`
	}
	if userResponse.Code != http.StatusOK || json.Unmarshal(userResponse.Body.Bytes(), &userPayload) != nil || userPayload.Data.Total != 1 || userPayload.Data.Page != 1 || len(userPayload.Data.Items) != 1 || userPayload.Data.Items[0].MainUserID != "43" {
		t.Fatalf("non-admin user list escaped self scope: status=%d body=%s data=%+v", userResponse.Code, userResponse.Body.String(), userPayload.Data)
	}

	invalidPage := httptest.NewRequest(http.MethodGet, "/api/v1/agent/users?page_size=201", nil)
	invalidPage.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: adminSession})
	invalidPageResponse := httptest.NewRecorder()
	server.ServeHTTP(invalidPageResponse, invalidPage)
	if invalidPageResponse.Code != http.StatusBadRequest || !strings.Contains(invalidPageResponse.Body.String(), "INVALID_PAGINATION") {
		t.Fatalf("invalid user page size status=%d body=%s", invalidPageResponse.Code, invalidPageResponse.Body.String())
	}
	invalidSearch := httptest.NewRequest(http.MethodGet, "/api/v1/agent/users?q="+strings.Repeat("x", 513), nil)
	invalidSearch.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: adminSession})
	invalidSearchResponse := httptest.NewRecorder()
	server.ServeHTTP(invalidSearchResponse, invalidSearch)
	if invalidSearchResponse.Code != http.StatusBadRequest || !strings.Contains(invalidSearchResponse.Body.String(), "INVALID_SEARCH") {
		t.Fatalf("invalid user search status=%d body=%s", invalidSearchResponse.Code, invalidSearchResponse.Body.String())
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

	badRequest := httptest.NewRequest(http.MethodGet, "/api/v1/agent/usage?page_size=201", nil)
	badRequest.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: adminSession})
	badResponse := httptest.NewRecorder()
	server.ServeHTTP(badResponse, badRequest)
	if badResponse.Code != http.StatusBadRequest || !strings.Contains(badResponse.Body.String(), "INVALID_PAGINATION") {
		t.Fatalf("invalid page size status=%d body=%s", badResponse.Code, badResponse.Body.String())
	}
}

func TestAgentTaskHistoryEndpointRestoresTasksWithSessionUserScope(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("task history must use persisted AgentAPI task IDs without an upstream query: %s", r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	if _, err := server.store.UpsertUser(server.cfg.AgentID, "43", "other@example.com", "Other"); err != nil {
		t.Fatal(err)
	}
	if err := server.store.CreditAgent(server.cfg.AgentID, 500, "seed-task-history", "seed", "test"); err != nil {
		t.Fatal(err)
	}
	if err := server.store.Allocate(server.cfg.AgentID, "42", 200, "allocate-task-history", "order-task-history", "test"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := server.store.PrepareSettlement(server.cfg.AgentID, "42", "42", "request-image-1", "request-image-1", 100); err != nil {
		t.Fatal(err)
	}
	if _, err := server.store.RecordVideoTask(server.cfg.AgentID, "42", "video-older", "request-video-older", "processing"); err != nil {
		t.Fatal(err)
	}
	if _, err := server.store.RecordImageTask(server.cfg.AgentID, "42", "image-newer", "request-image-1", "processing"); err != nil {
		t.Fatal(err)
	}
	if _, err := server.store.RecordVideoTask(server.cfg.AgentID, "43", "video-other-user", "request-other-user", "queued"); err != nil {
		t.Fatal(err)
	}
	// Set distinct persisted timestamps so pagination order is deterministic.
	if _, err := server.store.db.Exec(`UPDATE video_tasks SET updated_at=100 WHERE task_id='video-older'`); err != nil {
		t.Fatal(err)
	}
	if _, err := server.store.db.Exec(`UPDATE image_tasks SET updated_at=200 WHERE task_id='image-newer'`); err != nil {
		t.Fatal(err)
	}
	if _, err := server.store.db.Exec(`UPDATE video_tasks SET updated_at=300 WHERE task_id='video-other-user'`); err != nil {
		t.Fatal(err)
	}
	ownerSession, err := server.store.CreateSession("42", []byte(`{"id":"42"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	userSession, err := server.store.CreateSession("43", []byte(`{"id":"43"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	readPage := func(cookie, path string) (int, *httptest.ResponseRecorder, []AgentTaskView, int) {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if cookie != "" {
			req.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: cookie})
		}
		response := httptest.NewRecorder()
		server.ServeHTTP(response, req)
		var payload struct {
			Data struct {
				Items []AgentTaskView `json:"items"`
				Total int             `json:"total"`
			} `json:"data"`
		}
		if response.Code == http.StatusOK {
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatalf("decode task history response: %v body=%s", err, response.Body.String())
			}
		}
		return response.Code, response, payload.Data.Items, payload.Data.Total
	}
	status, ownerPage, ownerItems, ownerTotal := readPage(ownerSession, "/api/v1/agent/tasks?page=1&page_size=1")
	if status != http.StatusOK || ownerTotal != 2 || len(ownerItems) != 1 || ownerItems[0].TaskID != "image-newer" || ownerItems[0].TaskType != "image" || ownerItems[0].SettlementStatus != "pending" || ownerItems[0].ReservedCents != 100 {
		t.Fatalf("unexpected owner task history page: status=%d total=%d items=%+v body=%s", status, ownerTotal, ownerItems, ownerPage.Body.String())
	}
	if ownerPage.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("task history must not be cached: %q", ownerPage.Header().Get("Cache-Control"))
	}
	status, _, ownerItems, ownerTotal = readPage(ownerSession, "/api/v1/agent/tasks?page=2&page_size=1")
	if status != http.StatusOK || ownerTotal != 2 || len(ownerItems) != 1 || ownerItems[0].TaskID != "video-older" {
		t.Fatalf("unexpected second owner task history page: status=%d total=%d items=%+v", status, ownerTotal, ownerItems)
	}
	status, _, userItems, userTotal := readPage(userSession, "/api/v1/agent/tasks")
	if status != http.StatusOK || userTotal != 1 || len(userItems) != 1 || userItems[0].TaskID != "video-other-user" {
		t.Fatalf("task history escaped the current user's mapping: status=%d total=%d items=%+v", status, userTotal, userItems)
	}
	status, _, _, _ = readPage("", "/api/v1/agent/tasks")
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated task history status=%d", status)
	}
	status, response, _, _ := readPage(ownerSession, "/api/v1/agent/tasks?page_size=201")
	if status != http.StatusBadRequest || !strings.Contains(response.Body.String(), "INVALID_PAGINATION") {
		t.Fatalf("invalid task page size status=%d body=%s", status, response.Body.String())
	}
}

func TestStreamingModelRelayForwardsChunksAndSettlesAfterEOF(t *testing.T) {
	adminReads := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/agent-runtime/owner":
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

func TestVideoTaskPollIsOwnerScopedAndSettlesOriginalRequest(t *testing.T) {
	var usageVisible atomic.Bool
	videoPolls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/agent-runtime/owner":
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 42, "balance": 10.0}))
		case "/api/v1/agent-runtime/usage":
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

func TestAsyncImageTaskPollIsOwnerScopedAndSettlesOriginalRequest(t *testing.T) {
	var usageVisible atomic.Bool
	var imageCreates atomic.Int32
	imagePolls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/agent-runtime/owner":
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 42, "balance": 10.0}))
		case "/api/v1/agent-runtime/usage":
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
				case "/api/v1/agent-runtime/owner":
					_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 99, "balance": 10.0}))
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

func TestFailedAsyncImagePollReleasesReservationWithoutUsage(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/agent-runtime/owner":
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 42, "balance": 10.0}))
		case "/api/v1/agent-runtime/usage":
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
	if err != nil || len(usage) != 1 || usage[0].SettlementStatus != "released" || usage[0].ActualCents != 0 {
		t.Fatalf("failed image reservation was not released: %+v err=%v", usage, err)
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
				case "/api/v1/agent-runtime/owner":
					_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 99, "balance": 10.0}))
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

func TestStaleVideoTaskReconcilerReleasesFailedTask(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/videos/video-stale":
			_, _ = io.WriteString(w, `{"id":"video-stale","status":"failed"}`)
		case "/api/v1/agent-runtime/usage":
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
	server.reconcileStaleVideoTasks(context.Background())
	usage, err := server.store.Usage(server.cfg.AgentID, "42", 10)
	if err != nil || len(usage) != 1 || usage[0].SettlementStatus != "released" {
		t.Fatalf("stale failed video was not released: %+v err=%v", usage, err)
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
		case "/api/v1/agent-runtime/usage":
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
	server.reconcileStaleVideoTasks(context.Background())
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
		case "/api/v1/agent-runtime/usage":
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
	server.reconcileStaleVideoTasks(context.Background())
	usage, err := server.store.Usage(server.cfg.AgentID, "42", 10)
	if err != nil || len(usage) != 1 || usage[0].SettlementStatus != "pending" {
		t.Fatalf("failed usage lookup must keep failed video reservation pending: %+v err=%v", usage, err)
	}
}

func TestStaleImageTaskReconcilerReleasesFailedTask(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/agent-runtime/usage" {
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
	server.reconcileStaleImageTasks(context.Background())
	usage, err := server.store.Usage(server.cfg.AgentID, "42", 10)
	if err != nil || len(usage) != 1 || usage[0].SettlementStatus != "released" {
		t.Fatalf("stale failed image was not released: %+v err=%v", usage, err)
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
		case "/api/v1/agent-runtime/usage":
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
	server.reconcileStaleImageTasks(context.Background())
	usage, err := server.store.Usage(server.cfg.AgentID, "42", 10)
	if err != nil || len(usage) != 1 || usage[0].SettlementStatus != "confirmed" || usage[0].UsageID != "703" || usage[0].ActualCents != 25 {
		t.Fatalf("known main-site usage was not settled for failed image task: %+v err=%v", usage, err)
	}
}

func TestUncertainRelayStaysPendingInsteadOfRefunding(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/agent-runtime/owner" {
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
	if user.BalanceCents != 400 {
		t.Fatalf("uncertain charge was incorrectly refunded: %+v", user)
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
		case "/api/v1/agent-runtime/owner":
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
	if user.BalanceCents != 400 {
		t.Fatalf("duplicate request changed local wallet twice: %+v", user)
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
		req.Header.Set("Authorization", "Bearer sk-agent-not-used")
		server.ServeHTTP(probe, req)
		if probe.Code != http.StatusForbidden || !strings.Contains(probe.Body.String(), "MODEL_ROUTE_FORBIDDEN") {
			t.Fatalf("model route %s was not rejected: status=%d body=%s", route, probe.Code, probe.Body.String())
		}
	}
	unknownAsync := httptest.NewRecorder()
	unknownAsyncRequest := httptest.NewRequest(http.MethodPost, "/v1/images/generations/async/extra", strings.NewReader(`{"model":"gpt-image-2"}`))
	unknownAsyncRequest.Header.Set("Authorization", "Bearer sk-agent-not-used")
	server.ServeHTTP(unknownAsync, unknownAsyncRequest)
	if unknownAsync.Code != http.StatusForbidden || !strings.Contains(unknownAsync.Body.String(), "MODEL_ROUTE_FORBIDDEN") {
		t.Fatalf("unknown async image route was not rejected: status=%d body=%s", unknownAsync.Code, unknownAsync.Body.String())
	}
}

func TestAgentAdminAllocationRejectsForeignOrigin(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("CSRF-rejected allocation must not call upstream: %s", r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	sessionID, err := server.store.CreateSession("42", []byte(`{"id":"42","email":"u@example.com"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/admin/users/42/allocate", strings.NewReader(`{"amount":"1.00"}`))
	req.Host = "agent.example.com"
	req.Header.Set("Origin", "https://evil.example.com")
	req.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "CSRF_ORIGIN_REJECTED") {
		t.Fatalf("foreign allocation origin status=%d body=%s", rec.Code, rec.Body.String())
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
	req := httptest.NewRequest(http.MethodPut, "/api/v1/agent/admin/branding", strings.NewReader(`{"name":"Brand Agent","site_name":"Brand Site","site_logo":"https://cdn.example.com/logo.svg"}`))
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
	if agent.Name != "Brand Agent" || agent.SiteName != "Brand Site" || agent.SiteLogo != "https://cdn.example.com/logo.svg" {
		t.Fatalf("branding was not persisted: %+v", agent)
	}
	settings := httptest.NewRecorder()
	server.ServeHTTP(settings, httptest.NewRequest(http.MethodGet, "/api/v1/settings/public", nil))
	if settings.Code != http.StatusOK || !strings.Contains(settings.Body.String(), "Brand Site") || !strings.Contains(settings.Body.String(), "cdn.example.com/logo.svg") {
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
}

func TestAgentAdminAllocationSynchronizesOwnerAndAllocatesUnderOneLock(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agent-runtime/owner" {
			t.Errorf("unexpected owner lookup path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 42, "email": "owner@example.com", "balance": 10.0}))
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	sessionID, err := server.store.CreateSession("42", []byte(`{"id":"42","email":"owner@example.com"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/admin/users/42/allocate", strings.NewReader(`{"amount_cents":100}`))
	req.Host = "agent.example.com"
	req.Header.Set("Origin", "https://agent.example.com")
	req.Header.Set("Idempotency-Key", "admin-allocate-lock")
	req.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("allocation status=%d body=%s", rec.Code, rec.Body.String())
	}
	user, err := server.store.User(server.cfg.AgentID, "42")
	if err != nil {
		t.Fatal(err)
	}
	if user.BalanceCents != 100 {
		t.Fatalf("allocation did not credit user: %+v", user)
	}
	agent, err := server.store.Agent(server.cfg.AgentID)
	if err != nil {
		t.Fatal(err)
	}
	if agent.WalletAvailable != 900 || agent.WalletAllocated != 100 {
		t.Fatalf("unexpected synchronized wallet: %+v", agent)
	}
}

func TestAgentAdminMappedUserStatusUsesScopedRuntimeAPIAndGatesModels(t *testing.T) {
	var statusUpdates, modelRequests int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/agent-runtime/users/43/status":
			statusUpdates++
			if r.Method != http.MethodPatch || r.Header.Get("X-AgentAPI-Runtime-Control") != testRuntimeControlCredential || r.Header.Get("x-api-key") != "" || r.Header.Get("Authorization") != "" {
				t.Errorf("unexpected scoped runtime request: method=%s control=%q", r.Method, r.Header.Get("X-AgentAPI-Runtime-Control"))
			}
			var payload map[string]json.RawMessage
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("decode main user status update: %v", err)
			}
			if len(payload) != 1 || string(payload["status"]) == "" {
				t.Errorf("status update included non-status fields: %s", mustJSON(payload))
			}
			var status string
			_ = json.Unmarshal(payload["status"], &status)
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"agent_id": "agent-test", "user_id": 43, "status": status}))
		case "/v1/models":
			modelRequests++
			_ = json.NewEncoder(w).Encode(map[string]any{"object": "list", "data": []any{}})
		default:
			t.Errorf("unexpected upstream path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	if _, err := server.store.UpsertUser(server.cfg.AgentID, "43", "mapped@example.com", "Mapped"); err != nil {
		t.Fatal(err)
	}
	userSession, err := server.store.CreateSession("43", []byte(`{"id":"43","email":"mapped@example.com"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	ownerSession, err := server.store.CreateSession("42", []byte(`{"id":"42","email":"owner@example.com"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	_, agentKey, err := server.store.CreateAPIKey(server.cfg.AgentID, "43", "status test")
	if err != nil {
		t.Fatal(err)
	}
	updateStatus := func(status string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/agent/admin/users/43/status", strings.NewReader(`{"status":"`+status+`","balance":999999}`))
		req.Host = "agent.example.com"
		req.Header.Set("Origin", "https://agent.example.com")
		req.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: ownerSession})
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		return rec
	}

	disabled := updateStatus("disabled")
	if disabled.Code != http.StatusOK || !strings.Contains(disabled.Body.String(), `"status":"disabled"`) {
		t.Fatalf("disable status=%d body=%s", disabled.Code, disabled.Body.String())
	}
	user, err := server.store.User(server.cfg.AgentID, "43")
	if err != nil || user.Status != "disabled" {
		t.Fatalf("local Agent gate was not disabled: user=%+v err=%v", user, err)
	}
	modelReq := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	modelReq.Header.Set("Authorization", "Bearer "+agentKey)
	modelResp := httptest.NewRecorder()
	server.ServeHTTP(modelResp, modelReq)
	if modelResp.Code != http.StatusForbidden || !strings.Contains(modelResp.Body.String(), "AGENT_USER_DISABLED") {
		t.Fatalf("disabled user model access status=%d body=%s", modelResp.Code, modelResp.Body.String())
	}
	sessionModelReq := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	sessionModelReq.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: userSession})
	sessionModelResp := httptest.NewRecorder()
	server.ServeHTTP(sessionModelResp, sessionModelReq)
	if sessionModelResp.Code != http.StatusUnauthorized {
		t.Fatalf("disabled user's existing session remained usable: status=%d body=%s", sessionModelResp.Code, sessionModelResp.Body.String())
	}

	enabled := updateStatus("active")
	if enabled.Code != http.StatusOK || !strings.Contains(enabled.Body.String(), `"status":"active"`) {
		t.Fatalf("enable status=%d body=%s", enabled.Code, enabled.Body.String())
	}
	user, err = server.store.User(server.cfg.AgentID, "43")
	if err != nil || user.Status != "active" {
		t.Fatalf("local Agent gate was not re-enabled: user=%+v err=%v", user, err)
	}
	modelReq = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	modelReq.Header.Set("Authorization", "Bearer "+agentKey)
	modelResp = httptest.NewRecorder()
	server.ServeHTTP(modelResp, modelReq)
	if modelResp.Code != http.StatusOK || strings.Contains(modelResp.Body.String(), testRuntimeControlCredential) {
		t.Fatalf("enabled user model discovery leaked runtime credential: status=%d body=%s", modelResp.Code, modelResp.Body.String())
	}
	sessionModelReq = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	sessionModelReq.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: userSession})
	sessionModelResp = httptest.NewRecorder()
	server.ServeHTTP(sessionModelResp, sessionModelReq)
	if sessionModelResp.Code != http.StatusOK {
		t.Fatalf("enabled user's existing session did not recover: status=%d body=%s", sessionModelResp.Code, sessionModelResp.Body.String())
	}
	if statusUpdates != 2 || modelRequests != 0 {
		t.Fatalf("status updates/model calls = %d/%d, want 2/0", statusUpdates, modelRequests)
	}
}

func TestAgentAdminMappedUserDisableRemainsLocallyBlockedWhenMainUpdateFails(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agent-runtime/users/43/status" || r.Method != http.MethodPatch {
			t.Errorf("unexpected main API call: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 403, "message": "not allowed"})
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	if _, err := server.store.UpsertUser(server.cfg.AgentID, "43", "mapped@example.com", "Mapped"); err != nil {
		t.Fatal(err)
	}
	ownerSession, err := server.store.CreateSession("42", []byte(`{"id":"42"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/agent/admin/users/43/status", strings.NewReader(`{"status":"disabled"}`))
	req.Host = "agent.example.com"
	req.Header.Set("Origin", "https://agent.example.com")
	req.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: ownerSession})
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "MAIN_USER_STATUS_UPDATE_FAILED") {
		t.Fatalf("failed main update status=%d body=%s", rec.Code, rec.Body.String())
	}
	user, err := server.store.User(server.cfg.AgentID, "43")
	if err != nil || user.Status != "disabled" {
		t.Fatalf("failed main update did not retain fail-closed local state: user=%+v err=%v", user, err)
	}
}

func TestAgentAdminMappedUserStatusRejectsUnauthorizedTargetsAndRequests(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("invalid user status operation must not call Sub2API: %s %s", r.Method, r.URL.Path)
		http.Error(w, "unexpected upstream call", http.StatusInternalServerError)
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

	tests := []struct {
		name, target, status, session, origin, forwardedHost, wantError string
		wantCode                                                        int
	}{
		{name: "non-owner", target: "43", status: "disabled", session: memberSession, origin: "https://agent.example.com", wantCode: http.StatusForbidden, wantError: "AGENT_ADMIN_REQUIRED"},
		{name: "cross-origin with forged forwarded host", target: "43", status: "disabled", session: ownerSession, origin: "https://attacker.example", forwardedHost: "attacker.example", wantCode: http.StatusForbidden, wantError: "CSRF_ORIGIN_REJECTED"},
		{name: "owner-protected", target: "42", status: "disabled", session: ownerSession, origin: "https://agent.example.com", wantCode: http.StatusConflict, wantError: "OWNER_STATUS_PROTECTED"},
		{name: "unmapped-user", target: "404", status: "disabled", session: ownerSession, origin: "https://agent.example.com", wantCode: http.StatusNotFound, wantError: "AGENT_USER_NOT_FOUND"},
		{name: "unsupported-status", target: "43", status: "suspended", session: ownerSession, origin: "https://agent.example.com", wantCode: http.StatusBadRequest, wantError: "INVALID_USER_STATUS"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPatch, "/api/v1/agent/admin/users/"+test.target+"/status", strings.NewReader(`{"status":"`+test.status+`"}`))
			req.Host = "agent.example.com"
			req.Header.Set("Origin", test.origin)
			if test.forwardedHost != "" {
				req.Header.Set("X-Forwarded-Host", test.forwardedHost)
				req.Header.Set("Forwarded", "for=203.0.113.10;host="+test.forwardedHost+";proto=https")
			}
			req.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: test.session})
			rec := httptest.NewRecorder()
			server.ServeHTTP(rec, req)
			if rec.Code != test.wantCode || !strings.Contains(rec.Body.String(), test.wantError) {
				t.Fatalf("status update status=%d body=%s, want %d %s", rec.Code, rec.Body.String(), test.wantCode, test.wantError)
			}
		})
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
	unknown := httptest.NewRequest(http.MethodGet, "/api/v1/settings/public", nil)
	unknown.Host = "other.example.com"
	unknown.Header.Set("X-Forwarded-Host", "agent.example.com")
	unknown.Header.Set("Forwarded", "host=agent.example.com;proto=https")
	unknownResponse := httptest.NewRecorder()
	server.ServeHTTP(unknownResponse, unknown)
	if unknownResponse.Code != http.StatusMisdirectedRequest || !strings.Contains(unknownResponse.Body.String(), "HOST_NOT_ALLOWED") {
		t.Fatalf("unknown host status=%d body=%s", unknownResponse.Code, unknownResponse.Body.String())
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
