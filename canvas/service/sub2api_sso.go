package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/tigerowo/infinite-canvas/model"
	"github.com/tigerowo/infinite-canvas/repository"
)

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

func VerifySub2APITicket(raw, secret string, at time.Time) (Sub2APISSOPayload, error) {
	var payload Sub2APISSOPayload
	secret = strings.TrimSpace(secret)
	if len(secret) < 32 || len(raw) > 8192 {
		return payload, errors.New("SSO is disabled or ticket is invalid")
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return payload, errors.New("invalid SSO ticket")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(parts[0]))
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(signature, mac.Sum(nil)) {
		return payload, errors.New("invalid SSO signature")
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || json.Unmarshal(body, &payload) != nil {
		return Sub2APISSOPayload{}, errors.New("invalid SSO payload")
	}
	if err := validateSSOPayload(payload); err != nil {
		return Sub2APISSOPayload{}, err
	}
	if payload.IssuedAt <= 0 || payload.ExpiresAt <= payload.IssuedAt || payload.ExpiresAt <= at.Unix() || payload.IssuedAt > at.Add(30*time.Second).Unix() || payload.ExpiresAt > at.Add(150*time.Second).Unix() || payload.ExpiresAt-payload.IssuedAt > 120 {
		return Sub2APISSOPayload{}, errors.New("expired SSO ticket")
	}
	payload.Subject, payload.Nonce = strings.TrimSpace(payload.Subject), strings.TrimSpace(payload.Nonce)
	digest := sha256.Sum256([]byte(payload.Nonce))
	consumed, err := repository.ConsumeSSOTicket(hex.EncodeToString(digest[:]), payload.ExpiresAt, at.Unix())
	if err != nil {
		return Sub2APISSOPayload{}, err
	}
	if !consumed {
		return Sub2APISSOPayload{}, errors.New("SSO ticket already used")
	}
	return payload, nil
}

func validateSSOPayload(payload Sub2APISSOPayload) error {
	if strings.TrimSpace(payload.Subject) == "" || strings.TrimSpace(payload.Nonce) == "" {
		return errors.New("missing SSO identity")
	}
	for _, field := range []struct {
		value string
		limit int
	}{
		{payload.Subject, 160}, {payload.Nonce, 160}, {payload.Username, 160}, {payload.Email, 160}, {payload.DisplayName, 160}, {payload.AvatarURL, 2048}, {payload.Next, 2048},
	} {
		if utf8.RuneCountInString(field.value) > field.limit || strings.IndexFunc(field.value, unicode.IsControl) >= 0 {
			return errors.New("invalid SSO identity field")
		}
	}
	return nil
}

func CompleteSub2APISSO(payload Sub2APISSOPayload) (model.AuthSession, error) {
	if err := validateSSOPayload(payload); err != nil {
		return model.AuthSession{}, err
	}
	subject := strings.TrimSpace(payload.Subject)
	digest := sha256.Sum256([]byte(subject))
	id := "sub2api-" + hex.EncodeToString(digest[:])
	user, exists, err := repository.GetUserByID(id)
	if err != nil {
		return model.AuthSession{}, err
	}
	if !exists {
		password, err := hashPassword(uuid.NewString())
		if err != nil {
			return model.AuthSession{}, err
		}
		user, err = repository.FindOrCreateSub2APIUser(model.User{
			ID: id, Username: id, Password: password, Sub2APISubject: subject,
			Email: payload.Email, DisplayName: firstNonEmpty(payload.DisplayName, payload.Username, "Sub2API"),
			Role: model.UserRoleUser, Status: model.UserStatusActive, AffCode: newAffCode(), CreatedAt: now(), UpdatedAt: now(),
		})
		if err != nil {
			return model.AuthSession{}, err
		}
	}
	if user.Sub2APISubject != subject || user.Status != model.UserStatusActive {
		return model.AuthSession{}, errors.New("SSO account unavailable")
	}
	if err := repository.RecordSub2APILogin(user.ID, now()); err != nil {
		return model.AuthSession{}, err
	}
	return newSession(user)
}

func SSONext(value string) string {
	if len(value) > 2048 || strings.IndexFunc(value, unicode.IsControl) >= 0 || !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") || strings.Contains(value, "\\") {
		return "/"
	}
	return value
}
