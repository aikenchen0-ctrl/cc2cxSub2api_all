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

func TestAgentBalanceNotifyUsesOnlyCurrentUserAndFiltersProfile(t *testing.T) {
	var operationCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/settings/public" {
			if r.Header.Get("Authorization") != "Bearer private-main-user-token" {
				t.Errorf("balance notification request did not use current user token: %v", r.Header)
			}
		}
		for _, forbidden := range []string{"X-API-Key", "X-Sub2API-Satellite", "X-AgentAPI-Runtime-Control", "Cookie"} {
			if r.Header.Get(forbidden) != "" {
				t.Errorf("unexpected credential %s forwarded upstream", forbidden)
			}
		}
		switch {
		case r.URL.Path == "/api/v1/auth/me":
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 42, "email": "user@example.com"}))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/user/profile":
			operationCalls.Add(1)
			_ = json.NewEncoder(w).Encode(envelope(balanceNotifyProfile(false, 3.5, []map[string]any{{"email": "alerts@example.com", "disabled": false, "verified": true}})))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/settings/public":
			operationCalls.Add(1)
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"balance_low_notify_enabled": true, "balance_low_notify_threshold": 5.0, "smtp_password": "must-not-leak"}))
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/user":
			operationCalls.Add(1)
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if len(payload) != 1 || payload["balance_notify_enabled"] != true {
				t.Errorf("unexpected notification settings payload: %#v", payload)
			}
			_ = json.NewEncoder(w).Encode(envelope(balanceNotifyProfile(true, 3.5, []map[string]any{{"email": "alerts@example.com", "disabled": false, "verified": true}})))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/user/notify-email/send-code":
			operationCalls.Add(1)
			if r.Header.Get("Accept-Language") != "zh-CN" {
				t.Errorf("locale was not forwarded safely: %q", r.Header.Get("Accept-Language"))
			}
			assertJSONFields(t, r, map[string]any{"email": "new@example.com"})
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"success": true, "verification_secret": "must-not-leak"}))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/user/notify-email/verify":
			operationCalls.Add(1)
			assertJSONFields(t, r, map[string]any{"email": "new@example.com", "code": "123456"})
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"success": true, "user": map[string]any{"role": "admin"}}))
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/user/notify-email/toggle":
			operationCalls.Add(1)
			assertJSONFields(t, r, map[string]any{"email": "alerts@example.com", "disabled": true})
			_ = json.NewEncoder(w).Encode(envelope(balanceNotifyProfile(true, 3.5, []map[string]any{{"email": "alerts@example.com", "disabled": true, "verified": true}})))
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/user/notify-email":
			operationCalls.Add(1)
			assertJSONFields(t, r, map[string]any{"email": "alerts@example.com"})
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"success": true, "smtp_password": "must-not-leak"}))
		default:
			t.Errorf("unexpected upstream request: %s %s", r.Method, r.URL.Path)
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

	requests := []struct{ method, path, body string }{
		{http.MethodGet, "/api/v1/agent/balance-notify", ""},
		{http.MethodPut, "/api/v1/agent/balance-notify", `{"enabled":true,"role":"admin"}`},
		{http.MethodPost, "/api/v1/agent/balance-notify/send-code", `{"email":"NEW@example.com","smtp_password":"forged"}`},
		{http.MethodPost, "/api/v1/agent/balance-notify/verify", `{"email":"new@example.com","code":"123456","role":"admin"}`},
		{http.MethodPut, "/api/v1/agent/balance-notify/toggle", `{"email":"alerts@example.com","disabled":true,"user_id":99}`},
		{http.MethodDelete, "/api/v1/agent/balance-notify/email", `{"email":"alerts@example.com","admin":true}`},
	}
	for _, test := range requests {
		req := httptest.NewRequest(test.method, "http://agent.local"+test.path, strings.NewReader(test.body))
		req.AddCookie(cookie)
		if test.method != http.MethodGet {
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Origin", "http://agent.local")
		}
		if strings.HasSuffix(test.path, "/send-code") {
			req.Header.Set("Accept-Language", "zh-CN")
		}
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s %s status=%d body=%s", test.method, test.path, rec.Code, rec.Body.String())
		}
		if rec.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("response is cacheable: %v", rec.Header())
		}
		for _, forbidden := range []string{"must-not-leak", "smtp_password", `"role"`, `"user"`, "verification_secret"} {
			if strings.Contains(rec.Body.String(), forbidden) {
				t.Fatalf("field %q leaked from %s: %s", forbidden, test.path, rec.Body.String())
			}
		}
	}
	if operationCalls.Load() != 9 {
		t.Fatalf("unexpected main operation count: %d", operationCalls.Load())
	}

	before := operationCalls.Load()
	crossOrigin := httptest.NewRequest(http.MethodPut, "http://agent.local/api/v1/agent/balance-notify", strings.NewReader(`{"enabled":false}`))
	crossOrigin.Header.Set("Content-Type", "application/json")
	crossOrigin.Header.Set("Origin", "https://attacker.example")
	crossOrigin.AddCookie(cookie)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, crossOrigin)
	if rec.Code != http.StatusForbidden || operationCalls.Load() != before {
		t.Fatalf("cross-origin update reached main: status=%d calls=%d", rec.Code, operationCalls.Load())
	}
}

func TestAgentBalanceNotifyRejectsSSOOnlyAndUnsafeMainData(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		switch r.URL.Path {
		case "/api/v1/auth/me":
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 42}))
		case "/api/v1/user/profile":
			_ = json.NewEncoder(w).Encode(envelope(balanceNotifyProfile(true, 1, []map[string]any{{"email": "bad\r\n@example.com", "verified": true}})))
		case "/api/v1/settings/public":
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"balance_low_notify_enabled": true, "balance_low_notify_threshold": 5}))
		}
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	ssoID, err := server.store.CreateSession("42", []byte(`{"id":"42"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "http://agent.local/api/v1/agent/balance-notify", nil)
	req.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: ssoID})
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || calls.Load() != 0 {
		t.Fatalf("SSO-only session reached main: status=%d calls=%d", rec.Code, calls.Load())
	}

	passwordID, err := server.store.CreateSession("42", []byte(`{"id":"42"}`), "private-main-user-token", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodGet, "http://agent.local/api/v1/agent/balance-notify", nil)
	req.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: passwordID})
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway || strings.Contains(rec.Body.String(), "bad\\r\\n@example.com") || strings.Contains(rec.Body.String(), "bad@example.com") {
		t.Fatalf("unsafe main profile was not rejected: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func balanceNotifyProfile(enabled bool, threshold float64, emails []map[string]any) map[string]any {
	return map[string]any{
		"id": 42, "email": "user@example.com", "balance_notify_enabled": enabled,
		"balance_notify_threshold": threshold, "balance_notify_extra_emails": emails,
		"role": "admin", "balance": 999, "access_token": "must-not-leak",
	}
}

func assertJSONFields(t *testing.T, r *http.Request, want map[string]any) {
	t.Helper()
	var got map[string]any
	if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Errorf("unexpected payload fields: got=%#v want=%#v", got, want)
		return
	}
	for key, value := range want {
		if got[key] != value {
			t.Errorf("unexpected payload %s=%#v want %#v", key, got[key], value)
		}
	}
}
