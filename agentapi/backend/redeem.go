package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// AgentRedeemResult is the browser-safe result of redeeming a code for the
// current user. The plaintext code, notes, group and any other user are omitted.
type AgentRedeemResult struct {
	Type   string  `json:"type"`
	Value  float64 `json:"value"`
	Status string  `json:"status"`
}

// AgentRedeemHistoryItem is one current-user redemption. It does not include
// the code, internal notes, or a user selector.
type AgentRedeemHistoryItem struct {
	Type      string  `json:"type"`
	Value     float64 `json:"value"`
	Status    string  `json:"status"`
	UsedAt    string  `json:"used_at,omitempty"`
	CreatedAt string  `json:"created_at,omitempty"`
}

func (c *MainClient) RedeemCode(ctx context.Context, mainUserID, code string) (json.RawMessage, error) {
	return c.satelliteUserMutationJSON(ctx, mainUserID, "/v1/sub2api/redeem", map[string]string{"code": code})
}

func (c *MainClient) RedeemHistory(ctx context.Context, mainUserID string) (json.RawMessage, error) {
	return c.satelliteUserJSON(ctx, mainUserID, "/v1/sub2api/redeem/history")
}

func (s *Server) handleAgentRedeem(w http.ResponseWriter, r *http.Request, requestID string) {
	w.Header().Set("Cache-Control", "no-store")
	session, _, ok := s.requireSession(w, r, requestID)
	if !ok {
		return
	}
	switch r.URL.Path {
	case "/api/v1/agent/redeem":
		s.redeemForSession(w, r, requestID, session.MainUserID)
	case "/api/v1/agent/redeem/history":
		s.redeemHistoryForSession(w, r, requestID, session.MainUserID)
	default:
		s.writeError(w, http.StatusNotFound, requestID, "NOT_FOUND", "not found")
	}
}

func (s *Server) redeemForSession(w http.ResponseWriter, r *http.Request, requestID, mainUserID string) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	var payload struct {
		Code string `json:"code"`
	}
	if err := decodeJSON(r, &payload, 8<<10); err != nil {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REQUEST", "invalid redeem request")
		return
	}
	code := strings.TrimSpace(payload.Code)
	if len(code) < 4 || len(code) > 64 {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REDEEM_CODE", "redeem code is invalid")
		return
	}
	data, err := s.main.RedeemCode(r.Context(), mainUserID, code)
	if err != nil {
		s.writeMainError(w, requestID, err)
		return
	}
	result, err := whitelistRedeemResult(data)
	if err != nil {
		s.writeError(w, http.StatusBadGateway, requestID, "SATELLITE_INVALID_RESPONSE", "main-site redeem returned an invalid response")
		return
	}
	s.writeData(w, http.StatusOK, requestID, result)
}

func (s *Server) redeemHistoryForSession(w http.ResponseWriter, r *http.Request, requestID, mainUserID string) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	data, err := s.main.RedeemHistory(r.Context(), mainUserID)
	if err != nil {
		s.writeMainError(w, requestID, err)
		return
	}
	var rawItems []map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawItems); err != nil {
		s.writeError(w, http.StatusBadGateway, requestID, "SATELLITE_INVALID_RESPONSE", "main-site redeem history returned an invalid response")
		return
	}
	items := make([]AgentRedeemHistoryItem, 0, len(rawItems))
	for _, item := range rawItems {
		items = append(items, AgentRedeemHistoryItem{
			Type: fieldString(item, "type"), Value: fieldFloat(item, "value"), Status: fieldString(item, "status"),
			UsedAt: fieldString(item, "used_at"), CreatedAt: fieldString(item, "created_at"),
		})
	}
	s.writeData(w, http.StatusOK, requestID, map[string]any{"items": items, "total": len(items)})
}

func whitelistRedeemResult(data json.RawMessage) (AgentRedeemResult, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return AgentRedeemResult{}, err
	}
	result := AgentRedeemResult{Type: fieldString(raw, "type"), Value: fieldFloat(raw, "value"), Status: fieldString(raw, "status")}
	if result.Type == "" || result.Status == "" {
		return AgentRedeemResult{}, invalidAffiliateResponse()
	}
	return result, nil
}
