package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAPIKeyUsageIsKeyScopedAndUsesAuthoritativeSnapshots(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agent-runtime/owner" {
			t.Fatalf("unexpected upstream path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 42, "email": "u@example.com", "balance": 123.45}))
	}))
	defer upstream.Close()
	server := testServer(t, upstream)

	first, firstRaw, err := server.store.CreateAPIKey(server.cfg.AgentID, "42", "first key")
	if err != nil {
		t.Fatal(err)
	}
	_, secondRaw, err := server.store.CreateAPIKey(server.cfg.AgentID, "42", "second key")
	if err != nil {
		t.Fatal(err)
	}
	requestID := "usage-key-one"
	if _, _, err := server.store.PrepareDirectSettlement(server.cfg.AgentID, "42", "42", requestID, requestID, 100); err != nil {
		t.Fatal(err)
	}
	if err := server.store.SetSettlementAPIKey(server.cfg.AgentID, requestID, first.ID); err != nil {
		t.Fatal(err)
	}
	if err := server.store.SetSettlementModel(server.cfg.AgentID, requestID, "gpt-5.5"); err != nil {
		t.Fatal(err)
	}
	mismatch := false
	if err := server.store.FinalizeSettlement(server.cfg.AgentID, requestID, requestID, "confirmed", 25, "", MainUsageSnapshot{
		Source: "sub2api_user_usage", InputTokens: 100, OutputTokens: 25,
		CacheReadTokens: 10, CacheCreationTokens: 5, ActualCostNanos: 250_000_000,
		ActualCostReported: true, UpstreamModelMismatch: &mismatch,
	}); err != nil {
		t.Fatal(err)
	}

	query := func(raw string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/v1/usage?days=7&timezone=UTC", nil)
		req.Header.Set("Authorization", "Bearer "+raw)
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		return rec
	}
	firstResponse := query(firstRaw)
	if firstResponse.Code != http.StatusOK {
		t.Fatalf("first key status=%d body=%s", firstResponse.Code, firstResponse.Body.String())
	}
	if firstResponse.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("key usage response is cacheable: %v", firstResponse.Header())
	}
	if strings.Contains(firstResponse.Body.String(), firstRaw) || strings.Contains(firstResponse.Body.String(), "main_user") {
		t.Fatalf("key usage leaked a credential or main identity: %s", firstResponse.Body.String())
	}
	var payload struct {
		Balance float64 `json:"balance"`
		Key     struct {
			Name   string `json:"name"`
			Prefix string `json:"prefix"`
		} `json:"key"`
		Usage struct {
			Total keyUsageStats `json:"total"`
		} `json:"usage"`
		Models []map[string]any `json:"model_stats"`
	}
	if err := json.Unmarshal(firstResponse.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Balance != 123.45 || payload.Key.Name != "first key" || payload.Key.Prefix != first.Prefix {
		t.Fatalf("unexpected safe key metadata: %#v", payload)
	}
	if payload.Usage.Total.Requests != 1 || payload.Usage.Total.InputTokens != 100 || payload.Usage.Total.ActualCost != .25 || len(payload.Models) != 1 {
		t.Fatalf("unexpected authoritative usage: %#v", payload)
	}

	secondResponse := query(secondRaw)
	if secondResponse.Code != http.StatusOK || strings.Contains(secondResponse.Body.String(), "gpt-5.5") {
		t.Fatalf("second key observed first key usage: status=%d body=%s", secondResponse.Code, secondResponse.Body.String())
	}
}

func TestAPIKeyUsageRejectsInvalidRevokedAndUnboundedQueries(t *testing.T) {
	upstream := httptest.NewServer(http.NotFoundHandler())
	defer upstream.Close()
	server := testServer(t, upstream)
	key, raw, err := server.store.CreateAPIKey(server.cfg.AgentID, "42", "revoked")
	if err != nil {
		t.Fatal(err)
	}
	if err := server.store.RevokeAPIKey(server.cfg.AgentID, "42", key.ID); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"not-an-agent-key", raw} {
		req := httptest.NewRequest(http.MethodGet, "/v1/usage", nil)
		req.Header.Set("Authorization", "Bearer "+value)
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "INVALID_AGENT_API_KEY") {
			t.Fatalf("invalid key status=%d body=%s", rec.Code, rec.Body.String())
		}
	}
	_, active, err := server.store.CreateAPIKey(server.cfg.AgentID, "42", "active")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/usage?start_date=2026-01-01&end_date=2026-09-29", nil)
	req.Header.Set("Authorization", "Bearer "+active)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "INVALID_DATE_RANGE") {
		t.Fatalf("unbounded range status=%d body=%s", rec.Code, rec.Body.String())
	}

	post := httptest.NewRequest(http.MethodPost, "/v1/usage", strings.NewReader(`{}`))
	post.Header.Set("Authorization", "Bearer "+active)
	postResponse := httptest.NewRecorder()
	server.ServeHTTP(postResponse, post)
	if postResponse.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status=%d body=%s", postResponse.Code, postResponse.Body.String())
	}
}
