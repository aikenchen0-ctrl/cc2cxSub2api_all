package main

import (
	"context"
	"encoding/json"
	"net/http"
)

type adminAvailableGroup struct {
	ID                 int64   `json:"id"`
	Name               string  `json:"name"`
	Platform           string  `json:"platform"`
	SubscriptionType   string  `json:"subscription_type"`
	RateMultiplier     float64 `json:"rate_multiplier"`
	PeakRateEnabled    bool    `json:"peak_rate_enabled"`
	PeakStart          string  `json:"peak_start"`
	PeakEnd            string  `json:"peak_end"`
	PeakRateMultiplier float64 `json:"peak_rate_multiplier"`
	IsExclusive        bool    `json:"is_exclusive"`
}

type adminPricingInterval struct {
	MinTokens            int      `json:"min_tokens"`
	MaxTokens            *int     `json:"max_tokens"`
	TierLabel            string   `json:"tier_label,omitempty"`
	InputPrice           *float64 `json:"input_price"`
	OutputPrice          *float64 `json:"output_price"`
	CacheWritePrice      *float64 `json:"cache_write_price"`
	CacheWrite1hPrice    *float64 `json:"cache_write_1h_price"`
	CacheReadPrice       *float64 `json:"cache_read_price"`
	InputMultiplier      *float64 `json:"input_multiplier"`
	OutputMultiplier     *float64 `json:"output_multiplier"`
	CacheWriteMultiplier *float64 `json:"cache_write_multiplier"`
	CacheReadMultiplier  *float64 `json:"cache_read_multiplier"`
	PerRequestPrice      *float64 `json:"per_request_price"`
}

type adminModelPricing struct {
	BillingMode                  string                 `json:"billing_mode"`
	InputPrice                   *float64               `json:"input_price"`
	OutputPrice                  *float64               `json:"output_price"`
	CacheWritePrice              *float64               `json:"cache_write_price"`
	CacheWrite1hPrice            *float64               `json:"cache_write_1h_price"`
	CacheReadPrice               *float64               `json:"cache_read_price"`
	MaxReasoningEffortMultiplier *float64               `json:"max_reasoning_effort_multiplier,omitempty"`
	ImageInputPrice              *float64               `json:"image_input_price"`
	ImageOutputPrice             *float64               `json:"image_output_price"`
	PerRequestPrice              *float64               `json:"per_request_price"`
	Intervals                    []adminPricingInterval `json:"intervals"`
}

type adminAvailableModel struct {
	Name     string             `json:"name"`
	Platform string             `json:"platform"`
	Pricing  *adminModelPricing `json:"pricing"`
}

type adminChannelPlatform struct {
	Platform        string                `json:"platform"`
	Groups          []adminAvailableGroup `json:"groups"`
	SupportedModels []adminAvailableModel `json:"supported_models"`
}

type adminAvailableChannel struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Platforms   []adminChannelPlatform `json:"platforms"`
}

type adminChannelCatalog struct {
	Channels       []adminAvailableChannel `json:"channels"`
	UserGroupRates map[string]float64      `json:"user_group_rates"`
}

func (c *MainClient) AvailableChannels(ctx context.Context, mainUserID string) (adminChannelCatalog, error) {
	data, err := c.satelliteUserJSON(ctx, mainUserID, "/v1/sub2api/available-channels")
	if err != nil {
		return adminChannelCatalog{}, err
	}
	var result adminChannelCatalog
	if err := json.Unmarshal(data, &result); err != nil {
		return adminChannelCatalog{}, &MainAPIError{Status: http.StatusBadGateway, Code: "SATELLITE_INVALID_RESPONSE", Message: "main-site channel catalogue returned an invalid response"}
	}
	if result.Channels == nil {
		result.Channels = []adminAvailableChannel{}
	}
	if result.UserGroupRates == nil {
		result.UserGroupRates = map[string]float64{}
	}
	return result, nil
}

// handleAgentAdminChannels exposes the current administrator's user-visible
// Sub2API channel catalogue without granting any main-site channel management
// capability. AgentAPI's local model policy remains the only writable layer.
func (s *Server) handleAgentAdminChannels(w http.ResponseWriter, r *http.Request, requestID string, session Session) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	payload, err := s.main.AvailableChannels(r.Context(), session.MainUserID)
	if err != nil {
		s.writeMainError(w, requestID, err)
		return
	}
	s.writeData(w, http.StatusOK, requestID, payload)
}
