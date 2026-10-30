package main

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/agentapi/payment"
	stripe "github.com/stripe/stripe-go/v85"
)

func TestPaymentPortSignedJSONWebhooks(t *testing.T) {
	for _, key := range []string{"stripe", "airwallex"} {
		t.Run(key, func(t *testing.T) {
			upstream := httptest.NewServer(http.NotFoundHandler())
			defer upstream.Close()
			server := testServer(t, upstream)
			p, err := server.store.CreateStationProvider("agent-test", StationPaymentProvider{ProviderKey: key, Name: key, Config: map[string]string{"secretKey": "sk_test", "publishableKey": "pk_test", "webhookSecret": "test-signing-secret", "clientId": "client-test", "apiKey": "api-test", "apiBase": "https://api.airwallex.com/api/v1", "currency": "USD"}})
			if err != nil {
				t.Fatal(err)
			}
			_, err = server.store.CreateStationOrder(StationPaymentOrder{OutTradeNo: "signed-json", Amount: 10, PayAmount: 10, Currency: "USD", Status: "PENDING", ProviderInstanceID: strconv.FormatInt(p.ID, 10)}, "agent-test", "42", time.Now().Add(time.Hour))
			if err != nil {
				t.Fatal(err)
			}
			for _, currency := range []string{"EUR", "USD"} {
				var body string
				if key == "stripe" {
					body = fmt.Sprintf(`{"id":"evt_test","object":"event","api_version":%q,"type":"payment_intent.succeeded","data":{"object":{"id":"pi_test","amount":1000,"currency":%q,"metadata":{"orderId":"signed-json"}}}}`, stripe.APIVersion, strings.ToLower(currency))
				} else {
					body = fmt.Sprintf(`{"name":"payment_intent.succeeded","data":{"object":{"id":"int_test","merchant_order_id":"signed-json","amount":10,"currency":%q,"status":"SUCCEEDED"}}}`, currency)
				}
				timestamp := strconv.FormatInt(time.Now().Unix(), 10)
				if key == "airwallex" {
					timestamp = strconv.FormatInt(time.Now().UnixMilli(), 10)
				}
				prefix := timestamp
				if key == "stripe" {
					prefix += "."
				}
				mac := hmac.New(sha256.New, []byte("test-signing-secret"))
				_, _ = mac.Write([]byte(prefix + body))
				sig := fmt.Sprintf("%x", mac.Sum(nil))
				req := httptest.NewRequest("POST", "http://agent.local"+stationWebhookPath("agent-test", key), strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				if key == "stripe" {
					req.Header.Set("Stripe-Signature", "t="+timestamp+",v1="+sig)
				} else {
					req.Header.Set("X-Timestamp", timestamp)
					req.Header.Set("X-Signature", sig)
				}
				rec := httptest.NewRecorder()
				server.ServeHTTP(rec, req)
				if currency == "EUR" && rec.Code < 400 {
					t.Fatal("wrong currency accepted")
				}
				if currency == "USD" && rec.Code != 200 {
					t.Fatalf("canonical HTTP signature headers rejected: %d %s", rec.Code, rec.Body.String())
				}
			}
		})
	}
}

func TestPaymentPortEarlyWebhookReplayAndCrossTenantIsolation(t *testing.T) {
	upstream := httptest.NewServer(http.NotFoundHandler())
	defer upstream.Close()
	server := testServer(t, upstream)
	if err := server.store.UpsertAgent(Config{AgentID: "other", AgentName: "Other", SiteName: "Other", OwnerMainUserID: "77", BillingMode: "user_upstream"}); err != nil {
		t.Fatal(err)
	}
	if _, err := server.store.UpsertUser("other", "77", "other@example.com", "Other"); err != nil {
		t.Fatal(err)
	}
	otherProvider, err := server.store.CreateStationProvider("other", StationPaymentProvider{ProviderKey: "easypay", Name: "Other merchant", Config: map[string]string{"pid": "2002", "pkey": "other-secret", "apiBase": "https://other.example"}})
	if err != nil {
		t.Fatal(err)
	}
	notify := func(agent, orderNo, money string) *httptest.ResponseRecorder {
		values := url.Values{"money": {money}, "out_trade_no": {orderNo}, "pid": {"1001"}, "trade_no": {"fast-paid"}, "trade_status": {"TRADE_SUCCESS"}}
		pairs := []string{}
		for _, key := range []string{"money", "out_trade_no", "pid", "trade_no", "trade_status"} {
			pairs = append(pairs, key+"="+values.Get(key))
		}
		values.Set("sign", fmt.Sprintf("%x", md5.Sum([]byte(strings.Join(pairs, "&")+"mine-secret"))))
		req := httptest.NewRequest("POST", "http://agent.local"+stationWebhookPath(agent, "easypay"), strings.NewReader(values.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		return rec
	}
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		orderNo := r.Form.Get("out_trade_no")
		order, err := server.store.StationOrderByNo("agent-test", orderNo)
		if err != nil || order.Status != "PENDING" {
			t.Errorf("order not reserved before gateway call: %v", err)
			w.WriteHeader(500)
			return
		}
		_, err = server.store.CreateStationOrder(StationPaymentOrder{OutTradeNo: orderNo, Amount: 10, PayAmount: 10, ProviderInstanceID: strconv.FormatInt(otherProvider.ID, 10), Status: "PENDING"}, "other", "77", time.Now().Add(time.Hour))
		if err != nil {
			t.Error(err)
			w.WriteHeader(500)
			return
		}
		if rec := notify("other", orderNo, r.Form.Get("money")); rec.Code != 401 {
			t.Errorf("foreign callback accepted: %d", rec.Code)
		}
		for i := 0; i < 2; i++ {
			if rec := notify("agent-test", orderNo, r.Form.Get("money")); rec.Code != 200 || rec.Body.String() != "success" {
				t.Errorf("early callback %d %s", rec.Code, rec.Body.String())
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 1, "qrcode": "https://pay.example/qr"})
	}))
	defer gateway.Close()
	settings := defaultStationPaymentSettings()
	settings.Enabled = true
	settings.EnabledTypes = []string{"easypay"}
	if err = server.store.SaveStationPaymentSettings("agent-test", settings); err != nil {
		t.Fatal(err)
	}
	if _, err = server.store.db.Exec(`UPDATE station_payment_settings SET balance_recharge_multiplier=1.25 WHERE agent_id=?`, "agent-test"); err != nil {
		t.Fatal(err)
	}
	_, err = server.store.CreateStationProvider("agent-test", StationPaymentProvider{ProviderKey: "easypay", Name: "Mine", Enabled: true, PaymentMode: "qrcode", SupportedTypes: []string{"alipay"}, Config: map[string]string{"pid": "1001", "pkey": "mine-secret", "apiBase": gateway.URL}})
	if err != nil {
		t.Fatal(err)
	}
	cookie := stationSession(t, server, "agent-test", "42", tenantRoleMember)
	response := stationRequest(server, cookie, "POST", "/api/v1/agent/payment/orders", `{"amount":10,"payment_type":"alipay","order_type":"balance"}`)
	if response.Code != 201 {
		t.Fatalf("create %d %s", response.Code, response.Body.String())
	}
	result := decodeData[AgentPaymentCreateResult](t, response.Body.Bytes())
	if result.Status != "COMPLETED" {
		t.Fatalf("early payment status lost: %s", result.Status)
	}
	var count int
	var cents int64
	if err = server.store.db.QueryRow(`SELECT COUNT(*),COALESCE(SUM(amount_cents),0) FROM wallet_ledger WHERE agent_id=? AND kind='user_recharge'`, "agent-test").Scan(&count, &cents); err != nil {
		t.Fatal(err)
	}
	if count != 1 || cents != 1000 {
		t.Fatalf("replayed credit count=%d amount=%d", count, cents)
	}
	otherOrder, _ := server.store.StationOrderByNo("other", result.OutTradeNo)
	if otherOrder.Status != "PENDING" {
		t.Fatal("other station order credited")
	}
	if err = server.store.db.QueryRow(`SELECT COUNT(*) FROM wallet_ledger WHERE agent_id=? AND kind='user_recharge'`, "other").Scan(&count); err != nil || count != 0 {
		t.Fatalf("other station wallet changed: %v %d", err, count)
	}
}

func TestPaymentPortPartialUpdatesAndTenantBoundCallbacks(t *testing.T) {
	upstream := httptest.NewServer(http.NotFoundHandler())
	defer upstream.Close()
	server := testServer(t, upstream)
	owner := stationSession(t, server, "agent-test", "42", tenantRoleOwner)
	if err := server.store.UpsertAgent(Config{AgentID: "other", AgentName: "Other", SiteName: "Other", OwnerMainUserID: "77", BillingMode: "user_upstream"}); err != nil {
		t.Fatal(err)
	}
	if _, err := server.store.UpsertUser("other", "77", "other@example.com", "Other"); err != nil {
		t.Fatal(err)
	}
	other := stationSession(t, server, "other", "77", tenantRoleOwner)
	member := stationSession(t, server, "agent-test", "42", tenantRoleMember)
	created := stationRequest(server, owner, "POST", "/api/v1/agent/admin/billing/providers", `{"agent_id":"other","provider_key":"easypay","name":"Mine","enabled":true,"supported_types":["alipay"],"payment_mode":"popup","config":{"pid":"1001","pkey":"private-merchant-secret","apiBase":"https://pay.example","notifyUrl":"https://notify.example/api/v1/agent/billing/webhook/other/easypay"}}`)
	if created.Code != 201 {
		t.Fatalf("create %d %s", created.Code, created.Body.String())
	}
	provider := decodeData[StationPaymentProvider](t, created.Body.Bytes())
	path := fmt.Sprintf("/api/v1/agent/admin/billing/providers/%d", provider.ID)
	if strings.Contains(created.Body.String(), "private-merchant-secret") || !strings.Contains(provider.Config["notifyUrl"], "/agent-test/easypay") {
		t.Fatalf("unsafe create response: %s", created.Body.String())
	}
	var cipher string
	if err := server.store.db.QueryRow(`SELECT config_cipher FROM station_payment_providers WHERE agent_id=? AND id=?`, "agent-test", provider.ID).Scan(&cipher); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(cipher, "private-merchant-secret") {
		t.Fatal("merchant secret stored in plaintext")
	}
	for _, patch := range []string{`{"sort_order":9}`, `{"refund_enabled":true,"allow_user_refund":true}`, `{"config":{"pkey":"","cidAlipay":"channel-a"}}`} {
		response := stationRequest(server, owner, "PUT", path, patch)
		if response.Code != 200 {
			t.Fatalf("patch %d %s", response.Code, response.Body.String())
		}
	}
	saved, err := server.store.StationProvider("agent-test", provider.ID, true)
	if err != nil || saved.Config["pkey"] != "private-merchant-secret" || saved.Config["cidAlipay"] != "channel-a" || !saved.Enabled || saved.SortOrder != 9 || !saved.AllowUserRefund {
		t.Fatalf("partial update lost fields: %+v %v", saved, err)
	}
	for _, method := range []string{"PUT", "DELETE"} {
		response := stationRequest(server, other, method, path, `{"name":"stolen","agent_id":"agent-test"}`)
		if response.Code != 404 {
			t.Fatalf("cross-tenant %s: %d %s", method, response.Code, response.Body.String())
		}
	}
	for _, cookie := range []string{member, ""} {
		response := stationRequest(server, cookie, "PUT", path, `{"name":"forbidden"}`)
		if response.Code != 401 && response.Code != 403 {
			t.Fatalf("non-owner update: %d", response.Code)
		}
	}
	before, _ := server.store.StationPaymentSettings("other")
	response := stationRequest(server, owner, "PUT", "/api/v1/agent/admin/billing/config?agent_id=other", `{"agent_id":"other","help_text":"mine","enabled_payment_types":["easypay"]}`)
	if response.Code != 200 {
		t.Fatalf("partial config %d %s", response.Code, response.Body.String())
	}
	after, _ := server.store.StationPaymentSettings("other")
	own, _ := server.store.StationPaymentSettings("agent-test")
	if after.HelpText != before.HelpText || own.HelpText != "mine" || own.MaxPendingOrders != 3 {
		t.Fatal("configuration changed wrong tenant or reset omitted fields")
	}

	order, err := server.store.CreateStationOrder(StationPaymentOrder{OutTradeNo: "protected", ProviderInstanceID: strconv.FormatInt(provider.ID, 10), Status: "PENDING"}, "agent-test", "42", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	for _, patch := range []string{`{"config":{"pkey":"rotated"}}`, `{"config":{"pid":"2002"}}`, `{"supported_types":[]}`} {
		response = stationRequest(server, owner, "PUT", path, patch)
		if response.Code != 409 {
			t.Fatalf("pending protection %d %s", response.Code, response.Body.String())
		}
	}
	response = stationRequest(server, owner, "DELETE", path, `{}`)
	if response.Code != 409 {
		t.Fatalf("delete pending %d", response.Code)
	}
	response = stationRequest(server, owner, "PUT", path, `{"enabled":false,"refund_enabled":false}`)
	if response.Code != 200 {
		t.Fatalf("safe update pending %d %s", response.Code, response.Body.String())
	}
	saved, _ = server.store.StationProvider("agent-test", provider.ID, true)
	if saved.AllowUserRefund || saved.Config["pkey"] != "private-merchant-secret" {
		t.Fatal("refund cascade or secret preservation failed")
	}
	if err = server.store.UpdateStationOrderStatus("agent-test", order.ID, "PENDING", "CANCELLED", ""); err != nil {
		t.Fatal(err)
	}
	response = stationRequest(server, owner, "DELETE", path, `{}`)
	if response.Code != 200 {
		t.Fatalf("delete finished %d %s", response.Code, response.Body.String())
	}
}

func TestPaymentPortUserQuotasAndConcurrentPendingOrders(t *testing.T) {
	upstream := httptest.NewServer(http.NotFoundHandler())
	defer upstream.Close()
	server := testServer(t, upstream)
	settings := defaultStationPaymentSettings()
	settings.Enabled = true
	settings.EnabledTypes = []string{"easypay"}
	settings.DailyLimit = 15
	settings.MaxPendingOrders = 10
	if err := server.store.SaveStationPaymentSettings("agent-test", settings); err != nil {
		t.Fatal(err)
	}
	_, err := server.store.CreateStationProvider("agent-test", StationPaymentProvider{ProviderKey: "easypay", Name: "Test", Enabled: true, PaymentMode: "popup", SupportedTypes: []string{"alipay", "wxpay"}, Config: map[string]string{"pid": "1001", "pkey": "secret", "apiBase": "https://pay.example"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"42", "43"} {
		if _, err = server.store.UpsertUser("agent-test", id, id+"@example.com", id); err != nil {
			t.Fatal(err)
		}
	}
	a := stationSession(t, server, "agent-test", "42", tenantRoleMember)
	b := stationSession(t, server, "agent-test", "43", tenantRoleMember)
	for _, cookie := range []string{a, b} {
		response := stationRequest(server, cookie, "POST", "/api/v1/agent/payment/orders", `{"amount":10,"payment_type":"alipay","order_type":"balance"}`)
		if response.Code != 201 {
			t.Fatalf("independent user quota: %d %s", response.Code, response.Body.String())
		}
		result := decodeData[AgentPaymentCreateResult](t, response.Body.Bytes())
		if result.PaymentMode != "popup" {
			t.Fatal("popup mode not returned")
		}
	}
	response := stationRequest(server, a, "POST", "/api/v1/agent/payment/orders", `{"amount":10,"payment_type":"wxpay","order_type":"balance"}`)
	if response.Code != 400 || !strings.Contains(response.Body.String(), "DAILY_LIMIT_EXCEEDED") {
		t.Fatalf("quota across methods: %d %s", response.Code, response.Body.String())
	}
	if _, err = server.store.db.Exec(`UPDATE station_payment_orders SET expires_at=1 WHERE agent_id=?`, "agent-test"); err != nil {
		t.Fatal(err)
	}
	settings.DailyLimit = 0
	settings.MaxPendingOrders = 1
	if err = server.store.SaveStationPaymentSettings("agent-test", settings); err != nil {
		t.Fatal(err)
	}
	results := make(chan int, 6)
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- stationRequest(server, a, "POST", "/api/v1/agent/payment/orders", `{"amount":10,"payment_type":"alipay","order_type":"balance"}`).Code
		}()
	}
	wg.Wait()
	close(results)
	created := 0
	for code := range results {
		if code == 201 {
			created++
		} else if code != 429 {
			t.Errorf("unexpected concurrent response %d", code)
		}
	}
	if created != 1 {
		t.Fatalf("pending limit bypassed: %d created", created)
	}
}

func TestPaymentPortCurrencyFeesAndInstanceLimits(t *testing.T) {
	upstream := httptest.NewServer(http.NotFoundHandler())
	defer upstream.Close()
	store := testServer(t, upstream).store
	settings := defaultStationPaymentSettings()
	settings.EnabledTypes = []string{"stripe", "easypay"}
	settings.RechargeFeeRate = 2.5
	settings.SubscriptionUSDToCNYRate = 7.15
	for _, tc := range []struct {
		currency, order string
		amount, want    float64
		pay             string
	}{{"CNY", "subscription", 10, 71.5, "73.29"}, {"USD", "subscription", 10, 10, "10.25"}, {"JPY", "balance", 100, 100, "103"}} {
		amount := stationGatewayAmount(tc.amount, tc.order, tc.currency, settings)
		if amount != tc.want || payment.CalculatePayAmountForCurrency(amount, settings.RechargeFeeRate, tc.currency) != tc.pay {
			t.Fatalf("currency %s: %v", tc.currency, amount)
		}
	}
	providers := []StationPaymentProvider{{ID: 1, ProviderKey: "easypay", Enabled: true, SupportedTypes: []string{"alipay"}, Limits: `{"alipay":{"singleMax":10}}`}, {ID: 2, ProviderKey: "easypay", Enabled: true, SupportedTypes: []string{"alipay"}, Limits: `{"alipay":{"singleMax":11}}`}}
	chosen, _, ok, err := store.selectStationProvider("agent-test", providers, settings, "alipay", 10)
	if err != nil || !ok || chosen.ID != 2 {
		t.Fatalf("fee-inclusive maximum not honored: %d %v %v", chosen.ID, ok, err)
	}
	settings.VisibleMethodAlipaySource = "official_alipay"
	if _, _, ok, err = store.selectStationProvider("agent-test", providers, settings, "alipay", 10); ok || err != nil {
		t.Fatal("source selection ignored")
	}
	settings.VisibleMethodAlipaySource = ""
	providers = []StationPaymentProvider{{ID: 1, ProviderKey: "stripe", Enabled: true, Config: map[string]string{"currency": "JPY"}, Limits: `{"stripe":{"singleMax":50}}`}, {ID: 2, ProviderKey: "stripe", Enabled: true, Config: map[string]string{"currency": "USD"}}}
	if _, _, ok, err = store.selectStationProvider("agent-test", providers, settings, "stripe", 100); ok || err != nil {
		t.Fatal("checkout silently changed currency")
	}
}

func TestPaymentPortCustomMethodAndInvalidProviderConfiguration(t *testing.T) {
	good := StationPaymentProvider{ProviderKey: "easypay", Name: "custom", Enabled: true, SupportedTypes: []string{"crypto"}, Config: map[string]string{"pid": "1", "pkey": "secret", "apiBase": "https://pay.example", "customMethods": `[{"type":"crypto","upstreamType":"usdt.trc20","displayName":"USDT"}]`}}
	if err := prepareStationProvider(&good, "station-a", "https://station.example"); err != nil {
		t.Fatal(err)
	}
	if methodLimitFromProvider(good, "crypto", defaultStationPaymentSettings()).DisplayName != "USDT" {
		t.Fatal("custom label missing")
	}
	for _, raw := range []string{`[{"type":"alipay_fake","upstreamType":"usdt"}]`, `[{"type":"crypto","upstreamType":"../evil"}]`, `[{"type":"crypto","upstreamType":"usdt"},{"type":"crypto","upstreamType":"coin"}]`, `{}`} {
		good.Config["customMethods"] = raw
		if err := prepareStationProvider(&good, "station-a", "https://station.example"); err == nil {
			t.Fatalf("accepted invalid custom methods %s", raw)
		}
	}
	for _, key := range []string{"easypay", "alipay", "wxpay", "stripe", "airwallex"} {
		p := StationPaymentProvider{ProviderKey: key, Name: key, Enabled: true}
		if err := prepareStationProvider(&p, "station-a", "https://station.example"); err == nil {
			t.Errorf("enabled %s with missing credentials", key)
		}
	}
	if _, err := stationCallbackURL("javascript:alert(1)", "https://a.example", "/callback"); err == nil {
		t.Fatal("invalid callback allowed")
	}
}
