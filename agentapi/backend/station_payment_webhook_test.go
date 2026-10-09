package main

import (
	"crypto/md5"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestStationWebhookChecksAmountAndRecoversWalletCredit(t *testing.T) {
	upstream := httptest.NewServer(http.NotFoundHandler())
	defer upstream.Close()
	server := testServer(t, upstream)
	provider, err := server.store.CreateStationProvider("agent-test", StationPaymentProvider{ProviderKey: "easypay", Name: "pay", Config: map[string]string{"pid": "1001", "pkey": "test-secret", "apiBase": "https://pay.example"}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = server.store.CreateStationOrder(StationPaymentOrder{OutTradeNo: "webhook-test", Amount: 15, PayAmount: 10, ProviderInstanceID: strconv.FormatInt(provider.ID, 10), Status: "PENDING"}, "agent-test", "42", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	notify := func(money string) *httptest.ResponseRecorder {
		values := url.Values{"money": {money}, "out_trade_no": {"webhook-test"}, "pid": {"1001"}, "trade_no": {"trade-test"}, "trade_status": {"TRADE_SUCCESS"}}
		pairs := []string{}
		for _, key := range []string{"money", "out_trade_no", "pid", "trade_no", "trade_status"} {
			pairs = append(pairs, key+"="+values.Get(key))
		}
		values.Set("sign", fmt.Sprintf("%x", md5.Sum([]byte(strings.Join(pairs, "&")+"test-secret"))))
		req := httptest.NewRequest(http.MethodPost, "http://agent.local/api/v1/agent/billing/webhook/agent-test/easypay", strings.NewReader(values.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		return rec
	}
	bad := notify("9.99")
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("wrong amount: %d %s", bad.Code, bad.Body.String())
	}
	order, _ := server.store.StationOrderByNo("agent-test", "webhook-test")
	if order.Status != "PENDING" {
		t.Fatalf("wrong amount completed order: %s", order.Status)
	}
	// Simulate the previous failure boundary: order committed, wallet not yet credited.
	if _, _, err := server.store.CompleteStationOrder("agent-test", "webhook-test", "trade-test"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		rec := notify("10.00")
		if rec.Code != http.StatusOK || rec.Body.String() != "success" {
			t.Fatalf("notify: %d %s", rec.Code, rec.Body.String())
		}
	}
	var count int
	var credit int64
	if err := server.store.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(amount_cents),0) FROM wallet_ledger WHERE agent_id='agent-test' AND kind='user_recharge' AND request_id='recharge:webhook-test'`).Scan(&count, &credit); err != nil {
		t.Fatal(err)
	}
	if count != 1 || credit != 1500 {
		t.Fatalf("wallet count=%d credit=%d", count, credit)
	}
}
