package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// AgentAffiliateInvitee is the browser-safe invitee projection. Main-site
// user IDs and any future administrative fields are deliberately omitted.
type AgentAffiliateInvitee struct {
	Email       string  `json:"email"`
	Username    string  `json:"username"`
	CreatedAt   *string `json:"created_at,omitempty"`
	TotalRebate float64 `json:"total_rebate"`
}

// AgentAffiliateDetail mirrors the Sub2API user page while keeping identity
// selection entirely server-side through the AgentAPI session.
type AgentAffiliateDetail struct {
	AffCode                    string                  `json:"aff_code"`
	AffCount                   int                     `json:"aff_count"`
	AffQuota                   float64                 `json:"aff_quota"`
	AffFrozenQuota             float64                 `json:"aff_frozen_quota"`
	AffHistoryQuota            float64                 `json:"aff_history_quota"`
	EffectiveRebateRatePercent float64                 `json:"effective_rebate_rate_percent"`
	Invitees                   []AgentAffiliateInvitee `json:"invitees"`
}

type AgentAffiliateTransfer struct {
	TransferredQuota float64 `json:"transferred_quota"`
	Balance          float64 `json:"balance"`
}

func invalidAffiliateResponse() error {
	return &MainAPIError{Status: http.StatusBadGateway, Code: "SATELLITE_INVALID_RESPONSE", Message: "main-site affiliate lookup returned an invalid response"}
}

func (c *MainClient) Affiliate(ctx context.Context, mainUserID string) (AgentAffiliateDetail, error) {
	data, err := c.satelliteUserJSON(ctx, mainUserID, "/v1/sub2api/affiliate")
	if err != nil {
		return AgentAffiliateDetail{}, err
	}
	var result AgentAffiliateDetail
	if err := json.Unmarshal(data, &result); err != nil || strings.TrimSpace(result.AffCode) == "" {
		return AgentAffiliateDetail{}, invalidAffiliateResponse()
	}
	if result.Invitees == nil {
		result.Invitees = []AgentAffiliateInvitee{}
	}
	return result, nil
}

func (c *MainClient) TransferAffiliateQuota(ctx context.Context, mainUserID string) (AgentAffiliateTransfer, error) {
	data, err := c.satelliteUserMutationJSON(ctx, mainUserID, "/v1/sub2api/affiliate/transfer", map[string]any{})
	if err != nil {
		return AgentAffiliateTransfer{}, err
	}
	var result AgentAffiliateTransfer
	if err := json.Unmarshal(data, &result); err != nil {
		return AgentAffiliateTransfer{}, invalidAffiliateResponse()
	}
	return result, nil
}

func (c *MainClient) BindAffiliateCode(ctx context.Context, mainUserID, affCode string) error {
	_, err := c.satelliteUserMutationJSON(ctx, mainUserID, "/v1/sub2api/affiliate/bind", map[string]string{"aff_code": strings.ToUpper(strings.TrimSpace(affCode))})
	return err
}

func (s *Server) handleAgentAffiliate(w http.ResponseWriter, r *http.Request, requestID string) {
	w.Header().Set("Cache-Control", "no-store")
	session, _, ok := s.requireSession(w, r, requestID)
	if !ok {
		return
	}

	switch r.URL.Path {
	case "/api/v1/agent/affiliate":
		if r.Method != http.MethodGet {
			s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		result, err := s.main.Affiliate(r.Context(), session.MainUserID)
		if err != nil {
			s.writeMainError(w, requestID, err)
			return
		}
		s.writeData(w, http.StatusOK, requestID, result)
	case "/api/v1/agent/affiliate/transfer":
		if r.Method != http.MethodPost {
			s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		result, err := s.main.TransferAffiliateQuota(r.Context(), session.MainUserID)
		if err != nil {
			s.writeMainError(w, requestID, err)
			return
		}
		s.writeData(w, http.StatusOK, requestID, result)
	default:
		s.writeError(w, http.StatusNotFound, requestID, "NOT_FOUND", "not found")
	}
}
