package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPaymentStatsSamplesOnlyCurrentTenantMembers(t *testing.T) {
	seen := map[string]int{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "" || strings.Contains(r.URL.Path, "/admin/") {
			t.Fatalf("payment stats used an admin route: %s %#v", r.URL.Path, r.Header)
		}
		user := r.Header.Get("X-Sub2API-On-Behalf-Of")
		seen[user]++
		amount := 10.0
		if user == "77" {
			amount = 5
		}
		_ = json.NewEncoder(w).Encode(envelope(map[string]any{
			"items": []any{map[string]any{"id": 1, "pay_amount": amount, "amount": amount, "currency": "USD", "status": "COMPLETED", "payment_type": "wxpay"}},
			"total": 1, "page": 1, "page_size": 20, "pages": 1,
		}))
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	if _, err := server.store.UpsertUser(server.cfg.AgentID, "77", "member@example.com", "Member"); err != nil {
		t.Fatal(err)
	}
	ownerSession, err := server.store.CreateSession("42", json.RawMessage(`{"id":"42"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "http://agent.local/api/v1/agent/admin/payments?main_user_id=999", nil)
	req.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: ownerSession})
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"completed_pay_amount":15`) || seen["999"] != 0 || seen["42"] != 1 || seen["77"] != 1 {
		t.Fatalf("status=%d seen=%v body=%s", rec.Code, seen, rec.Body.String())
	}
	memberSession, err := server.store.CreateSession("77", json.RawMessage(`{"id":"77"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	denied := httptest.NewRequest(http.MethodGet, "http://agent.local/api/v1/agent/admin/payments", nil)
	denied.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: memberSession})
	deniedRec := httptest.NewRecorder()
	server.ServeHTTP(deniedRec, denied)
	if deniedRec.Code != http.StatusForbidden {
		t.Fatalf("member status=%d body=%s", deniedRec.Code, deniedRec.Body.String())
	}
}
