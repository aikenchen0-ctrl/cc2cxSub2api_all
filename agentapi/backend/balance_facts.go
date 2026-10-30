package main

import (
	"errors"
	"net/http"
	"strings"
	"time"
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
		localCents, localErr := s.store.UserWalletCents(session.AgentID, session.MainUserID)
		localBilling := localErr == nil && s.stationBillingPolicy(session.AgentID).BillLocalWallet
		mainBalance, frozen, mainFound := s.lookupDisplayBalance(r.Context(), session.MainUserID)
		payload := map[string]any{"main_user_id": session.MainUserID}
		payload["station_balance_cents"] = localCents
		if localBilling {
			// Station billing spends the local wallet, so this is the number the
			// user can actually use. The main-site balance stays visible as a
			// separate, clearly labelled figure.
			payload["balance_cents"] = localCents
			payload["frozen_balance_cents"] = int64(0)
			payload["source"] = "station_wallet"
			payload["balance_available"] = true
			if mainFound {
				payload["main_balance_cents"] = mainBalance
			}
		} else {
			if !mainFound {
				s.writeError(w, http.StatusServiceUnavailable, requestID, "MAIN_BALANCE_UNAVAILABLE", "main balance lookup is temporarily unavailable")
				return
			}
			payload["balance_cents"] = mainBalance
			payload["frozen_balance_cents"] = frozen
			payload["source"] = "admin_key"
			payload["balance_available"] = true
			payload["main_balance_cents"] = mainBalance
		}
		s.writeData(w, http.StatusOK, requestID, payload)
	case "/api/v1/agent/balance/history":
		if _, walletErr := s.store.UserWalletCents(session.AgentID, session.MainUserID); walletErr == nil && s.stationBillingPolicy(session.AgentID).BillLocalWallet {
			s.writeStationWalletHistory(w, r, requestID, session)
			return
		}
		s.writeBalanceHistory(w, r, requestID, session.MainUserID)
	default:
		s.writeError(w, http.StatusNotFound, requestID, "NOT_FOUND", "not found")
	}
}

// writeStationWalletHistory reports the ledger that actually backs the station
// wallet (recharges and model usage), instead of the unrelated main-site list.
func (s *Server) writeStationWalletHistory(w http.ResponseWriter, r *http.Request, requestID string, session Session) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	rows, err := s.store.UserWalletLedger(session.AgentID, session.MainUserID, 50)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to load station wallet ledger")
		return
	}
	total, totalErr := s.store.UserWalletRechargedCents(session.AgentID, session.MainUserID)
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		label := "余额变动"
		switch row.Kind {
		case "user_recharge":
			label = "充值"
		case "user_consume":
			label = "模型调用"
		}
		items = append(items, map[string]any{
			"type":          label,
			"kind":          row.Kind,
			"value":         float64(row.AmountCents) / 100,
			"balance_after": float64(row.BalanceAfterCents) / 100,
			"status":        "success",
			"created_at":    time.Unix(row.CreatedAt, 0).UTC().Format(time.RFC3339),
			"note":          row.Note,
		})
	}
	result := map[string]any{
		"items": items, "total": len(items), "page": 1, "page_size": len(items),
		"source": "station_wallet",
	}
	if totalErr == nil {
		result["total_recharged"] = float64(total) / 100
	}
	s.writeData(w, http.StatusOK, requestID, result)
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
