package main

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSecretEnvPrefersFileBackedSecret(t *testing.T) {
	t.Setenv("AGENTAPI_TEST_SECRET", "inline-value")
	path := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(path, []byte("file-value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENTAPI_TEST_SECRET_FILE", path)
	if got := secretEnv("AGENTAPI_TEST_SECRET"); got != "file-value" {
		t.Fatalf("secretEnv()=%q, want file-backed value", got)
	}
}

func TestSecretEnvFallsBackWhenFileIsUnavailable(t *testing.T) {
	t.Setenv("AGENTAPI_TEST_SECRET", "inline-value")
	t.Setenv("AGENTAPI_TEST_SECRET_FILE", filepath.Join(t.TempDir(), "missing"))
	if got := secretEnv("AGENTAPI_TEST_SECRET"); got != "inline-value" {
		t.Fatalf("secretEnv()=%q, want inline fallback", got)
	}
}

func TestDecodeOptionalJSONAcceptsEmptyBody(t *testing.T) {
	req := httptest.NewRequest("POST", "/", strings.NewReader(""))
	var payload struct {
		RequestID string `json:"request_id"`
	}
	hasBody, err := decodeOptionalJSON(req, &payload, 1024)
	if err != nil || hasBody || payload.RequestID != "" {
		t.Fatalf("decodeOptionalJSON()=(%v, %v, %+v), want empty body", hasBody, err, payload)
	}
}

func TestDecodeOptionalJSONHandlesChunkedJSON(t *testing.T) {
	req := httptest.NewRequest("POST", "/", strings.NewReader(`{"request_id":"req-1"}`))
	var payload struct {
		RequestID string `json:"request_id"`
	}
	hasBody, err := decodeOptionalJSON(req, &payload, 1024)
	if err != nil || !hasBody || payload.RequestID != "req-1" {
		t.Fatalf("decodeOptionalJSON()=(%v, %v, %+v), want request id", hasBody, err, payload)
	}
}

func TestLoadConfigSeparatesControlAndModelRelayBases(t *testing.T) {
	for name, value := range map[string]string{
		"AGENT_ID":                          "agent-local",
		"SUB2API_APP_CREDENTIAL":            "",
		"SUB2API_APP_CREDENTIAL_FILE":       "",
		"SESSION_SECRET":                    strings.Repeat("s", 32),
		"SESSION_SECRET_FILE":               "",
		"AGENT_BILLING_MODE":                "owner_upstream",
		"AGENT_INITIAL_BALANCE":             "",
		"AGENT_PAYMENT_ENABLED":             "false",
		"AGENT_PAYMENT_WEBHOOK_SECRET":      "",
		"AGENT_PAYMENT_WEBHOOK_SECRET_FILE": "",
	} {
		t.Setenv(name, value)
	}

	t.Setenv("SUB2API_SATELLITE", "agentapi")
	t.Setenv("AGENTAPI_SSO_AUDIENCE", "agentapi")
	t.Setenv("AGENT_DOMAIN", "")
	t.Run("rejects an unregistered satellite slug", func(t *testing.T) {
		t.Setenv("SUB2API_SATELLITE", "other-satellite")
		if _, err := LoadConfig(); err == nil || !strings.Contains(err.Error(), "registered slug agentapi") {
			t.Fatalf("unregistered satellite slug error = %v, want rejection", err)
		}
	})
	t.Run("rejects an unregistered SSO audience", func(t *testing.T) {
		t.Setenv("AGENTAPI_SSO_AUDIENCE", "other-audience")
		if _, err := LoadConfig(); err == nil || !strings.Contains(err.Error(), "registered audience agentapi") {
			t.Fatalf("unregistered SSO audience error = %v, want rejection", err)
		}
	})
	t.Run("Docker relay overrides model URL without replacing control URL or LINK", func(t *testing.T) {
		t.Setenv("MAIN_API_URL", "https://management.example.test/api/v1")
		t.Setenv("MAIN_MODEL_URL", "https://model.example.test/v1")
		t.Setenv("SUB2API_RELAY_BASE_URL", "http://sub2api:8080/v1")
		t.Setenv("LINK", "https://public.example.test")

		cfg, err := LoadConfig()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.MainAPIBaseURL != "https://management.example.test/api/v1" {
			t.Fatalf("management API base = %q, want MAIN_API_URL", cfg.MainAPIBaseURL)
		}
		if cfg.MainModelBaseURL != "http://sub2api:8080/v1" {
			t.Fatalf("model base = %q, want Docker relay URL", cfg.MainModelBaseURL)
		}
	})

	t.Run("MAIN_MODEL_URL is used when Docker relay is unset", func(t *testing.T) {
		t.Setenv("MAIN_API_URL", "https://management.example.test")
		t.Setenv("MAIN_MODEL_URL", "https://dedicated-model.example.test/v1")
		t.Setenv("SUB2API_RELAY_BASE_URL", "")
		t.Setenv("LINK", "https://public.example.test")

		cfg, err := LoadConfig()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.MainAPIBaseURL != "https://management.example.test/api/v1" || cfg.MainModelBaseURL != "https://dedicated-model.example.test/v1" {
			t.Fatalf("resolved bases = %q / %q, want explicit management/model URLs", cfg.MainAPIBaseURL, cfg.MainModelBaseURL)
		}
	})

	t.Run("LINK remains the public model fallback", func(t *testing.T) {
		t.Setenv("MAIN_API_URL", "https://management.example.test")
		t.Setenv("MAIN_MODEL_URL", "")
		t.Setenv("SUB2API_RELAY_BASE_URL", "")
		t.Setenv("LINK", "https://public.example.test")

		cfg, err := LoadConfig()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.MainAPIBaseURL != "https://management.example.test/api/v1" || cfg.MainModelBaseURL != "https://public.example.test/v1" {
			t.Fatalf("resolved bases = %q / %q, want management API and public LINK", cfg.MainAPIBaseURL, cfg.MainModelBaseURL)
		}
	})

	t.Run("scheme-less local LINK gets HTTP for both bases", func(t *testing.T) {
		t.Setenv("MAIN_API_URL", "")
		t.Setenv("MAIN_MODEL_URL", "")
		t.Setenv("SUB2API_RELAY_BASE_URL", "")
		t.Setenv("LINK", "localhost:18080")

		cfg, err := LoadConfig()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.MainAPIBaseURL != "http://localhost:18080/api/v1" || cfg.MainModelBaseURL != "http://localhost:18080/v1" {
			t.Fatalf("resolved bases = %q / %q, want HTTP loopback bases", cfg.MainAPIBaseURL, cfg.MainModelBaseURL)
		}
	})
}

func TestLoadConfigAllowsStoredPaymentConfigWithoutBootstrapSecret(t *testing.T) {
	for name, value := range map[string]string{
		"AGENT_ID":                          "agent-local",
		"SUB2API_APP_CREDENTIAL":            "shared-satellite-credential",
		"SUB2API_APP_CREDENTIAL_FILE":       "",
		"SUB2API_SATELLITE":                 "agentapi",
		"AGENTAPI_SSO_AUDIENCE":             "agentapi",
		"SESSION_SECRET":                    strings.Repeat("s", 32),
		"SESSION_SECRET_FILE":               "",
		"AGENT_BILLING_MODE":                "owner_upstream",
		"AGENT_INITIAL_BALANCE":             "",
		"AGENT_PAYMENT_ENABLED":             "true",
		"AGENT_PAYMENT_WEBHOOK_SECRET":      "",
		"AGENT_PAYMENT_WEBHOOK_SECRET_FILE": "",
	} {
		t.Setenv(name, value)
	}

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig rejected a restart that can use stored payment config: %v", err)
	}
	if !cfg.PaymentEnabled || cfg.PaymentWebhookSecret != "" {
		t.Fatalf("unexpected payment bootstrap config: enabled=%v secret=%q", cfg.PaymentEnabled, cfg.PaymentWebhookSecret)
	}
}

func TestLoadConfigDoesNotInventSharedTenant(t *testing.T) {
	for name, value := range map[string]string{
		"AGENT_ID":                     "",
		"AGENT_DOMAIN":                 "",
		"AGENT_OWNER_MAIN_USER_ID":     "",
		"AGENTAPI_SHARED_HOSTS":        "Shared.Example.Test:443, shared.example.test, tenant-entry.example.test",
		"SUB2API_SATELLITE":            "agentapi",
		"AGENTAPI_SSO_AUDIENCE":        "agentapi",
		"SESSION_SECRET":               strings.Repeat("s", 32),
		"SUB2API_SSO_SECRET":           strings.Repeat("o", 32),
		"SUB2API_APP_CREDENTIAL":       "shared-satellite-credential",
		"AGENT_INITIAL_BALANCE":        "",
		"AGENT_PAYMENT_ENABLED":        "false",
		"AGENT_PAYMENT_WEBHOOK_SECRET": "",
	} {
		t.Setenv(name, value)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AgentID != "" {
		t.Fatalf("shared runtime invented process tenant %q", cfg.AgentID)
	}
	if len(cfg.SharedHosts) != 2 || cfg.SharedHosts[0] != "shared.example.test" || cfg.SharedHosts[1] != "tenant-entry.example.test" {
		t.Fatalf("shared hosts were not normalized and deduplicated: %#v", cfg.SharedHosts)
	}
}

func TestLoadConfigRequiresValidSharedRuntimeHosts(t *testing.T) {
	for name, value := range map[string]string{
		"AGENT_ID":                     "",
		"AGENT_DOMAIN":                 "",
		"AGENT_OWNER_MAIN_USER_ID":     "",
		"SUB2API_SATELLITE":            "agentapi",
		"AGENTAPI_SSO_AUDIENCE":        "agentapi",
		"SESSION_SECRET":               strings.Repeat("s", 32),
		"SUB2API_SSO_SECRET":           strings.Repeat("o", 32),
		"SUB2API_APP_CREDENTIAL":       "shared-satellite-credential",
		"AGENT_INITIAL_BALANCE":        "",
		"AGENT_PAYMENT_ENABLED":        "false",
		"AGENT_PAYMENT_WEBHOOK_SECRET": "",
	} {
		t.Setenv(name, value)
	}
	t.Run("missing", func(t *testing.T) {
		t.Setenv("AGENTAPI_SHARED_HOSTS", "")
		if _, err := LoadConfig(); err == nil || !strings.Contains(err.Error(), "AGENTAPI_SHARED_HOSTS is required") {
			t.Fatalf("missing shared hosts error=%v, want rejection", err)
		}
	})
	t.Run("URL is rejected", func(t *testing.T) {
		t.Setenv("AGENTAPI_SHARED_HOSTS", "https://agent.example.test")
		if _, err := LoadConfig(); err == nil || !strings.Contains(err.Error(), "invalid host") {
			t.Fatalf("URL shared host error=%v, want rejection", err)
		}
	})
	t.Run("wildcard is rejected", func(t *testing.T) {
		t.Setenv("AGENTAPI_SHARED_HOSTS", "*.example.test")
		if _, err := LoadConfig(); err == nil || !strings.Contains(err.Error(), "invalid host") {
			t.Fatalf("wildcard shared host error=%v, want rejection", err)
		}
	})
}

func TestLoadConfigRejectsIncompleteSharedRuntimeSecrets(t *testing.T) {
	base := map[string]string{
		"AGENT_ID":                     "",
		"AGENT_DOMAIN":                 "",
		"AGENT_OWNER_MAIN_USER_ID":     "",
		"AGENTAPI_SHARED_HOSTS":        "agent.example.test",
		"SUB2API_SATELLITE":            "agentapi",
		"AGENTAPI_SSO_AUDIENCE":        "agentapi",
		"AGENT_INITIAL_BALANCE":        "",
		"AGENT_PAYMENT_ENABLED":        "false",
		"AGENT_PAYMENT_WEBHOOK_SECRET": "",
		"SESSION_SECRET_FILE":          "",
		"SUB2API_SSO_SECRET_FILE":      "",
		"SUB2API_APP_CREDENTIAL_FILE":  "",
	}
	for name, value := range base {
		t.Setenv(name, value)
	}

	tests := []struct {
		name    string
		app     string
		sso     string
		session string
		want    string
	}{
		{name: "missing app credential", sso: strings.Repeat("o", 32), session: strings.Repeat("s", 32), want: "SUB2API_APP_CREDENTIAL is required"},
		{name: "weak SSO secret", app: "app-credential", sso: "short", session: strings.Repeat("s", 32), want: "SUB2API_SSO_SECRET must contain at least 32"},
		{name: "weak session secret", app: "app-credential", sso: strings.Repeat("o", 32), session: "short", want: "SESSION_SECRET must contain at least 32"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("SUB2API_APP_CREDENTIAL", tc.app)
			t.Setenv("SUB2API_SSO_SECRET", tc.sso)
			t.Setenv("SESSION_SECRET", tc.session)
			if _, err := LoadConfig(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("LoadConfig error=%v, want %q", err, tc.want)
			}
		})
	}
}

func TestLoadConfigRejectsPartialCompatibilityTenant(t *testing.T) {
	for name, value := range map[string]string{
		"AGENT_ID":                 "",
		"AGENT_DOMAIN":             "agent.example.test",
		"AGENT_OWNER_MAIN_USER_ID": "",
		"SUB2API_SATELLITE":        "agentapi",
		"AGENTAPI_SSO_AUDIENCE":    "agentapi",
		"SESSION_SECRET":           strings.Repeat("s", 32),
		"AGENT_INITIAL_BALANCE":    "",
	} {
		t.Setenv(name, value)
	}
	if _, err := LoadConfig(); err == nil || !strings.Contains(err.Error(), "require an explicit AGENT_ID") {
		t.Fatalf("partial compatibility tenant error=%v, want rejection", err)
	}
}
