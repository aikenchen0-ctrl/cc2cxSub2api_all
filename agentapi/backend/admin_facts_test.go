package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBalanceHistoryUsesAdminKeyAndDropsSecrets(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/admin/users/42/balance-history" || r.Header.Get("x-api-key") != "test-main-admin-key" {
			t.Fatalf("unexpected admin read %s %#v", r.URL.Path, r.Header)
		}
		if r.URL.Query().Get("user_id") != "" {
			t.Fatalf("browser selector reached upstream: %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(envelope(map[string]any{
			"items": []any{map[string]any{"code": "GIFT-1", "type": "balance", "value": 10, "status": "used", "notes": "private", "used_by": 42, "created_at": "2026-09-29T00:00:00Z"}},
			"total": 1, "page": 1, "page_size": 20, "total_recharged": 10,
		}))
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	sessionID, err := server.store.CreateSession("42", json.RawMessage(`{"id":"42"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "http://agent.local/api/v1/agent/balance/history?main_user_id=99", nil)
	req.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, `"type":"balance"`) || strings.Contains(body, "GIFT-1") || strings.Contains(body, "private") || strings.Contains(body, "used_by") {
		t.Fatalf("status=%d body=%s", rec.Code, body)
	}
}

func TestMemberFactsStayInsideCurrentTenant(t *testing.T) {
	seenUsage := ""
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.RawQuery, "999") || strings.Contains(r.URL.Path, "/999") {
			t.Fatalf("outsider reached upstream: %s", r.URL.RequestURI())
		}
		switch {
		case r.URL.Path == "/api/v1/admin/users/77":
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 77, "balance": 3.5, "frozen_balance": 0, "notes": "hidden"}))
		case r.URL.Path == "/api/v1/admin/users/77/balance-history":
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"items": []any{}, "total": 0, "page": 1, "page_size": 20, "total_recharged": 0}))
		case r.URL.Path == "/api/v1/admin/usage":
			seenUsage = r.URL.RawQuery
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{
				"items": []any{map[string]any{"user_id": 77, "account_id": 9, "request_id": "req-1", "model": "gpt-5.5", "input_tokens": 3, "output_tokens": 4, "total_cost": 0.1, "actual_cost": 0.08, "created_at": "2026-09-29T00:00:00Z", "ip_address": "10.0.0.8"}},
				"total": 1, "page": 1, "page_size": 20,
			}))
		default:
			t.Fatalf("unexpected admin path %s", r.URL.Path)
		}
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
	denied := httptest.NewRequest(http.MethodGet, "http://agent.local/api/v1/agent/admin/member-facts?main_user_id=999", nil)
	denied.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	deniedRec := httptest.NewRecorder()
	server.ServeHTTP(deniedRec, denied)
	if deniedRec.Code != http.StatusNotFound || seenUsage != "" {
		t.Fatalf("outsider status=%d seen=%q", deniedRec.Code, seenUsage)
	}
	req := httptest.NewRequest(http.MethodGet, "http://agent.local/api/v1/agent/admin/member-facts?main_user_id=77", nil)
	req.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, `"model":"gpt-5.5"`) || !strings.Contains(seenUsage, "user_id=77") || strings.Contains(body, "account_id") || strings.Contains(body, "10.0.0.8") || strings.Contains(body, "hidden") {
		t.Fatalf("status=%d body=%s seen=%s", rec.Code, body, seenUsage)
	}
}
