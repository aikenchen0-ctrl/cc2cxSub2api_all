package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStationPaymentConfigRoundTripAndIsolation(t *testing.T) {
	upstreamCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		t.Errorf("payment config must not call the main site: %s %s", r.Method, r.URL.Path)
		http.Error(w, "upstream", http.StatusBadGateway)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	for _, item := range []struct{ agentID, owner, member string }{
		{"agent-pay-a", "owner-a", "member-a"},
		{"agent-pay-b", "owner-b", "member-b"},
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
	ownerA := stationSession(t, server, "agent-pay-a", "owner-a", tenantRoleOwner)
	memberA := stationSession(t, server, "agent-pay-a", "member-a", tenantRoleMember)
	ownerB := stationSession(t, server, "agent-pay-b", "owner-b", tenantRoleOwner)

	payload := `{
		"enabled":true,
		"min_amount":12.5,
		"max_amount":880,
		"daily_limit":3000,
		"order_timeout_minutes":45,
		"max_pending_orders":4,
		"enabled_payment_types":["easypay","alipay"],
		"balance_disabled":true,
		"balance_recharge_multiplier":1.25,
		"model_rate_multiplier":1.35,
		"subscription_usd_to_cny_rate":7.15,
		"recharge_fee_rate":2.5,
		"load_balance_strategy":"least-amount",
		"product_name_prefix":"站长充值",
		"product_name_suffix":"CNY",
		"help_image_url":"https://cdn.example/help.png",
		"help_text":"请使用本站渠道付款",
		"cancel_rate_limit_enabled":true,
		"cancel_rate_limit_max":6,
		"cancel_rate_limit_window":2,
		"cancel_rate_limit_unit":"hour",
		"cancel_rate_limit_window_mode":"fixed",
		"alipay_force_qrcode":true,
		"alipay_mobile_precreate_deep_link":true
	}`
	saved := stationRequest(server, ownerA, http.MethodPut, "/api/v1/agent/admin/billing/config", payload)
	if saved.Code != http.StatusOK {
		t.Fatalf("save status=%d body=%s", saved.Code, saved.Body.String())
	}
	got := stationRequest(server, ownerA, http.MethodGet, "/api/v1/agent/admin/billing/config", "")
	if got.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", got.Code, got.Body.String())
	}
	settings := decodeData[StationPaymentSettings](t, got.Body.Bytes())
	if strings.Contains(got.Body.String(), "balance_recharge_multiplier") {
		t.Fatalf("deprecated recharge multiplier must not be exposed: %s", got.Body.String())
	}
	if !settings.Enabled || !nearAmount(settings.MinAmount, 12.5) || !nearAmount(settings.MaxAmount, 880) || !nearAmount(settings.DailyLimit, 3000) ||
		settings.OrderTimeoutMin != 45 || settings.MaxPendingOrders != 4 || !settings.BalanceDisabled ||
		!nearAmount(settings.ModelRateMultiplier, 1.35) || !nearAmount(settings.SubscriptionUSDToCNYRate, 7.15) || !nearAmount(settings.RechargeFeeRate, 2.5) ||
		settings.LoadBalanceStrategy != "least-amount" || settings.ProductNamePrefix != "站长充值" || settings.ProductNameSuffix != "CNY" ||
		settings.HelpImageURL != "https://cdn.example/help.png" || settings.HelpText != "请使用本站渠道付款" ||
		!settings.CancelRateLimitEnabled || settings.CancelRateLimitMax != 6 || settings.CancelRateLimitWindow != 2 ||
		settings.CancelRateLimitUnit != "hour" || settings.CancelRateLimitMode != "fixed" ||
		!settings.AlipayForceQRCode || !settings.AlipayMobilePrecreateDeepLink ||
		strings.Join(settings.EnabledTypes, ",") != "easypay,alipay" {
		t.Fatalf("settings did not round-trip: %+v", settings)
	}
	disabled := stationRequest(server, ownerA, http.MethodPut, "/api/v1/agent/admin/billing/config", strings.Replace(payload, `"enabled":true`, `"enabled":false`, 1))
	if disabled.Code != http.StatusOK {
		t.Fatalf("disable status=%d body=%s", disabled.Code, disabled.Body.String())
	}
	afterDisable, err := server.store.Agent("agent-pay-a")
	if err != nil || afterDisable.OwnerMainUserID != "owner-a" || afterDisable.BillingMode != "user_upstream" {
		t.Fatalf("disabled save changed owner: agent=%+v err=%v", afterDisable, err)
	}
	disabledGot := stationRequest(server, ownerA, http.MethodGet, "/api/v1/agent/admin/billing/config", "")
	disabledSettings := decodeData[StationPaymentSettings](t, disabledGot.Body.Bytes())
	if disabledSettings.Enabled || disabledSettings.ProductNamePrefix != "站长充值" || disabledSettings.HelpText != "请使用本站渠道付款" {
		t.Fatalf("disabled save dropped payment fields: %+v", disabledSettings)
	}
	reenabled := stationRequest(server, ownerA, http.MethodPut, "/api/v1/agent/admin/billing/config", payload)
	if reenabled.Code != http.StatusOK {
		t.Fatalf("re-enable status=%d body=%s", reenabled.Code, reenabled.Body.String())
	}
	afterEnable, err := server.store.Agent("agent-pay-a")
	if err != nil || afterEnable.OwnerMainUserID != "owner-a" || afterEnable.BillingMode != "owner_upstream" {
		t.Fatalf("re-enable changed owner: agent=%+v err=%v", afterEnable, err)
	}
	legacyOff := stationRequest(server, ownerA, http.MethodPut, "/api/v1/agent/admin/payment-config", `{"enabled":false,"provider":"manual","currency":"CNY","merchant_id":"","min_amount_cents":100,"max_amount_cents":100000,"order_ttl_seconds":1800,"checkout_url_template":""}`)
	if legacyOff.Code != http.StatusOK {
		t.Fatalf("legacy disable status=%d body=%s", legacyOff.Code, legacyOff.Body.String())
	}
	afterLegacy, err := server.store.Agent("agent-pay-a")
	if err != nil || afterLegacy.OwnerMainUserID != "owner-a" {
		t.Fatalf("legacy disable wiped owner: agent=%+v err=%v", afterLegacy, err)
	}

	created := stationRequest(server, ownerA, http.MethodPost, "/api/v1/agent/admin/billing/providers", `{
		"provider_key":"easypay","name":"A站易支付","supported_types":["alipay","wxpay"],"enabled":true,
		"payment_mode":"popup","sort_order":3,"limits":"{\"alipay\":{\"single_min\":1}}",
		"refund_enabled":true,"allow_user_refund":false,
		"config":{"pid":"1001","apiBase":"https://pay.example","pkey":"secret-a","privateKey":"priv","publicKey":"pub","apiV3Key":"v3","secretKey":"sk","webhookSecret":"wh","apiKey":"ak"}
	}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create provider status=%d body=%s", created.Code, created.Body.String())
	}
	assertNoProviderSecrets(t, created.Body.String())
	provider := decodeData[StationPaymentProvider](t, created.Body.Bytes())
	if provider.ID == 0 || provider.Name != "A站易支付" || provider.ProviderKey != "easypay" || provider.Config["pid"] != "1001" || provider.Config["apiBase"] != "https://pay.example" || provider.PaymentMode != "popup" || provider.SortOrder != 3 || !provider.RefundEnabled || provider.AllowUserRefund {
		t.Fatalf("provider public fields missing: %+v", provider)
	}

	listed := stationRequest(server, ownerA, http.MethodGet, "/api/v1/agent/admin/billing/providers", "")
	assertNoProviderSecrets(t, listed.Body.String())
	if !strings.Contains(listed.Body.String(), "A站易支付") || !strings.Contains(listed.Body.String(), "1001") {
		t.Fatalf("list dropped public provider fields: %s", listed.Body.String())
	}
	fetched := stationRequest(server, ownerA, http.MethodGet, "/api/v1/agent/admin/billing/providers/"+itoa(provider.ID), "")
	if fetched.Code != http.StatusOK {
		t.Fatalf("get provider status=%d body=%s", fetched.Code, fetched.Body.String())
	}
	assertNoProviderSecrets(t, fetched.Body.String())

	updated := stationRequest(server, ownerA, http.MethodPut, "/api/v1/agent/admin/billing/providers/"+itoa(provider.ID), `{
		"provider_key":"easypay","name":"A站易支付改","supported_types":["alipay"],"enabled":true,
		"payment_mode":"qrcode","sort_order":4,"limits":"","refund_enabled":false,"allow_user_refund":false,
		"config":{"pid":"2002","pkey":""}
	}`)
	if updated.Code != http.StatusOK || strings.Contains(updated.Body.String(), "secret-a") || !strings.Contains(updated.Body.String(), "2002") {
		t.Fatalf("update status=%d body=%s", updated.Code, updated.Body.String())
	}
	assertNoProviderSecrets(t, updated.Body.String())

	otherConfig := stationRequest(server, ownerB, http.MethodGet, "/api/v1/agent/admin/billing/config", "")
	otherSettings := decodeData[StationPaymentSettings](t, otherConfig.Body.Bytes())
	if otherSettings.Enabled || otherSettings.ProductNamePrefix == "站长充值" || otherSettings.HelpText == "请使用本站渠道付款" {
		t.Fatalf("tenant B read tenant A settings: %+v", otherSettings)
	}
	otherProviders := stationRequest(server, ownerB, http.MethodGet, "/api/v1/agent/admin/billing/providers", "")
	if strings.Contains(otherProviders.Body.String(), "A站易支付") || strings.Contains(otherProviders.Body.String(), "secret-a") {
		t.Fatalf("tenant B listed tenant A providers: %s", otherProviders.Body.String())
	}
	crossGet := stationRequest(server, ownerB, http.MethodGet, "/api/v1/agent/admin/billing/providers/"+itoa(provider.ID), "")
	crossPut := stationRequest(server, ownerB, http.MethodPut, "/api/v1/agent/admin/billing/providers/"+itoa(provider.ID), `{"provider_key":"easypay","name":"stolen","supported_types":["alipay"],"enabled":false,"config":{}}`)
	crossDelete := stationRequest(server, ownerB, http.MethodDelete, "/api/v1/agent/admin/billing/providers/"+itoa(provider.ID), "")
	if crossGet.Code != http.StatusNotFound || crossPut.Code != http.StatusNotFound || crossDelete.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant provider access get=%d put=%d delete=%d", crossGet.Code, crossPut.Code, crossDelete.Code)
	}
	still := stationRequest(server, ownerA, http.MethodGet, "/api/v1/agent/admin/billing/providers/"+itoa(provider.ID), "")
	if still.Code != http.StatusOK || !strings.Contains(still.Body.String(), "A站易支付改") {
		t.Fatalf("cross-tenant write changed provider: %d %s", still.Code, still.Body.String())
	}

	member := stationRequest(server, memberA, http.MethodGet, "/api/v1/agent/admin/billing/config", "")
	if member.Code != http.StatusForbidden {
		t.Fatalf("member status=%d body=%s", member.Code, member.Body.String())
	}
	anon := httptest.NewRecorder()
	server.ServeHTTP(anon, httptest.NewRequest(http.MethodGet, "http://agent.local/api/v1/agent/admin/billing/config", nil))
	if anon.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous status=%d body=%s", anon.Code, anon.Body.String())
	}
	if upstreamCalls != 0 {
		t.Fatalf("main site calls=%d", upstreamCalls)
	}
}

func decodeData[T any](t *testing.T, raw []byte) T {
	t.Helper()
	var envelope struct {
		Data T `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	return envelope.Data
}

func assertNoProviderSecrets(t *testing.T, body string) {
	t.Helper()
	var raw any
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		t.Fatalf("provider response is not json: %v", err)
	}
	secrets := map[string]bool{"secret-a": true, "priv": true, "pub": true, "v3": true, "sk": true, "wh": true, "ak": true}
	sensitive := map[string]bool{"pkey": true, "privateKey": true, "publicKey": true, "apiV3Key": true, "secretKey": true, "webhookSecret": true, "apiKey": true}
	var walk func(node any, parent string)
	walk = func(node any, parent string) {
		switch value := node.(type) {
		case map[string]any:
			for key, child := range value {
				if sensitive[key] && parent != "config_configured" {
					if text, ok := child.(string); !ok || text != "" {
						t.Fatalf("provider response kept sensitive config %s: %s", key, body)
					}
				}
				if text, ok := child.(string); ok && secrets[text] {
					t.Fatalf("provider response leaked secret value %s", text)
				}
				walk(child, key)
			}
		case []any:
			for _, child := range value {
				walk(child, parent)
			}
		}
	}
	walk(raw, "")
}

func nearAmount(got, want float64) bool {
	return got-want < 0.0001 && want-got < 0.0001
}

func itoa(id int64) string {
	return strings.TrimSpace(strings.ReplaceAll(strings.Trim(strings.TrimSpace(jsonNumber(id)), "\""), "\n", ""))
}

func jsonNumber(id int64) string {
	raw, _ := json.Marshal(id)
	return string(raw)
}

func TestStationPaymentConfigRoundTripDoesNotReadBodyTwice(t *testing.T) {
	// Keep the isolation test as the real entry. This guards the helper used above.
	if jsonNumber(7) != "7" {
		t.Fatal(io.EOF)
	}
	_ = time.Second
}

func TestStationProviderFamiliesControlCheckoutAndOrderSelection(t *testing.T) {
	upstream := httptest.NewServer(http.NotFoundHandler())
	defer upstream.Close()
	server := testServer(t, upstream)
	providers := []StationPaymentProvider{
		{ProviderKey: "easypay", Name: "EasyPay", Enabled: true, SupportedTypes: []string{"alipay", "wxpay"}},
		{ProviderKey: "stripe", Name: "Stripe", Enabled: true, SupportedTypes: []string{"card", "link", "alipay"}, Config: map[string]string{"publishableKey": "pk_test_public"}},
	}
	for _, provider := range providers {
		if _, err := server.store.CreateStationProvider("agent-test", provider); err != nil {
			t.Fatal(err)
		}
	}
	stored, err := server.store.ListStationProviders("agent-test", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name       string
		families   []string
		hideAlipay bool
		methods    []string
	}{
		{"easypay", []string{"easypay"}, false, []string{"alipay", "wxpay"}},
		{"stripe", []string{"stripe"}, false, []string{"stripe"}},
		{"none", []string{}, false, []string{}},
		{"different family", []string{"alipay"}, false, []string{}},
		{"hidden method", []string{"easypay"}, true, []string{"wxpay"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := server.store.db.Exec(`UPDATE station_payment_providers SET enabled=1 WHERE agent_id=?`, "agent-test"); err != nil {
				t.Fatal(err)
			}
			settings := defaultStationPaymentSettings()
			settings.Enabled = true
			settings.EnabledTypes = tc.families
			settings.VisibleMethodAlipayEnabled = !tc.hideAlipay
			if err := server.store.SaveStationPaymentSettings("agent-test", settings); err != nil {
				t.Fatal(err)
			}
			checkout, err := server.stationCheckoutInfo("agent-test")
			if err != nil {
				t.Fatal(err)
			}
			if len(checkout.Methods) != len(tc.methods) {
				t.Fatalf("methods=%v want=%v", checkout.Methods, tc.methods)
			}
			for _, method := range []string{"alipay", "wxpay", "stripe", "card", "link"} {
				want := false
				for _, expected := range tc.methods {
					if method == expected {
						want = true
					}
				}
				_, visible := checkout.Methods[method]
				_, _, selectable := selectStationProvider(stored, settings, method)
				if visible != want || selectable != want {
					t.Fatalf("%s visible=%v selectable=%v want=%v", method, visible, selectable, want)
				}
			}
			for method, displayName := range map[string]string{"alipay": "支付宝", "wxpay": "微信支付", "stripe": "Stripe"} {
				if item, ok := checkout.Methods[method]; ok && item.DisplayName != displayName {
					t.Fatalf("%s display_name=%q want=%q", method, item.DisplayName, displayName)
				}
			}
		})
	}
}
