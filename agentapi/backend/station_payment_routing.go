package main

import (
	"strconv"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/agentapi/payment"
	"github.com/shopspring/decimal"
)

// Select after filtering amount limits, so an exhausted instance does not hide
// another usable instance. Counters are isolated by tenant and checkout method.
func (s *Store) selectStationProvider(agentID string, providers []StationPaymentProvider, settings StationPaymentSettings, method string, amount float64) (StationPaymentProvider, AgentPaymentMethodLimit, bool, error) {
	return s.selectStationProviderForOrder(agentID, providers, settings, method, amount, "balance")
}

func stationGatewayAmount(amount float64, orderType, currency string, settings StationPaymentSettings) float64 {
	value := decimal.NewFromFloat(amount)
	if orderType == "subscription" && currency == "CNY" && settings.SubscriptionUSDToCNYRate > 0 {
		value = value.Mul(decimal.NewFromFloat(settings.SubscriptionUSDToCNYRate))
	}
	result, _ := value.Round(int32(payment.CurrencyMaxFractionDigits(currency))).Float64()
	return result
}

func stationMethodSourceMatches(provider StationPaymentProvider, method string, settings StationPaymentSettings) bool {
	source := ""
	if method == "alipay" {
		source = settings.VisibleMethodAlipaySource
	}
	if method == "wxpay" {
		source = settings.VisibleMethodWxpaySource
	}
	if source == "" {
		return true
	}
	expected := map[string]string{"official_alipay": "alipay", "easypay_alipay": "easypay", "official_wxpay": "wxpay", "easypay_wxpay": "easypay"}[source]
	return expected != "" && provider.ProviderKey == expected
}

func (s *Store) SumStationUserPayAmountSince(agentID, userID string, since time.Time) (float64, error) {
	var total float64
	err := s.db.QueryRow(`SELECT COALESCE(SUM(pay_amount),0) FROM station_payment_orders WHERE agent_id=? AND main_user_id=? AND created_at>=? AND (status IN ('PAID','RECHARGING','COMPLETED','REFUND_REQUESTED','REFUNDING','REFUND_PENDING','PARTIALLY_REFUNDED','REFUND_FAILED') OR (status='PENDING' AND expires_at>?))`, agentID, userID, since.UTC().Unix(), s.clock().UTC().Unix()).Scan(&total)
	return total, err
}

func (s *Store) stationProviderDailyUsed(agentID string, provider StationPaymentProvider, method string) (float64, error) {
	now := s.clock().UTC()
	var used float64
	err := s.db.QueryRow(`SELECT COALESCE(SUM(pay_amount),0) FROM station_payment_orders WHERE agent_id=? AND provider_id=? AND (?='stripe' OR payment_type=?) AND created_at>=? AND (status IN ('PAID','RECHARGING','COMPLETED','REFUND_REQUESTED','REFUNDING','REFUND_PENDING','PARTIALLY_REFUNDED','REFUND_FAILED') OR (status='PENDING' AND expires_at>?))`, agentID, provider.ID, provider.ProviderKey, method, now.Truncate(24*time.Hour).Unix(), now.Unix()).Scan(&used)
	return used, err
}

func (s *Store) selectStationProviderForOrder(agentID string, providers []StationPaymentProvider, settings StationPaymentSettings, method string, amount float64, orderType string) (StationPaymentProvider, AgentPaymentMethodLimit, bool, error) {
	type candidate struct {
		provider StationPaymentProvider
		limit    AgentPaymentMethodLimit
		used     float64
	}
	candidates := []candidate{}
	// Use the same first currency as checkout-info so the displayed unit cannot
	// silently change when another currency's instance is selected.
	currency := ""
	for _, provider := range providers {
		_, limit, ok := selectStationProvider([]StationPaymentProvider{provider}, settings, method)
		if !ok {
			continue
		}
		if currency == "" {
			currency = limit.Currency
		}
		if currency != limit.Currency {
			continue
		}
		if orderType == "balance" {
			if _, err := payment.AmountToMinorUnit(strconv.FormatFloat(amount, 'f', -1, 64), currency); err != nil {
				continue
			}
		}
		baseAmount := stationGatewayAmount(amount, orderType, limit.Currency, settings)
		payAmount, _ := strconv.ParseFloat(payment.CalculatePayAmountForCurrency(baseAmount, limit.FeeRate, limit.Currency), 64)
		if payAmount <= 0 || limit.SingleMin > 0 && payAmount < limit.SingleMin || limit.SingleMax > 0 && payAmount > limit.SingleMax {
			continue
		}
		used, err := s.stationProviderDailyUsed(agentID, provider, method)
		if err != nil {
			return StationPaymentProvider{}, AgentPaymentMethodLimit{}, false, err
		}
		if limit.DailyLimit > 0 && used+payAmount > limit.DailyLimit {
			continue
		}
		candidates = append(candidates, candidate{provider, limit, used})
	}
	if len(candidates) == 0 {
		return StationPaymentProvider{}, AgentPaymentMethodLimit{}, false, nil
	}
	chosen := candidates[0]
	if settings.LoadBalanceStrategy == "least-amount" {
		for _, item := range candidates[1:] {
			if item.used < chosen.used {
				chosen = item
			}
		}
	} else {
		value, _ := s.paymentCounters.LoadOrStore(struct{ agent, method string }{agentID, method}, &atomic.Uint64{})
		chosen = candidates[(value.(*atomic.Uint64).Add(1)-1)%uint64(len(candidates))]
	}
	return chosen.provider, chosen.limit, true, nil
}
