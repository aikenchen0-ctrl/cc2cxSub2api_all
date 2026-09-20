package handler

import (
	"strings"
	"testing"
)

func TestSafeJuNextRejectsExternalAndBackslashRedirects(t *testing.T) {
	for _, value := range []string{"https://evil.example", "//evil.example", `/\\evil.example`, "\r\nLocation: https://evil.example", "/\t/evil.example", "/projects\x00", "/" + strings.Repeat("a", 2048)} {
		if got := safeJuNext(value); got != "/projects" {
			t.Fatalf("unsafe next %q accepted as %q", value, got)
		}
	}
	if got := safeJuNext("/projects?tab=recent"); got != "/projects?tab=recent" {
		t.Fatalf("valid next changed to %q", got)
	}
}

func TestJuSSOCallbackValidation(t *testing.T) {
	for _, raw := range []string{"", "javascript:alert(1)", "https://user:password@ju.example/api/auth/sso/callback", "https://ju.example/api/auth/sso/callback#x", "https://ju.example/api/auth/sso/callback?token=x", "https://ju.example/other", "http://ju.example/api/auth/sso/callback", "https:///api/auth/sso/callback"} {
		if _, err := parseJuCallback(raw); err == nil {
			t.Errorf("accepted invalid callback %q", raw)
		}
	}
	for _, raw := range []string{"https://ju.example/api/auth/sso/callback", "http://localhost:3000/api/auth/sso/callback", "http://127.0.0.1:3517/api/auth/sso/callback", "http://[::1]:3000/api/auth/sso/callback"} {
		if _, err := parseJuCallback(raw); err != nil {
			t.Errorf("rejected valid callback: %v", err)
		}
	}
}
