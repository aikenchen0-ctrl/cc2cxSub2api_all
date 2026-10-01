package main

import (
	"strings"
	"testing"
)

func TestPublicRechargeOriginDoesNotFallBackToInternalMain(t *testing.T) {
	t.Setenv("MAIN_API_URL", "http://sub2api:8080")
	t.Setenv("LINK", "")
	t.Setenv("AGENTAPI_SHARED_HOSTS", "agent.example.test")
	t.Setenv("SUB2API_APP_CREDENTIAL", "app-secret")
	t.Setenv("SUB2API_SSO_SECRET", strings.Repeat("s", 32))
	t.Setenv("SESSION_SECRET", strings.Repeat("t", 32))
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PublicMainURL != "" {
		t.Fatalf("internal/public fallback leaked: %q", cfg.PublicMainURL)
	}
	t.Setenv("LINK", "https://public.example.com")
	cfg, err = LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PublicMainURL != "https://public.example.com" {
		t.Fatalf("wrong public link: %q", cfg.PublicMainURL)
	}
}
