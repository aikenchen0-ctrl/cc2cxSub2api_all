package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOrdersUseMappedUserAndWhitelistMainSiteFields(t *testing.T) {
	requests := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("Authorization") != "Bearer app-secret" || r.Header.Get("X-Sub2API-On-Behalf-Of") != "42" || r.Header.Get("X-Sub2API-Satellite") != "agentapi" {
			t.Fatalf("unexpected satellite headers: %#v", r.Header)
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sub2api/orders":
			if r.URL.Query().Get("page") != "2" || r.URL.Query().Get("page_size") != "25" || r.URL.Query().Get("status") != "COMPLETED" {
				t.Fatalf("unexpected order query: %s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{
				"items": []any{map[string]any{"id": 7, "user_id": 42, "amount": 12.5, "pay_amount": 13, "currency": "USD", "status": "COMPLETED", "created_at": "2026-09-29T00:00:00Z", "expires_at": "2026-09-30T00:00:00Z", "provider_instance_id": "stripe-1", "provider_secret": "must-not-leak"}},
				"total": 1, "page": 2, "page_size": 25, "pages": 2,
			}))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sub2api/orders/refund-eligible-providers":
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"provider_instance_ids": []string{"stripe-1"}}))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sub2api/orders/7/cancel":
			_ = json.NewEncoder(w).Encode(envelope(map[string]string{"message": "cancelled"}))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sub2api/orders/7/refund-request":
			var body struct {
				Reason string `json:"reason"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Reason != "duplicate charge" {
				t.Fatalf("unexpected refund body: %#v err=%v", body, err)
			}
			_ = json.NewEncoder(w).Encode(envelope(map[string]string{"message": "requested"}))
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
	request := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "http://agent.local"+path, strings.NewReader(body))
		req.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		return rec
	}

	list := request(http.MethodGet, "/api/v1/agent/orders?page=2&page_size=25&status=completed&main_user_id=99", "")
	if list.Code != http.StatusOK || strings.Contains(list.Body.String(), "user_id") || strings.Contains(list.Body.String(), "provider_secret") || !strings.Contains(list.Body.String(), `"id":7`) {
		t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
	}
	eligible := request(http.MethodGet, "/api/v1/agent/orders/refund-eligible-providers", "")
	if eligible.Code != http.StatusOK || !strings.Contains(eligible.Body.String(), "stripe-1") {
		t.Fatalf("eligible status=%d body=%s", eligible.Code, eligible.Body.String())
	}
	cancel := request(http.MethodPost, "/api/v1/agent/orders/7/cancel", `{}`)
	if cancel.Code != http.StatusOK {
		t.Fatalf("cancel status=%d body=%s", cancel.Code, cancel.Body.String())
	}
	refund := request(http.MethodPost, "/api/v1/agent/orders/7/refund-request", `{"reason":" duplicate charge "}`)
	if refund.Code != http.StatusOK {
		t.Fatalf("refund status=%d body=%s", refund.Code, refund.Body.String())
	}
	if requests != 4 {
		t.Fatalf("upstream calls=%d", requests)
	}
	if list.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("orders must not be cached")
	}
}

func TestOrdersRequireSessionAndValidateInputsBeforeUpstream(t *testing.T) {
	upstreamCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { upstreamCalls++; http.Error(w, "unexpected", 500) }))
	defer upstream.Close()
	server := testServer(t, upstream)

	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://agent.local/api/v1/agent/orders", nil))
	if rec.Code != http.StatusUnauthorized || upstreamCalls != 0 {
		t.Fatalf("status=%d calls=%d", rec.Code, upstreamCalls)
	}

	sessionID, err := server.store.CreateSession("42", json.RawMessage(`{"id":"42"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ path, body string }{
		{"/api/v1/agent/orders?status=ROOT", ""},
		{"/api/v1/agent/orders/nope/cancel", `{}`},
		{"/api/v1/agent/orders/7/refund-request", `{"reason":"x"}`},
	} {
		req := httptest.NewRequest(map[bool]string{true: http.MethodGet, false: http.MethodPost}[tc.body == ""], "http://agent.local"+tc.path, strings.NewReader(tc.body))
		req.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("path=%s status=%d body=%s", tc.path, rec.Code, rec.Body.String())
		}
	}
	if upstreamCalls != 0 {
		t.Fatalf("invalid requests reached upstream: %d", upstreamCalls)
	}
}
