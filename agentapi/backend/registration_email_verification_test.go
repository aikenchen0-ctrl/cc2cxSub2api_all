package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmailVerifiedRegistrationVerifiesBeforeCreatingMainUser(t *testing.T) {
	var verificationCalls, creationCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/satellite/agentapi/verify-registration-email":
			verificationCalls++
			if r.Header.Get("Authorization") != "Bearer app-secret" || r.Header.Get("X-Sub2API-Satellite") != "agentapi" {
				t.Errorf("verification did not use the application credential: %v", r.Header)
			}
			var payload map[string]any
			_ = json.NewDecoder(r.Body).Decode(&payload)
			if payload["email"] != "new@example.com" || payload["verify_code"] != "123456" {
				t.Errorf("unexpected verification payload: %#v", payload)
			}
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"message": "verified"}))
		case "/api/v1/admin/users":
			creationCalls++
			if verificationCalls != 1 {
				t.Error("main user was created before email verification")
			}
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 43, "email": "new@example.com", "username": "New User", "role": "user"}))
		case "/api/v1/auth/login":
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{
				"access_token": "private-user-token", "refresh_token": "private-refresh-token",
				"user": map[string]any{"id": 43, "email": "new@example.com", "username": "New User"},
			}))
		default:
			t.Errorf("unexpected upstream request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	server := testServer(t, upstream)
	server.cfg.EmailVerifyEnabled = true
	server.main = NewMainClient(server.cfg)
	req := httptest.NewRequest(http.MethodPost, "http://agent.local/api/v1/auth/register", strings.NewReader(`{"email":"new@example.com","password":"password123","username":"New User","verify_code":"123456"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://agent.local")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("registration status=%d body=%s", rec.Code, rec.Body.String())
	}
	if verificationCalls != 1 || creationCalls != 1 {
		t.Fatalf("unexpected upstream calls: verify=%d create=%d", verificationCalls, creationCalls)
	}
	if strings.Contains(rec.Body.String(), "private-user-token") || strings.Contains(rec.Body.String(), "private-refresh-token") {
		t.Fatalf("main credentials leaked to the browser: %s", rec.Body.String())
	}
}

func TestEmailVerifiedRegistrationRejectsMissingOrInvalidCodeBeforeUserCreation(t *testing.T) {
	for _, tc := range []struct {
		name       string
		body       string
		wantStatus int
	}{
		{name: "missing", body: `{"email":"new@example.com","password":"password123"}`, wantStatus: http.StatusBadRequest},
		{name: "invalid", body: `{"email":"new@example.com","password":"password123","verify_code":"000000"}`, wantStatus: http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var creationCalls int
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v1/admin/users" {
					creationCalls++
				}
				if r.URL.Path == "/api/v1/auth/satellite/agentapi/verify-registration-email" {
					w.WriteHeader(http.StatusBadRequest)
					_ = json.NewEncoder(w).Encode(map[string]any{"code": "INVALID_VERIFY_CODE", "message": "invalid code"})
					return
				}
				http.NotFound(w, r)
			}))
			defer upstream.Close()
			server := testServer(t, upstream)
			server.cfg.EmailVerifyEnabled = true
			server.main = NewMainClient(server.cfg)
			req := httptest.NewRequest(http.MethodPost, "http://agent.local/api/v1/auth/register", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Origin", "http://agent.local")
			rec := httptest.NewRecorder()
			server.ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus || creationCalls != 0 {
				t.Fatalf("status=%d creates=%d body=%s", rec.Code, creationCalls, rec.Body.String())
			}
		})
	}
}

func TestSendRegistrationVerifyCodeIsSameOriginAndNeverReturnsCredentials(t *testing.T) {
	var upstreamCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		if r.URL.Path != "/api/v1/auth/satellite/agentapi/send-verify-code" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(envelope(map[string]any{"countdown": 45, "app_credential": "must-not-leak"}))
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	server.cfg.EmailVerifyEnabled = true
	server.main = NewMainClient(server.cfg)

	crossOrigin := httptest.NewRequest(http.MethodPost, "http://agent.local/api/v1/auth/send-verify-code", strings.NewReader(`{"email":"new@example.com"}`))
	crossOrigin.Header.Set("Origin", "https://attacker.example")
	crossOriginResponse := httptest.NewRecorder()
	server.ServeHTTP(crossOriginResponse, crossOrigin)
	if crossOriginResponse.Code != http.StatusMethodNotAllowed || upstreamCalls != 0 {
		t.Fatalf("cross-origin send was not rejected: status=%d calls=%d", crossOriginResponse.Code, upstreamCalls)
	}

	req := httptest.NewRequest(http.MethodPost, "http://agent.local/api/v1/auth/send-verify-code", strings.NewReader(`{"email":"new@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://agent.local")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || upstreamCalls != 1 || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("send status=%d calls=%d headers=%v body=%s", rec.Code, upstreamCalls, rec.Header(), rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "must-not-leak") || strings.Contains(rec.Body.String(), "app-secret") {
		t.Fatalf("upstream credential-like data leaked: %s", rec.Body.String())
	}
}
