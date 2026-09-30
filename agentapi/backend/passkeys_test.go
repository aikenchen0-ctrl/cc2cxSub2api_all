package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestAgentPasskeyConfigAndManagementStayUserScoped(t *testing.T) {
	var passkeyCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/settings/public":
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{
				"passkey_enabled": true, "passkey_configured": true,
				"passkey_rp_id": "example.test", "passkey_rp_origins": []string{"https://agent.example.test"},
			}))
		case "/api/v1/user/passkeys":
			passkeyCalls.Add(1)
			if r.Header.Get("Authorization") != "Bearer current-user-token" {
				t.Errorf("passkey list did not use current user token: %v", r.Header)
			}
			assertNoPasskeyPrivilegedHeaders(t, r)
			_ = json.NewEncoder(w).Encode(envelope([]map[string]any{{
				"id": 7, "name": "Laptop", "created_at": "2026-09-29T08:00:00Z", "backup": true,
				"credential": "must-not-leak", "user_id": 42,
			}}))
		case "/api/v1/user/passkeys/register/begin":
			passkeyCalls.Add(1)
			if r.Header.Get("Authorization") != "Bearer current-user-token" {
				t.Errorf("register begin did not use current user token")
			}
			assertNoPasskeyPrivilegedHeaders(t, r)
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{
				"session_token": "one-use-token", "options": map[string]any{"publicKey": map[string]any{"challenge": "AQID"}},
				"access_token": "must-not-leak",
			}))
		case "/api/v1/user/passkeys/7":
			passkeyCalls.Add(1)
			if r.Header.Get("Authorization") != "Bearer current-user-token" {
				t.Errorf("passkey mutation did not use current user token")
			}
			assertNoPasskeyPrivilegedHeaders(t, r)
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"success": true, "secret": "must-not-leak"}))
		default:
			t.Errorf("unexpected upstream passkey request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	sessionID, err := server.store.CreateSession("42", []byte(`{"id":"42","email":"u@example.com"}`), "current-user-token", "refresh-token", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	cookie := &http.Cookie{Name: server.cfg.CookieName, Value: sessionID}

	configRequest := httptest.NewRequest(http.MethodGet, "https://agent.example.test/api/v1/auth/passkey/config", nil)
	configResponse := httptest.NewRecorder()
	server.ServeHTTP(configResponse, configRequest)
	if configResponse.Code != http.StatusOK || !strings.Contains(configResponse.Body.String(), `"enabled":true`) || strings.Contains(configResponse.Body.String(), "passkey_rp_origins") {
		t.Fatalf("unexpected passkey config response: status=%d body=%s", configResponse.Code, configResponse.Body.String())
	}

	requests := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/api/v1/agent/passkeys", ""},
		{http.MethodPost, "/api/v1/agent/passkeys/register/begin", `{"password":"current-password","admin":true}`},
		{http.MethodPatch, "/api/v1/agent/passkeys/7", `{"name":"Security key","user_id":999}`},
		{http.MethodDelete, "/api/v1/agent/passkeys/7", `{"password":"current-password","access_token":"forged"}`},
	}
	for _, test := range requests {
		req := httptest.NewRequest(test.method, "https://agent.example.test"+test.path, strings.NewReader(test.body))
		req.AddCookie(cookie)
		if test.method != http.MethodGet {
			req.Header.Set("Origin", "https://agent.example.test")
			req.Header.Set("Content-Type", "application/json")
		}
		response := httptest.NewRecorder()
		server.ServeHTTP(response, req)
		if response.Code != http.StatusOK {
			t.Fatalf("%s %s status=%d body=%s", test.method, test.path, response.Code, response.Body.String())
		}
		if response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("passkey response is cacheable: %v", response.Header())
		}
		for _, forbidden := range []string{"must-not-leak", "current-user-token", "refresh-token", "forged", `"user_id"`} {
			if strings.Contains(response.Body.String(), forbidden) {
				t.Fatalf("sensitive field leaked from %s: %s", test.path, response.Body.String())
			}
		}
	}
	if passkeyCalls.Load() != 4 {
		t.Fatalf("unexpected passkey operation count: %d", passkeyCalls.Load())
	}

	wrongOrigin := httptest.NewRequest(http.MethodGet, "https://other.example.test/api/v1/auth/passkey/config", nil)
	wrongOriginResponse := httptest.NewRecorder()
	server.ServeHTTP(wrongOriginResponse, wrongOrigin)
	if wrongOriginResponse.Code != http.StatusOK || !strings.Contains(wrongOriginResponse.Body.String(), `"enabled":false`) {
		t.Fatalf("unlisted origin was enabled: %s", wrongOriginResponse.Body.String())
	}

	before := passkeyCalls.Load()
	crossOrigin := httptest.NewRequest(http.MethodDelete, "https://agent.example.test/api/v1/agent/passkeys/7", strings.NewReader(`{"password":"current-password"}`))
	crossOrigin.Header.Set("Origin", "https://attacker.example")
	crossOrigin.AddCookie(cookie)
	crossOriginResponse := httptest.NewRecorder()
	server.ServeHTTP(crossOriginResponse, crossOrigin)
	if crossOriginResponse.Code != http.StatusForbidden || passkeyCalls.Load() != before {
		t.Fatalf("cross-origin passkey mutation reached upstream: status=%d body=%s", crossOriginResponse.Code, crossOriginResponse.Body.String())
	}
}

func TestAgentPasskeyLoginCreatesOnlyAgentSessionAndRejectsSSOSessionManagement(t *testing.T) {
	var finishCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/settings/public":
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{
				"passkey_enabled": true, "passkey_configured": true,
				"passkey_rp_id": "example.test", "passkey_rp_origins": []string{"https://agent.example.test"},
			}))
		case "/api/v1/auth/passkey/login/begin":
			if r.Header.Get("Authorization") != "" {
				t.Errorf("passkey login begin unexpectedly used authorization")
			}
			assertNoPasskeyPrivilegedHeaders(t, r)
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{
				"session_token": "login-token", "options": map[string]any{"publicKey": map[string]any{"challenge": "AQID"}},
			}))
		case "/api/v1/auth/passkey/login/finish":
			finishCalls.Add(1)
			if r.Header.Get("Authorization") != "" {
				t.Errorf("passkey login finish unexpectedly used authorization")
			}
			assertNoPasskeyPrivilegedHeaders(t, r)
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{
				"access_token": "private-main-access", "refresh_token": "private-main-refresh", "expires_in": 3600,
				"user": map[string]any{"id": 42, "email": "u@example.com", "role": "admin", "password_hash": "must-not-leak"},
			}))
		default:
			t.Errorf("unexpected upstream path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	server := testServer(t, upstream)

	begin := httptest.NewRequest(http.MethodPost, "https://agent.example.test/api/v1/auth/passkey/login/begin", strings.NewReader(`{"turnstile_token":"proof","admin":true}`))
	begin.Header.Set("Origin", "https://agent.example.test")
	begin.Header.Set("Content-Type", "application/json")
	beginResponse := httptest.NewRecorder()
	server.ServeHTTP(beginResponse, begin)
	if beginResponse.Code != http.StatusOK || strings.Contains(beginResponse.Body.String(), "admin") {
		t.Fatalf("unexpected passkey begin response: status=%d body=%s", beginResponse.Code, beginResponse.Body.String())
	}

	credential := `{"id":"credential-id","rawId":"Y3JlZGVudGlhbC1pZA","type":"public-key","response":{"authenticatorData":"AQ","clientDataJSON":"Ag","signature":"Aw","userHandle":"NA"}}`
	finish := httptest.NewRequest(http.MethodPost, "https://agent.example.test/api/v1/auth/passkey/login/finish", strings.NewReader(`{"session_token":"login-token","credential":`+credential+`}`))
	finish.Header.Set("Origin", "https://agent.example.test")
	finish.Header.Set("Content-Type", "application/json")
	finishResponse := httptest.NewRecorder()
	server.ServeHTTP(finishResponse, finish)
	if finishResponse.Code != http.StatusOK {
		t.Fatalf("passkey finish status=%d body=%s", finishResponse.Code, finishResponse.Body.String())
	}
	for _, forbidden := range []string{"private-main-access", "private-main-refresh", "password_hash", "must-not-leak"} {
		if strings.Contains(finishResponse.Body.String(), forbidden) {
			t.Fatalf("main credential leaked in browser response: %s", finishResponse.Body.String())
		}
	}
	if finishCalls.Load() != 1 || len(finishResponse.Result().Cookies()) == 0 || !finishResponse.Result().Cookies()[0].HttpOnly {
		t.Fatalf("passkey login did not establish HttpOnly Agent session: headers=%v", finishResponse.Header())
	}

	ssoSessionID, err := server.store.CreateSession("42", []byte(`{"id":"42"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	list := httptest.NewRequest(http.MethodGet, "https://agent.example.test/api/v1/agent/passkeys", nil)
	list.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: ssoSessionID})
	listResponse := httptest.NewRecorder()
	server.ServeHTTP(listResponse, list)
	if listResponse.Code != http.StatusForbidden || !strings.Contains(listResponse.Body.String(), "MAIN_USER_TOKEN_REQUIRED") {
		t.Fatalf("SSO-only session could manage passkeys: status=%d body=%s", listResponse.Code, listResponse.Body.String())
	}

	oversized := httptest.NewRequest(http.MethodPost, "https://agent.example.test/api/v1/auth/passkey/login/finish", strings.NewReader(`{"session_token":"x","credential":{"id":"`+strings.Repeat("a", maxPasskeyBody)+`"}}`))
	oversized.Header.Set("Origin", "https://agent.example.test")
	oversized.Header.Set("Content-Type", "application/json")
	oversizedResponse := httptest.NewRecorder()
	server.ServeHTTP(oversizedResponse, oversized)
	if oversizedResponse.Code != http.StatusBadRequest || finishCalls.Load() != 1 {
		t.Fatalf("oversized credential reached upstream: status=%d calls=%d", oversizedResponse.Code, finishCalls.Load())
	}
}

func assertNoPasskeyPrivilegedHeaders(t *testing.T, r *http.Request) {
	t.Helper()
	for _, header := range []string{"X-API-Key", "X-Sub2API-Satellite", "X-Sub2API-On-Behalf-Of", "X-AgentAPI-Runtime-Control", "Cookie"} {
		if value := r.Header.Get(header); value != "" {
			t.Errorf("unexpected privileged header %s=%q", header, value)
		}
	}
}
