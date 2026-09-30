package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type AgentAffiliateInviteRecord struct {
	InviterID       int64   `json:"inviter_id"`
	InviterEmail    string  `json:"inviter_email"`
	InviterUsername string  `json:"inviter_username"`
	InviteeID       int64   `json:"invitee_id"`
	InviteeEmail    string  `json:"invitee_email"`
	InviteeUsername string  `json:"invitee_username"`
	AffCode         string  `json:"aff_code"`
	TotalRebate     float64 `json:"total_rebate"`
	CreatedAt       string  `json:"created_at"`
}

type AgentAffiliateRebateRecord struct {
	OrderID         int64   `json:"order_id"`
	OutTradeNo      string  `json:"out_trade_no"`
	InviterID       int64   `json:"inviter_id"`
	InviterEmail    string  `json:"inviter_email"`
	InviterUsername string  `json:"inviter_username"`
	InviteeID       int64   `json:"invitee_id"`
	InviteeEmail    string  `json:"invitee_email"`
	InviteeUsername string  `json:"invitee_username"`
	OrderAmount     float64 `json:"order_amount"`
	PayAmount       float64 `json:"pay_amount"`
	RebateAmount    float64 `json:"rebate_amount"`
	PaymentType     string  `json:"payment_type"`
	OrderStatus     string  `json:"order_status"`
	CreatedAt       string  `json:"created_at"`
}

type AgentAffiliateTransferRecord struct {
	LedgerID            int64    `json:"ledger_id"`
	UserID              int64    `json:"user_id"`
	UserEmail           string   `json:"user_email"`
	Username            string   `json:"username"`
	Amount              float64  `json:"amount"`
	BalanceAfter        *float64 `json:"balance_after,omitempty"`
	AvailableQuotaAfter *float64 `json:"available_quota_after,omitempty"`
	FrozenQuotaAfter    *float64 `json:"frozen_quota_after,omitempty"`
	HistoryQuotaAfter   *float64 `json:"history_quota_after,omitempty"`
	SnapshotAvailable   bool     `json:"snapshot_available"`
	CreatedAt           string   `json:"created_at"`
}

type AgentAffiliateRecordPage[T any] struct {
	Items    []T   `json:"items"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
	Pages    int   `json:"pages"`
}

type affiliateRecordQuery struct {
	Page       int
	PageSize   int
	Search     string
	StartAt    *time.Time
	EndAt      *time.Time
	SortBy     string
	Descending bool
	MainUserID string
}

func (c *MainClient) AffiliateInvites(ctx context.Context, mainUserID string, page, pageSize int, search string, startAt, endAt *time.Time) (AgentAffiliateRecordPage[AgentAffiliateInviteRecord], error) {
	var result AgentAffiliateRecordPage[AgentAffiliateInviteRecord]
	err := c.affiliateRecords(ctx, mainUserID, "invites", page, pageSize, search, startAt, endAt, &result)
	return result, err
}

func (c *MainClient) AffiliateRebates(ctx context.Context, mainUserID string, page, pageSize int, search string, startAt, endAt *time.Time) (AgentAffiliateRecordPage[AgentAffiliateRebateRecord], error) {
	var result AgentAffiliateRecordPage[AgentAffiliateRebateRecord]
	err := c.affiliateRecords(ctx, mainUserID, "rebates", page, pageSize, search, startAt, endAt, &result)
	return result, err
}

func (c *MainClient) AffiliateTransfers(ctx context.Context, mainUserID string, page, pageSize int, search string, startAt, endAt *time.Time) (AgentAffiliateRecordPage[AgentAffiliateTransferRecord], error) {
	var result AgentAffiliateRecordPage[AgentAffiliateTransferRecord]
	err := c.affiliateRecords(ctx, mainUserID, "transfers", page, pageSize, search, startAt, endAt, &result)
	return result, err
}

func (c *MainClient) affiliateRecords(ctx context.Context, mainUserID, kind string, page, pageSize int, search string, startAt, endAt *time.Time, target any) error {
	query := url.Values{}
	query.Set("page", strconv.Itoa(page))
	query.Set("page_size", strconv.Itoa(pageSize))
	query.Set("sort_by", "created_at")
	query.Set("sort_order", "desc")
	if search != "" {
		query.Set("search", search)
	}
	if startAt != nil {
		query.Set("start_at", startAt.UTC().Format(time.RFC3339))
	}
	if endAt != nil {
		query.Set("end_at", endAt.UTC().Format(time.RFC3339))
	}
	data, err := c.satelliteUserJSON(ctx, mainUserID, "/v1/sub2api/affiliate/"+kind+"?"+query.Encode())
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, target); err != nil {
		return &MainAPIError{Status: http.StatusBadGateway, Code: "SATELLITE_INVALID_RESPONSE", Message: "main-site affiliate record lookup returned an invalid response"}
	}
	return nil
}

func (s *Server) handleAgentAdminAffiliates(w http.ResponseWriter, r *http.Request, requestID string) {
	const basePath = "/api/v1/agent/admin/affiliates/"
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	kind := strings.TrimPrefix(r.URL.Path, basePath)
	if kind != "invites" && kind != "rebates" && kind != "transfers" {
		s.writeError(w, http.StatusNotFound, requestID, "NOT_FOUND", "not found")
		return
	}
	query, err := parseAffiliateRecordQuery(r)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_AFFILIATE_FILTER", err.Error())
		return
	}
	result, err := s.agentAdminAffiliateRecords(r.Context(), kind, query)
	if err != nil {
		if errors.Is(err, errNotFound) {
			s.writeError(w, http.StatusNotFound, requestID, "MAPPED_USER_NOT_FOUND", "mapped user not found")
			return
		}
		s.writeMainError(w, requestID, err)
		return
	}
	s.writeData(w, http.StatusOK, requestID, result)
}

func parseAffiliateRecordQuery(r *http.Request) (affiliateRecordQuery, error) {
	query := affiliateRecordQuery{
		Page:       boundedPositiveInt(r.URL.Query().Get("page"), 1, 1, 1000000),
		PageSize:   boundedPositiveInt(r.URL.Query().Get("page_size"), 20, 1, 100),
		Search:     strings.TrimSpace(r.URL.Query().Get("search")),
		SortBy:     strings.TrimSpace(r.URL.Query().Get("sort_by")),
		Descending: r.URL.Query().Get("sort_order") != "asc",
		MainUserID: strings.TrimSpace(r.URL.Query().Get("main_user_id")),
	}
	if len(query.Search) > 200 || len(query.SortBy) > 64 || len(query.MainUserID) > 64 {
		return query, errors.New("affiliate filter is too long")
	}
	var err error
	if raw := strings.TrimSpace(r.URL.Query().Get("start_at")); raw != "" {
		value, parseErr := time.Parse("2006-01-02", raw)
		if parseErr != nil {
			return query, errors.New("invalid start date")
		}
		query.StartAt = &value
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("end_at")); raw != "" {
		value, parseErr := time.Parse("2006-01-02", raw)
		if parseErr != nil {
			return query, errors.New("invalid end date")
		}
		value = value.Add(24*time.Hour - time.Nanosecond)
		query.EndAt = &value
	}
	if query.StartAt != nil && query.EndAt != nil && query.StartAt.After(*query.EndAt) {
		err = errors.New("start date must not be after end date")
	}
	return query, err
}

func (s *Server) agentAdminAffiliateRecords(ctx context.Context, kind string, query affiliateRecordQuery) (any, error) {
	allUsers, err := s.allMappedUsers()
	if err != nil {
		return nil, err
	}
	mapped := make(map[int64]AgentUserView, len(allUsers))
	for _, user := range allUsers {
		id, parseErr := strconv.ParseInt(user.MainUserID, 10, 64)
		if parseErr == nil && id > 0 {
			mapped[id] = user
		}
	}
	users := allUsers
	if query.MainUserID != "" {
		user, userErr := s.store.User(s.cfg.AgentID, query.MainUserID)
		if userErr != nil {
			return nil, userErr
		}
		users = []AgentUserView{user}
	}
	switch kind {
	case "invites":
		return s.aggregateAffiliateInvites(ctx, users, mapped, query)
	case "rebates":
		return s.aggregateAffiliateRebates(ctx, users, mapped, query)
	default:
		return s.aggregateAffiliateTransfers(ctx, users, mapped, query)
	}
}

func fetchAffiliatePages[T any](ctx context.Context, users []AgentUserView, fetch func(context.Context, string, int) (AgentAffiliateRecordPage[T], error)) ([]T, error) {
	type result struct {
		items []T
		err   error
	}
	results := make(chan result, len(users))
	semaphore := make(chan struct{}, 6)
	var wg sync.WaitGroup
	for _, user := range users {
		user := user
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case semaphore <- struct{}{}:
			case <-ctx.Done():
				results <- result{err: ctx.Err()}
				return
			}
			defer func() { <-semaphore }()
			items := make([]T, 0)
			for page := 1; ; page++ {
				upstream, err := fetch(ctx, user.MainUserID, page)
				if err != nil {
					results <- result{err: err}
					return
				}
				items = append(items, upstream.Items...)
				if len(upstream.Items) == 0 || int64(len(items)) >= upstream.Total {
					break
				}
			}
			results <- result{items: items}
		}()
	}
	wg.Wait()
	close(results)
	all := make([]T, 0)
	for result := range results {
		if result.err != nil {
			return nil, result.err
		}
		all = append(all, result.items...)
	}
	return all, nil
}

func (s *Server) aggregateAffiliateInvites(ctx context.Context, users []AgentUserView, mapped map[int64]AgentUserView, query affiliateRecordQuery) (AgentAffiliateRecordPage[AgentAffiliateInviteRecord], error) {
	items, err := fetchAffiliatePages(ctx, users, func(ctx context.Context, userID string, page int) (AgentAffiliateRecordPage[AgentAffiliateInviteRecord], error) {
		return s.main.AffiliateInvites(ctx, userID, page, 100, query.Search, query.StartAt, query.EndAt)
	})
	if err != nil {
		return AgentAffiliateRecordPage[AgentAffiliateInviteRecord]{}, err
	}
	filtered := items[:0]
	for _, item := range items {
		inviter, inviterOK := mapped[item.InviterID]
		invitee, inviteeOK := mapped[item.InviteeID]
		if !inviterOK || !inviteeOK {
			continue
		}
		item.InviterEmail, item.InviterUsername = inviter.Email, inviter.DisplayName
		item.InviteeEmail, item.InviteeUsername = invitee.Email, invitee.DisplayName
		filtered = append(filtered, item)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		return affiliateInviteLess(filtered[i], filtered[j], query.SortBy, query.Descending)
	})
	return paginateAffiliateRecords(filtered, query), nil
}

func (s *Server) aggregateAffiliateRebates(ctx context.Context, users []AgentUserView, mapped map[int64]AgentUserView, query affiliateRecordQuery) (AgentAffiliateRecordPage[AgentAffiliateRebateRecord], error) {
	items, err := fetchAffiliatePages(ctx, users, func(ctx context.Context, userID string, page int) (AgentAffiliateRecordPage[AgentAffiliateRebateRecord], error) {
		return s.main.AffiliateRebates(ctx, userID, page, 100, query.Search, query.StartAt, query.EndAt)
	})
	if err != nil {
		return AgentAffiliateRecordPage[AgentAffiliateRebateRecord]{}, err
	}
	filtered := items[:0]
	for _, item := range items {
		inviter, inviterOK := mapped[item.InviterID]
		invitee, inviteeOK := mapped[item.InviteeID]
		if !inviterOK || !inviteeOK {
			continue
		}
		item.InviterEmail, item.InviterUsername = inviter.Email, inviter.DisplayName
		item.InviteeEmail, item.InviteeUsername = invitee.Email, invitee.DisplayName
		filtered = append(filtered, item)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		return affiliateRebateLess(filtered[i], filtered[j], query.SortBy, query.Descending)
	})
	return paginateAffiliateRecords(filtered, query), nil
}

func (s *Server) aggregateAffiliateTransfers(ctx context.Context, users []AgentUserView, mapped map[int64]AgentUserView, query affiliateRecordQuery) (AgentAffiliateRecordPage[AgentAffiliateTransferRecord], error) {
	items, err := fetchAffiliatePages(ctx, users, func(ctx context.Context, userID string, page int) (AgentAffiliateRecordPage[AgentAffiliateTransferRecord], error) {
		return s.main.AffiliateTransfers(ctx, userID, page, 100, query.Search, query.StartAt, query.EndAt)
	})
	if err != nil {
		return AgentAffiliateRecordPage[AgentAffiliateTransferRecord]{}, err
	}
	filtered := items[:0]
	for _, item := range items {
		user, ok := mapped[item.UserID]
		if !ok {
			continue
		}
		item.UserEmail, item.Username = user.Email, user.DisplayName
		filtered = append(filtered, item)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		return affiliateTransferLess(filtered[i], filtered[j], query.SortBy, query.Descending)
	})
	return paginateAffiliateRecords(filtered, query), nil
}

func paginateAffiliateRecords[T any](items []T, query affiliateRecordQuery) AgentAffiliateRecordPage[T] {
	total := int64(len(items))
	start := (query.Page - 1) * query.PageSize
	end := start + query.PageSize
	if start > len(items) {
		start = len(items)
	}
	if end > len(items) {
		end = len(items)
	}
	pages := 0
	if total > 0 {
		pages = int((total + int64(query.PageSize) - 1) / int64(query.PageSize))
	}
	return AgentAffiliateRecordPage[T]{Items: items[start:end], Total: total, Page: query.Page, PageSize: query.PageSize, Pages: pages}
}

func compareAffiliateValue(left, right string, descending bool) bool {
	if descending {
		return left > right
	}
	return left < right
}

func affiliateInviteLess(left, right AgentAffiliateInviteRecord, key string, descending bool) bool {
	switch key {
	case "inviter":
		return compareAffiliateValue(strings.ToLower(left.InviterEmail), strings.ToLower(right.InviterEmail), descending)
	case "invitee":
		return compareAffiliateValue(strings.ToLower(left.InviteeEmail), strings.ToLower(right.InviteeEmail), descending)
	case "aff_code":
		return compareAffiliateValue(left.AffCode, right.AffCode, descending)
	case "total_rebate":
		if descending {
			return left.TotalRebate > right.TotalRebate
		}
		return left.TotalRebate < right.TotalRebate
	default:
		return compareAffiliateValue(left.CreatedAt, right.CreatedAt, descending)
	}
}

func affiliateRebateLess(left, right AgentAffiliateRebateRecord, key string, descending bool) bool {
	switch key {
	case "order":
		if descending {
			return left.OrderID > right.OrderID
		}
		return left.OrderID < right.OrderID
	case "inviter":
		return compareAffiliateValue(strings.ToLower(left.InviterEmail), strings.ToLower(right.InviterEmail), descending)
	case "invitee":
		return compareAffiliateValue(strings.ToLower(left.InviteeEmail), strings.ToLower(right.InviteeEmail), descending)
	case "order_amount":
		if descending {
			return left.OrderAmount > right.OrderAmount
		}
		return left.OrderAmount < right.OrderAmount
	case "pay_amount":
		if descending {
			return left.PayAmount > right.PayAmount
		}
		return left.PayAmount < right.PayAmount
	case "rebate_amount":
		if descending {
			return left.RebateAmount > right.RebateAmount
		}
		return left.RebateAmount < right.RebateAmount
	case "payment_type":
		return compareAffiliateValue(left.PaymentType, right.PaymentType, descending)
	case "order_status":
		return compareAffiliateValue(left.OrderStatus, right.OrderStatus, descending)
	default:
		return compareAffiliateValue(left.CreatedAt, right.CreatedAt, descending)
	}
}

func affiliateTransferLess(left, right AgentAffiliateTransferRecord, key string, descending bool) bool {
	switch key {
	case "user":
		return compareAffiliateValue(strings.ToLower(left.UserEmail), strings.ToLower(right.UserEmail), descending)
	case "amount":
		if descending {
			return left.Amount > right.Amount
		}
		return left.Amount < right.Amount
	default:
		return compareAffiliateValue(left.CreatedAt, right.CreatedAt, descending)
	}
}
