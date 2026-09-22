package satellite

import (
	"testing"
	"time"
)

func TestLookupKnownApps(t *testing.T) {
	t.Parallel()
	for _, slug := range []string{"canvas", "ju", "livart", "ppt", "aicut", "screen2code", "aiexcel", "qrcode", "yibiao", "ai3d", "aihuoke"} {
		app, ok := Lookup(slug)
		if !ok || app.Slug != slug || app.CallbackPath == "" || app.Audience == "" {
			t.Fatalf("Lookup(%q) = %+v ok=%v", slug, app, ok)
		}
	}
	if _, ok := Lookup("does-not-exist"); ok {
		t.Fatal("unknown slug must fail")
	}
}

func TestNewSatelliteDefaultsUseDedicatedLocalPorts(t *testing.T) {
	t.Parallel()
	want := map[string]string{
		"yibiao":  "http://localhost:8081",
		"ai3d":    "http://localhost:5174",
		"aihuoke": "http://localhost:3001",
	}
	for slug, origin := range want {
		app, ok := Lookup(slug)
		if !ok {
			t.Fatalf("Lookup(%q) failed", slug)
		}
		if app.DefaultOrigin != origin {
			t.Fatalf("%s default origin = %q, want %q", slug, app.DefaultOrigin, origin)
		}
	}
}

func TestAppsUseShortLivedTickets(t *testing.T) {
	t.Parallel()
	for _, app := range Apps() {
		if app.TicketTTL <= 0 || app.TicketTTL > 2*time.Minute {
			t.Fatalf("%s ticket TTL = %s, want at most two minutes", app.Slug, app.TicketTTL)
		}
	}
}
