package main

import (
	"errors"
	"net/http"
	"strings"
)

func (s *Server) handleAgentBalance(w http.ResponseWriter, r *http.Request, requestID string) {
	w.Header().Set("Cache-Control", "no-store")
	session, _, ok := s.requireSession(w, r, requestID)
	if !ok {
		return
	}
	switch r.URL.Path {
	case "/api/v1/agent/balance":
		if r.Method != http.MethodGet {
			s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		balance, frozen, found := s.lookupDisplayBalance(r.Context(), session.MainUserID)
		if !found {
			s.writeError(w, http.StatusServiceUnavailable, requestID, "MAIN_BALANCE_UNAVAILABLE", "main balance lookup is temporarily unavailable")
			return
		}
		s.writeData(w, http.StatusOK, requestID, map[string]any{
			"main_user_id": session.MainUserID, "balance_cents": balance, "frozen_balance_cents": frozen, "source": "admin_key",
		})
	case "/api/v1/agent/balance/history":
		s.writeBalanceHistory(w, r, requestID, session.MainUserID)
	default:
		s.writeError(w, http.StatusNotFound, requestID, "NOT_FOUND", "not found")
	}
}

func (s *Server) handleAgentMemberFacts(w http.ResponseWriter, r *http.Request, requestID string, tenant TenantContext) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	mainUserID := strings.TrimSpace(r.URL.Query().Get("main_user_id"))
	if _, err := s.store.User(tenant.AgentID, mainUserID); err != nil {
		if errors.Is(err, errNotFound) {
			s.writeError(w, http.StatusNotFound, requestID, "NOT_FOUND", "user is not in this station")
			return
		}
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to load tenant user")
		return
	}
	page := boundedPositiveInt(r.URL.Query().Get("page"), 1, 1, 1000000)
	pageSize := boundedPositiveInt(r.URL.Query().Get("page_size"), 20, 1, 50)
	balance, frozen, found := s.lookupDisplayBalance(r.Context(), mainUserID)
	history, historyErr := s.main.AdminReadBalanceHistory(r.Context(), mainUserID, page, pageSize)
	usage, usageErr := s.main.AdminReadUserUsage(r.Context(), mainUserID, page, pageSize)
	result := map[string]any{
		"main_user_id": mainUserID,
		"balance_cents": func() int64 {
			if found {
				return balance
			}
			return 0
		}(),
		"frozen_balance_cents": frozen,
		"balance_available":    found,
		"history_available":    historyErr == nil,
		"usage_available":      usageErr == nil,
	}
	if historyErr == nil {
		result["history"] = history
	}
	if usageErr == nil {
		result["usage"] = usage
	}
	s.writeData(w, http.StatusOK, requestID, result)
}

func (s *Server) writeBalanceHistory(w http.ResponseWriter, r *http.Request, requestID, mainUserID string) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	page := boundedPositiveInt(r.URL.Query().Get("page"), 1, 1, 1000000)
	pageSize := boundedPositiveInt(r.URL.Query().Get("page_size"), 20, 1, 50)
	history, err := s.main.AdminReadBalanceHistory(r.Context(), mainUserID, page, pageSize)
	if err != nil {
		s.writeMainError(w, requestID, err)
		return
	}
	s.writeData(w, http.StatusOK, requestID, history)
}
