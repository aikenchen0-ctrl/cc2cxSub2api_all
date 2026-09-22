package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

const sessionTTL = 3 * 24 * time.Hour

type Config struct {
	Addr                        string
	WebDir                      string
	DatabasePath                string
	MainAPIBaseURL              string
	MainModelBaseURL            string
	AdminKey                    string
	AppCredential               string
	SatelliteSlug               string
	SSOSecret                   string
	SSOAudience                 string
	SessionSecret               string
	SessionSecretWeak           bool
	CookieName                  string
	CookieSecure                bool
	AgentID                     string
	AgentDomain                 string
	AgentName                   string
	SiteName                    string
	SiteLogo                    string
	BrandSync                   bool
	AgentDisabled               bool
	BillingMode                 string
	OwnerMainUserID             string
	InitialBalanceCents         int64
	MaxRequestCostCents         int64
	MainRequestTimeout          time.Duration
	ModelStreamTimeout          time.Duration
	HTTPWriteTimeout            time.Duration
	AutoBindExistingUsers       bool
	SettlementReconcileInterval time.Duration
	SettlementReconcileBatch    int
	VideoTaskReconcileAge       time.Duration
	PaymentEnabled              bool
	PaymentProvider             string
	PaymentCurrency             string
	PaymentWebhookSecret        string
	PaymentMinCents             int64
	PaymentMaxCents             int64
	PaymentOrderTTL             time.Duration
	PaymentCheckoutURLTemplate  string
}

func LoadConfig() (Config, error) {
	mainURL := firstNonEmpty(os.Getenv("MAIN_API_URL"), os.Getenv("LINK"), "http://localhost:18080")
	apiBase := normalizeAPIBase(mainURL)
	modelBase := firstNonEmpty(os.Getenv("MAIN_MODEL_URL"), mainURL)
	modelBase = normalizeModelBase(modelBase)

	secret := secretEnv("SESSION_SECRET")
	secretWeak := len(secret) < 32
	if secret == "" {
		secret = "agentapi-development-secret-change-me"
		slog.Warn("SESSION_SECRET is not set; using development fallback. Set a strong secret in production.")
	} else if secretWeak {
		slog.Warn("SESSION_SECRET is shorter than 32 bytes; /readyz will remain unavailable")
	}

	cookieSecure := true
	if raw := strings.TrimSpace(os.Getenv("COOKIE_SECURE")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("COOKIE_SECURE must be boolean: %w", err)
		}
		cookieSecure = value
	} else if strings.HasPrefix(apiBase, "http://") || strings.HasPrefix(modelBase, "http://") {
		// Local development is normally plain HTTP. Production should set this explicitly.
		cookieSecure = false
	}

	cfg := Config{
		Addr:                        firstNonEmpty(os.Getenv("AGENTAPI_ADDR"), ":8080"),
		WebDir:                      firstNonEmpty(os.Getenv("AGENTAPI_WEB_DIR"), "./web"),
		DatabasePath:                firstNonEmpty(os.Getenv("AGENTAPI_DATABASE_PATH"), "./data/agentapi.db"),
		MainAPIBaseURL:              apiBase,
		MainModelBaseURL:            modelBase,
		AdminKey:                    secretEnv("SUB2API_ADMIN_KEY"),
		AppCredential:               secretEnv("SUB2API_APP_CREDENTIAL"),
		SatelliteSlug:               firstNonEmpty(os.Getenv("SUB2API_SATELLITE"), "agentapi"),
		SSOSecret:                   secretEnv("SUB2API_SSO_SECRET"),
		SSOAudience:                 firstNonEmpty(os.Getenv("AGENTAPI_SSO_AUDIENCE"), firstNonEmpty(os.Getenv("SUB2API_SATELLITE"), "agentapi")),
		SessionSecret:               secret,
		SessionSecretWeak:           secretWeak,
		CookieName:                  firstNonEmpty(os.Getenv("AGENTAPI_COOKIE_NAME"), "agentapi_session"),
		CookieSecure:                cookieSecure,
		AgentID:                     firstNonEmpty(os.Getenv("AGENT_ID"), "agent-local"),
		AgentDomain:                 strings.TrimSpace(os.Getenv("AGENT_DOMAIN")),
		AgentName:                   firstNonEmpty(os.Getenv("AGENT_NAME"), "AgentAPI"),
		SiteName:                    firstNonEmpty(os.Getenv("AGENT_SITE_NAME"), "AgentAPI"),
		SiteLogo:                    strings.TrimSpace(os.Getenv("AGENT_SITE_LOGO")),
		BrandSync:                   envBool("AGENT_BRAND_SYNC", false),
		AgentDisabled:               !envBool("AGENT_ENABLED", true),
		BillingMode:                 firstNonEmpty(os.Getenv("AGENT_BILLING_MODE"), "owner_upstream"),
		OwnerMainUserID:             strings.TrimSpace(os.Getenv("AGENT_OWNER_MAIN_USER_ID")),
		InitialBalanceCents:         envCents("AGENT_INITIAL_BALANCE", 0),
		MaxRequestCostCents:         envCents("AGENT_MAX_REQUEST_COST", 100),
		MainRequestTimeout:          envDuration("MAIN_REQUEST_TIMEOUT", 90*time.Second),
		ModelStreamTimeout:          envDurationAllowZero("MODEL_STREAM_TIMEOUT", 10*time.Minute),
		HTTPWriteTimeout:            envDurationAllowZero("AGENTAPI_WRITE_TIMEOUT", 10*time.Minute),
		AutoBindExistingUsers:       envBool("AGENT_AUTO_BIND_EXISTING_USERS", false),
		SettlementReconcileInterval: envDurationAllowZero("AGENT_SETTLEMENT_RECONCILE_INTERVAL", 5*time.Minute),
		SettlementReconcileBatch:    envInt("AGENT_SETTLEMENT_RECONCILE_BATCH", 50, 1, 500),
		VideoTaskReconcileAge:       envDuration("AGENT_VIDEO_TASK_RECONCILE_AGE", 30*time.Minute),
		PaymentEnabled:              envBool("AGENT_PAYMENT_ENABLED", false),
		PaymentProvider:             firstNonEmpty(os.Getenv("AGENT_PAYMENT_PROVIDER"), "manual"),
		PaymentCurrency:             strings.ToUpper(firstNonEmpty(os.Getenv("AGENT_PAYMENT_CURRENCY"), "CNY")),
		PaymentWebhookSecret:        secretEnv("AGENT_PAYMENT_WEBHOOK_SECRET"),
		PaymentMinCents:             envCents("AGENT_PAYMENT_MIN_AMOUNT", 100),
		PaymentMaxCents:             envCents("AGENT_PAYMENT_MAX_AMOUNT", 1000000),
		PaymentOrderTTL:             envDuration("AGENT_PAYMENT_ORDER_TTL", 30*time.Minute),
		PaymentCheckoutURLTemplate:  strings.TrimSpace(os.Getenv("AGENT_PAYMENT_CHECKOUT_URL_TEMPLATE")),
	}
	if cfg.MaxRequestCostCents <= 0 {
		return Config{}, fmt.Errorf("AGENT_MAX_REQUEST_COST must be greater than zero")
	}
	if cfg.BillingMode != "owner_upstream" {
		return Config{}, fmt.Errorf("AGENT_BILLING_MODE must be owner_upstream")
	}
	// Local wallet seeding is useful for unit tests, but would break the
	// owner-authoritative accounting contract in a deployed AgentAPI. Refuse it
	// at configuration load time instead of silently creating spendable credit.
	if cfg.InitialBalanceCents != 0 {
		return Config{}, fmt.Errorf("AGENT_INITIAL_BALANCE is disabled for owner_upstream billing")
	}
	if cfg.PaymentMinCents <= 0 || cfg.PaymentMaxCents < cfg.PaymentMinCents {
		return Config{}, fmt.Errorf("AGENT_PAYMENT_MIN_AMOUNT/MAX_AMOUNT are invalid")
	}
	if cfg.PaymentOrderTTL <= 0 {
		return Config{}, fmt.Errorf("AGENT_PAYMENT_ORDER_TTL must be greater than zero")
	}
	if cfg.PaymentEnabled && strings.TrimSpace(cfg.PaymentWebhookSecret) == "" {
		return Config{}, fmt.Errorf("AGENT_PAYMENT_WEBHOOK_SECRET is required when AGENT_PAYMENT_ENABLED=true")
	}

	if cfg.AppCredential == "" {
		slog.Warn("SUB2API_APP_CREDENTIAL is not set; model relay requests will be rejected")
	}
	return cfg, nil
}

func normalizeAPIBase(value string) string {
	base := strings.TrimRight(strings.TrimSpace(value), "/")
	if base == "" {
		base = "http://localhost:18080"
	}
	if strings.HasSuffix(base, "/api/v1") {
		return base
	}
	return base + "/api/v1"
}

func normalizeModelBase(value string) string {
	base := strings.TrimRight(strings.TrimSpace(value), "/")
	if base == "" {
		base = "http://localhost:18080"
	}
	if strings.HasSuffix(base, "/api/v1") {
		base = strings.TrimSuffix(base, "/api/v1")
	}
	if strings.HasSuffix(base, "/v1") {
		return base
	}
	return base + "/v1"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// secretEnv supports the *_FILE convention used by Docker and orchestrators.
// File-backed secrets take precedence over inline values and are read only at
// process startup, keeping credentials out of frontend config and templates.
func secretEnv(name string) string {
	if fileName := strings.TrimSpace(os.Getenv(name + "_FILE")); fileName != "" {
		data, err := os.ReadFile(fileName)
		if err != nil {
			slog.Warn("secret file could not be read; falling back to inline value", "name", name, "file", fileName, "error", err)
		} else if value := strings.TrimSpace(string(data)); value != "" {
			return value
		}
	}
	return strings.TrimSpace(os.Getenv(name))
}

func envBool(name string, fallback bool) bool {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		slog.Warn("invalid boolean environment variable; using fallback", "name", name, "value", raw)
		return fallback
	}
	return value
}

func envDuration(name string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		slog.Warn("invalid duration environment variable; using fallback", "name", name, "value", raw)
		return fallback
	}
	return value
}

func envDurationAllowZero(name string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value < 0 {
		slog.Warn("invalid duration environment variable; using fallback", "name", name, "value", raw)
		return fallback
	}
	return value
}

func envInt(name string, fallback, minimum, maximum int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < minimum || value > maximum {
		slog.Warn("invalid integer environment variable; using fallback", "name", name, "value", raw)
		return fallback
	}
	return value
}

// envCents parses a decimal amount such as 12.50 without using floating point
// arithmetic. The value is stored as the smallest currency unit throughout the
// local ledger. A plain integer is interpreted as a whole currency unit.
func envCents(name string, fallback int64) int64 {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := parseCents(raw)
	if err != nil {
		slog.Warn("invalid currency environment variable; using fallback", "name", name, "value", raw, "error", err)
		return fallback
	}
	return value
}

func parseCents(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, fmt.Errorf("empty amount")
	}
	negative := strings.HasPrefix(raw, "-")
	if negative || strings.HasPrefix(raw, "+") {
		raw = raw[1:]
	}
	parts := strings.Split(raw, ".")
	if len(parts) > 2 || parts[0] == "" {
		return 0, fmt.Errorf("invalid amount")
	}
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || whole < 0 {
		return 0, fmt.Errorf("invalid whole amount")
	}
	frac := int64(0)
	if len(parts) == 2 {
		if parts[1] == "" || len(parts[1]) > 2 {
			return 0, fmt.Errorf("amount supports at most two decimal places")
		}
		frac, err = strconv.ParseInt(parts[1]+strings.Repeat("0", 2-len(parts[1])), 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid fractional amount")
		}
	}
	if whole > (int64(^uint64(0)>>1)-frac)/100 {
		return 0, fmt.Errorf("amount overflow")
	}
	value := whole*100 + frac
	if negative {
		value = -value
	}
	return value, nil
}

func sessionKey(secret string) []byte {
	sum := sha256.Sum256([]byte(secret))
	key := make([]byte, len(sum))
	copy(key, sum[:])
	return key
}

func hashToken(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
