package satellite

import "testing"

func TestNormalizeOrigin(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"localhost:18080":      "http://localhost:18080",
		"http://localhost:3522": "http://localhost:3522",
		"https://api.cc2.cx":    "https://api.cc2.cx",
		"api.cc2.cx":            "https://api.cc2.cx",
		"canvas.cc2.cx/":        "https://canvas.cc2.cx",
	}
	for in, want := range cases {
		if got := NormalizeOrigin(in, ""); got != want {
			t.Fatalf("NormalizeOrigin(%q)=%q want %q", in, got, want)
		}
	}
}

func TestProjectOriginAndCallback(t *testing.T) {
	t.Setenv("canvas_link", "http://localhost:3522")
	if got := ProjectOrigin("canvas", "http://localhost:1"); got != "http://localhost:3522" {
		t.Fatalf("ProjectOrigin=%q", got)
	}
	t.Setenv("CANVAS_SSO_CALLBACK_URL", "")
	if got := CallbackURL("CANVAS_SSO_CALLBACK_URL", "canvas", "http://localhost:3522", "/api/auth/sso/callback"); got != "http://localhost:3522/api/auth/sso/callback" {
		t.Fatalf("CallbackURL=%q", got)
	}
	t.Setenv("canvas_link", "https://canvas.cc2.cx")
	if got := CallbackURL("CANVAS_SSO_CALLBACK_URL", "canvas", "http://localhost:3522", "/api/auth/sso/callback"); got != "https://canvas.cc2.cx/api/auth/sso/callback" {
		t.Fatalf("cloud CallbackURL=%q", got)
	}
}

func TestModelOriginFromLINK(t *testing.T) {
	t.Setenv("LINK", "localhost:18080")
	t.Setenv("SUB2API_RELAY_BASE_URL", "")
	t.Setenv("SUB2API_BASE", "")
	if got := ModelOrigin("https://api.cc2.cx"); got != "http://localhost:18080" {
		t.Fatalf("ModelOrigin=%q", got)
	}
}
