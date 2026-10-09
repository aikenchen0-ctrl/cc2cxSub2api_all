package main

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestStationProviderRoutingStrategiesAndLimits(t *testing.T) {
	upstream := httptest.NewServer(http.NotFoundHandler())
	defer upstream.Close()
	s := testServer(t, upstream).store
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	s.clock = func() time.Time { return now }
	settings := defaultStationPaymentSettings()
	settings.EnabledTypes = []string{"easypay"}
	providers := []StationPaymentProvider{
		{ID: 1, ProviderKey: "easypay", Enabled: true, SupportedTypes: []string{"alipay"}},
		{ID: 2, ProviderKey: "easypay", Enabled: true, SupportedTypes: []string{"alipay"}},
	}
	choose := func(agent string, want int64) {
		t.Helper()
		got, _, ok, err := s.selectStationProvider(agent, providers, settings, "alipay", 10)
		if err != nil || !ok || got.ID != want {
			t.Fatalf("chosen=%d ok=%v err=%v want=%d", got.ID, ok, err, want)
		}
	}
	choose("agent-test", 1)
	choose("agent-test", 2)
	choose("other-agent", 1)
	choose("agent-test", 1)
	for _, item := range []struct {
		id      int64
		amount  float64
		expires time.Time
	}{
		{1, 100, now.Add(time.Hour)}, {2, 20, now.Add(time.Hour)}, {2, 1000, now.Add(-time.Hour)},
	} {
		_, err := s.CreateStationOrder(StationPaymentOrder{OutTradeNo: strconv.FormatFloat(item.amount, 'f', 0, 64), Amount: item.amount, PayAmount: item.amount, PaymentType: "alipay", ProviderInstanceID: strconv.FormatInt(item.id, 10), Status: "PENDING"}, "agent-test", "42", item.expires)
		if err != nil {
			t.Fatal(err)
		}
	}
	settings.LoadBalanceStrategy = "least-amount"
	choose("agent-test", 2)
	providers[1].Limits = `{"alipay":{"dailyLimit":25}}`
	choose("agent-test", 1)
	providers[1].Limits = `{"alipay":{"singleMin":11}}`
	choose("agent-test", 1)
	providers[0].Limits = `{"alipay":{"singleMax":9}}`
	_, _, ok, err := s.selectStationProvider("agent-test", providers, settings, "alipay", 10)
	if ok || err != nil {
		t.Fatalf("all channels exhausted: ok=%v err=%v", ok, err)
	}
}
