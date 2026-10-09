package main

import (
	"net/http"
	"strings"
)

const paymentStatsMemberCap = 25
const paymentStatsOrderPageSize = 20

// AgentPaymentStats is a tenant-scoped sample of member orders already stored
// on the main site. It is not a local ledger and does not grant balance edits.
type AgentPaymentStats struct {
	Source               string   `json:"source"`
	MembersTotal         int      `json:"members_total"`
	MembersSampled       int      `json:"members_sampled"`
	OrdersSampled        int      `json:"orders_sampled"`
	OrdersReported       int64    `json:"orders_reported"`
	CompletedCount       int      `json:"completed_count"`
	PendingCount         int      `json:"pending_count"`
	RefundedCount        int      `json:"refunded_count"`
	RefundRequestedCount int      `json:"refund_requested_count"`
	CompletedPayAmount   *float64 `json:"completed_pay_amount,omitempty"`
	Currency             string   `json:"currency,omitempty"`
	AmountAvailable      bool     `json:"amount_available"`
	Partial              bool     `json:"partial"`
	LookupFailures       int      `json:"lookup_failures"`
}

func (s *Server) handleAgentPaymentStats(w http.ResponseWriter, r *http.Request, requestID string, tenant TenantContext) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	members, total, err := s.store.UsersPageFiltered(tenant.AgentID, paymentStatsMemberCap, 0, "", "")
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to load tenant members")
		return
	}
	stats := AgentPaymentStats{
		Source:         "main_site_member_orders",
		MembersTotal:   total,
		MembersSampled: len(members),
		Partial:        total > len(members),
	}
	var completedAmount float64
	var currency string
	mixedCurrency := false
	for _, member := range members {
		page, err := s.main.Orders(r.Context(), member.MainUserID, 1, paymentStatsOrderPageSize, "")
		if err != nil {
			stats.LookupFailures++
			stats.Partial = true
			continue
		}
		stats.OrdersReported += page.Total
		if page.Total > int64(len(page.Items)) {
			stats.Partial = true
		}
		for _, order := range page.Items {
			stats.OrdersSampled++
			switch strings.ToUpper(order.Status) {
			case "COMPLETED":
				stats.CompletedCount++
				completedAmount += order.PayAmount
				orderCurrency := strings.ToUpper(strings.TrimSpace(order.Currency))
				if orderCurrency == "" {
					orderCurrency = "USD"
				}
				if currency == "" {
					currency = orderCurrency
				} else if currency != orderCurrency {
					mixedCurrency = true
				}
			case "PENDING":
				stats.PendingCount++
			case "REFUNDED":
				stats.RefundedCount++
			case "REFUND_REQUESTED":
				stats.RefundRequestedCount++
			}
		}
	}
	if !stats.Partial && stats.LookupFailures == 0 && !mixedCurrency {
		stats.AmountAvailable = true
		stats.CompletedPayAmount = &completedAmount
		if currency != "" {
			stats.Currency = currency
		}
	}
	s.writeData(w, http.StatusOK, requestID, stats)
}
