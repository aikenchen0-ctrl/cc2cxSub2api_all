package main

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestProvisioningControlDefaultsToManagedAgentIDs(t *testing.T) {
	t.Setenv("SESSION_SECRET", strings.Repeat("s", 32))
	t.Setenv("SESSION_SECRET_FILE", "")
	t.Setenv("AGENT_INITIAL_BALANCE", "")
	t.Setenv("AGENT_BILLING_MODE", "owner_upstream")
	t.Setenv("AGENT_PAYMENT_ENABLED", "false")
	t.Setenv("AGENT_PROVISIONING_CONTROL_STALE_AFTER", "")
	t.Setenv("AGENT_RUNTIME_CONTROL_CREDENTIAL", "agt_ctl_test-runtime-control-secret-0123456789abcdef")
	t.Setenv("AGENT_RUNTIME_CONTROL_CREDENTIAL_FILE", "")
	t.Setenv("SUB2API_APP_CREDENTIAL", "agt_model_test-model-relay-secret-0123456789abcdef")
	t.Setenv("SUB2API_APP_CREDENTIAL_FILE", "")
	t.Setenv("AGENT_DOMAIN", "")
	t.Setenv("SUB2API_SATELLITE", "agentapi")
	t.Setenv("AGENTAPI_SSO_AUDIENCE", "agentapi")

	for _, test := range []struct {
		name              string
		agentID           string
		domain            string
		override          string
		want              bool
		wantErrorContains string
	}{
		{name: "managed id", agentID: "agt_0123456789abcdef0123456789abcdef", domain: "agent.example.com", want: true},
		{name: "local id", agentID: "agent-local", want: false},
		{name: "explicit enable", agentID: "agent-local", override: "true", want: true},
		{name: "managed id requires host binding", agentID: "agt_0123456789abcdef0123456789abcdef", wantErrorContains: "AGENT_DOMAIN is required"},
		{name: "managed id rejects malformed host binding", agentID: "agt_0123456789abcdef0123456789abcdef", domain: "agent.example.com/path", wantErrorContains: "valid DNS hostname"},
		{name: "managed id cannot disable control", agentID: "agt_0123456789abcdef0123456789abcdef", domain: "agent.example.com", override: "false", wantErrorContains: "cannot be disabled for a managed Agent ID"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("AGENT_ID", test.agentID)
			t.Setenv("AGENT_DOMAIN", test.domain)
			t.Setenv("AGENT_PROVISIONING_CONTROL_ENABLED", test.override)
			cfg, err := LoadConfig()
			if test.wantErrorContains != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErrorContains) {
					t.Fatalf("managed Agent config error = %v, want %q", err, test.wantErrorContains)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.ProvisioningControlEnabled != test.want || cfg.ProvisioningControlStaleAfter != 90*time.Second {
				t.Fatalf("managed control config = %v / %s, want %v / 90s", cfg.ProvisioningControlEnabled, cfg.ProvisioningControlStaleAfter, test.want)
			}
		})
	}
}

func TestLoadConfigSeparatesControlAndModelRelayBases(t *testing.T) {
	for name, value := range map[string]string{
		"AGENT_ID":                              "agent-local",
		"AGENT_PROVISIONING_CONTROL_ENABLED":    "false",
		"AGENT_RUNTIME_CONTROL_CREDENTIAL":      "",
		"AGENT_RUNTIME_CONTROL_CREDENTIAL_FILE": "",
		"SUB2API_APP_CREDENTIAL":                "",
		"SUB2API_APP_CREDENTIAL_FILE":           "",
		"SESSION_SECRET":                        strings.Repeat("s", 32),
		"SESSION_SECRET_FILE":                   "",
		"AGENT_BILLING_MODE":                    "owner_upstream",
		"AGENT_INITIAL_BALANCE":                 "",
		"AGENT_PAYMENT_ENABLED":                 "false",
		"AGENT_PAYMENT_WEBHOOK_SECRET":          "",
		"AGENT_PAYMENT_WEBHOOK_SECRET_FILE":     "",
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
