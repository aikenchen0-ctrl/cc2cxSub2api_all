package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestStationCancellationQuotaAndWindow(t *testing.T) {
	upstream := httptest.NewServer(http.NotFoundHandler())
	defer upstream.Close()
	server := testServer(t, upstream)
	now := time.Date(2026, 10, 2, 12, 30, 0, 0, time.UTC)
	server.store.clock = func() time.Time { return now }
	settings := defaultStationPaymentSettings()
	settings.CancelRateLimitEnabled = true
	settings.CancelRateLimitMax = 1
	settings.CancelRateLimitUnit = "hour"
	order, err := server.store.CreateStationOrder(StationPaymentOrder{OutTradeNo: "cancel-test", Amount: 10, PayAmount: 10, PaymentType: "alipay", OrderType: "balance", Status: "PENDING"}, "agent-test", "42", now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := server.store.cancelStationUserOrder("agent-test", "another-user", order.ID, "wrong-user"); err == nil {
		t.Fatal("another user cancelled the order")
	}
	if err := server.store.cancelStationUserOrder("agent-test", "42", order.ID, "cancel-request"); err != nil {
		t.Fatal(err)
	}
	if err := server.store.cancelStationUserOrder("agent-test", "42", order.ID, "duplicate"); err == nil {
		t.Fatal("duplicate cancellation succeeded")
	}
	for _, tc := range []struct {
		agent, user string
		want        bool
	}{
		{"agent-test", "42", true}, {"agent-test", "another-user", false}, {"another-agent", "42", false},
	} {
		got, err := server.store.stationCancelRateLimited(tc.agent, tc.user, settings)
		if err != nil || got != tc.want {
			t.Fatalf("%+v got=%v err=%v", tc, got, err)
		}
	}
	now = now.Add(31 * time.Minute)
	settings.CancelRateLimitMode = "fixed"
	if limited, err := server.store.stationCancelRateLimited("agent-test", "42", settings); err != nil || limited {
		t.Fatalf("new fixed hour limited=%v err=%v", limited, err)
	}
	settings.CancelRateLimitMode = "rolling"
	if limited, err := server.store.stationCancelRateLimited("agent-test", "42", settings); err != nil || !limited {
		t.Fatalf("rolling window limited=%v err=%v", limited, err)
	}
	now = now.Add(30 * time.Minute)
	if limited, err := server.store.stationCancelRateLimited("agent-test", "42", settings); err != nil || limited {
		t.Fatalf("expired window limited=%v err=%v", limited, err)
	}
}
