package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

type browserSessionStoreSpy struct {
	ttl  time.Duration
	data *BrowserSessionData
}

func (s *browserSessionStoreSpy) StoreBrowserSession(_ context.Context, _ string, data *BrowserSessionData, ttl time.Duration) error {
	s.data = data
	s.ttl = ttl
	return nil
}

func (s *browserSessionStoreSpy) GetBrowserSession(context.Context, string) (*BrowserSessionData, error) {
	return s.data, nil
}

func (s *browserSessionStoreSpy) DeleteBrowserSession(context.Context, string) error { return nil }

func TestIssueBrowserSessionUsesThreeDayTTL(t *testing.T) {
	store := &browserSessionStoreSpy{}
	svc := &AuthService{browserSessionStore: store}

	_, ttl, err := svc.IssueBrowserSession(context.Background(), "access-jwt", time.Hour)
	if err != nil {
		t.Fatalf("IssueBrowserSession() error = %v", err)
	}
	if ttl != BrowserSessionTTL || store.ttl != BrowserSessionTTL {
		t.Fatalf("browser session TTL = %s (stored %s), want %s", ttl, store.ttl, BrowserSessionTTL)
	}
	if store.data == nil || store.data.ExpiresAt.Sub(store.data.CreatedAt) != BrowserSessionTTL {
		t.Fatalf("stored session expiry = %+v, want a three-day lifetime", store.data)
	}
}

func TestIssueBrowserSessionResignsServerJWTToThreeDayTTL(t *testing.T) {
	store := &browserSessionStoreSpy{}
	svc := &AuthService{
		browserSessionStore: store,
		cfg:                 &config.Config{JWT: config.JWTConfig{Secret: "browser-session-test-secret", ExpireHour: 24}},
	}
	original, err := svc.GenerateToken(context.Background(), &User{ID: 7, Email: "user@example.com", Role: RoleUser})
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}
	if _, _, err := svc.IssueBrowserSession(context.Background(), original, time.Hour); err != nil {
		t.Fatalf("IssueBrowserSession() error = %v", err)
	}
	if store.data == nil || store.data.AccessToken == original {
		t.Fatal("browser session should keep a separately signed server-side JWT")
	}
	claims, err := svc.ValidateToken(store.data.AccessToken)
	if err != nil {
		t.Fatalf("ValidateToken(browser session JWT) error = %v", err)
	}
	if claims.ExpiresAt == nil || claims.IssuedAt == nil || claims.ExpiresAt.Time.Sub(claims.IssuedAt.Time) != BrowserSessionTTL {
		t.Fatalf("browser session JWT lifetime = %v, want %v", claims.ExpiresAt, BrowserSessionTTL)
	}
}
