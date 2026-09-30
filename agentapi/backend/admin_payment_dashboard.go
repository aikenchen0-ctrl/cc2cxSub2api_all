package main

import (
	"context"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

type AgentAdminDailyPaymentStats struct {
	Date   string             `json:"date"`
	Amount map[string]float64 `json:"amount"`
	Count  int                `json:"count"`
}

type AgentAdminPaymentMethodStats struct {
	Type   string             `json:"type"`
	Amount map[string]float64 `json:"amount"`
	Count  int                `json:"count"`
}

type AgentAdminTopUserPaymentStats struct {
	MainUserID string  `json:"main_user_id"`
	Email      string  `json:"email,omitempty"`
	Name       string  `json:"name,omitempty"`
	Amount     float64 `json:"amount"`
}

type AgentAdminPaymentDashboard struct {
	TodayAmount    map[string]float64                         `json:"today_amount"`
	TotalAmount    map[string]float64                         `json:"total_amount"`
	TodayCount     int                                        `json:"today_count"`
	TotalCount     int                                        `json:"total_count"`
	AvgAmount      map[string]float64                         `json:"avg_amount"`
	PendingOrders  int                                        `json:"pending_orders"`
	DailySeries    []AgentAdminDailyPaymentStats              `json:"daily_series"`
	PaymentMethods []AgentAdminPaymentMethodStats             `json:"payment_methods"`
	TopUsers       map[string][]AgentAdminTopUserPaymentStats `json:"top_users"`
}

type agentAdminPaidOrder struct {
	Order  AgentOrder
	User   AgentUserView
	PaidAt time.Time
}

type agentAdminUserPaymentStats struct {
	Orders  []agentAdminPaidOrder
	Pending int64
	Err     error
}

var agentAdminPaidStatuses = []string{"COMPLETED", "PAID", "RECHARGING"}

func (s *Server) agentAdminPaymentDashboard(ctx context.Context, days int) (AgentAdminPaymentDashboard, error) {
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	paidSince := todayStart.AddDate(0, 0, -(days - 1))
	users, err := s.allMappedUsers()
	if err != nil {
		return AgentAdminPaymentDashboard{}, err
	}

	results := make(chan agentAdminUserPaymentStats, len(users))
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
				results <- agentAdminUserPaymentStats{Err: ctx.Err()}
				return
			}
			defer func() { <-semaphore }()
			orders, pending, fetchErr := s.fetchMappedUserPaymentStats(ctx, mapped, paidSince)
			results <- agentAdminUserPaymentStats{Orders: orders, Pending: pending, Err: fetchErr}
		}()
	}
	wg.Wait()
	close(results)

	orders := make([]agentAdminPaidOrder, 0)
	var pending int64
	for result := range results {
		if result.Err != nil {
			return AgentAdminPaymentDashboard{}, result.Err
		}
		orders = append(orders, result.Orders...)
		pending += result.Pending
	}
	return buildAgentAdminPaymentDashboard(orders, int(pending), paidSince, todayStart, days), nil
}

func (s *Server) fetchMappedUserPaymentStats(ctx context.Context, user AgentUserView, paidSince time.Time) ([]agentAdminPaidOrder, int64, error) {
	pendingPage, err := s.main.Orders(ctx, user.MainUserID, 1, 1, "PENDING")
	if err != nil {
		return nil, 0, err
	}
	result := make([]agentAdminPaidOrder, 0)
	for _, status := range agentAdminPaidStatuses {
		for page := 1; ; page++ {
			upstream, fetchErr := s.main.OrdersSince(ctx, user.MainUserID, page, 100, status, paidSince)
			if fetchErr != nil {
				return nil, 0, fetchErr
			}
			for _, order := range upstream.Items {
				if order.PaidAt == nil {
					continue
				}
				paidAt, parseErr := time.Parse(time.RFC3339Nano, *order.PaidAt)
				if parseErr != nil {
					return nil, 0, invalidOrderResponse()
				}
				if paidAt.Before(paidSince) {
					continue
				}
				result = append(result, agentAdminPaidOrder{Order: order, User: user, PaidAt: paidAt})
			}
			if len(upstream.Items) == 0 || int64(page*upstream.PageSize) >= upstream.Total {
				break
			}
		}
	}
	return result, pendingPage.Total, nil
}

func buildAgentAdminPaymentDashboard(orders []agentAdminPaidOrder, pending int, paidSince, todayStart time.Time, days int) AgentAdminPaymentDashboard {
	stats := AgentAdminPaymentDashboard{
		TodayAmount:    map[string]float64{},
		TotalAmount:    map[string]float64{},
		AvgAmount:      map[string]float64{},
		PendingOrders:  pending,
		DailySeries:    make([]AgentAdminDailyPaymentStats, 0, days),
		PaymentMethods: []AgentAdminPaymentMethodStats{},
		TopUsers:       map[string][]AgentAdminTopUserPaymentStats{},
	}
	daily := map[string]*AgentAdminDailyPaymentStats{}
	methods := map[string]*AgentAdminPaymentMethodStats{}
	topUsers := map[string]map[string]*AgentAdminTopUserPaymentStats{}
	currencyCounts := map[string]int{}

	for _, entry := range orders {
		currency := strings.ToUpper(strings.TrimSpace(entry.Order.Currency))
		if currency == "" {
			currency = "CNY"
		}
		amount := entry.Order.PayAmount
		stats.TotalAmount[currency] += amount
		stats.TotalCount++
		currencyCounts[currency]++
		localPaidAt := entry.PaidAt.In(todayStart.Location())
		if !localPaidAt.Before(todayStart) {
			stats.TodayAmount[currency] += amount
			stats.TodayCount++
		}

		dayKey := localPaidAt.Format("2006-01-02")
		day := daily[dayKey]
		if day == nil {
			day = &AgentAdminDailyPaymentStats{Date: dayKey, Amount: map[string]float64{}}
			daily[dayKey] = day
		}
		day.Amount[currency] += amount
		day.Count++

		methodKey := strings.TrimSpace(entry.Order.PaymentType)
		if methodKey == "" {
			methodKey = "unknown"
		}
		method := methods[methodKey]
		if method == nil {
			method = &AgentAdminPaymentMethodStats{Type: methodKey, Amount: map[string]float64{}}
			methods[methodKey] = method
		}
		method.Amount[currency] += amount
		method.Count++

		currencyUsers := topUsers[currency]
		if currencyUsers == nil {
			currencyUsers = map[string]*AgentAdminTopUserPaymentStats{}
			topUsers[currency] = currencyUsers
		}
		user := currencyUsers[entry.User.MainUserID]
		if user == nil {
			user = &AgentAdminTopUserPaymentStats{MainUserID: entry.User.MainUserID, Email: entry.User.Email, Name: entry.User.DisplayName}
			currencyUsers[entry.User.MainUserID] = user
		}
		user.Amount += amount
	}

	for currency, total := range stats.TotalAmount {
		stats.TotalAmount[currency] = roundAgentAdminAmount(total)
		stats.TodayAmount[currency] = roundAgentAdminAmount(stats.TodayAmount[currency])
		stats.AvgAmount[currency] = roundAgentAdminAmount(total / float64(currencyCounts[currency]))
	}
	for offset := 0; offset < days; offset++ {
		date := paidSince.AddDate(0, 0, offset).Format("2006-01-02")
		item := daily[date]
		if item == nil {
			item = &AgentAdminDailyPaymentStats{Date: date, Amount: map[string]float64{}}
		}
		roundAgentAdminCurrencyAmounts(item.Amount)
		stats.DailySeries = append(stats.DailySeries, *item)
	}
	methodKeys := make([]string, 0, len(methods))
	for key := range methods {
		methodKeys = append(methodKeys, key)
	}
	sort.Strings(methodKeys)
	for _, key := range methodKeys {
		roundAgentAdminCurrencyAmounts(methods[key].Amount)
		stats.PaymentMethods = append(stats.PaymentMethods, *methods[key])
	}
	for currency, byUser := range topUsers {
		items := make([]AgentAdminTopUserPaymentStats, 0, len(byUser))
		for _, user := range byUser {
			user.Amount = roundAgentAdminAmount(user.Amount)
			items = append(items, *user)
		}
		sort.Slice(items, func(i, j int) bool {
			if items[i].Amount != items[j].Amount {
				return items[i].Amount > items[j].Amount
			}
			return items[i].MainUserID < items[j].MainUserID
		})
		if len(items) > 10 {
			items = items[:10]
		}
		stats.TopUsers[currency] = items
	}
	return stats
}

func roundAgentAdminCurrencyAmounts(amounts map[string]float64) {
	for currency, amount := range amounts {
		amounts[currency] = roundAgentAdminAmount(amount)
	}
}

func roundAgentAdminAmount(amount float64) float64 {
	return math.Round(amount*100) / 100
}
