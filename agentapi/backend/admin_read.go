package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// adminGet is the only read path for the main-site administrator key.
// Callers must pass a fixed server path. Browser paths are never forwarded.
func (c *MainClient) adminGet(ctx context.Context, path string) (json.RawMessage, error) {
	if strings.TrimSpace(c.cfg.MainAdminAPIKey) == "" {
		return nil, &MainAPIError{Status: http.StatusServiceUnavailable, Code: "MAIN_ADMIN_NOT_CONFIGURED", Message: "main administrator key is not configured"}
	}
	if !strings.HasPrefix(path, "/admin/") || strings.Contains(path, "..") {
		return nil, &MainAPIError{Status: http.StatusBadRequest, Code: "ADMIN_PATH_REJECTED", Message: "unsupported admin lookup"}
	}
	headers := make(http.Header)
	headers.Set("Accept", "application/json")
	headers.Set("x-api-key", c.cfg.MainAdminAPIKey)
	response, raw, err := c.request(ctx, http.MethodGet, c.endpoint(path), nil, headers)
	if err != nil {
		return nil, &MainAPIError{Status: http.StatusServiceUnavailable, Code: "MAIN_ADMIN_UNAVAILABLE", Message: "main-site lookup is temporarily unavailable"}
	}
	data, err := unwrapMainResponse(response.StatusCode, raw)
	if err != nil {
		return nil, &MainAPIError{Status: http.StatusBadGateway, Code: "MAIN_ADMIN_UNAVAILABLE", Message: "main-site lookup is temporarily unavailable"}
	}
	return data, nil
}

func adminUserPath(mainUserID, suffix string) (string, error) {
	mainUserID = strings.TrimSpace(mainUserID)
	parsedID, err := strconv.ParseInt(mainUserID, 10, 64)
	if err != nil || parsedID <= 0 {
		return "", &MainAPIError{Status: http.StatusBadRequest, Code: "INVALID_USER", Message: "invalid user"}
	}
	return "/admin/users/" + url.PathEscape(mainUserID) + suffix, nil
}

type AgentBalanceHistoryItem struct {
	Type      string  `json:"type"`
	Value     float64 `json:"value"`
	Status    string  `json:"status"`
	CreatedAt string  `json:"created_at,omitempty"`
	UsedAt    string  `json:"used_at,omitempty"`
}

type AgentBalanceHistoryPage struct {
	Items          []AgentBalanceHistoryItem `json:"items"`
	Total          int64                     `json:"total"`
	Page           int                       `json:"page"`
	PageSize       int                       `json:"page_size"`
	TotalRecharged float64                   `json:"total_recharged"`
}

type AgentMainUsageItem struct {
	RequestID    string  `json:"request_id"`
	Model        string  `json:"model"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	TotalCost    float64 `json:"total_cost"`
	ActualCost   float64 `json:"actual_cost"`
	CreatedAt    string  `json:"created_at,omitempty"`
}

type AgentMainUsagePage struct {
	Items    []AgentMainUsageItem `json:"items"`
	Total    int64                `json:"total"`
	Page     int                  `json:"page"`
	PageSize int                  `json:"page_size"`
}

func (c *MainClient) AdminReadBalanceHistory(ctx context.Context, mainUserID string, page, pageSize int) (AgentBalanceHistoryPage, error) {
	path, err := adminUserPath(mainUserID, "/balance-history")
	if err != nil {
		return AgentBalanceHistoryPage{}, err
	}
	query := url.Values{}
	query.Set("page", strconv.Itoa(page))
	query.Set("page_size", strconv.Itoa(pageSize))
	data, err := c.adminGet(ctx, path+"?"+query.Encode())
	if err != nil {
		return AgentBalanceHistoryPage{}, err
	}
	return whitelistBalanceHistory(data)
}

func (c *MainClient) AdminReadUserUsage(ctx context.Context, mainUserID string, page, pageSize int) (AgentMainUsagePage, error) {
	parsedID, err := strconv.ParseInt(strings.TrimSpace(mainUserID), 10, 64)
	if err != nil || parsedID <= 0 {
		return AgentMainUsagePage{}, &MainAPIError{Status: http.StatusBadRequest, Code: "INVALID_USER", Message: "invalid user"}
	}
	query := url.Values{}
	query.Set("user_id", strconv.FormatInt(parsedID, 10))
	query.Set("page", strconv.Itoa(page))
	query.Set("page_size", strconv.Itoa(pageSize))
	data, err := c.adminGet(ctx, "/admin/usage?"+query.Encode())
	if err != nil {
		return AgentMainUsagePage{}, err
	}
	return whitelistAdminUsage(data)
}

func whitelistBalanceHistory(data json.RawMessage) (AgentBalanceHistoryPage, error) {
	var raw struct {
		Items          []map[string]json.RawMessage `json:"items"`
		Total          int64                        `json:"total"`
		Page           int                          `json:"page"`
		PageSize       int                          `json:"page_size"`
		TotalRecharged float64                      `json:"total_recharged"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return AgentBalanceHistoryPage{}, err
	}
	items := make([]AgentBalanceHistoryItem, 0, len(raw.Items))
	for _, item := range raw.Items {
		items = append(items, AgentBalanceHistoryItem{
			Type: fieldString(item, "type"), Value: fieldFloat(item, "value"), Status: fieldString(item, "status"),
			CreatedAt: fieldString(item, "created_at"), UsedAt: fieldString(item, "used_at"),
		})
	}
	if raw.Page < 1 {
		raw.Page = 1
	}
	if raw.PageSize < 1 {
		raw.PageSize = 20
	}
	return AgentBalanceHistoryPage{Items: items, Total: raw.Total, Page: raw.Page, PageSize: raw.PageSize, TotalRecharged: raw.TotalRecharged}, nil
}

func whitelistAdminUsage(data json.RawMessage) (AgentMainUsagePage, error) {
	var raw struct {
		Items    []map[string]json.RawMessage `json:"items"`
		Total    int64                        `json:"total"`
		Page     int                          `json:"page"`
		PageSize int                          `json:"page_size"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return AgentMainUsagePage{}, err
	}
	items := make([]AgentMainUsageItem, 0, len(raw.Items))
	for _, item := range raw.Items {
		items = append(items, AgentMainUsageItem{
			RequestID: fieldString(item, "request_id"), Model: fieldString(item, "model"),
			InputTokens: int64(fieldFloat(item, "input_tokens")), OutputTokens: int64(fieldFloat(item, "output_tokens")),
			TotalCost: fieldFloat(item, "total_cost"), ActualCost: fieldFloat(item, "actual_cost"), CreatedAt: fieldString(item, "created_at"),
		})
	}
	if raw.Page < 1 {
		raw.Page = 1
	}
	if raw.PageSize < 1 {
		raw.PageSize = 20
	}
	return AgentMainUsagePage{Items: items, Total: raw.Total, Page: raw.Page, PageSize: raw.PageSize}, nil
}
