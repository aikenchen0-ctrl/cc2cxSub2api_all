package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func signSub2APITicket(payload Sub2APISSOPayload, secret string) (string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(body)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(encoded))
	return encoded + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func TestVerifySub2APITicketAcceptsAndConsumesTicket(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	secret := strings.Repeat("s", 32)
	raw, err := signSub2APITicket(Sub2APISSOPayload{
		Subject: "sub-1", IssuedAt: now.Unix(), ExpiresAt: now.Add(time.Minute).Unix(), Nonce: "nonce-valid",
	}, secret)
	if err != nil {
		t.Fatal(err)
	}
	if payload, err := VerifySub2APITicket(raw, secret, now); err != nil || payload.Subject != "sub-1" {
		t.Fatalf("verify valid ticket: payload=%+v err=%v", payload, err)
	}
	if _, err := VerifySub2APITicket(raw, secret, now); err == nil || !strings.Contains(err.Error(), "already used") {
		t.Fatalf("expected replay rejection, got %v", err)
	}
}

func TestVerifySub2APITicketRejectsExpiredAndInvalidTickets(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	secret := strings.Repeat("s", 32)
	expired, err := signSub2APITicket(Sub2APISSOPayload{
		Subject: "sub-expired", IssuedAt: now.Add(-time.Minute).Unix(), ExpiresAt: now.Unix(), Nonce: "nonce-expired",
	}, secret)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifySub2APITicket(expired, secret, now); err == nil {
		t.Fatal("expected expired ticket rejection")
	}
	if _, err := VerifySub2APITicket("bad.ticket", secret, now); err == nil {
		t.Fatal("expected invalid signature rejection")
	}
	if _, err := VerifySub2APITicket(expired, strings.Repeat("x", 32), now); err == nil {
		t.Fatal("expected wrong-secret rejection")
	}
}

func TestSSONextRejectsExternalRedirects(t *testing.T) {
	if got := SSONext(Sub2APISSOPayload{Next: "https://evil.example"}); got != "/projects" {
		t.Fatalf("external next was accepted: %q", got)
	}
	if got := SSONext(Sub2APISSOPayload{Next: "//evil.example"}); got != "/projects" {
		t.Fatalf("protocol-relative next was accepted: %q", got)
	}
	if got := SSONext(Sub2APISSOPayload{Next: `/\evil.example`}); got != "/projects" {
		t.Fatalf("backslash redirect was accepted: %q", got)
	}
	if got := SSONext(Sub2APISSOPayload{Next: "/projects?tab=recent"}); got != "/projects?tab=recent" {
		t.Fatalf("valid next changed: %q", got)
	}
}
