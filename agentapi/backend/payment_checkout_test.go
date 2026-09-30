package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCreatePaymentOrderAbsolutizesMainWeChatOAuthURL(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/sub2api/payment/orders" {
			t.Fatalf("unexpected upstream request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer app-secret" || r.Header.Get("X-Sub2API-On-Behalf-Of") != "43" || r.Header.Get("X-Sub2API-Satellite") != "agentapi" {
			t.Fatalf("unexpected satellite headers: %#v", r.Header)
		}
		_ = json.NewEncoder(w).Encode(envelope(map[string]any{
			"result_type": "oauth_required",
			"oauth": map[string]any{
				"authorize_url": "/api/v1/auth/oauth/wechat/payment/start?amount=12.5&payment_type=wxpay&satellite_handoff=signed-token",
				"scope":         "snsapi_base",
			},
		}))
	}))
	defer upstream.Close()

	client := NewMainClient(Config{
		MainAPIBaseURL:     upstream.URL + "/api/v1",
		MainModelBaseURL:   upstream.URL + "/v1",
		PublicMainURL:      upstream.URL,
		AppCredential:      "app-secret",
		SatelliteSlug:      "agentapi",
		MainRequestTimeout: time.Second,
	})
	result, err := client.CreatePaymentOrder(context.Background(), "43", AgentPaymentOrderRequest{
		Amount:      12.5,
		PaymentType: "wxpay",
		OrderType:   "balance",
	}, "http://agent.local/payment/result")
	if err != nil {
		t.Fatal(err)
	}
	want := upstream.URL + "/api/v1/auth/oauth/wechat/payment/start?amount=12.5&payment_type=wxpay&satellite_handoff=signed-token"
	if result.OAuth == nil || result.OAuth.AuthorizeURL != want {
		t.Fatalf("authorize URL = %#v, want %q", result.OAuth, want)
	}
	if strings.Contains(result.OAuth.AuthorizeURL, "app-secret") {
		t.Fatalf("application credential leaked into browser URL: %q", result.OAuth.AuthorizeURL)
	}
}

func TestCreatePaymentOrderRejectsUntrustedWeChatOAuthURL(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(envelope(map[string]any{
			"result_type": "oauth_required",
			"oauth":       map[string]any{"authorize_url": "https://evil.example/steal"},
		}))
	}))
	defer upstream.Close()

	client := NewMainClient(Config{
		MainAPIBaseURL:     upstream.URL + "/api/v1",
		MainModelBaseURL:   upstream.URL + "/v1",
		PublicMainURL:      upstream.URL,
		AppCredential:      "app-secret",
		SatelliteSlug:      "agentapi",
		MainRequestTimeout: time.Second,
	})
	_, err := client.CreatePaymentOrder(context.Background(), "43", AgentPaymentOrderRequest{
		Amount:      12.5,
		PaymentType: "wxpay",
		OrderType:   "balance",
	}, "http://agent.local/payment/result")
	if err == nil {
		t.Fatal("expected malicious authorize URL to be rejected")
	}
	mainErr, ok := err.(*MainAPIError)
	if !ok || mainErr.Code != "SATELLITE_INVALID_RESPONSE" || strings.Contains(mainErr.Error(), "evil.example") {
		t.Fatalf("unexpected sanitized error: %#v", err)
	}
}

func TestPaymentCheckoutUsesSessionIdentityAndMainSiteLedger(t *testing.T) {
	requests := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("Authorization") != "Bearer app-secret" || r.Header.Get("X-Sub2API-On-Behalf-Of") != "43" || r.Header.Get("X-Sub2API-Satellite") != "agentapi" {
			t.Fatalf("unexpected satellite headers: %#v", r.Header)
		}
		if r.Header.Get("X-API-Key") != "" || r.Header.Get("X-Admin-Key") != "" {
			t.Fatalf("legacy or admin credential leaked upstream: %#v", r.Header)
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sub2api/payment/checkout-info":
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{
				"methods":    map[string]any{"alipay": map[string]any{"currency": "CNY", "display_name": "支付宝", "single_min": 10, "single_max": 1000, "fee_rate": 2, "available": true, "provider_secret": "must-not-leak"}},
				"global_min": 10, "global_max": 1000, "plans": []any{}, "balance_disabled": false,
				"balance_recharge_multiplier": 1, "subscription_usd_to_cny_rate": 7.2, "recharge_fee_rate": 2,
				"help_text": "help", "help_image_url": "", "stripe_publishable_key": "pk_test_public",
				"alipay_force_qrcode": false, "alipay_mobile_precreate_deep_link": false,
				"admin_key": "must-not-leak",
			}))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sub2api/payment/orders":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["amount"] != float64(50) || body["payment_type"] != "alipay" || body["order_type"] != "balance" || body["payment_source"] != "agentapi" || body["is_wechat_browser"] != true {
				t.Fatalf("unexpected order body: %#v", body)
			}
			if body["return_url"] != "http://agent.local/payment/result" {
				t.Fatalf("return URL was not server-derived: %#v", body["return_url"])
			}
			if _, exists := body["user_id"]; exists {
				t.Fatalf("browser-selected user reached main site: %#v", body)
			}
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{
				"order_id": 77, "amount": 50, "pay_amount": 51, "fee_rate": 2, "status": "PENDING",
				"result_type": "order_created", "payment_type": "alipay", "out_trade_no": "PAY-000077",
				"qr_code": "https://pay.example/qr/77", "currency": "CNY", "expires_at": "2026-09-29T01:00:00Z",
			}))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sub2api/payment/orders/verify":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["out_trade_no"] != "PAY-000077" {
				t.Fatalf("unexpected verify body: %#v err=%v", body, err)
			}
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{
				"id": 77, "amount": 50, "pay_amount": 51, "fee_rate": 2, "currency": "CNY", "payment_type": "alipay",
				"out_trade_no": "PAY-000077", "status": "COMPLETED", "order_type": "balance",
				"created_at": "2026-09-29T00:00:00Z", "expires_at": "2026-09-29T01:00:00Z", "refund_amount": 0,
			}))
		default:
			t.Fatalf("unexpected upstream request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer upstream.Close()

	server := testServer(t, upstream)
	if _, err := server.store.UpsertUser(server.cfg.AgentID, "43", "member@example.com", "Member"); err != nil {
		t.Fatal(err)
	}
	sessionID, err := server.store.CreateSession("43", json.RawMessage(`{"id":"43","email":"member@example.com"}`), "", "", time.Now().Add(sessionTTL))
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

	checkout := request(http.MethodGet, "/api/v1/agent/payment/checkout-info", "")
	if checkout.Code != http.StatusOK || !strings.Contains(checkout.Body.String(), "支付宝") || strings.Contains(checkout.Body.String(), "must-not-leak") || strings.Contains(checkout.Body.String(), "admin_key") {
		t.Fatalf("checkout status=%d body=%s", checkout.Code, checkout.Body.String())
	}
	created := request(http.MethodPost, "/api/v1/agent/payment/orders", `{"amount":50,"payment_type":"alipay","order_type":"balance","is_wechat_browser":true,"user_id":"999","return_url":"https://attacker.example"}`)
	if created.Code != http.StatusCreated || !strings.Contains(created.Body.String(), "PAY-000077") || strings.Contains(created.Body.String(), "app-secret") {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	verified := request(http.MethodPost, "/api/v1/agent/payment/orders/verify", `{"out_trade_no":"PAY-000077"}`)
	if verified.Code != http.StatusOK || !strings.Contains(verified.Body.String(), "COMPLETED") {
		t.Fatalf("verify status=%d body=%s", verified.Code, verified.Body.String())
	}
	if requests != 3 {
		t.Fatalf("upstream calls=%d", requests)
	}
	if checkout.Header().Get("Cache-Control") != "no-store" || created.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("payment responses must not be cached")
	}
}

func TestPaymentCheckoutRequiresSessionAndRejectsCrossOriginMutations(t *testing.T) {
	upstreamCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		http.Error(w, `{"secret":"upstream-private"}`, http.StatusInternalServerError)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)

	unauthenticated := httptest.NewRecorder()
	server.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, "http://agent.local/api/v1/agent/payment/checkout-info", nil))
	if unauthenticated.Code != http.StatusUnauthorized || upstreamCalls != 0 {
		t.Fatalf("unauthenticated status=%d calls=%d", unauthenticated.Code, upstreamCalls)
	}

	sessionID, err := server.store.CreateSession("42", json.RawMessage(`{"id":"42"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	foreign := httptest.NewRequest(http.MethodPost, "http://agent.local/api/v1/agent/payment/orders", strings.NewReader(`{"amount":50,"payment_type":"alipay","order_type":"balance"}`))
	foreign.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	foreign.Header.Set("Content-Type", "application/json")
	foreign.Header.Set("Origin", "https://attacker.example")
	foreignResponse := httptest.NewRecorder()
	server.ServeHTTP(foreignResponse, foreign)
	if foreignResponse.Code != http.StatusForbidden || !strings.Contains(foreignResponse.Body.String(), "CSRF_ORIGIN_REJECTED") || upstreamCalls != 0 {
		t.Fatalf("cross-origin status=%d calls=%d body=%s", foreignResponse.Code, upstreamCalls, foreignResponse.Body.String())
	}

	invalid := httptest.NewRequest(http.MethodPost, "http://agent.local/api/v1/agent/payment/orders/verify", strings.NewReader(`{"out_trade_no":"../bad"}`))
	invalid.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	invalid.Header.Set("Content-Type", "application/json")
	invalid.Header.Set("Origin", "http://agent.local")
	invalidResponse := httptest.NewRecorder()
	server.ServeHTTP(invalidResponse, invalid)
	if invalidResponse.Code != http.StatusBadRequest || upstreamCalls != 0 {
		t.Fatalf("invalid verify status=%d calls=%d body=%s", invalidResponse.Code, upstreamCalls, invalidResponse.Body.String())
	}
}
