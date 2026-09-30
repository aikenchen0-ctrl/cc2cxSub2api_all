package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAgentAdminSubscriptionsAggregateOnlyMappedUsers(t *testing.T) {
	var mu sync.Mutex
	seenUsers := map[string]int{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/sub2api/subscriptions" {
			t.Errorf("unexpected upstream request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer app-secret" || r.Header.Get("X-Sub2API-Satellite") != "agentapi" {
			t.Errorf("unexpected satellite credentials: %#v", r.Header)
		}
		if r.Header.Get("X-Admin-Key") != "" || r.Header.Get("X-API-Key") != "" {
			t.Errorf("administrator credential leaked upstream: %#v", r.Header)
		}
		mainUserID := r.Header.Get("X-Sub2API-On-Behalf-Of")
		mu.Lock()
		seenUsers[mainUserID]++
		mu.Unlock()
		items := []any{}
		if mainUserID == "42" {
			items = []any{map[string]any{
				"id": 10, "group_id": 3, "starts_at": "2026-09-01T00:00:00Z", "expires_at": "2026-10-01T00:00:00Z", "status": "active",
				"daily_usage_usd": 2.5, "weekly_usage_usd": 5, "monthly_usage_usd": 9,
				"group":       map[string]any{"id": 3, "name": "OpenAI Pro", "platform": "openai", "rate_multiplier": 1, "daily_limit_usd": 10, "upstream_secret": "must-not-leak"},
				"admin_notes": "must-not-leak",
			}}
		} else if mainUserID == "84" {
			items = []any{map[string]any{
				"id": 20, "group_id": 4, "starts_at": "2026-09-20T00:00:00Z", "expires_at": "2026-10-20T00:00:00Z", "status": "active",
				"daily_usage_usd": 1, "weekly_usage_usd": 2, "monthly_usage_usd": 3,
				"group": map[string]any{"id": 4, "name": "Gemini Pro", "platform": "gemini", "rate_multiplier": 1, "daily_limit_usd": 8},
			}}
		}
		_ = json.NewEncoder(w).Encode(envelope(items))
	}))
	defer upstream.Close()

	server := testServer(t, upstream)
	if _, err := server.store.UpsertUser(server.cfg.AgentID, "84", "second@example.com", "Second User"); err != nil {
		t.Fatal(err)
	}
	sessionID, err := server.store.CreateSession("42", json.RawMessage(`{"id":"42"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "http://agent.local/api/v1/agent/admin/subscriptions?status=active&platform=gemini&page=1&page_size=20", nil)
	req.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, expected := range []string{`"id":20`, `"main_user_id":"84"`, `"user_email":"second@example.com"`, `"name":"Gemini Pro"`, `"total":1`} {
		if !strings.Contains(body, expected) {
			t.Fatalf("missing %s in %s", expected, body)
		}
	}
	for _, forbidden := range []string{`"id":10`, "must-not-leak", "upstream_secret", "admin_notes"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("filtered or sensitive field %q leaked: %s", forbidden, body)
		}
	}
	mu.Lock()
	if seenUsers["42"] != 1 || seenUsers["84"] != 1 || len(seenUsers) != 2 {
		t.Fatalf("unexpected OBO users: %#v", seenUsers)
	}
	mu.Unlock()
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("subscription administration response must not be cached")
	}
}

func TestAgentAdminSubscriptionsRejectOrdinaryUsersAndUnmappedTargets(t *testing.T) {
	upstreamCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	if _, err := server.store.UpsertUser(server.cfg.AgentID, "84", "ordinary@example.com", "Ordinary"); err != nil {
		t.Fatal(err)
	}
	ordinarySession, err := server.store.CreateSession("84", json.RawMessage(`{"id":"84"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	ordinaryReq := httptest.NewRequest(http.MethodGet, "http://agent.local/api/v1/agent/admin/subscriptions", nil)
	ordinaryReq.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: ordinarySession})
	ordinaryRec := httptest.NewRecorder()
	server.ServeHTTP(ordinaryRec, ordinaryReq)
	if ordinaryRec.Code != http.StatusForbidden {
		t.Fatalf("ordinary user status=%d body=%s", ordinaryRec.Code, ordinaryRec.Body.String())
	}

	adminSession, err := server.store.CreateSession("42", json.RawMessage(`{"id":"42"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	unmappedReq := httptest.NewRequest(http.MethodGet, "http://agent.local/api/v1/agent/admin/subscriptions?main_user_id=999", nil)
	unmappedReq.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: adminSession})
	unmappedRec := httptest.NewRecorder()
	server.ServeHTTP(unmappedRec, unmappedReq)
	if unmappedRec.Code != http.StatusNotFound || !strings.Contains(unmappedRec.Body.String(), "MAPPED_USER_NOT_FOUND") {
		t.Fatalf("unmapped status=%d body=%s", unmappedRec.Code, unmappedRec.Body.String())
	}
	if upstreamCalls != 0 {
		t.Fatalf("rejected requests reached main site: %d", upstreamCalls)
	}
}
