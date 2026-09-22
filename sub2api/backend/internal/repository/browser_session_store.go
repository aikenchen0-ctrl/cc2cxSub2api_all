package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const browserSessionKeyPrefix = "browser_session:"

type browserSessionStore struct {
	rdb *redis.Client
}

// NewBrowserSessionStore creates the Redis-backed opaque browser session
// store used by the panel's top-level SSO navigation.
func NewBrowserSessionStore(rdb *redis.Client) service.BrowserSessionStore {
	return &browserSessionStore{rdb: rdb}
}

func browserSessionKey(sessionID string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(sessionID)))
	return browserSessionKeyPrefix + hex.EncodeToString(sum[:])
}

func (s *browserSessionStore) StoreBrowserSession(ctx context.Context, sessionID string, data *service.BrowserSessionData, ttl time.Duration) error {
	if s == nil || s.rdb == nil {
		return fmt.Errorf("browser session store is not configured")
	}
	if strings.TrimSpace(sessionID) == "" || data == nil || strings.TrimSpace(data.AccessToken) == "" {
		return fmt.Errorf("browser session data is invalid")
	}
	if ttl <= 0 {
		return fmt.Errorf("browser session ttl must be positive")
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal browser session: %w", err)
	}
	if err := s.rdb.Set(ctx, browserSessionKey(sessionID), raw, ttl).Err(); err != nil {
		return fmt.Errorf("store browser session: %w", err)
	}
	return nil
}

func (s *browserSessionStore) GetBrowserSession(ctx context.Context, sessionID string) (*service.BrowserSessionData, error) {
	if s == nil || s.rdb == nil {
		return nil, fmt.Errorf("browser session store is not configured")
	}
	if strings.TrimSpace(sessionID) == "" {
		return nil, service.ErrBrowserSessionNotFound
	}
	raw, err := s.rdb.Get(ctx, browserSessionKey(sessionID)).Bytes()
	if err == redis.Nil {
		return nil, service.ErrBrowserSessionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get browser session: %w", err)
	}
	var data service.BrowserSessionData
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("unmarshal browser session: %w", err)
	}
	if strings.TrimSpace(data.AccessToken) == "" {
		return nil, service.ErrBrowserSessionNotFound
	}
	return &data, nil
}

func (s *browserSessionStore) DeleteBrowserSession(ctx context.Context, sessionID string) error {
	if s == nil || s.rdb == nil || strings.TrimSpace(sessionID) == "" {
		return nil
	}
	return s.rdb.Del(ctx, browserSessionKey(sessionID)).Err()
}
