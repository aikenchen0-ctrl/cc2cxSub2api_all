package main

import "testing"

func TestPublicRechargeOriginDoesNotFallBackToInternalMain(t *testing.T) {
	t.Setenv("MAIN_API_URL", "http://sub2api:8080")
	t.Setenv("LINK", "")
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
