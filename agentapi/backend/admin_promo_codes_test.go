package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAgentAdminPromoCodesExposeSafeUnconfiguredFundingBoundary(t *testing.T) {
	var upstreamCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	sessionID, _ := server.store.CreateSession("42", json.RawMessage(`{"id":"42"}`), "", "", time.Now().Add(sessionTTL))
	req := httptest.NewRequest(http.MethodGet, "http://agent.local/api/v1/agent/admin/promo-codes", nil)
	req.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || upstreamCalls != 0 {
		t.Fatalf("status=%d upstream_calls=%d body=%s", rec.Code, upstreamCalls, rec.Body.String())
	}
	body := rec.Body.String()
	for _, expected := range []string{`"feature_enabled":false`, `"funding_mode":"unconfigured"`, `"authority":"sub2api_main"`, `"redemption_enabled":false`, `"can_create":false`, `"items":[]`} {
		if !strings.Contains(body, expected) {
			t.Fatalf("missing %s in %s", expected, body)
		}
	}
	for _, forbidden := range []string{"admin_key", "super_key", "application_credential", "owner_balance", "main_token"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("unsafe field %q leaked in %s", forbidden, body)
		}
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("promo-code response must not be cached")
	}
}

func TestAgentAdminPromoCodesRejectOrdinaryUsersAndWrites(t *testing.T) {
	upstream := httptest.NewServer(http.NotFoundHandler())
	defer upstream.Close()
	server := testServer(t, upstream)
	if _, err := server.store.UpsertUser(server.cfg.AgentID, "84", "ordinary@example.com", "Ordinary"); err != nil {
		t.Fatal(err)
	}
	ordinaryID, _ := server.store.CreateSession("84", json.RawMessage(`{"id":"84"}`), "", "", time.Now().Add(sessionTTL))
	adminID, _ := server.store.CreateSession("42", json.RawMessage(`{"id":"42"}`), "", "", time.Now().Add(sessionTTL))
	tests := []struct {
		name, method, session string
		want                  int
	}{
		{name: "ordinary", method: http.MethodGet, session: ordinaryID, want: http.StatusForbidden},
		{name: "create", method: http.MethodPost, session: adminID, want: http.StatusMethodNotAllowed},
		{name: "update", method: http.MethodPut, session: adminID, want: http.StatusMethodNotAllowed},
		{name: "delete", method: http.MethodDelete, session: adminID, want: http.StatusMethodNotAllowed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(test.method, "http://agent.local/api/v1/agent/admin/promo-codes", strings.NewReader(`{"bonus_amount":100}`))
			req.Header.Set("Origin", "http://agent.local")
			req.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: test.session})
			rec := httptest.NewRecorder()
			server.ServeHTTP(rec, req)
			if rec.Code != test.want {
				t.Fatalf("status=%d want=%d body=%s", rec.Code, test.want, rec.Body.String())
			}
		})
	}
}
