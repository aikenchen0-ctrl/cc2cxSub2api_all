package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestUsageInsightsHandlerUsesSessionScope(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("insights must not request unscoped upstream data")
	}))
	defer upstream.Close()
	s := testServer(t, upstream)
	if _, err := s.store.UpsertUser(s.cfg.AgentID, "43", "other@example.com", "Other"); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	s.store.clock = func() time.Time { return now.Add(-time.Minute) }
	for _, id := range []string{"42", "43"} {
		if _, _, err := s.store.PrepareDirectSettlement(s.cfg.AgentID, id, id, "request-"+id, "usage-"+id, 100); err != nil {
			t.Fatal(err)
		}
	}
	s.store.clock = func() time.Time { return now }
	for _, tc := range []struct {
		user, window string
		code         int
		requests     int64
	}{
		{"", "24h", 401, 0}, {"43", "invalid", 400, 0}, {"43", "24h", 200, 1}, {"42", "24h", 200, 2},
	} {
		req := httptest.NewRequest("GET", "/api/v1/agent/usage/insights?window="+tc.window+"&main_user_id=42&agent_id=other-agent", nil)
		if tc.user != "" {
			cookie, err := s.store.CreateSession(tc.user, []byte(`{"id":"`+tc.user+`"}`), "", "", now.Add(time.Hour))
			if err != nil {
				t.Fatal(err)
			}
			req.AddCookie(&http.Cookie{Name: s.cfg.CookieName, Value: cookie})
		}
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, req)
		if rec.Code != tc.code {
			t.Fatalf("user=%s status=%d body=%s", tc.user, rec.Code, rec.Body.String())
		}
		if tc.code == 200 {
			var response struct {
				Data UsageInsights `json:"data"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.Data.Requests != tc.requests {
				t.Fatalf("scope leaked: %+v", response.Data)
			}
			if rec.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("missing no-store")
			}
		}
	}
}

func TestUsageInsightsIsolationWindowAndMissingActual(t *testing.T) {
	s := testStore(t)
	start := time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	mismatch := true
	snapshot := MainUsageSnapshot{Source: "sub2api_user_usage", ActualCostReported: true, ActualCostNanos: 123, TotalCostNanos: 456, InputTokens: 7, UpstreamModelMismatch: &mismatch}
	for _, row := range []struct {
		agent, user, id string
		at              time.Time
		snap            MainUsageSnapshot
	}{
		{"agent-test", "42", "included", start, snapshot},
		{"agent-test", "42", "missing", start.Add(time.Minute), MainUsageSnapshot{Source: "sub2api_user_usage", TotalCostNanos: 20}},
		{"agent-test", "42", "pending", start.Add(time.Minute), MainUsageSnapshot{}},
		{"agent-test", "42", "end-exclusive", end, snapshot},
		{"agent-test", "42", "before", start.Add(-time.Second), snapshot},
		{"agent-test", "other", "other-user", start, snapshot},
		{"other-agent", "42", "other-agent", start, snapshot},
	} {
		s.clock = func() time.Time { return row.at }
		if _, _, err := s.PrepareDirectSettlement(row.agent, row.user, row.user, row.id, row.id, 100); err != nil {
			t.Fatal(err)
		}
		if err := s.FinalizeSettlement(row.agent, row.id, row.id, "pending", 0, "", row.snap); err != nil {
			t.Fatal(err)
		}
	}
	result, err := s.UsageInsights("agent-test", "42", start, end)
	if err != nil {
		t.Fatal(err)
	}
	if result.Requests != 3 || result.Measured != 2 || result.ActualCost != 123 || result.StandardCost != 476 || result.MissingActual != 1 || result.Unobserved != 1 || result.Status != "partial" || result.RouteMismatch != 1 || result.InputTokens != 7 {
		t.Fatalf("unexpected insights: %+v", result)
	}
	if len(result.Trend) != 1 || result.Trend[0].Requests != 3 {
		t.Fatalf("bad trend: %+v", result.Trend)
	}
	admin, err := s.UsageInsights("agent-test", "", start, end)
	if err != nil || admin.Requests != 4 || admin.Scope != "agent" {
		t.Fatalf("admin scope: %+v %v", admin, err)
	}
	empty, err := s.UsageInsights("agent-test", "unknown", start, end)
	if err != nil || empty.Status != "empty" || empty.Requests != 0 {
		t.Fatalf("empty: %+v %v", empty, err)
	}
}

func TestUsageMetricsDoesNotPromoteEstimatesOrMissingActual(t *testing.T) {
	var metrics UsageMetrics
	metrics.add("confirmed", MainUsageSnapshot{Source: "user_balance_delta_fallback", ActualCostReported: true, ActualCostNanos: 999})
	metrics.add("confirmed", MainUsageSnapshot{Source: "sub2api_user_usage", ActualCostReported: true, ActualCostNanos: 0})
	if metrics.Measured != 1 || metrics.Unobserved != 1 || metrics.ActualCost != 0 || metrics.MissingActual != 0 {
		t.Fatalf("incorrect provenance: %+v", metrics)
	}
}
