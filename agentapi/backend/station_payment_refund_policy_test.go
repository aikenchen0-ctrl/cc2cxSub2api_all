package main

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestStationRefundPolicyControlsUserRequests(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("refund policy must not read the main site: %s", r.URL.Path)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	session := stationSession(t, server, "agent-test", "42", tenantRoleMember)
	provider, err := server.store.CreateStationProvider("agent-test", StationPaymentProvider{ProviderKey: "easypay", Name: "test", RefundEnabled: true, Config: map[string]string{"pkey": "secret"}})
	if err != nil {
		t.Fatal(err)
	}
	order, err := server.store.CreateStationOrder(StationPaymentOrder{OutTradeNo: "refund-policy", Amount: 10, PayAmount: 10, ProviderInstanceID: strconv.FormatInt(provider.ID, 10), Status: "COMPLETED"}, "agent-test", "42", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/agent/billing/orders/" + strconv.FormatInt(order.ID, 10) + "/refund-request"
	denied := stationRequest(server, session, http.MethodPost, path, `{"reason":"duplicate payment"}`)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("disabled policy: %d %s", denied.Code, denied.Body.String())
	}
	eligible := stationRequest(server, session, http.MethodGet, "/api/v1/agent/billing/orders/refund-eligible-providers", "")
	if eligible.Code != http.StatusOK || !strings.Contains(eligible.Body.String(), `"provider_instance_ids":[]`) {
		t.Fatalf("eligibility: %s", eligible.Body.String())
	}
	provider.AllowUserRefund = true
	if _, err := server.store.UpdateStationProvider("agent-test", provider.ID, provider, false); err != nil {
		t.Fatal(err)
	}
	eligible = stationRequest(server, session, http.MethodGet, "/api/v1/agent/billing/orders/refund-eligible-providers", "")
	if !strings.Contains(eligible.Body.String(), `"provider_instance_ids":["`+strconv.FormatInt(provider.ID, 10)+`"]`) || strings.Contains(eligible.Body.String(), "secret") {
		t.Fatalf("eligibility: %s", eligible.Body.String())
	}
	tooLong := stationRequest(server, session, http.MethodPost, path, `{"reason":"`+strings.Repeat("x", 501)+`"}`)
	if tooLong.Code != http.StatusBadRequest {
		t.Fatalf("long reason: %d", tooLong.Code)
	}
	accepted := stationRequest(server, session, http.MethodPost, path, `{"reason":"duplicate payment"}`)
	if accepted.Code != http.StatusOK {
		t.Fatalf("enabled policy: %d %s", accepted.Code, accepted.Body.String())
	}
	updated, err := server.store.StationOrder("agent-test", order.ID)
	if err != nil || updated.Status != "REFUND_REQUESTED" {
		t.Fatalf("state=%s err=%v", updated.Status, err)
	}
}
