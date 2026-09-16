package handler

import "testing"

func TestSafeJuNextRejectsExternalAndBackslashRedirects(t *testing.T) {
	for _, value := range []string{"https://evil.example", "//evil.example", `/\\evil.example`, "\r\nLocation: https://evil.example"} {
		if got := safeJuNext(value); got != "/projects" {
			t.Fatalf("unsafe next %q accepted as %q", value, got)
		}
	}
	if got := safeJuNext("/projects?tab=recent"); got != "/projects?tab=recent" {
		t.Fatalf("valid next changed to %q", got)
	}
}
