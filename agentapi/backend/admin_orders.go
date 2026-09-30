package main

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// AgentAdminOrder is the tenant-administrator view of a main-site order. The
// payment facts are still the sanitized user-scoped Sub2API result; AgentAPI
// only attaches the local mapping that proves which tenant user owns it.
type AgentAdminOrder struct {
	AgentOrder
	MainUserID      string `json:"main_user_id"`
	UserEmail       string `json:"user_email,omitempty"`
	UserDisplayName string `json:"user_display_name,omitempty"`
}

type AgentAdminOrderPage struct {
	Items    []AgentAdminOrder `json:"items"`
	Total    int64             `json:"total"`
	Page     int               `json:"page"`
	PageSize int               `json:"page_size"`
	Pages    int               `json:"pages"`
}

var allowedAdminOrderStatuses = map[string]bool{
	"": true, "PENDING": true, "PAID": true, "COMPLETED": true,
	"RECHARGING": true,
	"EXPIRED":    true, "CANCELLED": true, "FAILED": true,
	"REFUND_REQUESTED": true, "REFUNDING": true, "REFUND_PENDING": true,
	"PARTIALLY_REFUNDED": true, "REFUNDED": true, "REFUND_FAILED": true,
}

func (s *Server) handleAgentAdminOrders(w http.ResponseWriter, r *http.Request, requestID string, actor Session) {
	const basePath = "/api/v1/agent/admin/orders"
	w.Header().Set("Cache-Control", "no-store")
	if r.URL.Path == basePath+"/dashboard" {
		if r.Method != http.MethodGet {
			s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		days := boundedPositiveInt(r.URL.Query().Get("days"), 30, 1, 90)
		if days != 7 && days != 30 && days != 90 {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_DASHBOARD_WINDOW", "dashboard window must be 7, 30, or 90 days")
			return
		}
		result, err := s.agentAdminPaymentDashboard(r.Context(), days)
		if err != nil {
			s.writeMainError(w, requestID, err)
			return
		}
		s.writeData(w, http.StatusOK, requestID, result)
		return
	}
	if r.URL.Path == basePath {
		if r.Method != http.MethodGet {
			s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		page := boundedPositiveInt(r.URL.Query().Get("page"), 1, 1, 1000)
		pageSize := boundedPositiveInt(r.URL.Query().Get("page_size"), 20, 1, 100)
		status := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("status")))
		if !allowedAdminOrderStatuses[status] {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_ORDER_STATUS", "unsupported order status")
			return
		}
		mainUserID := strings.TrimSpace(r.URL.Query().Get("main_user_id"))
		result, err := s.agentAdminOrders(r.Context(), page, pageSize, status, mainUserID)
		if err != nil {
			if errors.Is(err, errNotFound) {
				s.writeError(w, http.StatusNotFound, requestID, "MAPPED_USER_NOT_FOUND", "mapped user not found")
				return
			}
			s.writeMainError(w, requestID, err)
			return
		}
		s.writeData(w, http.StatusOK, requestID, result)
		return
	}

	relative := strings.TrimPrefix(r.URL.Path, basePath+"/")
	parts := strings.Split(relative, "/")
	if len(parts) != 3 || parts[2] != "cancel" || r.Method != http.MethodPost {
		s.writeError(w, http.StatusNotFound, requestID, "NOT_FOUND", "not found")
		return
	}
	if !sameOrigin(r) {
		s.writeError(w, http.StatusForbidden, requestID, "CSRF_ORIGIN_REJECTED", "cross-origin state-changing requests are not allowed")
		return
	}
	mainUserID := strings.TrimSpace(parts[0])
	orderID, err := strconv.ParseInt(parts[1], 10, 64)
	if mainUserID == "" || err != nil || orderID <= 0 {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_ORDER_ID", "invalid order target")
		return
	}
	if _, err := s.store.User(s.cfg.AgentID, mainUserID); err != nil {
		if errors.Is(err, errNotFound) {
			s.writeError(w, http.StatusNotFound, requestID, "MAPPED_USER_NOT_FOUND", "mapped user not found")
			return
		}
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to verify mapped user")
		return
	}
	if err := s.main.CancelOrder(r.Context(), mainUserID, orderID); err != nil {
		s.writeMainError(w, requestID, err)
		return
	}
	s.recordAudit("agent_admin", actor.MainUserID, "orders.cancel", "main_order", strconv.FormatInt(orderID, 10), requestID, "success", "mapped_user="+mainUserID)
	s.writeData(w, http.StatusOK, requestID, map[string]string{"message": "order cancelled"})
}

func (s *Server) agentAdminOrders(ctx context.Context, page, pageSize int, status, selectedMainUserID string) (AgentAdminOrderPage, error) {
	if selectedMainUserID != "" {
		user, err := s.store.User(s.cfg.AgentID, selectedMainUserID)
		if err != nil {
			return AgentAdminOrderPage{}, err
		}
		upstream, err := s.main.Orders(ctx, selectedMainUserID, page, pageSize, status)
		if err != nil {
			return AgentAdminOrderPage{}, err
		}
		items := decorateAdminOrders(upstream.Items, user)
		return AgentAdminOrderPage{Items: items, Total: upstream.Total, Page: upstream.Page, PageSize: upstream.PageSize, Pages: upstream.Pages}, nil
	}

	users, err := s.allMappedUsers()
	if err != nil {
		return AgentAdminOrderPage{}, err
	}
	if len(users) == 0 {
		return AgentAdminOrderPage{Items: []AgentAdminOrder{}, Page: page, PageSize: pageSize}, nil
	}

	// Each user's endpoint is already ordered newest-first. Fetching the first
	// page*pageSize rows from every user is sufficient to compute the exact
	// requested global page; totals come from the upstream pagination metadata.
	limit := page * pageSize
	type userResult struct {
		items []AgentAdminOrder
		total int64
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
			items, total, fetchErr := s.fetchMappedUserOrders(ctx, mapped, status, limit)
			results <- userResult{items: items, total: total, err: fetchErr}
		}()
	}
	wg.Wait()
	close(results)

	all := make([]AgentAdminOrder, 0, limit)
	var total int64
	for result := range results {
		if result.err != nil {
			return AgentAdminOrderPage{}, result.err
		}
		total += result.total
		all = append(all, result.items...)
	}
	sort.SliceStable(all, func(i, j int) bool {
		left, leftErr := time.Parse(time.RFC3339Nano, all[i].CreatedAt)
		right, rightErr := time.Parse(time.RFC3339Nano, all[j].CreatedAt)
		if leftErr == nil && rightErr == nil && !left.Equal(right) {
			return left.After(right)
		}
		if all[i].ID != all[j].ID {
			return all[i].ID > all[j].ID
		}
		return all[i].MainUserID < all[j].MainUserID
	})
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
	return AgentAdminOrderPage{Items: all[start:end], Total: total, Page: page, PageSize: pageSize, Pages: pages}, nil
}

func (s *Server) allMappedUsers() ([]AgentUserView, error) {
	result := make([]AgentUserView, 0, 500)
	for offset := 0; ; offset += 500 {
		items, total, err := s.store.UsersPage(s.cfg.AgentID, 500, offset, "")
		if err != nil {
			return nil, err
		}
		result = append(result, items...)
		if len(result) >= total || len(items) == 0 {
			return result, nil
		}
	}
}

func (s *Server) fetchMappedUserOrders(ctx context.Context, user AgentUserView, status string, limit int) ([]AgentAdminOrder, int64, error) {
	items := make([]AgentAdminOrder, 0, min(limit, 100))
	var total int64
	for page := 1; len(items) < limit; page++ {
		pageSize := min(100, limit-len(items))
		upstream, err := s.main.Orders(ctx, user.MainUserID, page, pageSize, status)
		if err != nil {
			return nil, 0, err
		}
		if page == 1 {
			total = upstream.Total
		}
		items = append(items, decorateAdminOrders(upstream.Items, user)...)
		if len(upstream.Items) == 0 || int64(len(items)) >= total {
			break
		}
	}
	return items, total, nil
}

func decorateAdminOrders(orders []AgentOrder, user AgentUserView) []AgentAdminOrder {
	result := make([]AgentAdminOrder, 0, len(orders))
	for _, order := range orders {
		result = append(result, AgentAdminOrder{
			AgentOrder:      order,
			MainUserID:      user.MainUserID,
			UserEmail:       user.Email,
			UserDisplayName: user.DisplayName,
		})
	}
	return result
}
