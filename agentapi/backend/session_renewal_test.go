package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAutomaticCredentialRefreshPreservesLocalExpiry(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/me":
			if r.Header.Get("Authorization") == "Bearer old-access" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"code":401}`))
				return
			}
			if r.Header.Get("Authorization") != "Bearer rotated-access" {
				t.Error("unexpected credential")
			}
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 42, "email": "u@example.com"}))
		case "/api/v1/auth/refresh":
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"access_token": "rotated-access", "refresh_token": "rotated-refresh"}))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer upstream.Close()
	s := testServer(t, upstream)
	expires := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	id, err := s.store.CreateSession("42", []byte(`{"id":42}`), "old-access", "old-refresh", expires)
	if err != nil {
		t.Fatal(err)
	}
	session, err := s.store.LoadSession(id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.currentSessionUser(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	updated, err := s.store.LoadSession(id)
	if err != nil {
		t.Fatal(err)
	}
	if !updated.ExpiresAt.Equal(expires) || updated.AccessToken != "rotated-access" || updated.RefreshToken != "rotated-refresh" || updated.MainUserID != "42" {
		t.Fatal("automatic credential rotation changed local lifetime or failed to preserve identity")
	}
}

func TestExplicitRefreshRenewsCookieAndStoredSessionTogether(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/auth/refresh" {
			t.Errorf("unexpected upstream request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
			return
		}
		_ = json.NewEncoder(w).Encode(envelope(map[string]any{"access_token": "rotated-access", "refresh_token": "rotated-refresh", "user": map[string]any{"id": 42, "email": "u@example.com"}}))
	}))
	defer upstream.Close()
	s := testServer(t, upstream)
	s.cfg.CookieSecure = true
	id, err := s.store.CreateSession("42", []byte(`{"id":42}`), "old-access", "old-refresh", time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "https://agent.local/api/v1/auth/refresh", nil)
	req.AddCookie(&http.Cookie{Name: s.cfg.CookieName, Value: id})
	rec := httptest.NewRecorder()
	before := time.Now().UTC()
	s.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("refresh failed: %d %s", rec.Code, rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected renewed cookie, got %d", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != s.cfg.CookieName || cookie.Value != id || cookie.MaxAge != 259200 || !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" {
		t.Fatal("renewed cookie does not preserve session security and three-day lifetime")
	}
	stored, err := s.store.LoadSession(id)
	if err != nil {
		t.Fatal(err)
	}
	if !stored.ExpiresAt.Equal(cookie.Expires) || stored.ExpiresAt.Before(before.Add(sessionTTL-time.Second)) || stored.ExpiresAt.After(time.Now().Add(sessionTTL)) {
		t.Fatal("cookie and database expiry do not match the three-day renewal")
	}
	if stored.MainUserID != "42" || stored.AccessToken != "rotated-access" || stored.RefreshToken != "rotated-refresh" {
		t.Fatal("renewal lost bound identity or rotated credentials")
	}
	if strings.Contains(rec.Body.String(), "rotated-") {
		t.Fatal("upstream credentials leaked")
	}
}
