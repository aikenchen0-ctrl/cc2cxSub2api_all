package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAgentProfileAvatarUsesCurrentUserEndpointAndFiltersResponse(t *testing.T) {
	const token = "avatar-user-token"
	avatar := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("small-avatar"))
	var updateCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Errorf("avatar request did not use the current user token: %v", r.Header)
		}
		for _, forbidden := range []string{"X-API-Key", "X-Sub2API-Satellite", "X-AgentAPI-Runtime-Control", "Cookie"} {
			if r.Header.Get(forbidden) != "" {
				t.Errorf("credential %s leaked to avatar endpoint", forbidden)
			}
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/auth/me":
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 42, "email": "avatar@example.com"}))
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/user":
			updateCalls++
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("decode avatar update: %v", err)
			}
			if len(payload) != 1 || payload["avatar_url"] != avatar {
				t.Errorf("avatar update sent unexpected fields: %#v", payload)
			}
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{
				"id": 42, "email": "avatar@example.com", "username": "Avatar User", "avatar_url": avatar,
				"role": "admin", "access_token": "must-not-leak", "balance": 999,
			}))
		default:
			t.Errorf("unexpected Sub2API request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	sessionID, err := server.store.CreateSession("42", []byte(`{"id":42,"email":"avatar@example.com"}`), token, "refresh-token", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPut, "http://agent.local/api/v1/agent/profile/avatar", strings.NewReader(`{"avatar_url":"`+avatar+`","role":"admin"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://agent.local")
	request.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK || updateCalls != 1 {
		t.Fatalf("avatar update failed: status=%d calls=%d body=%s", response.Code, updateCalls, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("avatar response is cacheable: %v", response.Header())
	}
	for _, forbidden := range []string{"must-not-leak", "\"role\"", "\"balance\""} {
		if strings.Contains(response.Body.String(), forbidden) {
			t.Fatalf("upstream field %q leaked to browser: %s", forbidden, response.Body.String())
		}
	}
	if !strings.Contains(response.Body.String(), avatar) || !strings.Contains(response.Body.String(), `"can_edit":true`) {
		t.Fatalf("safe avatar profile missing from response: %s", response.Body.String())
	}
}

func TestAgentProfileAvatarRejectsUnsafeInputsBeforeUpstream(t *testing.T) {
	var upstreamCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		http.NotFound(w, r)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	sessionID, err := server.store.CreateSession("42", []byte(`{"id":42}`), "avatar-token", "refresh-token", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	cookie := &http.Cookie{Name: server.cfg.CookieName, Value: sessionID}
	oversized := `{"avatar_url":"data:image/png;base64,` + base64.StdEncoding.EncodeToString(make([]byte, maxAgentAvatarUploadBytes+1)) + `"}`
	tests := []struct {
		name   string
		origin string
		body   string
		status int
	}{
		{name: "external URL", origin: "http://agent.local", body: `{"avatar_url":"https://attacker.example/avatar.png"}`, status: http.StatusBadRequest},
		{name: "svg data", origin: "http://agent.local", body: `{"avatar_url":"data:image/svg+xml;base64,PHN2Zz48L3N2Zz4="}`, status: http.StatusBadRequest},
		{name: "invalid base64", origin: "http://agent.local", body: `{"avatar_url":"data:image/png;base64,%%%"}`, status: http.StatusBadRequest},
		{name: "oversized inline image", origin: "http://agent.local", body: oversized, status: http.StatusBadRequest},
		{name: "missing field", origin: "http://agent.local", body: `{}`, status: http.StatusBadRequest},
		{name: "cross origin", origin: "https://attacker.example", body: `{"avatar_url":""}`, status: http.StatusForbidden},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPut, "http://agent.local/api/v1/agent/profile/avatar", strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Origin", test.origin)
			request.AddCookie(cookie)
			response := httptest.NewRecorder()
			server.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status=%d want=%d body=%s", response.Code, test.status, response.Body.String())
			}
		})
	}
	if upstreamCalls != 0 {
		t.Fatalf("invalid avatar requests reached Sub2API %d time(s)", upstreamCalls)
	}
}

func TestAgentProfileAvatarRejectsIdentityOnlySession(t *testing.T) {
	var upstreamCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		http.NotFound(w, r)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	sessionID, err := server.store.CreateSession("42", []byte(`{"id":42}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPut, "http://agent.local/api/v1/agent/profile/avatar", strings.NewReader(`{"avatar_url":""}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://agent.local")
	request.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "MAIN_SESSION_REQUIRED") || upstreamCalls != 0 {
		t.Fatalf("identity-only session changed avatar: status=%d calls=%d body=%s", response.Code, upstreamCalls, response.Body.String())
	}
}
