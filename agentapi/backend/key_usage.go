package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type keyUsageStats struct {
	Requests            int64   `json:"requests"`
	InputTokens         int64   `json:"input_tokens"`
	OutputTokens        int64   `json:"output_tokens"`
	TotalTokens         int64   `json:"total_tokens"`
	CacheCreationTokens int64   `json:"cache_creation_tokens"`
	CacheReadTokens     int64   `json:"cache_read_tokens"`
	ActualCost          float64 `json:"actual_cost"`
}

func keyUsageStatsFrom(metrics UsageMetrics) keyUsageStats {
	return keyUsageStats{
		Requests: metrics.Requests, InputTokens: metrics.InputTokens, OutputTokens: metrics.OutputTokens,
		TotalTokens:         metrics.InputTokens + metrics.OutputTokens + metrics.CacheCreationTokens + metrics.CacheReadTokens,
		CacheCreationTokens: metrics.CacheCreationTokens, CacheReadTokens: metrics.CacheReadTokens,
		ActualCost: float64(metrics.ActualCost) / 1_000_000_000,
	}
}

func parseKeyUsageWindow(r *http.Request, now time.Time) (time.Time, time.Time, int, error) {
	days := 30
	if raw := strings.TrimSpace(r.URL.Query().Get("days")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || (parsed != 7 && parsed != 30 && parsed != 90) {
			return time.Time{}, time.Time{}, 0, errors.New("days must be 7, 30 or 90")
		}
		days = parsed
	}
	location := time.UTC
	if raw := strings.TrimSpace(r.URL.Query().Get("timezone")); raw != "" {
		if parsed, err := time.LoadLocation(raw); err == nil {
			location = parsed
		}
	}
	localNow := now.In(location)
	end := time.Date(localNow.Year(), localNow.Month(), localNow.Day()+1, 0, 0, 0, 0, location)
	start := end.AddDate(0, 0, -days)
	startRaw, endRaw := strings.TrimSpace(r.URL.Query().Get("start_date")), strings.TrimSpace(r.URL.Query().Get("end_date"))
	if startRaw != "" || endRaw != "" {
		if startRaw == "" || endRaw == "" {
			return time.Time{}, time.Time{}, 0, errors.New("start_date and end_date must be provided together")
		}
		parsedStart, errStart := time.ParseInLocation("2006-01-02", startRaw, location)
		parsedEnd, errEnd := time.ParseInLocation("2006-01-02", endRaw, location)
		if errStart != nil || errEnd != nil || parsedEnd.Before(parsedStart) || parsedEnd.Sub(parsedStart) > 90*24*time.Hour {
			return time.Time{}, time.Time{}, 0, errors.New("date range must be valid and at most 90 days")
		}
		start, end = parsedStart, parsedEnd.AddDate(0, 0, 1)
	}
	return start.UTC(), end.UTC(), days, nil
}

// handleAPIKeyUsage mirrors Sub2API's public GET /v1/usage contract while
// keeping the AgentAPI key tenant-local. It never forwards that key upstream.
func (s *Server) handleAPIKeyUsage(w http.ResponseWriter, r *http.Request, requestID string) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	principal, ok := s.requireAgentAPIKeyPrincipal(w, r, requestID)
	if !ok {
		return
	}
	start, end, dailyDays, err := parseKeyUsageWindow(r, s.store.clock().UTC())
	if err != nil {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_DATE_RANGE", err.Error())
		return
	}
	agentID := principal.Tenant.AgentID
	selected, err := s.store.APIKeyUsageInsights(agentID, principal.ProxyMainUserID, principal.APIKeyID, start, end)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to load API key usage")
		return
	}
	now := s.store.clock().UTC()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	today, err := s.store.APIKeyUsageInsights(agentID, principal.ProxyMainUserID, principal.APIKeyID, todayStart, todayStart.Add(24*time.Hour))
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to load today's API key usage")
		return
	}
	dailyStart := time.Date(now.Year(), now.Month(), now.Day()+1-dailyDays, 0, 0, 0, 0, time.UTC)
	daily, err := s.store.APIKeyUsageInsights(agentID, principal.ProxyMainUserID, principal.APIKeyID, dailyStart, todayStart.Add(24*time.Hour))
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to load daily API key usage")
		return
	}
	balance := float64(principal.User.BalanceCents) / 100
	if current, mainErr := s.main.AdminGetUser(r.Context(), principal.ProxyMainUserID); mainErr == nil {
		balance = float64(current.Balance) / 100
	}
	dailyRows := make([]map[string]any, 0, len(daily.Trend))
	for _, row := range daily.Trend {
		stats := keyUsageStatsFrom(row.UsageMetrics)
		dailyRows = append(dailyRows, map[string]any{
			"date": row.Key[:10], "requests": stats.Requests, "input_tokens": stats.InputTokens,
			"output_tokens": stats.OutputTokens, "cache_read_tokens": stats.CacheReadTokens,
			"cache_write_tokens": stats.CacheCreationTokens, "cost": stats.ActualCost, "actual_cost": stats.ActualCost,
		})
	}
	models := make([]map[string]any, 0, len(selected.Models))
	for _, row := range selected.Models {
		stats := keyUsageStatsFrom(row.UsageMetrics)
		models = append(models, map[string]any{
			"model": row.Key, "requests": stats.Requests, "input_tokens": stats.InputTokens,
			"output_tokens": stats.OutputTokens, "cache_creation_tokens": stats.CacheCreationTokens,
			"cache_read_tokens": stats.CacheReadTokens, "total_tokens": stats.TotalTokens, "actual_cost": stats.ActualCost,
		})
	}
	payload := map[string]any{
		"mode": "unrestricted", "isValid": true, "status": "active", "planName": "主站计费",
		"unit": "USD", "balance": balance, "remaining": balance,
		"key":         map[string]any{"name": principal.APIKeyName, "prefix": principal.APIKeyPrefix},
		"usage":       map[string]any{"today": keyUsageStatsFrom(today.UsageMetrics), "total": keyUsageStatsFrom(selected.UsageMetrics), "rpm": 0, "tpm": 0},
		"daily_usage": dailyRows, "model_stats": models,
		"source": selected.Source, "status_detail": selected.Status,
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(payload)
}

func (s *Server) requireAgentAPIKeyPrincipal(w http.ResponseWriter, r *http.Request, requestID string) (modelPrincipal, bool) {
	authorization := strings.TrimSpace(r.Header.Get("Authorization"))
	parts := strings.SplitN(authorization, " ", 2)
	key := ""
	if len(parts) == 2 {
		key = strings.TrimSpace(parts[1])
	}
	if len(parts) != 2 || !strings.EqualFold(strings.TrimSpace(parts[0]), "Bearer") || !strings.HasPrefix(key, "sk-") || strings.HasPrefix(key, "sk-super-") {
		s.writeError(w, http.StatusUnauthorized, requestID, "INVALID_AGENT_API_KEY", "a valid AgentAPI bearer key is required")
		return modelPrincipal{}, false
	}
	resolved, err := s.store.ResolveAPIKeyDetailsAnyTenant(key)
	if err != nil {
		if errors.Is(err, errNotFound) {
			s.writeError(w, http.StatusUnauthorized, requestID, "INVALID_AGENT_API_KEY", "AgentAPI API key is invalid or revoked")
			return modelPrincipal{}, false
		}
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to resolve AgentAPI API key")
		return modelPrincipal{}, false
	}
	user, err := s.store.User(resolved.AgentID, resolved.MainUserID)
	if err != nil || user.Status != "active" {
		s.writeError(w, http.StatusUnauthorized, requestID, "AGENT_USER_NOT_FOUND", "AgentAPI user mapping is unavailable")
		return modelPrincipal{}, false
	}
	tenant := TenantContext{AgentID: resolved.AgentID, MainUserID: resolved.MainUserID, Role: tenantRoleMember, Source: tenantSourceAPIKey}
	return modelPrincipal{Tenant: tenant, ProxyMainUserID: resolved.MainUserID, User: user, APIKeyID: resolved.ID, APIKeyName: resolved.Name, APIKeyPrefix: resolved.Prefix}, true
}
