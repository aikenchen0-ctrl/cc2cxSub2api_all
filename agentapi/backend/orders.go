package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// AgentOrder is the browser-safe subset of a Sub2API payment order. User IDs,
// provider configuration, credentials and any future upstream-only fields are
// intentionally absent so they cannot leak through this compatibility layer.
type AgentOrder struct {
	ID                  int64   `json:"id"`
	Amount              float64 `json:"amount"`
	PayAmount           float64 `json:"pay_amount"`
	FeeRate             float64 `json:"fee_rate"`
	Currency            string  `json:"currency"`
	PaymentType         string  `json:"payment_type"`
	OutTradeNo          string  `json:"out_trade_no"`
	Status              string  `json:"status"`
	OrderType           string  `json:"order_type"`
	CreatedAt           string  `json:"created_at"`
	ExpiresAt           string  `json:"expires_at"`
	PaidAt              *string `json:"paid_at,omitempty"`
	CompletedAt         *string `json:"completed_at,omitempty"`
	RefundAmount        float64 `json:"refund_amount"`
	RefundReason        *string `json:"refund_reason,omitempty"`
	RefundRequestedAt   *string `json:"refund_requested_at,omitempty"`
	RefundRequestedBy   *string `json:"refund_requested_by,omitempty"`
	RefundRequestReason *string `json:"refund_request_reason,omitempty"`
	PlanID              *int64  `json:"plan_id,omitempty"`
	ProviderInstanceID  *string `json:"provider_instance_id,omitempty"`
}

type AgentOrderPage struct {
	Items    []AgentOrder `json:"items"`
	Total    int64        `json:"total"`
	Page     int          `json:"page"`
	PageSize int          `json:"page_size"`
	Pages    int          `json:"pages"`
}

type AgentRefundEligibleProviders struct {
	ProviderInstanceIDs []string `json:"provider_instance_ids"`
}

func (c *MainClient) Orders(ctx context.Context, mainUserID string, page, pageSize int, status string) (AgentOrderPage, error) {
	return c.orders(ctx, mainUserID, page, pageSize, status, nil)
}

func (c *MainClient) OrdersSince(ctx context.Context, mainUserID string, page, pageSize int, status string, paidSince time.Time) (AgentOrderPage, error) {
	return c.orders(ctx, mainUserID, page, pageSize, status, &paidSince)
}

func (c *MainClient) orders(ctx context.Context, mainUserID string, page, pageSize int, status string, paidSince *time.Time) (AgentOrderPage, error) {
	query := url.Values{}
	query.Set("page", strconv.Itoa(page))
	query.Set("page_size", strconv.Itoa(pageSize))
	if status != "" {
		query.Set("status", status)
	}
	if paidSince != nil {
		query.Set("paid_since", paidSince.UTC().Format(time.RFC3339))
	}
	data, err := c.satelliteUserJSON(ctx, mainUserID, "/v1/sub2api/orders?"+query.Encode())
	if err != nil {
		return AgentOrderPage{}, err
	}
	var result AgentOrderPage
	if err := json.Unmarshal(data, &result); err != nil {
		return AgentOrderPage{}, invalidOrderResponse()
	}
	if result.Items == nil {
		result.Items = []AgentOrder{}
	}
	return result, nil
}

func (c *MainClient) RefundEligibleProviders(ctx context.Context, mainUserID string) (AgentRefundEligibleProviders, error) {
	data, err := c.satelliteUserJSON(ctx, mainUserID, "/v1/sub2api/orders/refund-eligible-providers")
	if err != nil {
		return AgentRefundEligibleProviders{}, err
	}
	var result AgentRefundEligibleProviders
	if err := json.Unmarshal(data, &result); err != nil {
		return AgentRefundEligibleProviders{}, invalidOrderResponse()
	}
	if result.ProviderInstanceIDs == nil {
		result.ProviderInstanceIDs = []string{}
	}
	return result, nil
}

func (c *MainClient) CancelOrder(ctx context.Context, mainUserID string, orderID int64) error {
	_, err := c.satelliteUserMutationJSON(ctx, mainUserID, fmt.Sprintf("/v1/sub2api/orders/%d/cancel", orderID), map[string]any{})
	return err
}

func (c *MainClient) RequestOrderRefund(ctx context.Context, mainUserID string, orderID int64, reason string) error {
	_, err := c.satelliteUserMutationJSON(ctx, mainUserID, fmt.Sprintf("/v1/sub2api/orders/%d/refund-request", orderID), map[string]string{"reason": reason})
	return err
}

func invalidOrderResponse() error {
	return &MainAPIError{Status: http.StatusBadGateway, Code: "SATELLITE_INVALID_RESPONSE", Message: "main-site order lookup returned an invalid response"}
}

var allowedOrderStatuses = map[string]bool{
	"": true, "PENDING": true, "COMPLETED": true, "FAILED": true,
	"REFUNDED": true, "REFUND_REQUESTED": true,
}

func (s *Server) handleAgentOrders(w http.ResponseWriter, r *http.Request, requestID string) {
	w.Header().Set("Cache-Control", "no-store")
	session, _, ok := s.requireSession(w, r, requestID)
	if !ok {
		return
	}
	const basePath = "/api/v1/agent/orders"
	if r.URL.Path == basePath {
		if r.Method != http.MethodGet {
			s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		page := boundedPositiveInt(r.URL.Query().Get("page"), 1, 1, 1000000)
		pageSize := boundedPositiveInt(r.URL.Query().Get("page_size"), 20, 1, 100)
		status := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("status")))
		if !allowedOrderStatuses[status] {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_ORDER_STATUS", "unsupported order status")
			return
		}
		orders, err := s.main.Orders(r.Context(), session.MainUserID, page, pageSize, status)
		if err != nil {
			s.writeMainError(w, requestID, err)
			return
		}
		s.writeData(w, http.StatusOK, requestID, orders)
		return
	}
	if r.URL.Path == basePath+"/refund-eligible-providers" {
		if r.Method != http.MethodGet {
			s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		result, err := s.main.RefundEligibleProviders(r.Context(), session.MainUserID)
		if err != nil {
			s.writeMainError(w, requestID, err)
			return
		}
		s.writeData(w, http.StatusOK, requestID, result)
		return
	}

	relative := strings.TrimPrefix(r.URL.Path, basePath+"/")
	parts := strings.Split(relative, "/")
	if len(parts) != 2 || r.Method != http.MethodPost {
		s.writeError(w, http.StatusNotFound, requestID, "NOT_FOUND", "not found")
		return
	}
	orderID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || orderID <= 0 {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_ORDER_ID", "invalid order ID")
		return
	}
	switch parts[1] {
	case "cancel":
		if err := s.main.CancelOrder(r.Context(), session.MainUserID, orderID); err != nil {
			s.writeMainError(w, requestID, err)
			return
		}
		s.writeData(w, http.StatusOK, requestID, map[string]string{"message": "order cancelled"})
	case "refund-request":
		var body struct {
			Reason string `json:"reason"`
		}
		if err := decodeJSON(r, &body, 16<<10); err != nil {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REQUEST", "invalid refund request")
			return
		}
		body.Reason = strings.TrimSpace(body.Reason)
		if len(body.Reason) < 2 || len(body.Reason) > 500 {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REFUND_REASON", "refund reason must contain 2 to 500 characters")
			return
		}
		if err := s.main.RequestOrderRefund(r.Context(), session.MainUserID, orderID, body.Reason); err != nil {
			s.writeMainError(w, requestID, err)
			return
		}
		s.writeData(w, http.StatusOK, requestID, map[string]string{"message": "refund requested"})
	default:
		s.writeError(w, http.StatusNotFound, requestID, "NOT_FOUND", "not found")
	}
}

func boundedPositiveInt(raw string, fallback, min, max int) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < min {
		return fallback
	}
	if value > max {
		return max
	}
	return value
}
