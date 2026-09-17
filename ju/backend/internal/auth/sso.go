package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"infinite-canvas/backend/internal/kernel"
	"infinite-canvas/backend/internal/model"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

const (
	ssoTicketMaxBytes    = 8 * 1024
	ssoTicketMaxLifetime = 2 * time.Minute
	ssoTicketClockSkew   = 30 * time.Second
	ssoReplayTimeout     = 2 * time.Second
)

// Sub2APISSOPayload is the minimal identity assertion exchanged between the
// two applications. It never contains a JWT or an API key.
type Sub2APISSOPayload struct {
	Subject     string `json:"sub"`
	Email       string `json:"email,omitempty"`
	Username    string `json:"username,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
	AvatarURL   string `json:"avatarUrl,omitempty"`
	IssuedAt    int64  `json:"iat"`
	ExpiresAt   int64  `json:"exp"`
	Nonce       string `json:"jti"`
	Next        string `json:"next,omitempty"`
}

var (
	ssoConsumedMu sync.Mutex
	ssoConsumed   = map[string]time.Time{}
	ssoUsername   = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)
)

// VerifySub2APITicket validates a short-lived HMAC ticket and consumes its
// nonce. Consumption is process-local to avoid adding a database table; use a
// shared Redis-backed one-time store when running multiple replicas.
func VerifySub2APITicket(raw, secret string, now time.Time) (Sub2APISSOPayload, error) {
	return verifySub2APITicket(raw, secret, now, nil)
}

// VerifySub2APITicketWithRedis uses Redis as the shared one-time ticket store.
// It keeps replay protection valid across restarts and multiple ju replicas.
func VerifySub2APITicketWithRedis(raw, secret string, now time.Time, client *redis.Client) (Sub2APISSOPayload, error) {
	if client == nil {
		return Sub2APISSOPayload{}, errors.New("sso replay store is not configured")
	}
	return verifySub2APITicket(raw, secret, now, client)
}

func verifySub2APITicket(raw, secret string, now time.Time, client *redis.Client) (Sub2APISSOPayload, error) {
	var zero Sub2APISSOPayload
	secret = strings.TrimSpace(secret)
	if len(secret) < 32 {
		return zero, errors.New("sub2api sso secret is not configured")
	}
	if len(raw) > ssoTicketMaxBytes {
		return zero, errors.New("sso ticket is too long")
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 2 || len(parts[0]) == 0 || len(parts[1]) == 0 {
		return zero, errors.New("invalid sso ticket")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(parts[0]))
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(signature, mac.Sum(nil)) {
		return zero, errors.New("invalid sso ticket signature")
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	var payload Sub2APISSOPayload
	if err != nil || json.Unmarshal(payloadBytes, &payload) != nil {
		return zero, errors.New("invalid sso ticket payload")
	}
	if err := validateSub2APISSOPayload(payload); err != nil {
		return zero, err
	}
	if payload.IssuedAt <= 0 || payload.ExpiresAt <= payload.IssuedAt || payload.ExpiresAt-payload.IssuedAt > int64(ssoTicketMaxLifetime/time.Second) || payload.ExpiresAt <= now.Unix() || payload.IssuedAt > now.Add(ssoTicketClockSkew).Unix() || payload.ExpiresAt > now.Add(ssoTicketMaxLifetime+ssoTicketClockSkew).Unix() {
		return zero, errors.New("expired sso ticket")
	}
	payload.Subject, payload.Nonce = strings.TrimSpace(payload.Subject), strings.TrimSpace(payload.Nonce)
	if client != nil {
		keyHash := sha256.Sum256([]byte(payload.Nonce))
		key := fmt.Sprintf("canvas:sso:sub2api:%x", keyHash[:])
		ttl := time.Unix(payload.ExpiresAt, 0).Sub(now)
		if ttl <= 0 {
			return zero, errors.New("expired sso ticket")
		}
		ctx, cancel := context.WithTimeout(context.Background(), ssoReplayTimeout)
		defer cancel()
		// Bound socket I/O too: callers may use go-redis's default options,
		// where ContextTimeoutEnabled is false. The clone shares the pool.
		ok, err := client.WithTimeout(ssoReplayTimeout).SetNX(ctx, key, "1", ttl).Result()
		if err != nil {
			return zero, fmt.Errorf("sso replay store unavailable: %w", err)
		}
		if !ok {
			return zero, errors.New("sso ticket already used")
		}
		return payload, nil
	}
	ssoConsumedMu.Lock()
	defer ssoConsumedMu.Unlock()
	for nonce, expires := range ssoConsumed {
		if !now.Before(expires) {
			delete(ssoConsumed, nonce)
		}
	}
	if _, exists := ssoConsumed[payload.Nonce]; exists {
		return zero, errors.New("sso ticket already used")
	}
	ssoConsumed[payload.Nonce] = time.Unix(payload.ExpiresAt, 0)
	return payload, nil
}

func validateSub2APISSOPayload(payload Sub2APISSOPayload) error {
	if strings.TrimSpace(payload.Subject) == "" || strings.TrimSpace(payload.Nonce) == "" {
		return errors.New("invalid sso identity or nonce")
	}
	for _, field := range []struct {
		value string
		limit int
	}{
		{payload.Subject, 160}, {payload.Nonce, 160}, {payload.Email, 160},
		{payload.Username, 160}, {payload.DisplayName, 160},
		{payload.AvatarURL, 2048}, {payload.Next, 2048},
	} {
		if utf8.RuneCountInString(field.value) > field.limit || strings.IndexFunc(field.value, unicode.IsControl) >= 0 {
			return errors.New("invalid sso identity field")
		}
	}
	return nil
}

// CompleteSub2APISSO maps the external identity to existing identity tables,
// provisions a normal user on first use, and creates a regular canvas session.
func (s *Service) CompleteSub2APISSO(payload Sub2APISSOPayload) (*AuthSessionResult, error) {
	provider, subject := "sub2api", strings.TrimSpace(payload.Subject)
	identity, err := s.repo.UserIdentity(provider, subject)
	var user *model.User
	if err == nil {
		user, err = s.repo.User(identity.UserID)
		if err != nil {
			return nil, err
		}
		identity.ProviderUsername = strings.TrimSpace(payload.Username)
		identity.AvatarURL = strings.TrimSpace(payload.AvatarURL)
		identity.UpdatedAt = time.Now()
		if err := s.repo.Save(identity); err != nil {
			return nil, err
		}
	} else if errors.Is(err, gorm.ErrRecordNotFound) {
		user, identity, err = s.createSub2APIUser(payload)
		if err != nil {
			return nil, err
		}
		if err := s.repo.CreateOAuthUser(user, identity); err != nil {
			return nil, err
		}
	} else {
		return nil, err
	}
	if user.Status != model.UserStatusActive {
		return nil, kernel.Forbidden("该账号已被禁用")
	}
	if err := s.host.EnsureSignupBonus(user.ID); err != nil {
		return nil, err
	}
	now := time.Now()
	user.LastLoginAt, user.UpdatedAt = &now, now
	if err := s.repo.Save(user); err != nil {
		return nil, err
	}
	s.host.RecordActivity(user.ID, "login", 1)
	return s.createAuthSession(user)
}

func (s *Service) createSub2APIUser(payload Sub2APISSOPayload) (*model.User, *model.UserIdentity, error) {
	base := ssoUsername.ReplaceAllString(strings.TrimSpace(payload.Username), "_")
	base = strings.Trim(base, "_-")
	if len(base) < 3 {
		base = "sub2api_" + shortSubject(payload.Subject)
	}
	if len(base) > 24 {
		base = base[:24]
	}
	username := base
	if existing, err := s.repo.UserByUsername(username); err == nil && existing != nil {
		username = kernel.TruncateRunes(base, 23) + "_" + shortSubject(payload.Subject)
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, err
	}
	email := NormalizeEmail(payload.Email)
	if email != "" && ValidateEmail(email) != nil {
		email = ""
	}
	if email != "" {
		if existing, err := s.repo.UserByEmail(email); err == nil && existing != nil {
			email = ""
		} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, err
		}
	}
	passwordHash, err := HashPassword(RandomToken())
	if err != nil {
		return nil, nil, err
	}
	now := time.Now()
	user := &model.User{ID: kernel.NewID(), Username: username, Email: email, DisplayName: NormalizeDisplayName(payload.DisplayName, username), Role: model.UserRoleUser, Status: model.UserStatusActive, PasswordHash: passwordHash, CreatedAt: now, UpdatedAt: now}
	identity := &model.UserIdentity{ID: kernel.NewID(), UserID: user.ID, Provider: "sub2api", Subject: strings.TrimSpace(payload.Subject), ProviderUsername: strings.TrimSpace(payload.Username), AvatarURL: strings.TrimSpace(payload.AvatarURL), CreatedAt: now, UpdatedAt: now}
	return user, identity, nil
}

func Sub2APISSOSecret() string { return os.Getenv("SUB2API_SSO_SECRET") }

func safeSSONext(value string) string {
	if len(value) > 2048 || strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return "/projects"
	}
	value = strings.TrimSpace(value)
	if value == "" || !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") || strings.Contains(value, "\\") {
		return "/projects"
	}
	return value
}

func SSONext(payload Sub2APISSOPayload) string { return safeSSONext(payload.Next) }

func (p Sub2APISSOPayload) String() string { return fmt.Sprintf("sub2api:%s", p.Subject) }
