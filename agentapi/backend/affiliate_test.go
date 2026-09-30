package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAffiliateUsesMappedUserAndWhitelistsMainSiteFields(t *testing.T) {
	requests := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("Authorization") != "Bearer app-secret" || r.Header.Get("X-Sub2API-On-Behalf-Of") != "42" || r.Header.Get("X-Sub2API-Satellite") != "agentapi" {
			t.Fatalf("unexpected satellite headers: %#v", r.Header)
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sub2api/affiliate":
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{
				"user_id": 42, "inviter_id": 9, "aff_code": "ABC123", "aff_count": 1,
				"aff_quota": 2.5, "aff_frozen_quota": 0.5, "aff_history_quota": 7.5,
				"effective_rebate_rate_percent": 20,
				"provider_secret":               "must-not-leak",
				"invitees":                      []any{map[string]any{"user_id": 88, "email": "m***@example.com", "username": "m***", "total_rebate": 2.5, "created_at": "2026-09-29T00:00:00Z", "notes": "must-not-leak"}},
			}))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sub2api/affiliate/transfer":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body) != 0 {
				t.Fatalf("unexpected transfer body: %#v err=%v", body, err)
			}
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"transferred_quota": 2.5, "balance": 12.5, "user_id": 42, "internal_ledger_id": 99}))
		default:
			t.Fatalf("unexpected upstream request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer upstream.Close()

	server := testServer(t, upstream)
	sessionID, err := server.store.CreateSession("42", json.RawMessage(`{"id":"42","email":"u@example.com"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "http://agent.local"+path, nil)
		req.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		return rec
	}

	detail := request(http.MethodGet, "/api/v1/agent/affiliate?main_user_id=99")
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), `"aff_code":"ABC123"`) || !strings.Contains(detail.Body.String(), `"email":"m***@example.com"`) {
		t.Fatalf("detail status=%d body=%s", detail.Code, detail.Body.String())
	}
	for _, forbidden := range []string{"user_id", "inviter_id", "provider_secret", "notes"} {
		if strings.Contains(detail.Body.String(), forbidden) {
			t.Fatalf("detail leaked %s: %s", forbidden, detail.Body.String())
		}
	}
	transfer := request(http.MethodPost, "/api/v1/agent/affiliate/transfer?main_user_id=99")
	if transfer.Code != http.StatusOK || !strings.Contains(transfer.Body.String(), `"transferred_quota":2.5`) || strings.Contains(transfer.Body.String(), "ledger") || strings.Contains(transfer.Body.String(), "user_id") {
		t.Fatalf("transfer status=%d body=%s", transfer.Code, transfer.Body.String())
	}
	if requests != 2 {
		t.Fatalf("upstream calls=%d", requests)
	}
	if detail.Header().Get("Cache-Control") != "no-store" || transfer.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("affiliate responses must not be cached")
	}
}

func TestAffiliateRequiresSessionAndRejectsUnsupportedMethodsBeforeUpstream(t *testing.T) {
	upstreamCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)

	unauthenticated := httptest.NewRecorder()
	server.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, "http://agent.local/api/v1/agent/affiliate", nil))
	if unauthenticated.Code != http.StatusUnauthorized || upstreamCalls != 0 {
		t.Fatalf("status=%d calls=%d", unauthenticated.Code, upstreamCalls)
	}

	sessionID, err := server.store.CreateSession("42", json.RawMessage(`{"id":"42"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/api/v1/agent/affiliate"},
		{http.MethodGet, "/api/v1/agent/affiliate/transfer"},
		{http.MethodGet, "/api/v1/agent/affiliate/unknown"},
	} {
		req := httptest.NewRequest(tc.method, "http://agent.local"+tc.path, nil)
		req.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed && rec.Code != http.StatusNotFound {
			t.Fatalf("%s %s status=%d body=%s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
	if upstreamCalls != 0 {
		t.Fatalf("invalid requests reached upstream: %d", upstreamCalls)
	}
}
