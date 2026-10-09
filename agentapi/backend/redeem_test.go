package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRedeemUsesCurrentUserAndHidesCodeManagementFields(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer app-secret" || r.Header.Get("X-Api-Key") != "" || r.Header.Get("X-Sub2API-On-Behalf-Of") != "42" {
			t.Fatalf("unexpected identity: %#v", r.Header)
		}
		if strings.Contains(r.URL.Path, "/admin/") {
			t.Fatalf("redeem reached an admin route: %s", r.URL.Path)
		}
		switch r.URL.Path {
		case "/v1/sub2api/redeem":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["code"] != "GIFT-1" || body["user_id"] != nil {
				t.Fatalf("unexpected redeem body: %#v", body)
			}
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{
				"code": "GIFT-1", "type": "balance", "value": 10, "status": "used", "notes": "admin note", "used_by": 42, "group_id": 7,
			}))
		case "/v1/sub2api/redeem/history":
			_ = json.NewEncoder(w).Encode(envelope([]any{map[string]any{
				"code": "GIFT-1", "type": "balance", "value": 10, "status": "used", "notes": "admin note", "used_at": "2026-09-29T00:00:00Z",
			}}))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	sessionID, err := server.store.CreateSession("42", json.RawMessage(`{"id":"42"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "http://agent.local/api/v1/agent/redeem?main_user_id=99", strings.NewReader(`{"code":"GIFT-1","user_id":99}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "GIFT-1") || strings.Contains(rec.Body.String(), "admin note") || strings.Contains(rec.Body.String(), "used_by") {
		t.Fatalf("redeem status=%d body=%s", rec.Code, rec.Body.String())
	}
	history := httptest.NewRequest(http.MethodGet, "http://agent.local/api/v1/agent/redeem/history", nil)
	history.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	historyRec := httptest.NewRecorder()
	server.ServeHTTP(historyRec, history)
	if historyRec.Code != http.StatusOK || strings.Contains(historyRec.Body.String(), "GIFT-1") || strings.Contains(historyRec.Body.String(), "notes") {
		t.Fatalf("history status=%d body=%s", historyRec.Code, historyRec.Body.String())
	}
}
