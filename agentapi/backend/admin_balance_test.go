package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAdminUserListReadsBalanceOnlyForCurrentTenant(t *testing.T) {
	seen := map[string]int{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, "/api/v1/admin/users/") {
			t.Fatalf("unexpected admin request %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "test-main-admin-key" || r.Header.Get("Authorization") != "" {
			t.Fatalf("admin balance query used the wrong credential: %#v", r.Header)
		}
		id := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/users/")
		seen[id]++
		_ = json.NewEncoder(w).Encode(envelope(map[string]any{
			"id": json.Number(id), "email": "hidden@example.com", "role": "user", "balance": 12.5, "frozen_balance": 1.25,
			"notes": "private-marker", "allowed_groups": []int{9}, "api_keys": []any{map[string]any{"key": "sk-secret"}},
		}))
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	if _, err := server.store.UpsertUser(server.cfg.AgentID, "77", "member@example.com", "Member"); err != nil {
		t.Fatal(err)
	}
	sessionID, err := server.store.CreateSession("42", json.RawMessage(`{"id":"42"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "http://agent.local/api/v1/agent/admin/users?main_user_id=999", nil)
	req.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, `"balance_cents":1250`) || !strings.Contains(body, `"frozen_balance_cents":125`) {
		t.Fatalf("status=%d body=%s", rec.Code, body)
	}
	for _, forbidden := range []string{"private-marker", "sk-secret", "allowed_groups", "test-main-admin-key"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("leaked %s: %s", forbidden, body)
		}
	}
	if seen["42"] != 1 || seen["77"] != 1 || seen["999"] != 0 {
		t.Fatalf("queried unexpected users: %#v", seen)
	}
}
