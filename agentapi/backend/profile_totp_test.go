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

func TestAgentTOTPUsesAuthenticatedUserSessionAndFiltersResponses(t *testing.T) {
	var operationCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer private-main-user-token" {
			t.Errorf("TOTP request did not use the current user's main-site token: %v", r.Header)
		}
		for _, forbidden := range []string{"X-API-Key", "X-Sub2API-Satellite", "X-AgentAPI-Runtime-Control", "Cookie"} {
			if r.Header.Get(forbidden) != "" {
				t.Errorf("unexpected credential %s forwarded to user TOTP endpoint", forbidden)
			}
		}
		switch r.URL.Path {
		case "/api/v1/auth/me":
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 42, "email": "user@example.com"}))
		case "/api/v1/user/totp/status":
			operationCalls.Add(1)
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{
				"enabled": false, "feature_enabled": true, "admin": true, "access_token": "must-not-leak",
			}))
		case "/api/v1/user/totp/verification-method":
			operationCalls.Add(1)
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"method": "password", "role": "admin"}))
		case "/api/v1/user/totp/send-code":
			operationCalls.Add(1)
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"success": true, "email": "hidden@example.com"}))
		case "/api/v1/user/totp/setup":
			operationCalls.Add(1)
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if len(payload) != 2 || payload["password"] != "current-password" || payload["email_code"] != "" {
				t.Errorf("unexpected setup payload: %#v", payload)
			}
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{
				"secret": "JBSWY3DPEHPK3PXP", "qr_code_url": "otpauth://totp/Agent:user@example.com?secret=JBSWY3DPEHPK3PXP", "setup_token": "one-time-setup-token", "countdown": 300,
				"access_token": "must-not-leak", "user": map[string]any{"role": "admin"},
			}))
		case "/api/v1/user/totp/enable":
			operationCalls.Add(1)
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if len(payload) != 2 || payload["totp_code"] != "123456" || payload["setup_token"] != "one-time-setup-token" {
				t.Errorf("unexpected enable payload: %#v", payload)
			}
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"success": true, "admin": true}))
		case "/api/v1/user/totp/disable":
			operationCalls.Add(1)
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"success": true, "secret": "must-not-leak"}))
		default:
			t.Errorf("unexpected upstream TOTP request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	sessionID, err := server.store.CreateSession("42", []byte(`{"id":"42","email":"user@example.com"}`), "private-main-user-token", "refresh-token", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	cookie := &http.Cookie{Name: server.cfg.CookieName, Value: sessionID}

	requests := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/api/v1/agent/totp/status", ""},
		{http.MethodGet, "/api/v1/agent/totp/verification-method", ""},
		{http.MethodPost, "/api/v1/agent/totp/send-code", `{}`},
		{http.MethodPost, "/api/v1/agent/totp/setup", `{"password":"current-password","admin":true}`},
		{http.MethodPost, "/api/v1/agent/totp/enable", `{"totp_code":"123456","setup_token":"one-time-setup-token","role":"admin"}`},
		{http.MethodPost, "/api/v1/agent/totp/disable", `{"password":"current-password","access_token":"forged"}`},
	}
	for _, test := range requests {
		req := httptest.NewRequest(test.method, "http://agent.local"+test.path, strings.NewReader(test.body))
		req.AddCookie(cookie)
		if test.method == http.MethodPost {
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Origin", "http://agent.local")
		}
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s %s status=%d body=%s", test.method, test.path, rec.Code, rec.Body.String())
		}
		if rec.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("TOTP response is cacheable: %v", rec.Header())
		}
		for _, forbidden := range []string{"must-not-leak", `"admin"`, `"role"`, `"user"`, `"access_token"`} {
			if strings.Contains(rec.Body.String(), forbidden) {
				t.Fatalf("upstream field %q leaked from %s: %s", forbidden, test.path, rec.Body.String())
			}
		}
	}
	if operationCalls.Load() != int32(len(requests)) {
		t.Fatalf("unexpected TOTP operation call count: %d", operationCalls.Load())
	}

	before := operationCalls.Load()
	crossOrigin := httptest.NewRequest(http.MethodPost, "http://agent.local/api/v1/agent/totp/disable", strings.NewReader(`{"password":"current-password"}`))
	crossOrigin.Header.Set("Content-Type", "application/json")
	crossOrigin.Header.Set("Origin", "https://attacker.example")
	crossOrigin.AddCookie(cookie)
	crossOriginResponse := httptest.NewRecorder()
	server.ServeHTTP(crossOriginResponse, crossOrigin)
	if crossOriginResponse.Code != http.StatusForbidden || operationCalls.Load() != before {
		t.Fatalf("cross-origin TOTP write reached upstream: status=%d calls=%d body=%s", crossOriginResponse.Code, operationCalls.Load(), crossOriginResponse.Body.String())
	}
}

func TestAgentTOTPRejectsSSOOnlySessionAndInvalidSetupResponse(t *testing.T) {
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		if r.URL.Path == "/api/v1/auth/me" {
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 42}))
			return
		}
		_ = json.NewEncoder(w).Encode(envelope(map[string]any{
			"secret": "SECRET", "qr_code_url": "https://evil.example/collect", "setup_token": "token", "countdown": 300,
		}))
	}))
	defer upstream.Close()
	server := testServer(t, upstream)

	ssoSessionID, err := server.store.CreateSession("42", []byte(`{"id":"42"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	ssoRequest := httptest.NewRequest(http.MethodGet, "http://agent.local/api/v1/agent/totp/status", nil)
	ssoRequest.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: ssoSessionID})
	ssoResponse := httptest.NewRecorder()
	server.ServeHTTP(ssoResponse, ssoRequest)
	if ssoResponse.Code != http.StatusForbidden || upstreamCalls.Load() != 0 {
		t.Fatalf("SSO-only session reached main TOTP endpoint: status=%d calls=%d body=%s", ssoResponse.Code, upstreamCalls.Load(), ssoResponse.Body.String())
	}

	passwordSessionID, err := server.store.CreateSession("42", []byte(`{"id":"42"}`), "private-main-user-token", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "http://agent.local/api/v1/agent/totp/setup", strings.NewReader(`{"password":"current-password"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://agent.local")
	request.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: passwordSessionID})
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusBadGateway || strings.Contains(response.Body.String(), "evil.example") || strings.Contains(response.Body.String(), "SECRET") {
		t.Fatalf("unsafe setup response was not rejected: status=%d body=%s", response.Code, response.Body.String())
	}
}
