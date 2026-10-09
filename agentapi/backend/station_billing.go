package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/agentapi/payment"
	payprovider "github.com/Wei-Shaw/agentapi/payment/provider"
)

// Station charging is owned by the tenant. Settings, provider credentials,
// plans and orders are stored with agent_id and never read across tenants.

type StationPaymentSettings struct {
	Enabled                       bool     `json:"enabled"`
	MinAmount                     float64  `json:"min_amount"`
	MaxAmount                     float64  `json:"max_amount"`
	DailyLimit                    float64  `json:"daily_limit"`
	OrderTimeoutMin               int      `json:"order_timeout_minutes"`
	MaxPendingOrders              int      `json:"max_pending_orders"`
	EnabledTypes                  []string `json:"enabled_payment_types"`
	BalanceDisabled               bool     `json:"balance_disabled"`
	BalanceRechargeMultiplier     float64  `json:"balance_recharge_multiplier"`
	SubscriptionUSDToCNYRate      float64  `json:"subscription_usd_to_cny_rate"`
	RechargeFeeRate               float64  `json:"recharge_fee_rate"`
	LoadBalanceStrategy           string   `json:"load_balance_strategy"`
	ProductNamePrefix             string   `json:"product_name_prefix"`
	ProductNameSuffix             string   `json:"product_name_suffix"`
	HelpImageURL                  string   `json:"help_image_url"`
	HelpText                      string   `json:"help_text"`
	CancelRateLimitEnabled        bool     `json:"cancel_rate_limit_enabled"`
	CancelRateLimitMax            int      `json:"cancel_rate_limit_max"`
	CancelRateLimitWindow         int      `json:"cancel_rate_limit_window"`
	CancelRateLimitUnit           string   `json:"cancel_rate_limit_unit"`
	CancelRateLimitMode           string   `json:"cancel_rate_limit_window_mode"`
	AlipayForceQRCode             bool     `json:"alipay_force_qrcode"`
	AlipayMobilePrecreateDeepLink bool     `json:"alipay_mobile_precreate_deep_link"`
	VisibleMethodAlipaySource     string   `json:"payment_visible_method_alipay_source"`
	VisibleMethodWxpaySource      string   `json:"payment_visible_method_wxpay_source"`
	VisibleMethodAlipayEnabled    bool     `json:"payment_visible_method_alipay_enabled"`
	VisibleMethodWxpayEnabled     bool     `json:"payment_visible_method_wxpay_enabled"`
}

type StationPaymentProvider struct {
	ID              int64             `json:"id"`
	ProviderKey     string            `json:"provider_key"`
	Name            string            `json:"name"`
	Config          map[string]string `json:"config,omitempty"`
	ConfigState     map[string]bool   `json:"config_configured,omitempty"`
	SupportedTypes  []string          `json:"supported_types"`
	Enabled         bool              `json:"enabled"`
	PaymentMode     string            `json:"payment_mode"`
	SortOrder       int               `json:"sort_order"`
	Limits          string            `json:"limits"`
	RefundEnabled   bool              `json:"refund_enabled"`
	AllowUserRefund bool              `json:"allow_user_refund"`
	WebhookPath     string            `json:"webhook_path,omitempty"`
}

type StationPaymentPlan struct {
	ID            int64    `json:"id"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Price         float64  `json:"price"`
	OriginalPrice *float64 `json:"original_price,omitempty"`
	Currency      string   `json:"currency"`
	ValidityDays  int      `json:"validity_days"`
	ValidityUnit  string   `json:"validity_unit"`
	Features      []string `json:"features"`
	ProductName   string   `json:"product_name"`
	ForSale       bool     `json:"for_sale"`
	SortOrder     int      `json:"sort_order"`
}

type StationPaymentOrder struct {
	ID                 int64   `json:"id"`
	OutTradeNo         string  `json:"out_trade_no"`
	Amount             float64 `json:"amount"`
	PayAmount          float64 `json:"pay_amount"`
	FeeRate            float64 `json:"fee_rate"`
	Currency           string  `json:"currency"`
	PaymentType        string  `json:"payment_type"`
	ProviderInstanceID string  `json:"provider_instance_id,omitempty"`
	OrderType          string  `json:"order_type"`
	PlanID             *int64  `json:"plan_id,omitempty"`
	Status             string  `json:"status"`
	PayURL             string  `json:"pay_url,omitempty"`
	QRCode             string  `json:"qr_code,omitempty"`
	CreatedAt          string  `json:"created_at"`
	ExpiresAt          string  `json:"expires_at"`
	PaidAt             *string `json:"paid_at,omitempty"`
	MainUserID         string  `json:"-"`
}

var stationProviderKeys = map[string]bool{
	"easypay": true, "alipay": true, "wxpay": true, "stripe": true, "airwallex": true,
}

var stationSensitiveConfigKeys = map[string]bool{
	"pkey": true, "privateKey": true, "publicKey": true, "apiV3Key": true,
	"secretKey": true, "webhookSecret": true, "apiKey": true,
}

func defaultStationPaymentSettings() StationPaymentSettings {
	return StationPaymentSettings{
		MinAmount: 1, MaxAmount: 10000, OrderTimeoutMin: 30, MaxPendingOrders: 3,
		EnabledTypes: []string{}, BalanceRechargeMultiplier: 1, LoadBalanceStrategy: "round-robin",
		CancelRateLimitMax: 10, CancelRateLimitWindow: 1, CancelRateLimitUnit: "day",
		CancelRateLimitMode: "rolling", VisibleMethodAlipayEnabled: true, VisibleMethodWxpayEnabled: true,
	}
}

func (s *Store) StationPaymentSettings(agentID string) (StationPaymentSettings, error) {
	settings := defaultStationPaymentSettings()
	var enabled, balanceDisabled, cancelEnabled, alipayQR, alipayDeep, alipayVisible, wxpayVisible int
	var types string
	err := s.db.QueryRow(`SELECT enabled, min_amount, max_amount, daily_limit, order_timeout_minutes, max_pending_orders,
		enabled_payment_types, balance_disabled, balance_recharge_multiplier, subscription_usd_to_cny_rate, recharge_fee_rate,
		load_balance_strategy, product_name_prefix, product_name_suffix, help_image_url, help_text,
		cancel_rate_limit_enabled, cancel_rate_limit_max, cancel_rate_limit_window, cancel_rate_limit_unit, cancel_rate_limit_window_mode,
		alipay_force_qrcode, alipay_mobile_precreate_deep_link, visible_alipay_source, visible_wxpay_source, visible_alipay_enabled, visible_wxpay_enabled
		FROM station_payment_settings WHERE agent_id=?`, strings.TrimSpace(agentID)).Scan(
		&enabled, &settings.MinAmount, &settings.MaxAmount, &settings.DailyLimit, &settings.OrderTimeoutMin, &settings.MaxPendingOrders,
		&types, &balanceDisabled, &settings.BalanceRechargeMultiplier, &settings.SubscriptionUSDToCNYRate, &settings.RechargeFeeRate,
		&settings.LoadBalanceStrategy, &settings.ProductNamePrefix, &settings.ProductNameSuffix, &settings.HelpImageURL, &settings.HelpText,
		&cancelEnabled, &settings.CancelRateLimitMax, &settings.CancelRateLimitWindow, &settings.CancelRateLimitUnit, &settings.CancelRateLimitMode,
		&alipayQR, &alipayDeep, &settings.VisibleMethodAlipaySource, &settings.VisibleMethodWxpaySource, &alipayVisible, &wxpayVisible,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return settings, nil
	}
	if err != nil {
		return StationPaymentSettings{}, err
	}
	settings.Enabled = enabled == 1
	settings.BalanceDisabled = balanceDisabled == 1
	settings.CancelRateLimitEnabled = cancelEnabled == 1
	settings.AlipayForceQRCode = alipayQR == 1
	settings.AlipayMobilePrecreateDeepLink = alipayDeep == 1
	settings.VisibleMethodAlipayEnabled = alipayVisible == 1
	settings.VisibleMethodWxpayEnabled = wxpayVisible == 1
	settings.EnabledTypes = splitCSV(types)
	return settings, nil
}

func (s *Store) SaveStationPaymentSettings(agentID string, settings StationPaymentSettings) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := s.clock().UTC().Unix()
	_, err = tx.Exec(`INSERT INTO station_payment_settings (
		agent_id, enabled, min_amount, max_amount, daily_limit, order_timeout_minutes, max_pending_orders,
		enabled_payment_types, balance_disabled, balance_recharge_multiplier, subscription_usd_to_cny_rate, recharge_fee_rate,
		load_balance_strategy, product_name_prefix, product_name_suffix, help_image_url, help_text,
		cancel_rate_limit_enabled, cancel_rate_limit_max, cancel_rate_limit_window, cancel_rate_limit_unit, cancel_rate_limit_window_mode,
		alipay_force_qrcode, alipay_mobile_precreate_deep_link, visible_alipay_source, visible_wxpay_source, visible_alipay_enabled, visible_wxpay_enabled, updated_at
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
	ON CONFLICT(agent_id) DO UPDATE SET
		enabled=excluded.enabled, min_amount=excluded.min_amount, max_amount=excluded.max_amount, daily_limit=excluded.daily_limit,
		order_timeout_minutes=excluded.order_timeout_minutes, max_pending_orders=excluded.max_pending_orders,
		enabled_payment_types=excluded.enabled_payment_types, balance_disabled=excluded.balance_disabled,
		balance_recharge_multiplier=excluded.balance_recharge_multiplier, subscription_usd_to_cny_rate=excluded.subscription_usd_to_cny_rate,
		recharge_fee_rate=excluded.recharge_fee_rate, load_balance_strategy=excluded.load_balance_strategy,
		product_name_prefix=excluded.product_name_prefix, product_name_suffix=excluded.product_name_suffix,
		help_image_url=excluded.help_image_url, help_text=excluded.help_text,
		cancel_rate_limit_enabled=excluded.cancel_rate_limit_enabled, cancel_rate_limit_max=excluded.cancel_rate_limit_max,
		cancel_rate_limit_window=excluded.cancel_rate_limit_window, cancel_rate_limit_unit=excluded.cancel_rate_limit_unit,
		cancel_rate_limit_window_mode=excluded.cancel_rate_limit_window_mode, alipay_force_qrcode=excluded.alipay_force_qrcode,
		alipay_mobile_precreate_deep_link=excluded.alipay_mobile_precreate_deep_link, visible_alipay_source=excluded.visible_alipay_source,
		visible_wxpay_source=excluded.visible_wxpay_source, visible_alipay_enabled=excluded.visible_alipay_enabled,
		visible_wxpay_enabled=excluded.visible_wxpay_enabled, updated_at=excluded.updated_at`,
		strings.TrimSpace(agentID), boolInt(settings.Enabled), settings.MinAmount, settings.MaxAmount, settings.DailyLimit, settings.OrderTimeoutMin, settings.MaxPendingOrders,
		strings.Join(settings.EnabledTypes, ","), boolInt(settings.BalanceDisabled), settings.BalanceRechargeMultiplier, settings.SubscriptionUSDToCNYRate, settings.RechargeFeeRate,
		settings.LoadBalanceStrategy, settings.ProductNamePrefix, settings.ProductNameSuffix, settings.HelpImageURL, settings.HelpText,
		boolInt(settings.CancelRateLimitEnabled), settings.CancelRateLimitMax, settings.CancelRateLimitWindow, settings.CancelRateLimitUnit, settings.CancelRateLimitMode,
		boolInt(settings.AlipayForceQRCode), boolInt(settings.AlipayMobilePrecreateDeepLink), settings.VisibleMethodAlipaySource, settings.VisibleMethodWxpaySource,
		boolInt(settings.VisibleMethodAlipayEnabled), boolInt(settings.VisibleMethodWxpayEnabled), now,
	)
	if err != nil {
		return err
	}
	mode := "user_upstream"
	if settings.Enabled {
		mode = "owner_upstream"
	}
	result, err := tx.Exec(`UPDATE agent_config SET billing_mode=?, updated_at=? WHERE agent_id=? AND (?=0 OR TRIM(billing_main_user_id)<>'')`, mode, now, strings.TrimSpace(agentID), boolInt(settings.Enabled))
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return fmt.Errorf("station or station owner is not configured")
	}
	args := []any{now, strings.TrimSpace(agentID)}
	query := `UPDATE station_payment_providers SET enabled=0, updated_at=? WHERE agent_id=?`
	if len(settings.EnabledTypes) > 0 {
		query += ` AND provider_key NOT IN (` + strings.TrimRight(strings.Repeat("?,", len(settings.EnabledTypes)), ",") + `)`
		for _, key := range settings.EnabledTypes {
			args = append(args, key)
		}
	}
	if _, err := tx.Exec(query, args...); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ListStationProviders(agentID string, includeSecrets bool) ([]StationPaymentProvider, error) {
	rows, err := s.db.Query(`SELECT id, provider_key, name, config_cipher, supported_types, enabled, payment_mode, sort_order, limits, refund_enabled, allow_user_refund
		FROM station_payment_providers WHERE agent_id=? ORDER BY sort_order, id`, strings.TrimSpace(agentID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []StationPaymentProvider{}
	for rows.Next() {
		item, err := scanStationProvider(s, rows, includeSecrets)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) StationProvider(agentID string, id int64, includeSecrets bool) (StationPaymentProvider, error) {
	row := s.db.QueryRow(`SELECT id, provider_key, name, config_cipher, supported_types, enabled, payment_mode, sort_order, limits, refund_enabled, allow_user_refund
		FROM station_payment_providers WHERE agent_id=? AND id=?`, strings.TrimSpace(agentID), id)
	item, err := scanStationProvider(s, row, includeSecrets)
	if errors.Is(err, sql.ErrNoRows) {
		return StationPaymentProvider{}, errNotFound
	}
	return item, err
}

func scanStationProvider(store *Store, scanner interface{ Scan(...any) error }, includeSecrets bool) (StationPaymentProvider, error) {
	var item StationPaymentProvider
	var cipher, types string
	var enabled, refund, allow int
	if err := scanner.Scan(&item.ID, &item.ProviderKey, &item.Name, &cipher, &types, &enabled, &item.PaymentMode, &item.SortOrder, &item.Limits, &refund, &allow); err != nil {
		return StationPaymentProvider{}, err
	}
	item.Enabled = enabled == 1
	item.RefundEnabled = refund == 1
	item.AllowUserRefund = allow == 1
	item.SupportedTypes = splitCSV(types)
	raw, err := store.decrypt(cipher)
	if err != nil {
		return StationPaymentProvider{}, err
	}
	config := map[string]string{}
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &config); err != nil {
			return StationPaymentProvider{}, err
		}
	}
	item.ConfigState = map[string]bool{}
	for key, value := range config {
		if stationConfigSecret(key) {
			item.ConfigState[key] = strings.TrimSpace(value) != ""
			if !includeSecrets {
				delete(config, key)
			}
		}
	}
	if includeSecrets {
		item.Config = config
	} else {
		item.Config = config
	}
	return item, nil
}

func (s *Store) CreateStationProvider(agentID string, item StationPaymentProvider) (StationPaymentProvider, error) {
	cipher, err := s.encryptConfig(item.Config)
	if err != nil {
		return StationPaymentProvider{}, err
	}
	now := s.clock().UTC().Unix()
	result, err := s.db.Exec(`INSERT INTO station_payment_providers (agent_id, provider_key, name, config_cipher, supported_types, enabled, payment_mode, sort_order, limits, refund_enabled, allow_user_refund, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`, strings.TrimSpace(agentID), item.ProviderKey, item.Name, cipher, strings.Join(item.SupportedTypes, ","), boolInt(item.Enabled), item.PaymentMode, item.SortOrder, item.Limits, boolInt(item.RefundEnabled), boolInt(item.AllowUserRefund), now, now)
	if err != nil {
		return StationPaymentProvider{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return StationPaymentProvider{}, err
	}
	return s.StationProvider(agentID, id, false)
}

func (s *Store) UpdateStationProvider(agentID string, id int64, item StationPaymentProvider, replaceConfig bool) (StationPaymentProvider, error) {
	current, err := s.StationProvider(agentID, id, true)
	if err != nil {
		return StationPaymentProvider{}, err
	}
	config := map[string]string{}
	for key, value := range current.Config {
		config[key] = value
	}
	if replaceConfig {
		for key, value := range item.Config {
			if stationConfigSecret(key) && strings.TrimSpace(value) == "" {
				continue
			}
			config[key] = value
		}
	}
	item.Config = config
	if pending, err := s.stationProviderPending(agentID, id); err != nil {
		return StationPaymentProvider{}, err
	} else if pending && stationProtectedProviderChange(current, item) {
		return StationPaymentProvider{}, errStationProviderBusy
	}
	cipher, err := s.encryptConfig(config)
	if err != nil {
		return StationPaymentProvider{}, err
	}
	now := s.clock().UTC().Unix()
	result, err := s.db.Exec(`UPDATE station_payment_providers SET name=?, config_cipher=?, supported_types=?, enabled=?, payment_mode=?, sort_order=?, limits=?, refund_enabled=?, allow_user_refund=?, updated_at=? WHERE agent_id=? AND id=?`,
		item.Name, cipher, strings.Join(item.SupportedTypes, ","), boolInt(item.Enabled), item.PaymentMode, item.SortOrder, item.Limits, boolInt(item.RefundEnabled), boolInt(item.AllowUserRefund), now, strings.TrimSpace(agentID), id)
	if err != nil {
		return StationPaymentProvider{}, err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return StationPaymentProvider{}, errNotFound
	}
	return s.StationProvider(agentID, id, false)
}

func (s *Store) DeleteStationProvider(agentID string, id int64) error {
	if _, err := s.StationProvider(agentID, id, false); err != nil {
		return err
	}
	if pending, err := s.stationProviderPending(agentID, id); err != nil {
		return err
	} else if pending {
		return errStationProviderBusy
	}
	result, err := s.db.Exec(`DELETE FROM station_payment_providers WHERE agent_id=? AND id=?`, strings.TrimSpace(agentID), id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return errNotFound
	}
	return nil
}

func (s *Store) encryptConfig(config map[string]string) (string, error) {
	if config == nil {
		config = map[string]string{}
	}
	raw, err := json.Marshal(config)
	if err != nil {
		return "", err
	}
	return s.encrypt(string(raw))
}

func (s *Store) ListStationPlans(agentID string, forSaleOnly bool) ([]StationPaymentPlan, error) {
	query := `SELECT id, name, description, price, original_price, currency, validity_days, validity_unit, features, product_name, for_sale, sort_order FROM station_payment_plans WHERE agent_id=?`
	if forSaleOnly {
		query += ` AND for_sale=1`
	}
	query += ` ORDER BY sort_order, id`
	rows, err := s.db.Query(query, strings.TrimSpace(agentID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []StationPaymentPlan{}
	for rows.Next() {
		item, err := scanStationPlan(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) StationPlan(agentID string, id int64) (StationPaymentPlan, error) {
	row := s.db.QueryRow(`SELECT id, name, description, price, original_price, currency, validity_days, validity_unit, features, product_name, for_sale, sort_order FROM station_payment_plans WHERE agent_id=? AND id=?`, strings.TrimSpace(agentID), id)
	item, err := scanStationPlan(row)
	if errors.Is(err, sql.ErrNoRows) {
		return StationPaymentPlan{}, errNotFound
	}
	return item, err
}

func scanStationPlan(scanner interface{ Scan(...any) error }) (StationPaymentPlan, error) {
	var item StationPaymentPlan
	var original sql.NullFloat64
	var features string
	var forSale int
	if err := scanner.Scan(&item.ID, &item.Name, &item.Description, &item.Price, &original, &item.Currency, &item.ValidityDays, &item.ValidityUnit, &features, &item.ProductName, &forSale, &item.SortOrder); err != nil {
		return StationPaymentPlan{}, err
	}
	item.ForSale = forSale == 1
	if original.Valid {
		value := original.Float64
		item.OriginalPrice = &value
	}
	item.Features = splitCSV(features)
	if item.Features == nil {
		item.Features = []string{}
	}
	return item, nil
}

func (s *Store) SaveStationPlan(agentID string, item StationPaymentPlan) (StationPaymentPlan, error) {
	now := s.clock().UTC().Unix()
	var original any
	if item.OriginalPrice != nil {
		original = *item.OriginalPrice
	}
	if item.ID == 0 {
		result, err := s.db.Exec(`INSERT INTO station_payment_plans (agent_id, name, description, price, original_price, currency, validity_days, validity_unit, features, product_name, for_sale, sort_order, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, strings.TrimSpace(agentID), item.Name, item.Description, item.Price, original, item.Currency, item.ValidityDays, item.ValidityUnit, strings.Join(item.Features, ","), item.ProductName, boolInt(item.ForSale), item.SortOrder, now, now)
		if err != nil {
			return StationPaymentPlan{}, err
		}
		id, err := result.LastInsertId()
		if err != nil {
			return StationPaymentPlan{}, err
		}
		return s.StationPlan(agentID, id)
	}
	result, err := s.db.Exec(`UPDATE station_payment_plans SET name=?, description=?, price=?, original_price=?, currency=?, validity_days=?, validity_unit=?, features=?, product_name=?, for_sale=?, sort_order=?, updated_at=? WHERE agent_id=? AND id=?`,
		item.Name, item.Description, item.Price, original, item.Currency, item.ValidityDays, item.ValidityUnit, strings.Join(item.Features, ","), item.ProductName, boolInt(item.ForSale), item.SortOrder, now, strings.TrimSpace(agentID), item.ID)
	if err != nil {
		return StationPaymentPlan{}, err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return StationPaymentPlan{}, errNotFound
	}
	return s.StationPlan(agentID, item.ID)
}

func (s *Store) DeleteStationPlan(agentID string, id int64) error {
	result, err := s.db.Exec(`DELETE FROM station_payment_plans WHERE agent_id=? AND id=?`, strings.TrimSpace(agentID), id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return errNotFound
	}
	return nil
}

func (s *Store) CreateStationOrder(order StationPaymentOrder, agentID, mainUserID string, expiresAt time.Time) (StationPaymentOrder, error) {
	now := s.clock().UTC()
	planID := int64(0)
	if order.PlanID != nil {
		planID = *order.PlanID
	}
	providerID, _ := strconv.ParseInt(order.ProviderInstanceID, 10, 64)
	result, err := s.db.Exec(`INSERT INTO station_payment_orders (agent_id, main_user_id, out_trade_no, amount, pay_amount, fee_rate, currency, payment_type, provider_id, order_type, plan_id, status, pay_url, qr_code, created_at, expires_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, strings.TrimSpace(agentID), mainUserID, order.OutTradeNo, order.Amount, order.PayAmount, order.FeeRate, order.Currency, order.PaymentType, providerID, order.OrderType, planID, order.Status, order.PayURL, order.QRCode, now.Unix(), expiresAt.UTC().Unix(), now.Unix())
	if err != nil {
		return StationPaymentOrder{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return StationPaymentOrder{}, err
	}
	return s.StationOrder(agentID, id)
}

func (s *Store) StationOrder(agentID string, id int64) (StationPaymentOrder, error) {
	row := s.db.QueryRow(`SELECT id, agent_id, main_user_id, out_trade_no, amount, pay_amount, fee_rate, currency, payment_type, provider_id, order_type, plan_id, status, pay_url, qr_code, created_at, expires_at, paid_at FROM station_payment_orders WHERE agent_id=? AND id=?`, strings.TrimSpace(agentID), id)
	return scanStationOrder(row)
}

func (s *Store) StationOrderByNo(agentID, outTradeNo string) (StationPaymentOrder, error) {
	row := s.db.QueryRow(`SELECT id, agent_id, main_user_id, out_trade_no, amount, pay_amount, fee_rate, currency, payment_type, provider_id, order_type, plan_id, status, pay_url, qr_code, created_at, expires_at, paid_at FROM station_payment_orders WHERE agent_id=? AND out_trade_no=?`, strings.TrimSpace(agentID), strings.TrimSpace(outTradeNo))
	return scanStationOrder(row)
}

func scanStationOrder(scanner interface{ Scan(...any) error }) (StationPaymentOrder, error) {
	var item StationPaymentOrder
	var agentID string
	var providerID int64
	var planID sql.NullInt64
	var created, expires, paid int64
	err := scanner.Scan(&item.ID, &agentID, &item.MainUserID, &item.OutTradeNo, &item.Amount, &item.PayAmount, &item.FeeRate, &item.Currency, &item.PaymentType, &providerID, &item.OrderType, &planID, &item.Status, &item.PayURL, &item.QRCode, &created, &expires, &paid)
	if errors.Is(err, sql.ErrNoRows) {
		return StationPaymentOrder{}, errNotFound
	}
	if err != nil {
		return StationPaymentOrder{}, err
	}
	if providerID > 0 {
		item.ProviderInstanceID = strconv.FormatInt(providerID, 10)
	}
	if planID.Valid {
		value := planID.Int64
		item.PlanID = &value
	}
	item.CreatedAt = time.Unix(created, 0).UTC().Format(time.RFC3339)
	item.ExpiresAt = time.Unix(expires, 0).UTC().Format(time.RFC3339)
	if paid > 0 {
		value := time.Unix(paid, 0).UTC().Format(time.RFC3339)
		item.PaidAt = &value
	}
	return item, nil
}

func (s *Store) ListStationOrders(agentID, mainUserID, status string, limit, offset int) ([]StationPaymentOrder, int64, error) {
	where := `WHERE agent_id=?`
	args := []any{strings.TrimSpace(agentID)}
	if mainUserID != "" {
		where += ` AND main_user_id=?`
		args = append(args, mainUserID)
	}
	if status != "" {
		where += ` AND status=?`
		args = append(args, status)
	}
	var total int64
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM station_payment_orders `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, limit, offset)
	rows, err := s.db.Query(`SELECT id, agent_id, main_user_id, out_trade_no, amount, pay_amount, fee_rate, currency, payment_type, provider_id, order_type, plan_id, status, pay_url, qr_code, created_at, expires_at, paid_at FROM station_payment_orders `+where+` ORDER BY id DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []StationPaymentOrder{}
	for rows.Next() {
		item, err := scanStationOrder(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (s *Store) CountPendingStationOrders(agentID, mainUserID string) (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM station_payment_orders WHERE agent_id=? AND main_user_id=? AND status='PENDING' AND expires_at>?`, strings.TrimSpace(agentID), mainUserID, s.clock().UTC().Unix()).Scan(&count)
	return count, err
}

func (s *Store) SumStationPayAmountSince(agentID, paymentType string, since time.Time) (float64, error) {
	var sum sql.NullFloat64
	err := s.db.QueryRow(`SELECT SUM(pay_amount) FROM station_payment_orders WHERE agent_id=? AND payment_type=? AND status IN ('PENDING','COMPLETED') AND created_at>=?`, strings.TrimSpace(agentID), paymentType, since.UTC().Unix()).Scan(&sum)
	if !sum.Valid {
		return 0, err
	}
	return sum.Float64, err
}

func (s *Store) UpdateStationOrderStatus(agentID string, id int64, from, to, reason string) error {
	now := s.clock().UTC().Unix()
	paid := int64(0)
	if to == "COMPLETED" {
		paid = now
	}
	result, err := s.db.Exec(`UPDATE station_payment_orders SET status=?, refund_reason=CASE WHEN ?<>'' THEN ? ELSE refund_reason END, paid_at=CASE WHEN ?>0 THEN ? ELSE paid_at END, updated_at=? WHERE agent_id=? AND id=? AND status=?`,
		to, reason, reason, paid, paid, now, strings.TrimSpace(agentID), id, from)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return errNotFound
	}
	return nil
}

func (s *Store) CompleteStationOrder(agentID, outTradeNo, providerTradeNo string) (StationPaymentOrder, bool, error) {
	order, err := s.StationOrderByNo(agentID, outTradeNo)
	if err != nil {
		return StationPaymentOrder{}, false, err
	}
	if order.Status == "COMPLETED" {
		return order, false, nil
	}
	if order.Status != "PENDING" {
		return order, false, errRechargeStateConflict
	}
	now := s.clock().UTC().Unix()
	result, err := s.db.Exec(`UPDATE station_payment_orders SET status='COMPLETED', provider_trade_no=?, paid_at=?, updated_at=? WHERE agent_id=? AND id=? AND status='PENDING'`, providerTradeNo, now, now, strings.TrimSpace(agentID), order.ID)
	if err != nil {
		return StationPaymentOrder{}, false, err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		current, lookupErr := s.StationOrderByNo(agentID, outTradeNo)
		return current, false, lookupErr
	}
	updated, err := s.StationOrder(agentID, order.ID)
	return updated, true, err
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func (s *Server) handleStationBillingAdmin(w http.ResponseWriter, r *http.Request, requestID string, tenant TenantContext) {
	w.Header().Set("Cache-Control", "no-store")
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/agent/admin/billing")
	if r.Method != http.MethodGet && (path == "/config" || strings.HasPrefix(path, "/providers")) {
		leaseCtx, release, err := s.acquireOperationLease(r.Context(), "station:payment:"+tenant.AgentID)
		if err != nil {
			s.writeError(w, 503, requestID, "PAYMENT_BUSY", "payment configuration is busy")
			return
		}
		defer release()
		r = r.WithContext(leaseCtx)
	}
	switch {
	case path == "/config" && r.Method == http.MethodGet:
		settings, err := s.store.StationPaymentSettings(tenant.AgentID)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to load station billing settings")
			return
		}
		s.writeData(w, http.StatusOK, requestID, settings)
	case path == "/config" && (r.Method == http.MethodPut || r.Method == http.MethodPatch):
		s.saveStationBillingSettings(w, r, requestID, tenant)
	case path == "/providers" && r.Method == http.MethodGet:
		items, err := s.store.ListStationProviders(tenant.AgentID, false)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to load payment providers")
			return
		}
		for i := range items {
			items[i].WebhookPath = stationWebhookPath(tenant.AgentID, items[i].ProviderKey)
		}
		s.writeData(w, http.StatusOK, requestID, map[string]any{"items": items})
	case path == "/providers" && r.Method == http.MethodPost:
		s.createStationProvider(w, r, requestID, tenant)
	case path == "/plans" && r.Method == http.MethodGet:
		items, err := s.store.ListStationPlans(tenant.AgentID, false)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to load plans")
			return
		}
		s.writeData(w, http.StatusOK, requestID, map[string]any{"items": items})
	case path == "/plans" && r.Method == http.MethodPost:
		s.saveStationPlan(w, r, requestID, tenant, 0)
	case path == "/orders" && r.Method == http.MethodGet:
		s.listStationOrders(w, r, requestID, tenant.AgentID, "")
	case path == "/dashboard" && r.Method == http.MethodGet:
		s.stationBillingDashboard(w, requestID, tenant.AgentID)
	case strings.HasPrefix(path, "/providers/"):
		s.mutateStationProvider(w, r, requestID, tenant, path)
	case strings.HasPrefix(path, "/plans/"):
		s.mutateStationPlan(w, r, requestID, tenant, path)
	case strings.HasPrefix(path, "/orders/"):
		s.mutateStationAdminOrder(w, r, requestID, tenant, path)
	default:
		s.writeError(w, http.StatusNotFound, requestID, "NOT_FOUND", "station billing endpoint not found")
	}
}

func (s *Server) saveStationBillingSettings(w http.ResponseWriter, r *http.Request, requestID string, tenant TenantContext) {
	if !sameOrigin(r) {
		s.writeError(w, http.StatusForbidden, requestID, "CSRF_ORIGIN_REJECTED", "cross-origin state-changing requests are not allowed")
		return
	}
	settings, err := s.store.StationPaymentSettings(tenant.AgentID)
	if err != nil {
		s.writeError(w, 500, requestID, "STORE_ERROR", "failed to load payment settings")
		return
	}
	if err := decodeJSON(r, &settings, maxJSONBody); err != nil {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REQUEST", "invalid billing settings")
		return
	}
	if err := validateStationSettings(&settings); err != nil {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_BILLING_SETTINGS", err.Error())
		return
	}
	agent, err := s.store.Agent(tenant.AgentID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to load tenant configuration")
		return
	}
	ownerID := strings.TrimSpace(agent.OwnerMainUserID)
	if settings.Enabled && ownerID == "" {
		s.writeError(w, http.StatusServiceUnavailable, requestID, "MAIN_OWNER_MISSING", "station owner is not configured")
		return
	}
	if err := s.store.SaveStationPaymentSettings(tenant.AgentID, settings); err != nil {
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to save billing settings")
		return
	}
	mode := "user_upstream"
	if settings.Enabled {
		mode = "owner_upstream"
	}
	s.recordTenantAudit(tenant.AgentID, "agent_admin", tenant.MainUserID, "billing.settings.update", "station_billing", tenant.AgentID, requestID, "success", mode)
	s.writeData(w, http.StatusOK, requestID, settings)
}

func validateStationSettings(settings *StationPaymentSettings) error {
	if settings.MinAmount < 0 || settings.MaxAmount < 0 || settings.DailyLimit < 0 || settings.MaxAmount > 0 && settings.MaxAmount < settings.MinAmount {
		return fmt.Errorf("amount limits are invalid")
	}
	if settings.OrderTimeoutMin < 1 || settings.OrderTimeoutMin > 7*24*60 {
		return fmt.Errorf("order timeout must be between 1 and 10080 minutes")
	}
	if settings.MaxPendingOrders < 1 || settings.MaxPendingOrders > 100 {
		return fmt.Errorf("max pending orders must be between 1 and 100")
	}
	if settings.BalanceRechargeMultiplier <= 0 || settings.BalanceRechargeMultiplier > 1000 {
		return fmt.Errorf("balance recharge multiplier must be greater than 0 and at most 1000")
	}
	if settings.RechargeFeeRate < 0 || settings.RechargeFeeRate > 100 {
		return fmt.Errorf("fee rate must be between 0 and 100")
	}
	if settings.SubscriptionUSDToCNYRate < 0 {
		return fmt.Errorf("subscription exchange rate is invalid")
	}
	switch settings.LoadBalanceStrategy {
	case "", "round-robin", "least-amount":
		if settings.LoadBalanceStrategy == "" {
			settings.LoadBalanceStrategy = "round-robin"
		}
	default:
		return fmt.Errorf("load balance strategy is invalid")
	}
	if utf8.RuneCountInString(settings.HelpText) > 2000 || utf8.RuneCountInString(settings.ProductNamePrefix) > 40 || utf8.RuneCountInString(settings.ProductNameSuffix) > 40 {
		return fmt.Errorf("display text is too long")
	}
	if settings.CancelRateLimitEnabled {
		if settings.CancelRateLimitMax < 1 || settings.CancelRateLimitWindow < 1 || settings.CancelRateLimitWindow > 36500 {
			return fmt.Errorf("cancellation count and window must be positive; window cannot exceed 36500")
		}
		if settings.CancelRateLimitUnit != "minute" && settings.CancelRateLimitUnit != "hour" && settings.CancelRateLimitUnit != "day" {
			return fmt.Errorf("cancellation window unit is invalid")
		}
		if settings.CancelRateLimitMode != "rolling" && settings.CancelRateLimitMode != "fixed" {
			return fmt.Errorf("cancellation window mode is invalid")
		}
	}
	settings.EnabledTypes = cleanPaymentTypes(settings.EnabledTypes)
	if settings.VisibleMethodAlipaySource != "" && settings.VisibleMethodAlipaySource != "official_alipay" && settings.VisibleMethodAlipaySource != "easypay_alipay" {
		return fmt.Errorf("invalid Alipay source")
	}
	if settings.VisibleMethodWxpaySource != "" && settings.VisibleMethodWxpaySource != "official_wxpay" && settings.VisibleMethodWxpaySource != "easypay_wxpay" {
		return fmt.Errorf("invalid WeChat source")
	}
	for _, key := range settings.EnabledTypes {
		if !stationProviderKeys[key] {
			return fmt.Errorf("unknown payment provider family")
		}
	}
	return nil
}

func cleanPaymentTypes(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] || len(value) > 32 {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func (s *Server) createStationProvider(w http.ResponseWriter, r *http.Request, requestID string, tenant TenantContext) {
	if !sameOrigin(r) {
		s.writeError(w, http.StatusForbidden, requestID, "CSRF_ORIGIN_REJECTED", "cross-origin state-changing requests are not allowed")
		return
	}
	item, err := decodeStationProvider(r)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_PROVIDER", err.Error())
		return
	}
	if err := prepareStationProvider(&item, tenant.AgentID, requestOrigin(r)); err != nil {
		s.writeError(w, 400, requestID, "INVALID_PROVIDER", err.Error())
		return
	}
	created, err := s.store.CreateStationProvider(tenant.AgentID, item)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to create payment provider")
		return
	}
	created.WebhookPath = stationWebhookPath(tenant.AgentID, created.ProviderKey)
	s.writeData(w, http.StatusCreated, requestID, created)
}

func (s *Server) mutateStationProvider(w http.ResponseWriter, r *http.Request, requestID string, tenant TenantContext, path string) {
	id, action, ok := splitStationID(strings.TrimPrefix(path, "/providers/"))
	if !ok || action != "" {
		s.writeError(w, http.StatusNotFound, requestID, "NOT_FOUND", "payment provider not found")
		return
	}
	switch r.Method {
	case http.MethodGet:
		item, err := s.store.StationProvider(tenant.AgentID, id, false)
		if errors.Is(err, errNotFound) {
			s.writeError(w, http.StatusNotFound, requestID, "NOT_FOUND", "payment provider not found")
			return
		}
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to load payment provider")
			return
		}
		item.WebhookPath = stationWebhookPath(tenant.AgentID, item.ProviderKey)
		s.writeData(w, http.StatusOK, requestID, item)
	case http.MethodPut, http.MethodPatch:
		if !sameOrigin(r) {
			s.writeError(w, http.StatusForbidden, requestID, "CSRF_ORIGIN_REJECTED", "cross-origin state-changing requests are not allowed")
			return
		}
		current, err := s.store.StationProvider(tenant.AgentID, id, true)
		if errors.Is(err, errNotFound) {
			s.writeError(w, 404, requestID, "NOT_FOUND", "payment provider not found")
			return
		}
		if err != nil {
			s.writeError(w, 500, requestID, "STORE_ERROR", "failed to load provider")
			return
		}
		item, err := decodeStationProviderPatch(r, current)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_PROVIDER", err.Error())
			return
		}
		if err := prepareStationProvider(&item, tenant.AgentID, requestOrigin(r)); err != nil {
			s.writeError(w, 400, requestID, "INVALID_PROVIDER", err.Error())
			return
		}
		updated, err := s.store.UpdateStationProvider(tenant.AgentID, id, item, true)
		if errors.Is(err, errNotFound) {
			s.writeError(w, http.StatusNotFound, requestID, "NOT_FOUND", "payment provider not found")
			return
		}
		if errors.Is(err, errStationProviderBusy) {
			s.writeError(w, 409, requestID, "PENDING_ORDERS", err.Error())
			return
		}
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to update payment provider")
			return
		}
		updated.WebhookPath = stationWebhookPath(tenant.AgentID, updated.ProviderKey)
		s.writeData(w, http.StatusOK, requestID, updated)
	case http.MethodDelete:
		if !sameOrigin(r) {
			s.writeError(w, http.StatusForbidden, requestID, "CSRF_ORIGIN_REJECTED", "cross-origin state-changing requests are not allowed")
			return
		}
		if err := s.store.DeleteStationProvider(tenant.AgentID, id); errors.Is(err, errNotFound) {
			s.writeError(w, http.StatusNotFound, requestID, "NOT_FOUND", "payment provider not found")
			return
		} else if errors.Is(err, errStationProviderBusy) {
			s.writeError(w, 409, requestID, "PENDING_ORDERS", err.Error())
			return
		} else if err != nil {
			s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to delete payment provider")
			return
		}
		s.writeData(w, http.StatusOK, requestID, map[string]string{"message": "provider deleted"})
	default:
		s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}

func decodeStationProvider(r *http.Request) (StationPaymentProvider, error) {
	var item StationPaymentProvider
	if err := decodeJSON(r, &item, maxJSONBody); err != nil {
		return StationPaymentProvider{}, fmt.Errorf("invalid provider")
	}
	return item, validateStationProviderFields(&item)
}

func validateStationProviderFields(item *StationPaymentProvider) error {
	item.ProviderKey = strings.TrimSpace(item.ProviderKey)
	item.Name = strings.TrimSpace(item.Name)
	item.PaymentMode = strings.TrimSpace(item.PaymentMode)
	if !stationProviderKeys[item.ProviderKey] {
		return fmt.Errorf("provider_key must be easypay, alipay, wxpay, stripe or airwallex")
	}
	if item.Name == "" || len(item.Name) > 100 {
		return fmt.Errorf("name must contain 1 to 100 characters")
	}
	item.SupportedTypes = cleanPaymentTypes(item.SupportedTypes)
	if len(item.Limits) > 8000 {
		return fmt.Errorf("limits are too large")
	}
	if strings.TrimSpace(item.Limits) != "" {
		var limits map[string]map[string]float64
		if err := json.Unmarshal([]byte(item.Limits), &limits); err != nil || limits == nil {
			return fmt.Errorf("limits must be a JSON object of payment method limits")
		}
		for _, fields := range limits {
			for key, value := range fields {
				if value < 0 || (key != "singleMin" && key != "singleMax" && key != "dailyLimit" && key != "single_min" && key != "single_max" && key != "daily_limit") {
					return fmt.Errorf("invalid channel limit")
				}
			}
			min, max := fields["singleMin"], fields["singleMax"]
			if min == 0 {
				min = fields["single_min"]
			}
			if max == 0 {
				max = fields["single_max"]
			}
			if max > 0 && min > max {
				return fmt.Errorf("channel maximum must not be less than minimum")
			}
		}
	}
	if !item.RefundEnabled {
		item.AllowUserRefund = false
	}
	if item.Config == nil {
		item.Config = map[string]string{}
	}
	for key, value := range item.Config {
		if len(key) > 64 || len(value) > 8000 {
			return fmt.Errorf("provider config field is too large")
		}
	}
	return validateStationProviderMethods(item)
}

func (s *Server) saveStationPlan(w http.ResponseWriter, r *http.Request, requestID string, tenant TenantContext, id int64) {
	if !sameOrigin(r) {
		s.writeError(w, http.StatusForbidden, requestID, "CSRF_ORIGIN_REJECTED", "cross-origin state-changing requests are not allowed")
		return
	}
	var item StationPaymentPlan
	if err := decodeJSON(r, &item, maxJSONBody); err != nil {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_PLAN", "invalid plan")
		return
	}
	item.ID = id
	item.Name = strings.TrimSpace(item.Name)
	item.Currency = strings.ToUpper(strings.TrimSpace(item.Currency))
	if item.Currency == "" {
		item.Currency = "CNY"
	}
	if item.Name == "" || item.Price < 0 || len(item.Currency) != 3 || item.ValidityDays < 1 {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_PLAN", "plan name, price, currency and validity are required")
		return
	}
	if item.ValidityUnit == "" {
		item.ValidityUnit = "day"
	}
	saved, err := s.store.SaveStationPlan(tenant.AgentID, item)
	if errors.Is(err, errNotFound) {
		s.writeError(w, http.StatusNotFound, requestID, "NOT_FOUND", "plan not found")
		return
	}
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to save plan")
		return
	}
	status := http.StatusOK
	if id == 0 {
		status = http.StatusCreated
	}
	s.writeData(w, status, requestID, saved)
}

func (s *Server) mutateStationPlan(w http.ResponseWriter, r *http.Request, requestID string, tenant TenantContext, path string) {
	id, action, ok := splitStationID(strings.TrimPrefix(path, "/plans/"))
	if !ok || action != "" {
		s.writeError(w, http.StatusNotFound, requestID, "NOT_FOUND", "plan not found")
		return
	}
	switch r.Method {
	case http.MethodPut, http.MethodPatch:
		s.saveStationPlan(w, r, requestID, tenant, id)
	case http.MethodDelete:
		if !sameOrigin(r) {
			s.writeError(w, http.StatusForbidden, requestID, "CSRF_ORIGIN_REJECTED", "cross-origin state-changing requests are not allowed")
			return
		}
		if err := s.store.DeleteStationPlan(tenant.AgentID, id); errors.Is(err, errNotFound) {
			s.writeError(w, http.StatusNotFound, requestID, "NOT_FOUND", "plan not found")
			return
		} else if err != nil {
			s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to delete plan")
			return
		}
		s.writeData(w, http.StatusOK, requestID, map[string]string{"message": "plan deleted"})
	default:
		s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}

func (s *Server) listStationOrders(w http.ResponseWriter, r *http.Request, requestID, agentID, mainUserID string) {
	page := boundedPositiveInt(r.URL.Query().Get("page"), 1, 1, 1000000)
	pageSize := boundedPositiveInt(r.URL.Query().Get("page_size"), 20, 1, 100)
	status := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("status")))
	items, total, err := s.store.ListStationOrders(agentID, mainUserID, status, pageSize, (page-1)*pageSize)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to load station orders")
		return
	}
	if mainUserID != "" {
		for i := range items {
			items[i].PayURL = ""
		}
	}
	pages := int((total + int64(pageSize) - 1) / int64(pageSize))
	s.writeData(w, http.StatusOK, requestID, map[string]any{"items": items, "total": total, "page": page, "page_size": pageSize, "pages": pages})
}

func (s *Server) stationBillingDashboard(w http.ResponseWriter, requestID, agentID string) {
	items, total, err := s.store.ListStationOrders(agentID, "", "", 200, 0)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to load billing dashboard")
		return
	}
	stats := map[string]any{"orders_total": total, "sampled": len(items), "partial": total > int64(len(items))}
	var completed, pending, refunded int
	var amount float64
	currency := ""
	mixed := false
	for _, item := range items {
		switch item.Status {
		case "COMPLETED":
			completed++
			amount += item.PayAmount
			if currency == "" {
				currency = item.Currency
			} else if currency != item.Currency {
				mixed = true
			}
		case "PENDING":
			pending++
		case "REFUNDED", "REFUND_REQUESTED":
			refunded++
		}
	}
	stats["completed_count"] = completed
	stats["pending_count"] = pending
	stats["refunded_count"] = refunded
	stats["amount_available"] = !mixed && total <= int64(len(items))
	if stats["amount_available"] == true {
		stats["completed_pay_amount"] = amount
		stats["currency"] = currency
	}
	s.writeData(w, http.StatusOK, requestID, stats)
}

func (s *Server) mutateStationAdminOrder(w http.ResponseWriter, r *http.Request, requestID string, tenant TenantContext, path string) {
	id, action, ok := splitStationID(strings.TrimPrefix(path, "/orders/"))
	if !ok || r.Method != http.MethodPost {
		s.writeError(w, http.StatusNotFound, requestID, "NOT_FOUND", "order action not found")
		return
	}
	if !sameOrigin(r) {
		s.writeError(w, http.StatusForbidden, requestID, "CSRF_ORIGIN_REJECTED", "cross-origin state-changing requests are not allowed")
		return
	}
	order, err := s.store.StationOrder(tenant.AgentID, id)
	if errors.Is(err, errNotFound) {
		s.writeError(w, http.StatusNotFound, requestID, "NOT_FOUND", "order not found")
		return
	}
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to load order")
		return
	}
	switch action {
	case "cancel":
		if err := s.store.UpdateStationOrderStatus(tenant.AgentID, id, "PENDING", "CANCELLED", ""); err != nil {
			s.writeError(w, http.StatusConflict, requestID, "ORDER_STATE_CONFLICT", "only a pending order can be cancelled")
			return
		}
	case "refund":
		s.refundStationOrder(w, r, requestID, tenant, order)
		return
	default:
		s.writeError(w, http.StatusNotFound, requestID, "NOT_FOUND", "order action not found")
		return
	}
	s.writeData(w, http.StatusOK, requestID, map[string]string{"message": "order updated"})
}

func (s *Server) handleStationBillingUser(w http.ResponseWriter, r *http.Request, requestID string) {
	w.Header().Set("Cache-Control", "no-store")
	tenant, session, _, ok := s.requireTenantSession(w, r, requestID)
	if !ok {
		return
	}
	switch {
	case r.URL.Path == "/api/v1/agent/billing/orders/refund-eligible-providers" && r.Method == http.MethodGet:
		providers, err := s.store.ListStationProviders(tenant.AgentID, false)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to load refund eligibility")
			return
		}
		ids := []string{}
		for _, provider := range providers {
			if provider.RefundEnabled && provider.AllowUserRefund {
				ids = append(ids, strconv.FormatInt(provider.ID, 10))
			}
		}
		s.writeData(w, http.StatusOK, requestID, map[string]any{"provider_instance_ids": ids})
	case r.URL.Path == "/api/v1/agent/billing/orders" && r.Method == http.MethodGet:
		s.listStationOrders(w, r, requestID, tenant.AgentID, session.MainUserID)
	case strings.HasPrefix(r.URL.Path, "/api/v1/agent/billing/orders/") && r.Method == http.MethodPost:
		s.userStationOrderAction(w, r, requestID, tenant, session.MainUserID)
	default:
		s.writeError(w, http.StatusNotFound, requestID, "NOT_FOUND", "station billing endpoint not found")
	}
}

func (s *Server) userStationOrderAction(w http.ResponseWriter, r *http.Request, requestID string, tenant TenantContext, mainUserID string) {
	if !sameOrigin(r) {
		s.writeError(w, http.StatusForbidden, requestID, "CSRF_ORIGIN_REJECTED", "cross-origin state-changing requests are not allowed")
		return
	}
	relative := strings.TrimPrefix(r.URL.Path, "/api/v1/agent/billing/orders/")
	id, action, ok := splitStationID(relative)
	if !ok {
		s.writeError(w, http.StatusNotFound, requestID, "NOT_FOUND", "order not found")
		return
	}
	order, err := s.store.StationOrder(tenant.AgentID, id)
	if err != nil || order.MainUserID != mainUserID {
		s.writeError(w, http.StatusNotFound, requestID, "NOT_FOUND", "order not found")
		return
	}
	switch action {
	case "cancel":
		if err := s.store.cancelStationUserOrder(tenant.AgentID, mainUserID, id, requestID); err != nil {
			s.writeError(w, http.StatusConflict, requestID, "ORDER_STATE_CONFLICT", "only a pending order can be cancelled")
			return
		}
		s.writeData(w, http.StatusOK, requestID, map[string]string{"message": "order cancelled"})
	case "refund-request":
		var body struct {
			Reason string `json:"reason"`
		}
		if err := decodeJSON(r, &body, 16<<10); err != nil || utf8.RuneCountInString(strings.TrimSpace(body.Reason)) < 2 || utf8.RuneCountInString(strings.TrimSpace(body.Reason)) > 500 {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REFUND_REASON", "refund reason must contain 2 to 500 characters")
			return
		}
		if order.Status != "COMPLETED" {
			s.writeError(w, http.StatusConflict, requestID, "ORDER_STATE_CONFLICT", "only a completed order can request a refund")
			return
		}
		providerID, _ := strconv.ParseInt(order.ProviderInstanceID, 10, 64)
		provider, err := s.store.StationProvider(tenant.AgentID, providerID, false)
		if err != nil || !provider.RefundEnabled || !provider.AllowUserRefund {
			s.writeError(w, http.StatusForbidden, requestID, "REFUND_NOT_ALLOWED", "this provider does not allow user refund requests")
			return
		}
		if err := s.store.UpdateStationOrderStatus(tenant.AgentID, id, "COMPLETED", "REFUND_REQUESTED", strings.TrimSpace(body.Reason)); err != nil {
			s.writeError(w, http.StatusConflict, requestID, "ORDER_STATE_CONFLICT", "refund request was not accepted")
			return
		}
		s.writeData(w, http.StatusOK, requestID, map[string]string{"message": "refund requested"})
	default:
		s.writeError(w, http.StatusNotFound, requestID, "NOT_FOUND", "order action not found")
	}
}

func (s *Server) stationCheckoutInfo(agentID string) (AgentCheckoutInfo, error) {
	settings, err := s.store.StationPaymentSettings(agentID)
	if err != nil {
		return AgentCheckoutInfo{}, err
	}
	info := AgentCheckoutInfo{
		Methods: map[string]AgentPaymentMethodLimit{}, GlobalMin: settings.MinAmount, GlobalMax: settings.MaxAmount,
		Plans: []AgentCheckoutPlan{}, BalanceDisabled: settings.BalanceDisabled || !settings.Enabled,
		BalanceRechargeMultiplier: settings.BalanceRechargeMultiplier, SubscriptionUSDToCNYRate: settings.SubscriptionUSDToCNYRate,
		RechargeFeeRate: settings.RechargeFeeRate, HelpText: settings.HelpText, HelpImageURL: settings.HelpImageURL,
		AlipayForceQRCode: settings.AlipayForceQRCode, AlipayMobilePrecreateDeepLink: settings.AlipayMobilePrecreateDeepLink,
	}
	if !settings.Enabled {
		return info, nil
	}
	providers, err := s.store.ListStationProviders(agentID, true)
	if err != nil {
		return AgentCheckoutInfo{}, err
	}
	for _, provider := range providers {
		if !stationProviderEnabled(provider, settings) {
			continue
		}
		if provider.ProviderKey == "stripe" {
			if key := provider.Config["publishableKey"]; key != "" {
				info.StripePublishableKey = key
			}
		}
		for _, paymentType := range stationProviderMethods(provider) {
			if !stationMethodSourceMatches(provider, paymentType, settings) {
				continue
			}
			if paymentType == "alipay" && !settings.VisibleMethodAlipayEnabled {
				continue
			}
			if paymentType == "wxpay" && !settings.VisibleMethodWxpayEnabled {
				continue
			}
			limit := methodLimitFromProvider(provider, paymentType, settings)
			used, err := s.store.stationProviderDailyUsed(agentID, provider, paymentType)
			if err != nil {
				return AgentCheckoutInfo{}, err
			}
			limit.DailyUsed = used
			limit.DailyRemaining = -1
			if limit.DailyLimit > 0 {
				limit.DailyRemaining = math.Max(0, limit.DailyLimit-used)
				limit.Available = limit.DailyRemaining > 0
			}
			if existing, exists := info.Methods[paymentType]; exists {
				if existing.Currency != limit.Currency {
					continue
				}
				limit.SingleMin = math.Min(existing.SingleMin, limit.SingleMin)
				if existing.SingleMax == 0 || limit.SingleMax == 0 {
					limit.SingleMax = 0
				} else {
					limit.SingleMax = math.Max(existing.SingleMax, limit.SingleMax)
				}
				if existing.DailyLimit == 0 || limit.DailyLimit == 0 {
					limit.DailyLimit = 0
					limit.DailyRemaining = -1
				} else {
					limit.DailyLimit = math.Max(existing.DailyLimit, limit.DailyLimit)
					limit.DailyRemaining = math.Max(existing.DailyRemaining, limit.DailyRemaining)
				}
				limit.DailyUsed += existing.DailyUsed
				limit.Available = limit.Available || existing.Available
			}
			info.Methods[paymentType] = limit
		}
	}
	plans, err := s.store.ListStationPlans(agentID, true)
	if err != nil {
		return AgentCheckoutInfo{}, err
	}
	for _, plan := range plans {
		info.Plans = append(info.Plans, AgentCheckoutPlan{
			ID: plan.ID, Name: plan.Name, Description: plan.Description, Price: plan.Price, OriginalPrice: plan.OriginalPrice,
			Currency: plan.Currency, ValidityDays: plan.ValidityDays, ValidityUnit: plan.ValidityUnit, Features: plan.Features, ProductName: plan.ProductName,
		})
	}
	return info, nil
}

func methodLimitFromProvider(provider StationPaymentProvider, paymentType string, settings StationPaymentSettings) AgentPaymentMethodLimit {
	limit := AgentPaymentMethodLimit{Currency: "CNY", DisplayName: paymentType, Available: true, SingleMin: 0, SingleMax: 0, DailyLimit: 0, FeeRate: settings.RechargeFeeRate}
	if provider.ProviderKey == "stripe" || provider.ProviderKey == "airwallex" {
		if currency, err := payment.NormalizePaymentCurrency(provider.Config["currency"]); err == nil {
			limit.Currency = currency
		}
	}
	if provider.ProviderKey == "easypay" {
		var methods []stationCustomMethod
		_ = json.Unmarshal([]byte(provider.Config["customMethods"]), &methods)
		for _, method := range methods {
			if method.Type == paymentType && method.DisplayName != "" {
				limit.DisplayName = method.DisplayName
			}
		}
	}
	var parsed map[string]map[string]float64
	if json.Unmarshal([]byte(provider.Limits), &parsed) == nil {
		if item, ok := parsed[paymentType]; ok {
			value := func(camel, legacy string) float64 {
				if v, exists := item[camel]; exists {
					return v
				}
				return item[legacy]
			}
			min, max, daily := value("singleMin", "single_min"), value("singleMax", "single_max"), value("dailyLimit", "daily_limit")
			if min > 0 || max > 0 || daily > 0 {
				limit.SingleMin, limit.SingleMax, limit.DailyLimit = min, max, daily
			}
		}
	}
	return limit
}

func (s *Server) createStationCheckoutOrder(w http.ResponseWriter, r *http.Request, requestID string, tenant TenantContext, session Session, request AgentPaymentOrderRequest) {
	leaseCtx, release, err := s.acquireOperationLease(r.Context(), "station:payment:"+tenant.AgentID)
	if err != nil {
		s.writeError(w, 503, requestID, "PAYMENT_BUSY", "payment checkout is busy")
		return
	}
	defer release()
	r = r.WithContext(leaseCtx)
	settings, err := s.store.StationPaymentSettings(tenant.AgentID)
	if err != nil || !settings.Enabled {
		s.writeError(w, http.StatusGone, requestID, "STATION_BILLING_DISABLED", "this station has not opened charging")
		return
	}
	limited, err := s.store.stationCancelRateLimited(tenant.AgentID, session.MainUserID, settings)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to check cancellation limit")
		return
	}
	if limited {
		s.writeError(w, http.StatusTooManyRequests, requestID, "CANCEL_RATE_LIMITED", "too many cancelled orders in the configured window")
		return
	}
	pending, err := s.store.CountPendingStationOrders(tenant.AgentID, session.MainUserID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to check pending orders")
		return
	}
	if pending >= settings.MaxPendingOrders {
		s.writeError(w, http.StatusTooManyRequests, requestID, "TOO_MANY_PENDING_ORDERS", "too many unpaid orders")
		return
	}
	amount := request.Amount
	var planID *int64
	if request.OrderType == "subscription" {
		plan, err := s.store.StationPlan(tenant.AgentID, request.PlanID)
		if err != nil || !plan.ForSale {
			s.writeError(w, http.StatusBadRequest, requestID, "PLAN_NOT_AVAILABLE", "plan is not for sale on this station")
			return
		}
		amount = plan.Price
		planID = &plan.ID
	}
	if request.OrderType == "balance" && settings.BalanceDisabled {
		s.writeError(w, http.StatusGone, requestID, "BALANCE_RECHARGE_DISABLED", "balance recharge is disabled on this station")
		return
	}
	providers, err := s.store.ListStationProviders(tenant.AgentID, true)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to load payment providers")
		return
	}
	if request.OrderType == "balance" && (amount < settings.MinAmount || settings.MaxAmount > 0 && amount > settings.MaxAmount) {
		s.writeError(w, 400, requestID, "INVALID_AMOUNT", "amount is outside the recharge limit")
		return
	}
	provider, limit, ok, err := s.store.selectStationProviderForOrder(tenant.AgentID, providers, settings, request.PaymentType, amount, request.OrderType)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to select payment provider")
		return
	}
	if !ok {
		s.writeError(w, http.StatusBadRequest, requestID, "PAYMENT_METHOD_UNAVAILABLE", "payment method is not configured on this station")
		return
	}
	amount = stationGatewayAmount(amount, request.OrderType, limit.Currency, settings)
	payText := payment.CalculatePayAmountForCurrency(amount, limit.FeeRate, limit.Currency)
	payAmount, _ := strconv.ParseFloat(payText, 64)
	if settings.DailyLimit > 0 {
		spent, err := s.store.SumStationUserPayAmountSince(tenant.AgentID, session.MainUserID, s.store.clock().UTC().Truncate(24*time.Hour))
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to check daily limit")
			return
		}
		if spent+payAmount > settings.DailyLimit {
			s.writeError(w, http.StatusBadRequest, requestID, "DAILY_LIMIT_EXCEEDED", "daily recharge limit has been reached")
			return
		}
	}
	feeRate := limit.FeeRate
	raw := make([]byte, 8)
	_, _ = rand.Read(raw)
	orderNo := "st" + hex.EncodeToString(raw)
	expires := s.store.clock().UTC().Add(time.Duration(settings.OrderTimeoutMin) * time.Minute)
	order := StationPaymentOrder{
		OutTradeNo: orderNo, Amount: amount, PayAmount: payAmount, FeeRate: feeRate, Currency: limit.Currency,
		PaymentType: request.PaymentType, ProviderInstanceID: strconv.FormatInt(provider.ID, 10), OrderType: request.OrderType,
		PlanID: planID, Status: "PENDING",
	}
	if request.OrderType == "balance" {
		order.Amount = math.Round(amount*settings.BalanceRechargeMultiplier*100) / 100
	}
	prefix := settings.ProductNamePrefix
	if prefix == "" {
		prefix = "充值"
	}
	subject := fmt.Sprintf("%s %.2f %s", prefix, amount, firstNonEmpty(settings.ProductNameSuffix, limit.Currency))
	created, err := s.store.CreateStationOrder(order, tenant.AgentID, session.MainUserID, expires)
	if err != nil {
		s.writeError(w, 500, requestID, "STORE_ERROR", "failed to reserve station order")
		return
	}
	started, err := s.startStationProviderPayment(r, tenant.AgentID, settings, provider, orderNo, payText, subject, request)
	if err != nil {
		_ = s.store.UpdateStationOrderStatus(tenant.AgentID, created.ID, "PENDING", "FAILED", "provider create failed")
		s.writeError(w, http.StatusBadGateway, requestID, "PAYMENT_PROVIDER_UNAVAILABLE", "payment provider could not create this order")
		return
	}
	_, err = s.store.db.Exec(`UPDATE station_payment_orders SET pay_url=?, qr_code=? WHERE agent_id=? AND id=?`, started.PayURL, started.QRCode, tenant.AgentID, created.ID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to create station order")
		return
	}
	created, err = s.store.StationOrder(tenant.AgentID, created.ID)
	if err != nil {
		s.writeError(w, 500, requestID, "STORE_ERROR", "failed to load station order")
		return
	}
	s.recordTenantAudit(tenant.AgentID, "agent_user", session.MainUserID, "billing.order.create", "station_order", created.OutTradeNo, requestID, "success", request.OrderType)
	result := AgentPaymentCreateResult{
		OrderID: created.ID, Amount: created.Amount, PayAmount: created.PayAmount, FeeRate: created.FeeRate, Status: created.Status,
		PaymentType: created.PaymentType, OutTradeNo: created.OutTradeNo, PayURL: created.PayURL, QRCode: created.QRCode,
		Currency: firstNonEmpty(started.Currency, created.Currency), CountryCode: started.CountryCode, PaymentEnv: started.PaymentEnv,
		ClientSecret: started.ClientSecret, IntentID: started.IntentID, ExpiresAt: created.ExpiresAt,
		PaymentMode: provider.PaymentMode, StripePublishableKey: provider.Config["publishableKey"],
		ResultType:                    firstNonEmpty(string(started.ResultType), stationResultType(created)),
		AlipayMobilePrecreateDeepLink: settings.AlipayMobilePrecreateDeepLink,
	}
	if started.OAuth != nil {
		result.OAuth = &AgentWechatOAuthInfo{AuthorizeURL: started.OAuth.AuthorizeURL, AppID: started.OAuth.AppID, OpenID: started.OAuth.OpenID, Scope: started.OAuth.Scope, State: started.OAuth.State, RedirectURL: started.OAuth.RedirectURL}
	}
	if started.JSAPI != nil {
		result.JSAPI = &AgentWechatJSAPI{AppID: started.JSAPI.AppID, TimeStamp: started.JSAPI.TimeStamp, NonceStr: started.JSAPI.NonceStr, Package: started.JSAPI.Package, SignType: started.JSAPI.SignType, PaySign: started.JSAPI.PaySign}
		result.JSAPIPayload = result.JSAPI
	}
	s.writeData(w, http.StatusCreated, requestID, result)
}

func stationResultType(order StationPaymentOrder) string {
	if order.QRCode != "" {
		return "qrcode"
	}
	if order.PayURL != "" {
		return "redirect"
	}
	return "pending"
}

// EnabledTypes contains provider families selected in the settings panel, not
// their checkout methods (for example, EasyPay exposes alipay and wxpay).
func stationProviderEnabled(provider StationPaymentProvider, settings StationPaymentSettings) bool {
	if !provider.Enabled {
		return false
	}
	for _, key := range settings.EnabledTypes {
		if key == provider.ProviderKey {
			return true
		}
	}
	return false
}

func stationProviderMethods(provider StationPaymentProvider) []string {
	if provider.ProviderKey == "stripe" {
		return []string{"stripe"}
	}
	return provider.SupportedTypes
}

func selectStationProvider(providers []StationPaymentProvider, settings StationPaymentSettings, paymentType string) (StationPaymentProvider, AgentPaymentMethodLimit, bool) {
	matches := []StationPaymentProvider{}
	for _, provider := range providers {
		if !stationProviderEnabled(provider, settings) {
			continue
		}
		if paymentType == "alipay" && !settings.VisibleMethodAlipayEnabled || paymentType == "wxpay" && !settings.VisibleMethodWxpayEnabled {
			continue
		}
		if !stationMethodSourceMatches(provider, paymentType, settings) {
			continue
		}
		for _, supported := range stationProviderMethods(provider) {
			if supported == paymentType {
				matches = append(matches, provider)
				break
			}
		}
	}
	if len(matches) == 0 {
		return StationPaymentProvider{}, AgentPaymentMethodLimit{}, false
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].SortOrder == matches[j].SortOrder {
			return matches[i].ID < matches[j].ID
		}
		return matches[i].SortOrder < matches[j].SortOrder
	})
	chosen := matches[0]
	return chosen, methodLimitFromProvider(chosen, paymentType, settings), true
}

func (s *Server) startStationProviderPayment(r *http.Request, agentID string, settings StationPaymentSettings, provider StationPaymentProvider, orderNo, money, subject string, request AgentPaymentOrderRequest) (*payment.CreatePaymentResponse, error) {
	origin := requestOrigin(r)
	cfg := map[string]string{}
	for key, value := range provider.Config {
		cfg[key] = value
	}
	cfg["paymentMode"] = provider.PaymentMode
	cfg["notifyUrl"], _ = stationCallbackURL(cfg["notifyUrl"], origin, stationWebhookPath(agentID, provider.ProviderKey))
	cfg["returnUrl"], _ = stationCallbackURL(cfg["returnUrl"], origin, "/payment/result")
	if cfg["notifyUrl"] == "" || cfg["returnUrl"] == "" {
		return nil, fmt.Errorf("invalid callback origin")
	}
	if settings.AlipayForceQRCode && provider.ProviderKey == "alipay" {
		cfg["paymentMode"] = ""
	}
	impl, err := payprovider.CreateProvider(provider.ProviderKey, strconv.FormatInt(provider.ID, 10), cfg)
	if err != nil {
		return nil, err
	}
	return impl.CreatePayment(r.Context(), payment.CreatePaymentRequest{
		OrderID: orderNo, Amount: money, PaymentType: request.PaymentType, Subject: subject,
		NotifyURL: cfg["notifyUrl"], ReturnURL: cfg["returnUrl"] + "?out_trade_no=" + url.QueryEscape(orderNo),
		OpenID: request.OpenID, ClientIP: clientIP(r), IsMobile: request.IsMobile != nil && *request.IsMobile && (request.PaymentType != "alipay" || !settings.AlipayForceQRCode || settings.AlipayMobilePrecreateDeepLink),
		AlipayMobilePrecreate: settings.AlipayMobilePrecreateDeepLink && request.IsMobile != nil && *request.IsMobile,
		InstanceSubMethods:    strings.Join(provider.SupportedTypes, ","),
	})
}

func clientIP(r *http.Request) string {
	host, _, _ := strings.Cut(r.RemoteAddr, ":")
	return host
}

func (s *Server) handleStationBillingWebhook(w http.ResponseWriter, r *http.Request, requestID string) {
	w.Header().Set("Cache-Control", "no-store")
	relative := strings.TrimPrefix(r.URL.Path, "/api/v1/agent/billing/webhook/")
	parts := strings.Split(relative, "/")
	if len(parts) != 2 || !stationProviderKeys[parts[1]] {
		s.writeError(w, http.StatusNotFound, requestID, "NOT_FOUND", "payment webhook not found")
		return
	}
	agentID := strings.TrimSpace(parts[0])
	providerKey := parts[1]
	if _, err := s.store.Agent(agentID); err != nil {
		s.writeError(w, http.StatusNotFound, requestID, "AGENT_NOT_FOUND", "agent tenant was not found")
		return
	}
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	raw := string(body)
	if raw == "" {
		raw = r.URL.RawQuery
	}
	headers := map[string]string{}
	for key, values := range r.Header {
		if len(values) > 0 {
			headers[key] = values[0]
			headers[strings.ToLower(key)] = values[0]
		}
	}
	providers, err := s.store.ListStationProviders(agentID, true)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to load payment providers")
		return
	}
	var note *payment.PaymentNotification
	var verifiedProviderID int64
	for _, item := range providers {
		if item.ProviderKey != providerKey {
			continue
		}
		config := make(map[string]string, len(item.Config)+2)
		for key, value := range item.Config {
			config[key] = value
		}
		config["notifyUrl"] = requestOrigin(r) + stationWebhookPath(agentID, item.ProviderKey)
		config["returnUrl"] = requestOrigin(r) + "/payment/result"
		impl, err := payprovider.CreateProvider(item.ProviderKey, strconv.FormatInt(item.ID, 10), config)
		if err != nil {
			continue
		}
		parsed, err := impl.VerifyNotification(r.Context(), raw, headers)
		if err != nil || parsed == nil || parsed.OrderID == "" {
			continue
		}
		if identity, ok := impl.(payment.MerchantIdentityProvider); ok {
			mismatch := false
			for key, expected := range identity.MerchantIdentityMetadata() {
				if expected != "" && parsed.Metadata[key] != expected {
					mismatch = true
					break
				}
			}
			if mismatch {
				continue
			}
		}
		matched, lookupErr := s.store.StationOrderByNo(agentID, parsed.OrderID)
		if lookupErr != nil || matched.ProviderInstanceID != strconv.FormatInt(item.ID, 10) {
			continue
		}
		note = parsed
		verifiedProviderID = item.ID
		break
	}
	if note == nil || (note.Status != payment.ProviderStatusSuccess && note.Status != payment.NotificationStatusSuccess && note.Status != payment.ProviderStatusPaid) {
		s.writeError(w, http.StatusUnauthorized, requestID, "PAYMENT_SIGNATURE_INVALID", "payment notification was not accepted")
		return
	}
	order, err := s.store.StationOrderByNo(agentID, note.OrderID)
	if err != nil || order.ProviderInstanceID != strconv.FormatInt(verifiedProviderID, 10) || (note.Metadata["currency"] != "" && order.Currency != "" && !strings.EqualFold(note.Metadata["currency"], order.Currency)) || math.IsNaN(note.Amount) || math.IsInf(note.Amount, 0) || math.Abs(order.PayAmount-note.Amount) > 0.001 {
		s.writeError(w, http.StatusBadRequest, requestID, "PAYMENT_NOTIFICATION_MISMATCH", "payment notification does not match the order")
		return
	}
	completed, _, err := s.store.CompleteStationOrder(agentID, note.OrderID, note.TradeNo)
	if err != nil {
		s.writeError(w, http.StatusConflict, requestID, "ORDER_STATE_CONFLICT", "station order could not be completed")
		return
	}
	// CreditUserRecharge is idempotent. Repeat it even when the order was already
	// completed, so a retry can recover a previous wallet write failure.
	{
		cents := int64(math.Round(completed.Amount * 100))
		if err := s.store.CreditUserRecharge(agentID, completed.MainUserID, cents, completed.OutTradeNo); err != nil {
			s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "payment was received but station credit failed")
			return
		}
	}
	if providerKey == "easypay" || providerKey == "alipay" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "success")
		return
	}
	s.writeData(w, http.StatusOK, requestID, map[string]string{"status": "ok"})
}

func stationWebhookPath(agentID, provider string) string {
	return "/api/v1/agent/billing/webhook/" + url.PathEscape(agentID) + "/" + provider
}

func requestOrigin(r *http.Request) string {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if parsed, err := url.Parse(origin); err == nil && parsed.Host != "" && (parsed.Scheme == "http" || parsed.Scheme == "https") {
		return parsed.Scheme + "://" + parsed.Host
	}
	scheme := "https"
	if r.TLS == nil {
		scheme = "http"
	}
	return scheme + "://" + r.Host
}

func splitStationID(value string) (int64, string, bool) {
	parts := strings.Split(strings.Trim(value, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		return 0, "", false
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id <= 0 {
		return 0, "", false
	}
	action := ""
	if len(parts) == 2 {
		action = parts[1]
	}
	if len(parts) > 2 {
		return 0, "", false
	}
	return id, action, true
}

func (s *Server) stationBillingEnabled(agentID string) bool {
	settings, err := s.store.StationPaymentSettings(agentID)
	return err == nil && settings.Enabled
}
