package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInvalidMainBalanceDoesNotForwardModelRequest(t *testing.T) {
	for _, body := range []string{`{}`, `{"balance":null}`, `{"balance":"invalid"}`} {
		t.Run(body, func(t *testing.T) {
			modelCalls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/sub2api/balance" {
					_, _ = w.Write([]byte(body))
					return
				}
				modelCalls++
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer upstream.Close()
			s := testServer(t, upstream)
			setTenantBillingModeForTest(t, s, "user_upstream")
			s.main = NewMainClient(s.cfg)
			_, key, err := s.store.CreateAPIKey(s.cfg.AgentID, "42", "balance test")
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.5","messages":[]}`))
			req.Header.Set("Authorization", "Bearer "+key)
			rec := httptest.NewRecorder()
			s.ServeHTTP(rec, req)
			if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "BILLING_BALANCE_UNAVAILABLE") || modelCalls != 0 {
				t.Fatalf("invalid balance: status=%d calls=%d response=%s", rec.Code, modelCalls, rec.Body.String())
			}
		})
	}
}

func TestMainBalanceValidityAndPositivePrecision(t *testing.T) {
	for _, tc := range []struct {
		body     string
		cents    int64
		positive bool
		valid    bool
	}{
		{`{"balance":0.0001}`, 0, true, true},
		{`{"balance":"0.0000000001"}`, 0, true, true},
		{`{"balance":0}`, 0, false, true},
		{`{"balance":-0.01}`, -1, false, true},
		{`{"balance":12.34}`, 1234, true, true},
		{`{}`, 0, false, false},
		{`{"balance":null}`, 0, false, false},
		{`{"balance":"invalid"}`, 0, false, false},
		{`{"balance":"NaN"}`, 0, false, false},
		{`{"balance":"Infinity"}`, 0, false, false},
	} {
		t.Run(tc.body, func(t *testing.T) {
			cents, positive, err := decodeMainBalance(json.RawMessage(tc.body))
			if (err == nil) != tc.valid || cents != tc.cents || positive != tc.positive {
				t.Fatalf("got cents=%d positive=%v err=%v", cents, positive, err)
			}
		})
	}
}

func TestMainBalanceFactsPreserveFrozenBalanceAndLegacyCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name        string
		body        string
		balance     int64
		frozen      int64
		shouldError bool
	}{
		{name: "current response", body: `{"balance":12.34,"frozen_balance":2.5}`, balance: 1234, frozen: 250},
		{name: "string precision", body: `{"balance":"0.01","frozen_balance":"0.0001"}`, balance: 1, frozen: 0},
		{name: "legacy response", body: `{"balance":12.34}`, balance: 1234, frozen: 0},
		{name: "invalid frozen balance", body: `{"balance":12.34,"frozen_balance":"invalid"}`, shouldError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			balance, frozen, _, err := decodeMainBalanceFacts(json.RawMessage(tc.body))
			if (err != nil) != tc.shouldError || balance != tc.balance || frozen != tc.frozen {
				t.Fatalf("got balance=%d frozen=%d err=%v", balance, frozen, err)
			}
		})
	}
}
