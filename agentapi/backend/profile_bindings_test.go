package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAgentProfileBindingsUseCurrentUserAndFilterUpstreamFields(t *testing.T) {
	const token = "bindings-user-token"
	var profileCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Errorf("identity binding request did not use current user token: %v", r.Header)
		}
		for _, forbidden := range []string{"X-API-Key", "X-Sub2API-Satellite", "X-AgentAPI-Runtime-Control", "Cookie"} {
			if r.Header.Get(forbidden) != "" {
				t.Errorf("credential %s leaked to identity binding endpoint", forbidden)
			}
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/auth/me":
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 42, "email": "user@example.com"}))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/user/profile":
			profileCalls++
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{
				"id": 42, "email": "user@example.com", "access_token": "must-not-leak",
				"auth_bindings": map[string]any{
					"email": map[string]any{"bound": true, "bound_count": 1, "can_bind": false, "can_unbind": false},
					"linuxdo": map[string]any{
						"bound": true, "bound_count": 2, "can_bind": false, "can_unbind": true,
						"display_name": "Linux User", "subject_hint": "lin***42", "note": "You can unbind this sign-in method.",
						"provider_subject": "secret-subject", "bind_start_path": "/api/v1/auth/linuxdo/bind",
					},
					"oidc": map[string]any{"bound": false, "can_bind": true, "bind_start_path": "/api/v1/auth/oidc/bind"},
				},
			}))
		default:
			t.Errorf("unexpected Sub2API request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	sessionID, err := server.store.CreateSession("42", []byte(`{"id":42}`), token, "refresh-token", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "http://agent.local/api/v1/agent/profile/bindings", nil)
	request.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK || profileCalls != 1 {
		t.Fatalf("bindings load failed: status=%d calls=%d body=%s", response.Code, profileCalls, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("bindings response is cacheable: %v", response.Header())
	}
	body := response.Body.String()
	for _, forbidden := range []string{"must-not-leak", "secret-subject", "bind_start_path", "/api/v1/auth/oidc/bind"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("upstream field %q leaked to browser: %s", forbidden, body)
		}
	}
	for _, expected := range []string{`"provider":"email"`, `"provider":"linuxdo"`, `"provider":"oidc"`, `"provider":"wechat"`, `"provider":"dingtalk"`, `"display_name":"Linux User"`, `"oauth_binding_supported":true`} {
		if !strings.Contains(body, expected) {
			t.Fatalf("safe binding field %q missing: %s", expected, body)
		}
	}
}

func TestAgentProfileOAuthBindingStartUsesSatelliteOBOAndReturnsPublicURL(t *testing.T) {
	var startCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/auth/me":
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 42, "email": "user@example.com"}))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sub2api/auth-identities/bind/start":
			startCalls++
			if r.Header.Get("Authorization") != "Bearer app-secret" || r.Header.Get("X-Sub2API-On-Behalf-Of") != "42" || r.Header.Get("X-Sub2API-Satellite") != "agentapi" {
				t.Errorf("unexpected satellite binding headers: %v", r.Header)
			}
			for _, forbidden := range []string{"X-API-Key", "X-AgentAPI-Runtime-Control", "Cookie"} {
				if r.Header.Get(forbidden) != "" {
					t.Errorf("credential %s leaked to binding start", forbidden)
				}
			}
			var payload map[string]string
			_ = json.NewDecoder(r.Body).Decode(&payload)
			if len(payload) != 1 || payload["provider"] != "oidc" {
				t.Errorf("unexpected binding payload: %#v", payload)
			}
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{
				"provider": "oidc", "authorize_url": "/api/v1/auth/oauth/oidc/bind/start?satellite_handoff=opaque", "method": "GET",
			}))
		default:
			t.Errorf("unexpected Sub2API request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	server.cfg.AppCredential = "app-secret"
	server.main.cfg.AppCredential = "app-secret"
	server.cfg.PublicMainURL = "https://main.example"
	sessionID, err := server.store.CreateSession("42", []byte(`{"id":42}`), "main-user-token", "refresh-token", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "http://agent.local/api/v1/agent/profile/bindings/oidc/start", strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://agent.local")
	request.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK || startCalls != 1 {
		t.Fatalf("binding start failed: status=%d calls=%d body=%s", response.Code, startCalls, response.Body.String())
	}
	body := response.Body.String()
	if !strings.Contains(body, `"authorize_url":"https://main.example/api/v1/auth/oauth/oidc/bind/start?satellite_handoff=opaque"`) || strings.Contains(body, "main-user-token") || strings.Contains(body, "app-secret") {
		t.Fatalf("unsafe or invalid binding response: %s", body)
	}
}

func TestAgentProfileEmailBindingForwardsOnlyExpectedPayload(t *testing.T) {
	const token = "email-binding-token"
	var sendCodeCalls, bindCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Errorf("email binding request did not use current user token: %v", r.Header)
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/auth/me":
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 42, "email": "old@example.com"}))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/user/account-bindings/email/send-code":
			sendCodeCalls++
			var payload map[string]any
			_ = json.NewDecoder(r.Body).Decode(&payload)
			if len(payload) != 1 || payload["email"] != "new@example.com" {
				t.Errorf("unexpected send-code payload: %#v", payload)
			}
			if r.Header.Get("Accept-Language") != "zh-CN" {
				t.Errorf("locale was not forwarded: %q", r.Header.Get("Accept-Language"))
			}
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"success": true}))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/user/account-bindings/email":
			bindCalls++
			var payload map[string]any
			_ = json.NewDecoder(r.Body).Decode(&payload)
			if len(payload) != 3 || payload["email"] != "new@example.com" || payload["verify_code"] != "123456" || payload["password"] != "current-password" {
				t.Errorf("unexpected bind payload: %#v", payload)
			}
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{
				"id": 42, "email": "new@example.com", "username": "User", "role": "admin", "balance": 999,
				"auth_bindings": map[string]any{"email": map[string]any{"bound": true, "bound_count": 1}},
			}))
		default:
			t.Errorf("unexpected Sub2API request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	sessionID, err := server.store.CreateSession("42", []byte(`{"id":42}`), token, "refresh-token", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	cookie := &http.Cookie{Name: server.cfg.CookieName, Value: sessionID}

	sendRequest := httptest.NewRequest(http.MethodPost, "http://agent.local/api/v1/agent/profile/bindings/email/send-code", strings.NewReader(`{"email":"new@example.com","admin_key":"forbidden"}`))
	sendRequest.Header.Set("Content-Type", "application/json")
	sendRequest.Header.Set("Origin", "http://agent.local")
	sendRequest.Header.Set("Accept-Language", "zh-CN")
	sendRequest.AddCookie(cookie)
	sendResponse := httptest.NewRecorder()
	server.ServeHTTP(sendResponse, sendRequest)
	if sendResponse.Code != http.StatusOK || sendCodeCalls != 1 {
		t.Fatalf("send-code failed: status=%d calls=%d body=%s", sendResponse.Code, sendCodeCalls, sendResponse.Body.String())
	}

	bindRequest := httptest.NewRequest(http.MethodPost, "http://agent.local/api/v1/agent/profile/bindings/email", strings.NewReader(`{"email":"new@example.com","verify_code":"123456","password":"current-password","role":"admin"}`))
	bindRequest.Header.Set("Content-Type", "application/json")
	bindRequest.Header.Set("Origin", "http://agent.local")
	bindRequest.AddCookie(cookie)
	bindResponse := httptest.NewRecorder()
	server.ServeHTTP(bindResponse, bindRequest)
	if bindResponse.Code != http.StatusOK || bindCalls != 1 {
		t.Fatalf("email bind failed: status=%d calls=%d body=%s", bindResponse.Code, bindCalls, bindResponse.Body.String())
	}
	for _, forbidden := range []string{`"role"`, `"balance"`, "current-password"} {
		if strings.Contains(bindResponse.Body.String(), forbidden) {
			t.Fatalf("sensitive field %q leaked in bind response: %s", forbidden, bindResponse.Body.String())
		}
	}
	if !strings.Contains(bindResponse.Body.String(), `"email":"new@example.com"`) || !strings.Contains(bindResponse.Body.String(), `"profile"`) {
		t.Fatalf("safe updated profile missing: %s", bindResponse.Body.String())
	}
}

func TestAgentProfileIdentityUnbindRevokesLocalSession(t *testing.T) {
	const token = "unbind-token"
	var unbindCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/auth/me":
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 42, "email": "user@example.com"}))
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/user/account-bindings/linuxdo":
			unbindCalls++
			if r.Header.Get("Authorization") != "Bearer "+token {
				t.Errorf("unbind request did not use current user token: %v", r.Header)
			}
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"success": true}))
		default:
			t.Errorf("unexpected Sub2API request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	sessionID, err := server.store.CreateSession("42", []byte(`{"id":42}`), token, "refresh-token", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodDelete, "http://agent.local/api/v1/agent/profile/bindings/linuxdo", nil)
	request.Header.Set("Origin", "http://agent.local")
	request.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK || unbindCalls != 1 || !strings.Contains(response.Body.String(), `"reauthenticate":true`) {
		t.Fatalf("identity unbind failed: status=%d calls=%d body=%s", response.Code, unbindCalls, response.Body.String())
	}
	if _, err := server.store.LoadSession(sessionID); err == nil {
		t.Fatal("local session remained valid after main identity unbind")
	}
	cookies := response.Result().Cookies()
	if len(cookies) == 0 || cookies[0].Name != server.cfg.CookieName || cookies[0].MaxAge != -1 {
		t.Fatalf("session cookie was not cleared: %#v", cookies)
	}
}

func TestAgentProfileBindingsRejectUnsafeRequestsBeforeUpstream(t *testing.T) {
	var upstreamCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		http.NotFound(w, r)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	editableSessionID, err := server.store.CreateSession("42", []byte(`{"id":42}`), "binding-token", "refresh-token", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	identityOnlySessionID, err := server.store.CreateSession("42", []byte(`{"id":42}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, method, path, origin, body, session string
		status                                    int
	}{
		{name: "cross origin", method: http.MethodPost, path: "/api/v1/agent/profile/bindings/email/send-code", origin: "https://attacker.example", body: `{"email":"new@example.com"}`, session: editableSessionID, status: http.StatusForbidden},
		{name: "invalid email", method: http.MethodPost, path: "/api/v1/agent/profile/bindings/email/send-code", origin: "http://agent.local", body: `{"email":"bad"}`, session: editableSessionID, status: http.StatusBadRequest},
		{name: "invalid code", method: http.MethodPost, path: "/api/v1/agent/profile/bindings/email", origin: "http://agent.local", body: `{"email":"new@example.com","verify_code":"123","password":"password"}`, session: editableSessionID, status: http.StatusBadRequest},
		{name: "email cannot unbind", method: http.MethodDelete, path: "/api/v1/agent/profile/bindings/email", origin: "http://agent.local", session: editableSessionID, status: http.StatusBadRequest},
		{name: "unknown provider", method: http.MethodDelete, path: "/api/v1/agent/profile/bindings/github", origin: "http://agent.local", session: editableSessionID, status: http.StatusBadRequest},
		{name: "identity-only session", method: http.MethodGet, path: "/api/v1/agent/profile/bindings", session: identityOnlySessionID, status: http.StatusForbidden},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, "http://agent.local"+test.path, strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			if test.origin != "" {
				request.Header.Set("Origin", test.origin)
			}
			request.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: test.session})
			response := httptest.NewRecorder()
			server.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status=%d want=%d body=%s", response.Code, test.status, response.Body.String())
			}
		})
	}
	if upstreamCalls != 0 {
		t.Fatalf("unsafe identity binding requests reached Sub2API %d time(s)", upstreamCalls)
	}
}
