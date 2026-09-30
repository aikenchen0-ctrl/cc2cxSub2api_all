package service

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSessionCookieOriginAndTTL(t *testing.T) {
	r := httptest.NewRequest("POST", "http://127.0.0.1/api", nil)
	r.Header.Set("X-Forwarded-Host", "canvas.cc2.cx")
	r.Header.Set("X-Forwarded-Proto", "https")
	r.Header.Set("Origin", "https://canvas.cc2.cx")
	if !SameOriginSessionRequest(r) { t.Fatal("trusted proxy origin rejected") }
	w := httptest.NewRecorder()
	SetSessionCookie(w, r, "test-session", 259200)
	c := w.Result().Cookies()[0]
	if !c.Secure || !c.HttpOnly || c.MaxAge != 259200 || c.SameSite != http.SameSiteLaxMode { t.Fatal("invalid session cookie") }
	r.Header.Set("Origin", "https://another.cc2.cx")
	if SameOriginSessionRequest(r) { t.Fatal("cross-origin sibling admitted") }
}
