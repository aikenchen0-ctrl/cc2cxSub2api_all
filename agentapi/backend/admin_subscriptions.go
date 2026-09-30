package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type AgentSubscriptionGroup struct {
	ID                 int64    `json:"id"`
	Name               string   `json:"name"`
	Description        string   `json:"description"`
	Platform           string   `json:"platform"`
	RateMultiplier     float64  `json:"rate_multiplier"`
	SubscriptionType   string   `json:"subscription_type"`
	DailyLimitUSD      *float64 `json:"daily_limit_usd"`
	WeeklyLimitUSD     *float64 `json:"weekly_limit_usd"`
	MonthlyLimitUSD    *float64 `json:"monthly_limit_usd"`
	PeakRateEnabled    bool     `json:"peak_rate_enabled"`
	PeakStart          string   `json:"peak_start"`
	PeakEnd            string   `json:"peak_end"`
	PeakRateMultiplier float64  `json:"peak_rate_multiplier"`
}

type AgentSubscription struct {
	ID                 int64                   `json:"id"`
	GroupID            int64                   `json:"group_id"`
	StartsAt           string                  `json:"starts_at"`
	ExpiresAt          string                  `json:"expires_at"`
	Status             string                  `json:"status"`
	DailyWindowStart   *string                 `json:"daily_window_start"`
	WeeklyWindowStart  *string                 `json:"weekly_window_start"`
	MonthlyWindowStart *string                 `json:"monthly_window_start"`
	DailyUsageUSD      float64                 `json:"daily_usage_usd"`
	WeeklyUsageUSD     float64                 `json:"weekly_usage_usd"`
	MonthlyUsageUSD    float64                 `json:"monthly_usage_usd"`
	Group              *AgentSubscriptionGroup `json:"group,omitempty"`
}

func (c *MainClient) Subscriptions(ctx context.Context, mainUserID string) ([]AgentSubscription, error) {
	data, err := c.satelliteUserJSON(ctx, mainUserID, "/v1/sub2api/subscriptions")
	if err != nil {
		return nil, err
	}
	var result []AgentSubscription
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, &MainAPIError{Status: http.StatusBadGateway, Code: "SATELLITE_INVALID_RESPONSE", Message: "main-site subscription lookup returned an invalid response"}
	}
	if result == nil {
		result = []AgentSubscription{}
	}
	return result, nil
}

// AgentAdminSubscription is a tenant-scoped view over one mapped user's
// Sub2API subscription. Subscription and quota facts remain authoritative in
// Sub2API; AgentAPI only attaches the local user mapping used by the admin UI.
type AgentAdminSubscription struct {
	AgentSubscription
	MainUserID      string `json:"main_user_id"`
	UserEmail       string `json:"user_email,omitempty"`
	UserDisplayName string `json:"user_display_name,omitempty"`
}

type AgentAdminSubscriptionPage struct {
	Items    []AgentAdminSubscription `json:"items"`
	Total    int64                    `json:"total"`
	Page     int                      `json:"page"`
	PageSize int                      `json:"page_size"`
	Pages    int                      `json:"pages"`
}

var allowedAdminSubscriptionStatuses = map[string]bool{
	"": true, "active": true, "expired": true, "revoked": true, "suspended": true,
}

func (s *Server) handleAgentAdminSubscriptions(w http.ResponseWriter, r *http.Request, requestID string) {
	const basePath = "/api/v1/agent/admin/subscriptions"
	w.Header().Set("Cache-Control", "no-store")
	if r.URL.Path != basePath {
		s.writeError(w, http.StatusNotFound, requestID, "NOT_FOUND", "not found")
		return
	}
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	page := boundedPositiveInt(r.URL.Query().Get("page"), 1, 1, 1000)
	pageSize := boundedPositiveInt(r.URL.Query().Get("page_size"), 20, 1, 100)
	status := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("status")))
	if !allowedAdminSubscriptionStatuses[status] {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_SUBSCRIPTION_STATUS", "unsupported subscription status")
		return
	}
	mainUserID := strings.TrimSpace(r.URL.Query().Get("main_user_id"))
	platform := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("platform")))
	if len(platform) > 64 || strings.ContainsAny(platform, "\r\n/\\") {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_PLATFORM", "invalid platform")
		return
	}
	var groupID int64
	if raw := strings.TrimSpace(r.URL.Query().Get("group_id")); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed <= 0 {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_GROUP_ID", "invalid subscription group")
			return
		}
		groupID = parsed
	}
	result, err := s.agentAdminSubscriptions(r.Context(), page, pageSize, status, mainUserID, groupID, platform)
	if err != nil {
		if errors.Is(err, errNotFound) {
			s.writeError(w, http.StatusNotFound, requestID, "MAPPED_USER_NOT_FOUND", "mapped user not found")
			return
		}
		s.writeMainError(w, requestID, err)
		return
	}
	s.writeData(w, http.StatusOK, requestID, result)
}

func (s *Server) agentAdminSubscriptions(ctx context.Context, page, pageSize int, status, selectedMainUserID string, groupID int64, platform string) (AgentAdminSubscriptionPage, error) {
	var users []AgentUserView
	if selectedMainUserID != "" {
		user, err := s.store.User(s.cfg.AgentID, selectedMainUserID)
		if err != nil {
			return AgentAdminSubscriptionPage{}, err
		}
		users = []AgentUserView{user}
	} else {
		var err error
		users, err = s.allMappedUsers()
		if err != nil {
			return AgentAdminSubscriptionPage{}, err
		}
	}
	if len(users) == 0 {
		return AgentAdminSubscriptionPage{Items: []AgentAdminSubscription{}, Page: page, PageSize: pageSize}, nil
	}

	type userResult struct {
		items []AgentAdminSubscription
		err   error
	}
	results := make(chan userResult, len(users))
	semaphore := make(chan struct{}, 6)
	var wg sync.WaitGroup
	for _, mapped := range users {
		mapped := mapped
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case semaphore <- struct{}{}:
			case <-ctx.Done():
				results <- userResult{err: ctx.Err()}
				return
			}
			defer func() { <-semaphore }()
			items, err := s.main.Subscriptions(ctx, mapped.MainUserID)
			if err != nil {
				results <- userResult{err: err}
				return
			}
			decorated := make([]AgentAdminSubscription, 0, len(items))
			for _, item := range items {
				if status != "" && strings.ToLower(item.Status) != status {
					continue
				}
				if groupID > 0 && item.GroupID != groupID {
					continue
				}
				if platform != "" && (item.Group == nil || strings.ToLower(item.Group.Platform) != platform) {
					continue
				}
				decorated = append(decorated, AgentAdminSubscription{
					AgentSubscription: item,
					MainUserID:        mapped.MainUserID, UserEmail: mapped.Email, UserDisplayName: mapped.DisplayName,
				})
			}
			results <- userResult{items: decorated}
		}()
	}
	wg.Wait()
	close(results)

	all := make([]AgentAdminSubscription, 0)
	for result := range results {
		if result.err != nil {
			return AgentAdminSubscriptionPage{}, result.err
		}
		all = append(all, result.items...)
	}
	sort.SliceStable(all, func(i, j int) bool {
		left := subscriptionSortTime(all[i])
		right := subscriptionSortTime(all[j])
		if !left.Equal(right) {
			return left.After(right)
		}
		if all[i].ID != all[j].ID {
			return all[i].ID > all[j].ID
		}
		return all[i].MainUserID < all[j].MainUserID
	})
	total := int64(len(all))
	start := (page - 1) * pageSize
	end := start + pageSize
	if start > len(all) {
		start = len(all)
	}
	if end > len(all) {
		end = len(all)
	}
	pages := 0
	if total > 0 {
		pages = int((total + int64(pageSize) - 1) / int64(pageSize))
	}
	return AgentAdminSubscriptionPage{Items: all[start:end], Total: total, Page: page, PageSize: pageSize, Pages: pages}, nil
}

func subscriptionSortTime(item AgentAdminSubscription) time.Time {
	for _, value := range []string{item.StartsAt, item.ExpiresAt} {
		if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
}
