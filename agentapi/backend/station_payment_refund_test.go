package main

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/agentapi/payment"
)

func TestStationRefundCallsProviderAndDebitsWalletOnce(t *testing.T) {
	calls := 0
	providerAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Query().Get("act") != "refund" {
			t.Errorf("unexpected request %s", r.URL)
		}
		_ = r.ParseForm()
		if r.Form.Get("money") != "10.00" || r.Form.Get("out_trade_no") != "refund-test" {
			t.Errorf("incorrect refund amount/order")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":1}`))
	}))
	defer providerAPI.Close()
	server := testServer(t, providerAPI)
	session := stationSession(t, server, "agent-test", "42", tenantRoleOwner)
	provider, err := server.store.CreateStationProvider("agent-test", StationPaymentProvider{ProviderKey: "easypay", Name: "pay", RefundEnabled: true, Config: map[string]string{"pid": "1001", "pkey": "test", "apiBase": providerAPI.URL}})
	if err != nil {
		t.Fatal(err)
	}
	order, err := server.store.CreateStationOrder(StationPaymentOrder{OutTradeNo: "refund-test", Amount: 15, PayAmount: 10, ProviderInstanceID: strconv.FormatInt(provider.ID, 10), Status: "PENDING"}, "agent-test", "42", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := server.store.CompleteStationOrder("agent-test", order.OutTradeNo, "trade-test"); err != nil {
		t.Fatal(err)
	}
	if err := server.store.CreditUserRecharge("agent-test", "42", 1500, order.OutTradeNo); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		response := stationRequest(server, session, http.MethodPost, "/api/v1/agent/admin/billing/orders/"+strconv.FormatInt(order.ID, 10)+"/refund", `{}`)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"REFUNDED"`) {
			t.Fatalf("refund %d: %d %s", i, response.Code, response.Body.String())
		}
	}
	if calls != 1 {
		t.Fatalf("provider calls=%d", calls)
	}
	var balance int64
	if err := server.store.db.QueryRow(`SELECT balance_cents FROM agent_user_wallets WHERE agent_id='agent-test' AND main_user_id='42'`).Scan(&balance); err != nil {
		t.Fatal(err)
	}
	if balance != 0 {
		t.Fatalf("balance=%d", balance)
	}
}

func TestStationRefundFailureReleasesReservationOnce(t *testing.T) {
	upstream := httptest.NewServer(http.NotFoundHandler())
	defer upstream.Close()
	server := testServer(t, upstream)
	order, err := server.store.CreateStationOrder(StationPaymentOrder{OutTradeNo: "failed-refund", Amount: 10, PayAmount: 10, Status: "COMPLETED"}, "agent-test", "42", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := server.store.CreditUserRecharge("agent-test", "42", 1000, order.OutTradeNo); err != nil {
		t.Fatal(err)
	}
	if err := server.store.beginStationRefund("agent-test", order, StationPaymentProvider{ProviderKey: "easypay", Config: map[string]string{}}); err != nil {
		t.Fatal(err)
	}
	if err := server.store.beginStationRefund("agent-test", order, StationPaymentProvider{}); err == nil {
		t.Fatal("duplicate refund accepted")
	}
	if err := server.store.finishStationRefund("agent-test", order, nil); err != nil {
		t.Fatal(err)
	}
	current, _ := server.store.StationOrder("agent-test", order.ID)
	if current.Status != "REFUND_PENDING" {
		t.Fatalf("ambiguous result=%s", current.Status)
	}
	for i := 0; i < 2; i++ {
		if err := server.store.finishStationRefund("agent-test", order, &payment.RefundResponse{Status: payment.ProviderStatusFailed}); err != nil {
			t.Fatal(err)
		}
	}
	var balance int64
	if err := server.store.db.QueryRow(`SELECT balance_cents FROM agent_user_wallets WHERE agent_id='agent-test' AND main_user_id='42'`).Scan(&balance); err != nil {
		t.Fatal(err)
	}
	if balance != 1000 {
		t.Fatalf("restored balance=%d", balance)
	}
}
