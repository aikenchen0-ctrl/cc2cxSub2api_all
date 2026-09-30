package main

import (
	"encoding/json"
	"net/http"
	"sort"
	"time"
)

// Adapted from cost-console's exact-window and provenance approach. These are
// local copies of main-site facts, not a new billing ledger or all main usage.
type UsageMetrics struct {
	Requests            int64 `json:"requests"`
	Measured            int64 `json:"measured"`
	MissingActual       int64 `json:"missing_actual"`
	Unobserved          int64 `json:"unobserved"`
	Pending             int64 `json:"pending"`
	InputTokens         int64 `json:"input_tokens"`
	OutputTokens        int64 `json:"output_tokens"`
	CacheReadTokens     int64 `json:"cache_read_tokens"`
	CacheCreationTokens int64 `json:"cache_creation_tokens"`
	StandardCost        int64 `json:"standard_cost_usd_nanos"`
	ActualCost          int64 `json:"actual_cost_usd_nanos"`
	RouteObserved       int64 `json:"route_observed"`
	RouteMismatch       int64 `json:"route_mismatch"`
}

type UsageMetricRow struct {
	Key string `json:"key"`
	UsageMetrics
}

type UsageInsights struct {
	Source    string    `json:"source"`
	Status    string    `json:"status"`
	Start     time.Time `json:"start"`
	End       time.Time `json:"end"`
	UpdatedAt time.Time `json:"updated_at"`
	Scope     string    `json:"scope"`
	UsageMetrics
	Models []UsageMetricRow `json:"models"`
	Trend  []UsageMetricRow `json:"trend"`
}

func authoritativeSnapshot(source string) bool {
	switch source {
	case "sub2api_user_usage", "sub2api_owner_usage", "sub2api_owner_runtime_usage", "sub2api_admin_usage":
		return true
	}
	return false
}

func (m *UsageMetrics) add(status string, snapshot MainUsageSnapshot) {
	m.Requests++
	if status == "pending" || status == "prepared" {
		m.Pending++
	}
	if !authoritativeSnapshot(snapshot.Source) {
		m.Unobserved++
		return
	}
	m.Measured++
	m.InputTokens += snapshot.InputTokens
	m.OutputTokens += snapshot.OutputTokens
	m.CacheReadTokens += snapshot.CacheReadTokens
	m.CacheCreationTokens += snapshot.CacheCreationTokens
	m.StandardCost += snapshot.TotalCostNanos
	if snapshot.ActualCostReported {
		m.ActualCost += snapshot.ActualCostNanos
	} else {
		m.MissingActual++
	}
	if snapshot.UpstreamModelMismatch != nil {
		m.RouteObserved++
		if *snapshot.UpstreamModelMismatch {
			m.RouteMismatch++
		}
	}
}

func (s *Store) UsageInsights(agentID, userID string, start, end time.Time) (UsageInsights, error) {
	return s.usageInsights(agentID, userID, 0, start, end)
}

func (s *Store) APIKeyUsageInsights(agentID, userID string, keyID int64, start, end time.Time) (UsageInsights, error) {
	if keyID <= 0 {
		return UsageInsights{}, errNotFound
	}
	return s.usageInsights(agentID, userID, keyID, start, end)
}

func (s *Store) usageInsights(agentID, userID string, keyID int64, start, end time.Time) (UsageInsights, error) {
	result := UsageInsights{Source: "local_main_usage_snapshots", Status: "empty", Start: start, End: end, UpdatedAt: s.clock().UTC(), Scope: "user", Models: []UsageMetricRow{}, Trend: []UsageMetricRow{}}
	if userID == "" {
		result.Scope = "agent"
	}
	query := `SELECT model, status, main_usage_snapshot, created_at FROM settlements WHERE agent_id=? AND created_at>=? AND created_at<?`
	args := []any{agentID, start.Unix(), end.Unix()}
	if userID != "" {
		query += ` AND proxy_main_user_id=?`
		args = append(args, userID)
	}
	if keyID > 0 {
		query += ` AND agent_api_key_id=?`
		args = append(args, keyID)
	}
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	models, trend := map[string]*UsageMetrics{}, map[string]*UsageMetrics{}
	bucket := time.Hour
	if end.Sub(start) > 24*time.Hour {
		bucket = 24 * time.Hour
	}
	for rows.Next() {
		var model, status, raw string
		var created int64
		if err := rows.Scan(&model, &status, &raw, &created); err != nil {
			return result, err
		}
		var snapshot MainUsageSnapshot
		if raw != "" {
			if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
				return result, err
			}
		}
		if model == "" {
			model = "未知模型"
		}
		key := time.Unix(created, 0).UTC().Truncate(bucket).Format(time.RFC3339)
		if models[model] == nil {
			models[model] = &UsageMetrics{}
		}
		if trend[key] == nil {
			trend[key] = &UsageMetrics{}
		}
		result.UsageMetrics.add(status, snapshot)
		models[model].add(status, snapshot)
		trend[key].add(status, snapshot)
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	for key, metrics := range models {
		result.Models = append(result.Models, UsageMetricRow{key, *metrics})
	}
	sort.Slice(result.Models, func(i, j int) bool { return result.Models[i].Key < result.Models[j].Key })
	for cursor := start.Truncate(bucket); cursor.Before(end); cursor = cursor.Add(bucket) {
		key := cursor.Format(time.RFC3339)
		metrics := trend[key]
		if metrics == nil {
			metrics = &UsageMetrics{}
		}
		result.Trend = append(result.Trend, UsageMetricRow{key, *metrics})
	}
	if result.Requests > 0 {
		result.Status = "measured"
		if result.Unobserved > 0 || result.MissingActual > 0 || result.Pending > 0 {
			result.Status = "partial"
		}
	}
	return result, nil
}

func (s *Server) handleUsageInsights(w http.ResponseWriter, r *http.Request, requestID string) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		s.writeError(w, 405, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	session, _, ok := s.requireSession(w, r, requestID)
	if !ok {
		return
	}
	duration, valid := map[string]time.Duration{"1h": time.Hour, "24h": 24 * time.Hour, "7d": 7 * 24 * time.Hour, "30d": 30 * 24 * time.Hour}[r.URL.Query().Get("window")]
	if !valid {
		s.writeError(w, 400, requestID, "INVALID_WINDOW", "window must be 1h, 24h, 7d or 30d")
		return
	}
	userID := session.MainUserID
	if s.isAgentAdmin(session) {
		userID = ""
	}
	// Persisted request times have second precision. A single end anchors all
	// dimensions, using the half-open interval [start,end).
	end := s.store.clock().UTC().Truncate(time.Second)
	result, err := s.store.UsageInsights(s.cfg.AgentID, userID, end.Add(-duration), end)
	if err != nil {
		s.writeError(w, 500, requestID, "STORE_ERROR", "failed to load usage insights")
		return
	}
	s.writeData(w, 200, requestID, result)
}
