package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAgentAdminChannelsUsesOwnerOBOAndKeepsMainFactsReadOnly(t *testing.T) {
	var calls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/v1/sub2api/available-channels" {
			t.Fatalf("unexpected upstream path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer app-secret" || r.Header.Get("X-Sub2API-On-Behalf-Of") != "42" || r.Header.Get("X-Sub2API-Satellite") != "agentapi" {
			t.Fatalf("unexpected satellite headers: %#v", r.Header)
		}
		for _, forbidden := range []string{"X-API-Key", "X-AgentAPI-Runtime-Control", "Cookie"} {
			if r.Header.Get(forbidden) != "" {
				t.Fatalf("credential %s leaked to channel lookup", forbidden)
			}
		}
		_ = json.NewEncoder(w).Encode(envelope(map[string]any{
			"channels": []any{map[string]any{
				"name": "Primary", "description": "Public channel", "secret": "must-not-leak",
				"platforms": []any{map[string]any{
					"platform":         "openai",
					"groups":           []any{map[string]any{"id": 7, "name": "standard", "platform": "openai", "rate_multiplier": 1.0}},
					"supported_models": []any{map[string]any{"name": "gpt-5.5", "platform": "openai", "pricing": map[string]any{"billing_mode": "token", "input_price": 0.000001, "admin_rule": "must-not-leak-pricing"}}},
				}},
			}},
			"user_group_rates": map[string]any{"7": 0.8},
		}))
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	sessionID, err := server.store.CreateSession("42", json.RawMessage(`{"id":"42"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "http://agent.local/api/v1/agent/admin/channels", nil)
	req.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || calls != 1 {
		t.Fatalf("admin channel lookup failed: status=%d calls=%d body=%s", rec.Code, calls, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("admin channel response is cacheable: %v", rec.Header())
	}
	body := rec.Body.String()
	if strings.Contains(body, "must-not-leak") || strings.Contains(body, "admin_rule") || !strings.Contains(body, "gpt-5.5") || !strings.Contains(body, `"7":0.8`) {
		t.Fatalf("unexpected admin channel response: %s", body)
	}
}

func TestAgentAdminChannelsRejectsOrdinaryUsersAndWrites(t *testing.T) {
	var calls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.NotFound(w, r)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	if _, err := server.store.UpsertUser(server.cfg.AgentID, "99", "ordinary@example.com", "Ordinary"); err != nil {
		t.Fatal(err)
	}
	ordinaryID, err := server.store.CreateSession("99", json.RawMessage(`{"id":"99"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	ownerID, err := server.store.CreateSession("42", json.RawMessage(`{"id":"42"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, method, session string
		status                int
	}{
		{name: "ordinary user", method: http.MethodGet, session: ordinaryID, status: http.StatusForbidden},
		{name: "owner write", method: http.MethodPost, session: ownerID, status: http.StatusMethodNotAllowed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(test.method, "http://agent.local/api/v1/agent/admin/channels", strings.NewReader(`{"name":"forbidden"}`))
			req.Header.Set("Origin", "http://agent.local")
			req.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: test.session})
			rec := httptest.NewRecorder()
			server.ServeHTTP(rec, req)
			if rec.Code != test.status {
				t.Fatalf("status=%d want=%d body=%s", rec.Code, test.status, rec.Body.String())
			}
		})
	}
	if calls != 0 {
		t.Fatalf("rejected admin channel requests reached Sub2API %d time(s)", calls)
	}
}
