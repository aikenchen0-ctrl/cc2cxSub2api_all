package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
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

type AgentAffiliateInvite struct {
	Email       string  `json:"email"`
	Username    string  `json:"username"`
	AffCode     string  `json:"aff_code"`
	TotalRebate float64 `json:"total_rebate"`
	CreatedAt   string  `json:"created_at,omitempty"`
}

type AgentAffiliateRebate struct {
	OutTradeNo   string  `json:"out_trade_no"`
	Email        string  `json:"email"`
	Username     string  `json:"username"`
	OrderAmount  float64 `json:"order_amount"`
	PayAmount    float64 `json:"pay_amount"`
	RebateAmount float64 `json:"rebate_amount"`
	PaymentType  string  `json:"payment_type"`
	OrderStatus  string  `json:"order_status"`
	CreatedAt    string  `json:"created_at,omitempty"`
}

type AgentAffiliateTransferRecord struct {
	Amount              float64  `json:"amount"`
	BalanceAfter        *float64 `json:"balance_after,omitempty"`
	AvailableQuotaAfter *float64 `json:"available_quota_after,omitempty"`
	CreatedAt           string   `json:"created_at,omitempty"`
}

type AgentAffiliateRecordPage struct {
	Items    any   `json:"items"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
	Pages    int   `json:"pages"`
}

func (c *MainClient) AffiliateRecords(ctx context.Context, mainUserID, kind string, page, pageSize int, search string) (json.RawMessage, error) {
	query := url.Values{}
	query.Set("page", strconv.Itoa(page))
	query.Set("page_size", strconv.Itoa(pageSize))
	if search != "" {
		query.Set("search", search)
	}
	return c.satelliteUserJSON(ctx, mainUserID, "/v1/sub2api/affiliate/"+kind+"?"+query.Encode())
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
	case "/api/v1/agent/affiliate/bind":
		s.bindAffiliate(w, r, requestID, session.MainUserID)
	case "/api/v1/agent/affiliate/invites", "/api/v1/agent/affiliate/rebates", "/api/v1/agent/affiliate/transfers":
		s.listAffiliateRecords(w, r, requestID, session.MainUserID)
	default:
		s.writeError(w, http.StatusNotFound, requestID, "NOT_FOUND", "not found")
	}
}

func (s *Server) bindAffiliate(w http.ResponseWriter, r *http.Request, requestID, mainUserID string) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	var payload struct {
		AffCode string `json:"aff_code"`
	}
	if err := decodeJSON(r, &payload, 8<<10); err != nil {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REQUEST", "invalid affiliate code")
		return
	}
	code := strings.ToUpper(strings.TrimSpace(payload.AffCode))
	if len(code) < 4 || len(code) > 32 {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_AFFILIATE_CODE", "affiliate code is invalid")
		return
	}
	if err := s.main.BindAffiliateCode(r.Context(), mainUserID, code); err != nil {
		s.writeMainError(w, requestID, err)
		return
	}
	s.writeData(w, http.StatusOK, requestID, map[string]bool{"bound": true})
}

func (s *Server) listAffiliateRecords(w http.ResponseWriter, r *http.Request, requestID, mainUserID string) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	if len(search) > 80 {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_SEARCH", "search is too long")
		return
	}
	page := boundedPositiveInt(r.URL.Query().Get("page"), 1, 1, 1000000)
	pageSize := boundedPositiveInt(r.URL.Query().Get("page_size"), 20, 1, 100)
	kind := strings.TrimPrefix(r.URL.Path, "/api/v1/agent/affiliate/")
	data, err := s.main.AffiliateRecords(r.Context(), mainUserID, kind, page, pageSize, search)
	if err != nil {
		s.writeMainError(w, requestID, err)
		return
	}
	pageResult, err := whitelistAffiliateRecords(kind, data)
	if err != nil {
		s.writeError(w, http.StatusBadGateway, requestID, "SATELLITE_INVALID_RESPONSE", "main-site affiliate lookup returned an invalid response")
		return
	}
	s.writeData(w, http.StatusOK, requestID, pageResult)
}

func whitelistAffiliateRecords(kind string, data json.RawMessage) (AgentAffiliateRecordPage, error) {
	var raw struct {
		Items    []map[string]json.RawMessage `json:"items"`
		Total    int64                        `json:"total"`
		Page     int                          `json:"page"`
		PageSize int                          `json:"page_size"`
		Pages    int                          `json:"pages"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return AgentAffiliateRecordPage{}, err
	}
	if raw.Items == nil {
		raw.Items = []map[string]json.RawMessage{}
	}
	var items any
	switch kind {
	case "invites":
		out := make([]AgentAffiliateInvite, 0, len(raw.Items))
		for _, item := range raw.Items {
			out = append(out, AgentAffiliateInvite{
				Email: fieldString(item, "invitee_email"), Username: fieldString(item, "invitee_username"),
				AffCode: fieldString(item, "aff_code"), TotalRebate: fieldFloat(item, "total_rebate"), CreatedAt: fieldString(item, "created_at"),
			})
		}
		items = out
	case "rebates":
		out := make([]AgentAffiliateRebate, 0, len(raw.Items))
		for _, item := range raw.Items {
			out = append(out, AgentAffiliateRebate{
				OutTradeNo: fieldString(item, "out_trade_no"), Email: fieldString(item, "invitee_email"), Username: fieldString(item, "invitee_username"),
				OrderAmount: fieldFloat(item, "order_amount"), PayAmount: fieldFloat(item, "pay_amount"), RebateAmount: fieldFloat(item, "rebate_amount"),
				PaymentType: fieldString(item, "payment_type"), OrderStatus: fieldString(item, "order_status"), CreatedAt: fieldString(item, "created_at"),
			})
		}
		items = out
	case "transfers":
		out := make([]AgentAffiliateTransferRecord, 0, len(raw.Items))
		for _, item := range raw.Items {
			record := AgentAffiliateTransferRecord{Amount: fieldFloat(item, "amount"), CreatedAt: fieldString(item, "created_at")}
			if value, ok := rawOptionalFloat(item, "balance_after"); ok {
				record.BalanceAfter = &value
			}
			if value, ok := rawOptionalFloat(item, "available_quota_after"); ok {
				record.AvailableQuotaAfter = &value
			}
			out = append(out, record)
		}
		items = out
	default:
		return AgentAffiliateRecordPage{}, fmt.Errorf("unsupported affiliate record kind")
	}
	if raw.Page < 1 {
		raw.Page = 1
	}
	if raw.PageSize < 1 {
		raw.PageSize = 20
	}
	if raw.Pages < 1 {
		raw.Pages = 1
	}
	return AgentAffiliateRecordPage{Items: items, Total: raw.Total, Page: raw.Page, PageSize: raw.PageSize, Pages: raw.Pages}, nil
}

func fieldString(item map[string]json.RawMessage, key string) string {
	return strings.TrimSpace(rawString(item[key]))
}

func fieldFloat(item map[string]json.RawMessage, key string) float64 {
	value, _ := rawOptionalFloat(item, key)
	return value
}

func rawOptionalFloat(item map[string]json.RawMessage, key string) (float64, bool) {
	raw, ok := item[key]
	if !ok || len(raw) == 0 || string(raw) == "null" {
		return 0, false
	}
	var value float64
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, false
	}
	return value, true
}
