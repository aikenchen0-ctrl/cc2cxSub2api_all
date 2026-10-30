package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStationBillingConfigAndOrdersAreIsolated(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("station billing must not call the main site: %s %s", r.Method, r.URL.Path)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	for _, item := range []struct{ agentID, owner, member string }{
		{"agent-a", "owner-a", "member-a"},
		{"agent-b", "owner-b", "member-b"},
	} {
		if err := server.store.UpsertAgent(Config{AgentID: item.agentID, AgentName: item.agentID, SiteName: item.agentID, BillingMode: "user_upstream", OwnerMainUserID: item.owner}); err != nil {
			t.Fatal(err)
		}
		if _, err := server.store.UpsertUser(item.agentID, item.owner, item.owner+"@example.com", item.owner); err != nil {
			t.Fatal(err)
		}
		if _, err := server.store.UpsertUser(item.agentID, item.member, item.member+"@example.com", item.member); err != nil {
			t.Fatal(err)
		}
	}

	ownerA := stationSession(t, server, "agent-a", "owner-a", tenantRoleOwner)
	memberA := stationSession(t, server, "agent-a", "member-a", tenantRoleMember)
	ownerB := stationSession(t, server, "agent-b", "owner-b", tenantRoleOwner)

	save := stationRequest(server, ownerA, http.MethodPut, "/api/v1/agent/admin/billing/config", `{
		"enabled":true,"min_amount":1,"max_amount":1000,"daily_limit":0,"order_timeout_minutes":30,"max_pending_orders":3,
		"enabled_payment_types":["easypay"],"balance_recharge_multiplier":1,"recharge_fee_rate":0,"load_balance_strategy":"round-robin",
		"payment_visible_method_alipay_enabled":true,"payment_visible_method_wxpay_enabled":true
	}`)
	if save.Code != http.StatusOK || !strings.Contains(save.Body.String(), `"enabled":true`) {
		t.Fatalf("save settings status=%d body=%s", save.Code, save.Body.String())
	}
	provider := stationRequest(server, ownerA, http.MethodPost, "/api/v1/agent/admin/billing/providers", `{
		"provider_key":"easypay","name":"A站易支付","supported_types":["alipay"],"enabled":true,"payment_mode":"popup","sort_order":1,
		"config":{"pid":"1001","pkey":"secret-a","apiBase":"https://pay.example"}
	}`)
	if provider.Code != http.StatusCreated || strings.Contains(provider.Body.String(), "secret-a") {
		t.Fatalf("provider status=%d body=%s", provider.Code, provider.Body.String())
	}
	plan := stationRequest(server, ownerA, http.MethodPost, "/api/v1/agent/admin/billing/plans", `{
		"name":"A站套餐","price":19,"currency":"CNY","validity_days":30,"for_sale":true,"features":["文本"]
	}`)
	if plan.Code != http.StatusCreated || !strings.Contains(plan.Body.String(), "A站套餐") {
		t.Fatalf("plan status=%d body=%s", plan.Code, plan.Body.String())
	}

	other := stationRequest(server, ownerB, http.MethodGet, "/api/v1/agent/admin/billing/providers", "")
	if other.Code != http.StatusOK || strings.Contains(other.Body.String(), "A站易支付") || strings.Contains(other.Body.String(), "secret-a") {
		t.Fatalf("tenant B saw tenant A providers: %d %s", other.Code, other.Body.String())
	}
	otherPlans := stationRequest(server, ownerB, http.MethodGet, "/api/v1/agent/admin/billing/plans", "")
	if strings.Contains(otherPlans.Body.String(), "A站套餐") {
		t.Fatalf("tenant B saw tenant A plans: %s", otherPlans.Body.String())
	}

	checkout := stationRequest(server, memberA, http.MethodGet, "/api/v1/agent/payment/checkout-info", "")
	if checkout.Code != http.StatusOK || !strings.Contains(checkout.Body.String(), "alipay") || strings.Contains(checkout.Body.String(), "secret-a") || strings.Contains(checkout.Body.String(), "pkey") {
		t.Fatalf("checkout status=%d body=%s", checkout.Code, checkout.Body.String())
	}
	if _, err := server.store.db.Exec(`UPDATE station_payment_settings SET balance_recharge_multiplier=1.25 WHERE agent_id=?`, "agent-a"); err != nil {
		t.Fatal(err)
	}
	created := stationRequest(server, memberA, http.MethodPost, "/api/v1/agent/payment/orders", `{"amount":20,"payment_type":"alipay","order_type":"balance","main_user_id":"owner-b"}`)
	if created.Code != http.StatusCreated || !strings.Contains(created.Body.String(), "pay.example") || strings.Contains(created.Body.String(), "secret-a") {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	var payload struct {
		Data struct {
			OutTradeNo string  `json:"out_trade_no"`
			Amount     float64 `json:"amount"`
		} `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &payload); err != nil || payload.Data.OutTradeNo == "" {
		t.Fatalf("order response: %s", created.Body.String())
	}
	if !nearAmount(payload.Data.Amount, 20) {
		t.Fatalf("balance order credited %v instead of the paid amount: %s", payload.Data.Amount, created.Body.String())
	}
	foreignVerify := stationRequest(server, ownerB, http.MethodPost, "/api/v1/agent/payment/orders/verify", `{"out_trade_no":"`+payload.Data.OutTradeNo+`"}`)
	if foreignVerify.Code != http.StatusNotFound {
		t.Fatalf("tenant B verified tenant A order: %d %s", foreignVerify.Code, foreignVerify.Body.String())
	}
	ordersB := stationRequest(server, ownerB, http.MethodGet, "/api/v1/agent/admin/billing/orders", "")
	if strings.Contains(ordersB.Body.String(), payload.Data.OutTradeNo) {
		t.Fatalf("tenant B listed tenant A order: %s", ordersB.Body.String())
	}
}

func stationSession(t *testing.T, server *Server, agentID, userID, role string) string {
	t.Helper()
	id, err := server.store.CreateTenantSession(agentID, userID, role, []byte(`{"id":"`+userID+`"}`), "", "", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func stationRequest(server *Server, sessionID, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "http://agent.local"+path, strings.NewReader(body))
	req.Host = "agent.local"
	req.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://agent.local")
	}
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	return rec
}
