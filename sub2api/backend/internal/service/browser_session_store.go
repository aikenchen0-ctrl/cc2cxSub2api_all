package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"time"
)

// ErrBrowserSessionNotFound is returned when an opaque browser session has
// expired or has already been removed.
var ErrBrowserSessionNotFound = errors.New("browser session not found")

// BrowserSessionData is kept by the server. The browser only receives the
// random session id, never the access JWT stored here.
type BrowserSessionData struct {
	AccessToken string    `json:"access_token"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// BrowserSessionStore persists opaque browser sessions. Implementations must
// store only a digest of the session id as the lookup key.
type BrowserSessionStore interface {
	StoreBrowserSession(ctx context.Context, sessionID string, data *BrowserSessionData, ttl time.Duration) error
	GetBrowserSession(ctx context.Context, sessionID string) (*BrowserSessionData, error)
	DeleteBrowserSession(ctx context.Context, sessionID string) error
}

func randomBrowserSessionID() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
