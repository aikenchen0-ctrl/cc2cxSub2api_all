package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestApplicationErrorCannotConfirmSettlementAndRecovers(t *testing.T) {
	healthy := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/sub2api/usage" || r.Header.Get("X-Sub2API-On-Behalf-Of") != "42" {
			t.Error("unexpected reconciliation request")
			w.WriteHeader(400)
			return
		}
		code := 500
		if healthy {
			code = 0
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": code, "message": "private-error", "data": map[string]any{"items": []any{map[string]any{"id": 91, "request_id": "recover", "actual_cost": 0.25}}}})
	}))
	defer upstream.Close()
	s := testServer(t, upstream)
	setTenantBillingModeForTest(t, s, "user_upstream")
	s.main = NewMainClient(s.cfg)
	if _, _, err := s.store.PrepareDirectSettlement(s.cfg.AgentID, "42", "42", "recover", "recover", 100); err != nil {
		t.Fatal(err)
	}
	if _, err := s.reconcileTenantSettlements(t.Context(), s.cfg.AgentID, "recover"); err == nil {
		t.Fatal("application failure accepted")
	}
	record, err := s.store.Settlement(s.cfg.AgentID, "recover")
	if err != nil || record.Status != "pending" || record.ActualCents != 0 {
		t.Fatalf("error confirmed a charge: %+v %v", record, err)
	}
	healthy = true
	if _, err := s.reconcileTenantSettlements(t.Context(), s.cfg.AgentID, "recover"); err != nil {
		t.Fatal(err)
	}
	record, err = s.store.Settlement(s.cfg.AgentID, "recover")
	if err != nil || record.Status != "confirmed" || record.ActualCents != 25 || record.UsageID != "91" {
		t.Fatalf("recovery failed: %+v %v", record, err)
	}
}

func TestSatelliteLookupErrorsNeverExposeUpstreamBody(t *testing.T) {
	for _, status := range []int{401, 403, 429, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"error":"private-token private-user internal-host"}`))
			}))
			defer upstream.Close()
			client := NewMainClient(Config{MainModelBaseURL: upstream.URL, AppCredential: "private-token", SatelliteSlug: "agentapi"})
			_, err := client.AdminFindUsageForUser(t.Context(), "42", "private-request")
			var mainErr *MainAPIError
			if !errors.As(err, &mainErr) || mainErr.Status != status || len(mainErr.Body) != 0 {
				t.Fatalf("expected sanitized typed error: %v", err)
			}
			for _, value := range []string{"private-token", "private-user", "internal-host", "private-request", upstream.URL} {
				if strings.Contains(err.Error(), value) {
					t.Fatal("lookup error leaked upstream details")
				}
			}
		})
	}
}

func TestSatelliteTransportErrorDoesNotExposeTarget(t *testing.T) {
	client := NewMainClient(Config{MainModelBaseURL: "://private-host", AppCredential: "private-token", SatelliteSlug: "agentapi"})
	_, err := client.AdminFindUsageForUser(t.Context(), "42", "private-request")
	var mainErr *MainAPIError
	if !errors.As(err, &mainErr) || mainErr.Code != "SATELLITE_LOOKUP_UNAVAILABLE" || strings.Contains(err.Error(), "private-") {
		t.Fatalf("unsafe transport error: %v", err)
	}
}

func TestSatelliteRejectsApplicationErrorsWithSuccessfulHTTPStatus(t *testing.T) {
	for _, body := range []string{
		`{"code":500,"message":"private-token","data":{"balance":100,"items":[{"request_id":"r","actual_cost":1}]}}`,
		`{"code":"private-error","data":{"balance":100}}`,
		`{"code":false,"data":{"balance":100}}`,
		`<html>private-token</html>`,
	} {
		t.Run(body, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
			defer upstream.Close()
			client := NewMainClient(Config{MainModelBaseURL: upstream.URL, AppCredential: "app", SatelliteSlug: "agentapi"})
			_, err := client.AdminFindUsageForUser(t.Context(), "42", "r")
			var apiErr *MainAPIError
			if !errors.As(err, &apiErr) || apiErr.Status != http.StatusBadGateway || strings.Contains(err.Error(), "private-") {
				t.Fatalf("application failure not safely rejected: %v", err)
			}
			if _, err := client.SatelliteUserBalance(t.Context(), "42"); err == nil {
				t.Fatal("error envelope used as balance")
			}
		})
	}
}

func TestSatelliteUsagePreservesSupportedSuccessFormats(t *testing.T) {
	for _, body := range []string{
		`{"items":[{"id":1,"request_id":"r","actual_cost":0.25}]}`,
		`[{"id":1,"request_id":"r","actual_cost":0.25}]`,
		`{"code":0,"data":{"items":[{"id":1,"request_id":"r","actual_cost":0.25}]}}`,
	} {
		t.Run(body, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
			defer upstream.Close()
			client := NewMainClient(Config{MainModelBaseURL: upstream.URL, AppCredential: "app", SatelliteSlug: "agentapi"})
			items, err := client.AdminFindUsageForUser(t.Context(), "42", "r")
			if err != nil || len(items) != 1 || items[0].ActualCents != 25 {
				t.Fatalf("valid usage rejected: %+v %v", items, err)
			}
		})
	}
}

func TestUsageLookupFailurePersistsOnlySafeError(t *testing.T) {
	var chargeID string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/sub2api/balance":
			_, _ = w.Write([]byte(`{"balance":1}`))
		case "/v1/chat/completions":
			chargeID = r.Header.Get("X-Request-ID")
			_, _ = w.Write([]byte(`{"choices":[]}`))
		case "/v1/sub2api/usage":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"private-token private-user internal-host"}`))
		default:
			t.Errorf("unexpected endpoint: %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer upstream.Close()
	s := testServer(t, upstream)
	setTenantBillingModeForTest(t, s, "user_upstream")
	s.cfg.MainUsageAPI = true
	s.main = NewMainClient(s.cfg)
	if _, err := s.store.UpsertUser(s.cfg.AgentID, "42", "member@example.com", "Member"); err != nil {
		t.Fatal(err)
	}
	_, key, err := s.store.CreateAPIKey(s.cfg.AgentID, "42", "safe-errors")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.5","messages":[]}`))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Idempotency-Key", "safe-errors")
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("successful model response lost: %d", rec.Code)
	}
	record, err := s.store.Settlement(s.cfg.AgentID, chargeID)
	if err != nil || record.Status != "pending" || !strings.Contains(record.Error, "SATELLITE_LOOKUP_FAILED") {
		t.Fatalf("pending safe error missing: %+v %v", record, err)
	}
	for _, secret := range []string{"private-token", "private-user", "internal-host"} {
		if strings.Contains(record.Error, secret) || strings.Contains(rec.Body.String(), secret) {
			t.Fatal("upstream details persisted or returned")
		}
	}
}
