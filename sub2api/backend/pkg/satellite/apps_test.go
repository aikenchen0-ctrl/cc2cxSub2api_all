package satellite

import "testing"

func TestLookupKnownApps(t *testing.T) {
	t.Parallel()
	for _, slug := range []string{"canvas", "ju", "livart", "ppt", "aicut", "screen2code", "aiexcel", "qrcode", "yibiao"} {
		app, ok := Lookup(slug)
		if !ok || app.Slug != slug || app.CallbackPath == "" || app.Audience == "" {
			t.Fatalf("Lookup(%q) = %+v ok=%v", slug, app, ok)
		}
	}
	if _, ok := Lookup("does-not-exist"); ok {
		t.Fatal("unknown slug must fail")
	}
}
