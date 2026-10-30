package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/agentapi/payment"
	payprovider "github.com/Wei-Shaw/agentapi/payment/provider"
)

// Station payment notifications are the fast path, never the only path. A
// single rejected callback (wrong Alipay public key, proxy dropping the POST,
// provider giving up after its retry window) used to leave a paid order in
// PENDING forever because nothing ever asked the provider what happened.
// These helpers query the provider directly and settle the order from the
// authoritative answer.

// stationQueryThrottle bounds how often one order may be queried upstream, so
// retries from a provider or a polling browser cannot amplify into a storm of
// trade queries.
var (
	stationQueryMu       sync.Mutex
	stationQueryLastSeen = map[string]time.Time{}
)

const stationQueryMinInterval = 10 * time.Second

func stationQueryAllowed(key string, now time.Time) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	stationQueryMu.Lock()
	defer stationQueryMu.Unlock()
	if last, ok := stationQueryLastSeen[key]; ok && now.Sub(last) < stationQueryMinInterval {
		return false
	}
	if len(stationQueryLastSeen) > 4096 {
		for k, v := range stationQueryLastSeen {
			if now.Sub(v) > 10*time.Minute {
				delete(stationQueryLastSeen, k)
			}
		}
	}
	stationQueryLastSeen[key] = now
	return true
}

// settleStationOrder completes a paid station order and credits the local
// wallet. CreditUserRecharge is idempotent per order number, so a repeated
// call can never double-credit a user.
func (s *Server) settleStationOrder(agentID, outTradeNo, providerTradeNo string) (StationPaymentOrder, error) {
	completed, _, err := s.store.CompleteStationOrder(agentID, outTradeNo, providerTradeNo)
	if err != nil {
		return StationPaymentOrder{}, err
	}
	cents := int64(math.Round(completed.Amount * 100))
	if err := s.store.CreditUserRecharge(agentID, completed.MainUserID, cents, completed.OutTradeNo); err != nil {
		return completed, err
	}
	return completed, nil
}

// stationProviderForOrder rebuilds the payment provider that issued an order
// so the trade status can be queried without a browser callback.
func (s *Server) stationProviderForOrder(agentID string, order StationPaymentOrder) (payment.Provider, error) {
	providerID, err := strconv.ParseInt(strings.TrimSpace(order.ProviderInstanceID), 10, 64)
	if err != nil || providerID <= 0 {
		return nil, errNotFound
	}
	item, err := s.store.StationProvider(agentID, providerID, true)
	if err != nil {
		return nil, err
	}
	config := make(map[string]string, len(item.Config)+2)
	for key, value := range item.Config {
		config[key] = value
	}
	if strings.TrimSpace(config["notifyUrl"]) == "" || strings.TrimSpace(config["returnUrl"]) == "" {
		if agent, agentErr := s.store.Agent(agentID); agentErr == nil && strings.TrimSpace(agent.Domain) != "" {
			origin := "https://" + strings.TrimSpace(agent.Domain)
			if strings.TrimSpace(config["notifyUrl"]) == "" {
				config["notifyUrl"] = origin + stationWebhookPath(agentID, item.ProviderKey)
			}
			if strings.TrimSpace(config["returnUrl"]) == "" {
				config["returnUrl"] = origin + "/payment/result"
			}
		}
	}
	return payprovider.CreateProvider(item.ProviderKey, order.ProviderInstanceID, config)
}

// reconcileStationOrder asks the provider whether a pending order was paid and
// settles it when it was. It returns true only when the order is settled.
func (s *Server) reconcileStationOrder(ctx context.Context, agentID, outTradeNo, reason string) (StationPaymentOrder, bool) {
	agentID = strings.TrimSpace(agentID)
	outTradeNo = strings.TrimSpace(outTradeNo)
	if agentID == "" || outTradeNo == "" {
		return StationPaymentOrder{}, false
	}
	order, err := s.store.StationOrderByNo(agentID, outTradeNo)
	if err != nil {
		return StationPaymentOrder{}, false
	}
	if !strings.EqualFold(order.Status, "PENDING") {
		return order, false
	}
	if !stationQueryAllowed("station:"+agentID+":"+outTradeNo, time.Now()) {
		return order, false
	}
	impl, err := s.stationProviderForOrder(agentID, order)
	if err != nil {
		slog.Warn("station order reconcile: provider unavailable", "agent_id", agentID, "out_trade_no", outTradeNo, "reason", reason, "error", err)
		return order, false
	}
	result, err := impl.QueryOrder(ctx, order.OutTradeNo)
	if err != nil {
		slog.Warn("station order reconcile: trade query failed", "agent_id", agentID, "out_trade_no", outTradeNo, "reason", reason, "error", err)
		return order, false
	}
	if result == nil || result.Status != payment.ProviderStatusPaid {
		return order, false
	}
	if math.IsNaN(result.Amount) || math.IsInf(result.Amount, 0) || math.Abs(order.PayAmount-result.Amount) > 0.01 {
		slog.Warn("station order reconcile: amount mismatch", "agent_id", agentID, "out_trade_no", outTradeNo, "order_amount", order.PayAmount, "provider_amount", result.Amount, "reason", reason)
		return order, false
	}
	completed, err := s.settleStationOrder(agentID, order.OutTradeNo, result.TradeNo)
	if err != nil {
		slog.Error("station order reconcile: settlement failed", "agent_id", agentID, "out_trade_no", outTradeNo, "reason", reason, "error", err)
		return order, false
	}
	slog.Info("station order settled from provider query", "agent_id", agentID, "out_trade_no", outTradeNo, "trade_no", result.TradeNo, "amount", completed.Amount, "reason", reason)
	return completed, true
}

// reconcilePendingStationOrders sweep runs from the background reconciler.
func (s *Server) reconcilePendingStationOrders(ctx context.Context, agentID string) {
	orders, err := s.store.PendingStationOrders(agentID, time.Now().Add(-7*24*time.Hour).Unix(), s.cfg.SettlementReconcileBatch)
	if err != nil {
		slog.Warn("station order reconciliation lookup failed", "agent_id", agentID, "error", err)
		return
	}
	settled := 0
	for _, order := range orders {
		if _, ok := s.reconcileStationOrder(ctx, agentID, order.OutTradeNo, "background"); ok {
			settled++
		}
	}
	if settled > 0 {
		slog.Info("background station order reconciliation completed", "agent_id", agentID, "settled", settled)
	}
}

// stationOrderIDFromRaw extracts the merchant order number from a callback body
// that failed signature verification. The value is only used to look up a
// local order before querying the provider, never trusted as proof of payment.
func stationOrderIDFromRaw(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	if values, err := url.ParseQuery(raw); err == nil {
		for _, key := range []string{"out_trade_no", "order_no", "orderNo", "outTradeNo"} {
			if value := strings.TrimSpace(values.Get(key)); value != "" && len(value) <= 128 {
				return value
			}
		}
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err == nil {
		for _, key := range []string{"out_trade_no", "order_no", "orderNo", "outTradeNo"} {
			if value, ok := payload[key].(string); ok {
				value = strings.TrimSpace(value)
				if value != "" && len(value) <= 128 {
					return value
				}
			}
		}
		if nested, ok := payload["data"].(map[string]any); ok {
			for _, key := range []string{"out_trade_no", "order_no", "orderNo", "outTradeNo"} {
				if value, ok := nested[key].(string); ok {
					value = strings.TrimSpace(value)
					if value != "" && len(value) <= 128 {
						return value
					}
				}
			}
		}
	}
	return ""
}

var _ = errors.Is
