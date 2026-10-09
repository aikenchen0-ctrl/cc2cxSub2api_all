package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/Wei-Shaw/agentapi/payment"
	payprovider "github.com/Wei-Shaw/agentapi/payment/provider"
)

var errStationProviderBusy = errors.New("payment provider has in-progress orders")

func stationConfigSecret(key string) bool {
	for field := range stationSensitiveConfigKeys {
		if strings.EqualFold(key, field) {
			return true
		}
	}
	return strings.EqualFold(key, "alipayPublicKey")
}

// Merge only fields supplied by the admin. Identity comes from the Session;
// neither a body agent_id nor the global main-site payment configuration is used.
func decodeStationProviderPatch(r *http.Request, current StationPaymentProvider) (StationPaymentProvider, error) {
	var patch map[string]json.RawMessage
	if err := decodeJSON(r, &patch, maxJSONBody); err != nil {
		return current, fmt.Errorf("invalid provider")
	}
	if raw, ok := patch["provider_key"]; ok {
		var key string
		if json.Unmarshal(raw, &key) != nil || key != current.ProviderKey {
			return current, fmt.Errorf("provider type cannot be changed")
		}
	}
	base, _ := json.Marshal(current)
	var merged map[string]json.RawMessage
	_ = json.Unmarshal(base, &merged)
	for key, value := range patch {
		if key == "id" || key == "agent_id" || key == "webhook_path" || key == "config_configured" {
			continue
		}
		if string(value) == "null" {
			return current, fmt.Errorf("%s cannot be null", key)
		}
		merged[key] = value
	}
	if raw, ok := patch["config"]; ok {
		var config map[string]string
		if json.Unmarshal(raw, &config) != nil {
			return current, fmt.Errorf("invalid provider config")
		}
		for key, value := range config {
			if stationConfigSecret(key) && strings.TrimSpace(value) == "" {
				continue
			}
			current.Config[key] = value
		}
	}
	merged["config"], _ = json.Marshal(current.Config)
	raw, _ := json.Marshal(merged)
	var item StationPaymentProvider
	if err := json.Unmarshal(raw, &item); err != nil {
		return item, fmt.Errorf("invalid provider")
	}
	return item, validateStationProviderFields(&item)
}

func stationCallbackURL(raw, fallback, path string) (string, error) {
	base := strings.TrimSpace(raw)
	if base == "" {
		base = fallback
	}
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return "", fmt.Errorf("callback address must be an HTTP(S) origin")
	}
	// Keep the configured public origin, but always derive the tenant and
	// provider path on the server. An old/copied site's path cannot redirect credit.
	u.Path, u.RawPath, u.RawQuery, u.Fragment = path, "", "", ""
	return u.String(), nil
}

func prepareStationProvider(item *StationPaymentProvider, agentID, origin string) error {
	if err := validateStationProviderFields(item); err != nil {
		return err
	}
	var err error
	item.Config["notifyUrl"], err = stationCallbackURL(item.Config["notifyUrl"], origin, stationWebhookPath(agentID, item.ProviderKey))
	if err != nil {
		return err
	}
	item.Config["returnUrl"], err = stationCallbackURL(item.Config["returnUrl"], origin, "/payment/result")
	if err != nil {
		return err
	}
	if item.Enabled {
		required := map[string][]string{
			"easypay":   {"pid", "pkey", "apiBase"},
			"alipay":    {"appId", "privateKey", "publicKey"},
			"wxpay":     {"appId", "mchId", "privateKey", "apiV3Key", "certSerial", "publicKey", "publicKeyId"},
			"stripe":    {"secretKey", "publishableKey", "webhookSecret"},
			"airwallex": {"clientId", "apiKey", "webhookSecret", "apiBase"},
		}
		for _, key := range required[item.ProviderKey] {
			if item.ProviderKey == "alipay" && key == "publicKey" && strings.TrimSpace(item.Config["alipayPublicKey"]) != "" {
				continue
			}
			if strings.TrimSpace(item.Config[key]) == "" {
				return fmt.Errorf("%s requires %s", item.ProviderKey, key)
			}
		}
		cfg := make(map[string]string, len(item.Config)+1)
		for key, value := range item.Config {
			cfg[key] = value
		}
		cfg["paymentMode"] = item.PaymentMode
		if _, err := payprovider.CreateProvider(item.ProviderKey, "validate", cfg); err != nil {
			// Provider parsing errors may contain PEM/config data; don't return those.
			return fmt.Errorf("%s payment credentials are incomplete or invalid", item.ProviderKey)
		}
	}
	return nil
}

type stationCustomMethod struct {
	Type         string `json:"type"`
	UpstreamType string `json:"upstreamType"`
	DisplayName  string `json:"displayName"`
}

var stationCustomCode = regexp.MustCompile(`^[a-z0-9_-]+$`)
var stationUpstreamCode = regexp.MustCompile(`^[a-z0-9_.-]+$`)

func validateStationProviderMethods(item *StationPaymentProvider) error {
	if item.ProviderKey == "stripe" || item.ProviderKey == "airwallex" {
		currency, err := payment.NormalizePaymentCurrency(item.Config["currency"])
		if err != nil {
			return err
		}
		item.Config["currency"] = currency
	}
	allowed := map[string][]string{
		"easypay": {"alipay", "wxpay"}, "alipay": {"alipay", "alipay_direct"},
		"wxpay": {"wxpay", "wxpay_direct"}, "stripe": {"card", "alipay", "wxpay", "link", "stripe"}, "airwallex": {"airwallex"},
	}
	types := map[string]bool{}
	for _, key := range allowed[item.ProviderKey] {
		types[key] = true
	}
	if item.ProviderKey == "easypay" && strings.TrimSpace(item.Config["customMethods"]) != "" {
		var methods []stationCustomMethod
		if json.Unmarshal([]byte(item.Config["customMethods"]), &methods) != nil {
			return fmt.Errorf("invalid custom payment methods")
		}
		for _, method := range methods {
			if !stationCustomCode.MatchString(method.Type) || !stationUpstreamCode.MatchString(method.UpstreamType) || len(method.Type) > 32 || len(method.UpstreamType) > 64 || len(method.DisplayName) > 200 || strings.HasPrefix(method.Type, "alipay") || strings.HasPrefix(method.Type, "wxpay") || types[method.Type] {
				return fmt.Errorf("custom payment methods must have unique non-reserved codes and valid upstream types")
			}
			types[method.Type] = true
		}
	}
	for _, method := range item.SupportedTypes {
		if !types[method] {
			return fmt.Errorf("unsupported payment method for %s", item.ProviderKey)
		}
	}
	switch item.ProviderKey {
	case "easypay":
		if item.PaymentMode == "" {
			item.PaymentMode = "qrcode"
		}
		if item.PaymentMode != "qrcode" && item.PaymentMode != "popup" {
			return fmt.Errorf("invalid EasyPay payment mode")
		}
	case "alipay":
		if item.PaymentMode != "" && item.PaymentMode != "redirect" {
			return fmt.Errorf("invalid Alipay payment mode")
		}
	default:
		if item.PaymentMode != "" {
			return fmt.Errorf("payment mode is not supported by this provider")
		}
	}
	return nil
}

func (s *Store) stationProviderPending(agentID string, id int64) (bool, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM station_payment_orders WHERE agent_id=? AND provider_id=? AND (status IN ('PAID','RECHARGING','REFUNDING','REFUND_PENDING') OR (status='PENDING' AND expires_at>?))`, agentID, id, s.clock().UTC().Unix()).Scan(&count)
	return count > 0, err
}

func stationProtectedProviderChange(before, after StationPaymentProvider) bool {
	protected := map[string][]string{
		"easypay": {"pid", "apiBase", "customMethods"}, "alipay": {"appId"},
		"wxpay":  {"appId", "mpAppId", "mchId", "publicKeyId", "certSerial"},
		"stripe": {"currency"}, "airwallex": {"clientId", "apiBase", "accountId", "currency"},
	}
	for key, value := range after.Config {
		if stationConfigSecret(key) && value != before.Config[key] {
			return true
		}
	}
	for _, key := range protected[before.ProviderKey] {
		if before.Config[key] != after.Config[key] {
			return true
		}
	}
	for _, old := range before.SupportedTypes {
		found := false
		for _, next := range after.SupportedTypes {
			if old == next {
				found = true
				break
			}
		}
		if !found {
			return true
		}
	}
	return false
}
