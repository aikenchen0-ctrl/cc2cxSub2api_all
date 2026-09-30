package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testCheckoutWithPlans() map[string]any {
	return map[string]any{
		"methods": map[string]any{},
		"plans": []any{
			map[string]any{
				"id": 7, "group_id": 3, "group_name": "OpenAI Pro", "group_platform": "openai",
				"name": "Main Pro", "description": "Main description", "features": []string{"Main feature"},
				"price": 29.9, "currency": "USD", "validity_days": 30, "validity_unit": "days",
				"product_name": "subscription", "provider_secret": "must-not-leak",
			},
			map[string]any{
				"id": 8, "group_id": 4, "group_name": "Claude", "name": "Main Claude",
				"description": "Second plan", "features": []string{"Second feature"},
				"price": 19.9, "currency": "USD", "validity_days": 30, "validity_unit": "days",
				"product_name": "subscription",
			},
		},
		"global_min": 1, "global_max": 1000,
		"balance_recharge_multiplier": 1, "subscription_usd_to_cny_rate": 7.2,
	}
}

func TestAgentPaymentPlansUseMainFactsAndTenantDisplayPolicy(t *testing.T) {
	upstreamCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		if r.Method != http.MethodGet || r.URL.Path != "/v1/sub2api/payment/checkout-info" {
			t.Fatalf("unexpected upstream request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer app-secret" || r.Header.Get("X-Sub2API-On-Behalf-Of") != "42" || r.Header.Get("X-Sub2API-Satellite") != "agentapi" {
			t.Fatalf("unexpected satellite identity headers: %#v", r.Header)
		}
		if r.Header.Get("X-Admin-Key") != "" || r.Header.Get("X-API-Key") != "" {
			t.Fatalf("admin or legacy credential leaked upstream: %#v", r.Header)
		}
		_ = json.NewEncoder(w).Encode(envelope(testCheckoutWithPlans()))
	}))
	defer upstream.Close()

	server := testServer(t, upstream)
	sessionID, err := server.store.CreateSession("42", json.RawMessage(`{"id":"42"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	cookie := &http.Cookie{Name: server.cfg.CookieName, Value: sessionID}

	request := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "http://agent.local"+path, strings.NewReader(body))
		req.AddCookie(cookie)
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Origin", "http://agent.local")
		}
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		return rec
	}

	listed := request(http.MethodGet, "/api/v1/agent/admin/payment/plans", "")
	if listed.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", listed.Code, listed.Body.String())
	}
	for _, expected := range []string{`"source_name":"Main Pro"`, `"enabled":true`, `"customized":false`, `"price":29.9`} {
		if !strings.Contains(listed.Body.String(), expected) {
			t.Fatalf("missing %s in %s", expected, listed.Body.String())
		}
	}
	if strings.Contains(listed.Body.String(), "must-not-leak") || strings.Contains(listed.Body.String(), "provider_secret") {
		t.Fatalf("main-site private plan field leaked: %s", listed.Body.String())
	}

	updated := request(http.MethodPut, "/api/v1/agent/admin/payment/plans/7", `{"enabled":false,"sort_order":2,"display_name":"Agent Pro","description":"Agent description","features":["Agent feature"]}`)
	if updated.Code != http.StatusOK || !strings.Contains(updated.Body.String(), `"name":"Agent Pro"`) || !strings.Contains(updated.Body.String(), `"enabled":false`) {
		t.Fatalf("update status=%d body=%s", updated.Code, updated.Body.String())
	}
	policy, err := server.store.AgentPlanPolicy(server.cfg.AgentID, 7)
	if err != nil || policy.Enabled || policy.DisplayName != "Agent Pro" || policy.SortOrder != 2 {
		t.Fatalf("unexpected saved policy: %+v err=%v", policy, err)
	}

	checkout := request(http.MethodGet, "/api/v1/agent/payment/checkout-info", "")
	if checkout.Code != http.StatusOK || strings.Contains(checkout.Body.String(), `"id":7`) || !strings.Contains(checkout.Body.String(), `"id":8`) {
		t.Fatalf("filtered checkout status=%d body=%s", checkout.Code, checkout.Body.String())
	}
	if upstreamCalls != 3 {
		t.Fatalf("upstream calls=%d", upstreamCalls)
	}
}

func TestHiddenAgentPlanCannotBypassCheckoutWithManualOrder(t *testing.T) {
	upstreamCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		http.Error(w, "order endpoint must not be reached", http.StatusInternalServerError)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	if _, err := server.store.UpsertAgentPlanPolicy(server.cfg.AgentID, AgentPlanPolicy{PlanID: 7, Enabled: false, SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	sessionID, err := server.store.CreateSession("42", json.RawMessage(`{"id":"42"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "http://agent.local/api/v1/agent/payment/orders", strings.NewReader(`{"payment_type":"stripe","order_type":"subscription","plan_id":7}`))
	req.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://agent.local")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "PLAN_NOT_AVAILABLE") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if upstreamCalls != 0 {
		t.Fatalf("hidden plan reached main-site order endpoint: %d", upstreamCalls)
	}
}
