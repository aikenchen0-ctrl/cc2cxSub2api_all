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

	"infinite-canvas/backend/internal/kernel"
	"infinite-canvas/backend/internal/model"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
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
	return verifySub2APITicket(raw, secret, now, client)
}

func verifySub2APITicket(raw, secret string, now time.Time, client *redis.Client) (Sub2APISSOPayload, error) {
	var zero Sub2APISSOPayload
	secret = strings.TrimSpace(secret)
	if len(secret) < 32 {
		return zero, errors.New("sub2api sso secret is not configured")
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
	if err != nil || json.Unmarshal(payloadBytes, &zero) != nil {
		return zero, errors.New("invalid sso ticket payload")
	}
	if strings.TrimSpace(zero.Subject) == "" || strings.TrimSpace(zero.Nonce) == "" || zero.ExpiresAt <= now.Unix() || zero.IssuedAt > now.Add(30*time.Second).Unix() {
		return zero, errors.New("expired sso ticket")
	}
	if client != nil {
		keyHash := sha256.Sum256([]byte(zero.Nonce))
		key := fmt.Sprintf("canvas:sso:sub2api:%x", keyHash[:])
		ttl := time.Until(time.Unix(zero.ExpiresAt, 0))
		if ttl <= 0 {
			return zero, errors.New("expired sso ticket")
		}
		ok, err := client.SetNX(context.Background(), key, "1", ttl).Result()
		if err != nil {
			return zero, fmt.Errorf("sso replay store unavailable: %w", err)
		}
		if !ok {
			return zero, errors.New("sso ticket already used")
		}
		return zero, nil
	}
	ssoConsumedMu.Lock()
	defer ssoConsumedMu.Unlock()
	for nonce, expires := range ssoConsumed {
		if now.After(expires) {
			delete(ssoConsumed, nonce)
		}
	}
	if _, exists := ssoConsumed[zero.Nonce]; exists {
		return zero, errors.New("sso ticket already used")
	}
	ssoConsumed[zero.Nonce] = time.Unix(zero.ExpiresAt, 0)
	return zero, nil
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
		return nil, kernel.Forbidden("璇ヨ处鍙峰凡琚鐢?")
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
	identity := &model.UserIdentity{ID: kernel.NewID(), UserID: user.ID, Provider: "sub2api", Subject: payload.Subject, ProviderUsername: payload.Username, AvatarURL: payload.AvatarURL, CreatedAt: now, UpdatedAt: now}
	return user, identity, nil
}

func Sub2APISSOSecret() string { return os.Getenv("SUB2API_SSO_SECRET") }

func safeSSONext(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") || strings.Contains(value, "\\") || strings.ContainsAny(value, "\r\n") {
		return "/projects"
	}
	return value
}

func SSONext(payload Sub2APISSOPayload) string { return safeSSONext(payload.Next) }

func (p Sub2APISSOPayload) String() string { return fmt.Sprintf("sub2api:%s", p.Subject) }
