package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"infinite-canvas/backend/internal/kernel"
	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/repository"

	"github.com/redis/go-redis/v9"
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
		Issuer: "sub2api", Audience: "ju",
		Subject: "sub-1", IssuedAt: now.Unix(), ExpiresAt: now.Add(time.Minute).Unix(), Nonce: kernel.NewID(),
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
		Issuer: "sub2api", Audience: "ju",
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
	for _, path := range []string{"/\t/evil.example", "/projects\x00", "/" + strings.Repeat("x", 2048)} {
		if got := SSONext(Sub2APISSOPayload{Next: path}); got != "/projects" {
			t.Fatalf("unsafe next was accepted: %q", got)
		}
	}
}

func TestVerifySub2APITicketLifetimeBounds(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	secret := strings.Repeat("s", 32)
	maxLifetime := int64((3 * 24 * time.Hour).Seconds())
	for _, test := range []struct {
		name            string
		issued, expires int64
		accept          bool
	}{
		{"maximum lifetime", 0, maxLifetime, true},
		{"maximum future skew", 30, maxLifetime + 30, true},
		{"just before expiry", -(maxLifetime - 1), 1, true},
		{"expired", -maxLifetime, 0, false},
		{"too long", 0, maxLifetime + 1, false},
		{"too long with skew", 30, maxLifetime + 31, false},
		{"future issue", 31, maxLifetime, false},
		{"zero lifetime", 10, 10, false},
		{"negative lifetime", 10, 9, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			payload := Sub2APISSOPayload{Issuer: "sub2api", Audience: "ju", Subject: "user-1", Nonce: kernel.NewID(), IssuedAt: now.Unix() + test.issued, ExpiresAt: now.Unix() + test.expires}
			raw, err := signSub2APITicket(payload, secret)
			if err != nil {
				t.Fatal(err)
			}
			actual, err := VerifySub2APITicket(raw, secret, now)
			if (err == nil) != test.accept {
				t.Fatalf("accept = %t, error = %v", test.accept, err)
			}
			if !test.accept && actual != (Sub2APISSOPayload{}) {
				t.Fatal("invalid ticket returned a usable identity")
			}
		})
	}
}

func TestVerifySub2APITicketRejectsInvalidPayload(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	secret := strings.Repeat("s", 32)
	for _, test := range []struct {
		name   string
		mutate func(*Sub2APISSOPayload)
	}{
		{"wrong issuer", func(p *Sub2APISSOPayload) { p.Issuer = "other" }},
		{"missing issuer", func(p *Sub2APISSOPayload) { p.Issuer = "" }},
		{"cross application", func(p *Sub2APISSOPayload) { p.Audience = "canvas" }},
		{"missing audience", func(p *Sub2APISSOPayload) { p.Audience = "" }},
		{"missing issue time", func(p *Sub2APISSOPayload) { p.IssuedAt = 0 }},
		{"negative issue time", func(p *Sub2APISSOPayload) { p.IssuedAt = -1 }},
		{"overflow expiry", func(p *Sub2APISSOPayload) { p.ExpiresAt = 1<<63 - 1 }},
		{"missing subject", func(p *Sub2APISSOPayload) { p.Subject = " " }},
		{"missing nonce", func(p *Sub2APISSOPayload) { p.Nonce = " " }},
		{"long subject", func(p *Sub2APISSOPayload) { p.Subject = strings.Repeat("s", 161) }},
		{"long nonce", func(p *Sub2APISSOPayload) { p.Nonce = strings.Repeat("n", 161) }},
		{"long email", func(p *Sub2APISSOPayload) { p.Email = strings.Repeat("e", 161) }},
		{"long username", func(p *Sub2APISSOPayload) { p.Username = strings.Repeat("u", 161) }},
		{"long display name", func(p *Sub2APISSOPayload) { p.DisplayName = strings.Repeat("d", 161) }},
		{"long avatar", func(p *Sub2APISSOPayload) { p.AvatarURL = strings.Repeat("a", 2049) }},
		{"long next", func(p *Sub2APISSOPayload) { p.Next = "/" + strings.Repeat("p", 2048) }},
		{"subject control", func(p *Sub2APISSOPayload) { p.Subject = "user\x00" }},
		{"nonce control", func(p *Sub2APISSOPayload) { p.Nonce = "nonce\n" }},
		{"redirect control", func(p *Sub2APISSOPayload) { p.Next = "/\t/evil.example" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			payload := Sub2APISSOPayload{Issuer: "sub2api", Audience: "ju", Subject: "user-1", Nonce: kernel.NewID(), IssuedAt: now.Unix(), ExpiresAt: now.Add(time.Minute).Unix()}
			test.mutate(&payload)
			raw, err := signSub2APITicket(payload, secret)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := VerifySub2APITicket(raw, secret, now); err == nil {
				t.Fatal("invalid signed payload was accepted")
			}
		})
	}
	if _, err := VerifySub2APITicket(strings.Repeat("x", ssoTicketMaxBytes+1), secret, now); err == nil || !strings.Contains(err.Error(), "too long") {
		t.Fatalf("oversized ticket was not rejected before parsing: %v", err)
	}
	if _, err := VerifySub2APITicket("bad.ticket", "short-secret", now); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("short secret was not rejected: %v", err)
	}
}

func TestVerifySub2APITicketRejectsMalformedEncodings(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	secret := strings.Repeat("s", 32)
	for _, raw := range []string{"", ".", "body.", ".signature", "a.b.c", "body.%%%"} {
		if _, err := VerifySub2APITicket(raw, secret, now); err == nil {
			t.Errorf("malformed ticket %q was accepted", raw)
		}
	}
	for _, body := range []string{"%%%", base64.RawURLEncoding.EncodeToString([]byte("not-json")), base64.RawURLEncoding.EncodeToString([]byte("null"))} {
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write([]byte(body))
		raw := body + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
		if _, err := VerifySub2APITicket(raw, secret, now); err == nil {
			t.Fatal("valid signature must not bypass payload validation")
		}
	}
}

func TestVerifySub2APITicketConcurrentLocalReplay(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	secret := strings.Repeat("s", 32)
	raw, err := signSub2APITicket(Sub2APISSOPayload{Issuer: "sub2api", Audience: "ju", Subject: "user-1", Nonce: kernel.NewID(), IssuedAt: now.Unix(), ExpiresAt: now.Add(time.Minute).Unix()}, secret)
	if err != nil {
		t.Fatal(err)
	}
	var accepted atomic.Int32
	var group sync.WaitGroup
	for range 8 {
		group.Add(1)
		go func() {
			defer group.Done()
			if _, err := VerifySub2APITicket(raw, secret, now); err == nil {
				accepted.Add(1)
			}
		}()
	}
	group.Wait()
	if accepted.Load() != 1 {
		t.Fatalf("accepted %d concurrent uses, want one", accepted.Load())
	}
}

func TestVerifySub2APITicketRedisReplayAndTTL(t *testing.T) {
	address := strings.TrimSpace(os.Getenv("CANVAS_TEST_SSO_REDIS_ADDR"))
	if address == "" {
		t.Skip("requires a dedicated test Redis via CANVAS_TEST_SSO_REDIS_ADDR")
	}
	first := redis.NewClient(&redis.Options{Addr: address, MaxRetries: -1})
	second := redis.NewClient(&redis.Options{Addr: address, MaxRetries: -1})
	t.Cleanup(func() { _ = first.Close(); _ = second.Close() })
	now := time.Unix(1_800_000_000, 250_000_000)
	secret := strings.Repeat("s", 32)
	payload := Sub2APISSOPayload{Issuer: "sub2api", Audience: "ju", Subject: "user-redis", Nonce: kernel.NewID(), IssuedAt: now.Unix(), ExpiresAt: now.Add(time.Minute).Unix()}
	raw, err := signSub2APITicket(payload, secret)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := VerifySub2APITicketWithRedis(raw, secret, now, first)
	if err != nil || actual != payload {
		t.Fatalf("Redis verification lost identity: %+v, %v", actual, err)
	}
	if _, err := VerifySub2APITicketWithRedis(raw, secret, now, second); err == nil || !strings.Contains(err.Error(), "already used") {
		t.Fatalf("second client accepted consumed ticket: %v", err)
	}
	hash := sha256.Sum256([]byte(payload.Nonce))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	ttl, err := second.PTTL(ctx, fmt.Sprintf("canvas:sso:sub2api:%x", hash[:])).Result()
	wantTTL := time.Unix(payload.ExpiresAt, 0).Sub(now)
	if err != nil || ttl > wantTTL || ttl < wantTTL-2*time.Second {
		t.Fatalf("Redis TTL = %v, want near %v using verification clock, err = %v", ttl, wantTTL, err)
	}
	var accepted atomic.Int32
	var group sync.WaitGroup
	payload.Nonce = kernel.NewID()
	raw, err = signSub2APITicket(payload, secret)
	if err != nil {
		t.Fatal(err)
	}
	for index := range 8 {
		group.Add(1)
		go func() {
			defer group.Done()
			client := first
			if index%2 == 1 {
				client = second
			}
			if _, err := VerifySub2APITicketWithRedis(raw, secret, now, client); err == nil {
				accepted.Add(1)
			}
		}()
	}
	group.Wait()
	if accepted.Load() != 1 {
		t.Fatalf("Redis accepted %d concurrent uses, want one", accepted.Load())
	}
}

func TestVerifySub2APITicketRedisFailsClosedAndBoundsIO(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	secret := strings.Repeat("s", 32)
	raw, err := signSub2APITicket(Sub2APISSOPayload{Issuer: "sub2api", Audience: "ju", Subject: "user-1", Nonce: kernel.NewID(), IssuedAt: now.Unix(), ExpiresAt: now.Add(time.Minute).Unix()}, secret)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifySub2APITicketWithRedis(raw, secret, now, nil); err == nil {
		t.Fatal("missing Redis must not fall back to a process-local replay store")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			accepted <- conn
		}
	}()
	client := redis.NewClient(&redis.Options{Addr: listener.Addr().String(), MaxRetries: -1, ReadTimeout: time.Minute, WriteTimeout: time.Minute})
	t.Cleanup(func() { _ = client.Close() })
	started := time.Now()
	_, err = VerifySub2APITicketWithRedis(raw, secret, now, client)
	if err == nil || !strings.Contains(err.Error(), "replay store unavailable") {
		t.Fatalf("unresponsive Redis must reject the ticket: %v", err)
	}
	if elapsed := time.Since(started); elapsed > ssoReplayTimeout+time.Second {
		t.Fatalf("Redis I/O ignored bounded timeout: %v", elapsed)
	}
	select {
	case conn := <-accepted:
		_ = conn.Close()
	default:
	}
	if _, err := VerifySub2APITicket(raw, secret, now); err != nil {
		t.Fatalf("Redis failure should not consume a process-local nonce: %v", err)
	}
}

func TestCompleteSub2APISSORetainsIdentityAndRejectsDisabledUser(t *testing.T) {
	svc, db := newPasswordResetTestService(t)
	if err := db.AutoMigrate(&model.UserIdentity{}, &model.CreditAccount{}); err != nil {
		t.Fatal(err)
	}
	payload := Sub2APISSOPayload{Subject: " subject-1 ", Username: "sub2api_test", DisplayName: "SSO user"}
	first, err := svc.CompleteSub2APISSO(payload)
	if err != nil {
		t.Fatal(err)
	}
	if first == nil || first.Session == "" || first.User.ID == "" {
		t.Fatal("SSO ticket completion did not issue a ju session")
	}
	if first.MaxAgeSecs != int((3 * 24 * time.Hour).Seconds()) {
		t.Fatalf("SSO session max age = %d, want three days", first.MaxAgeSecs)
	}
	second, err := svc.CompleteSub2APISSO(payload)
	if err != nil || first.User.ID != second.User.ID {
		t.Fatalf("SSO identity was not reused: %v", err)
	}
	for _, table := range []any{&model.User{}, &model.UserIdentity{}, &model.CreditAccount{}} {
		var count int64
		if err := db.Model(table).Count(&count).Error; err != nil || count != 1 {
			t.Fatalf("identity provisioning created duplicate rows: %T count %d, err %v", table, count, err)
		}
	}
	if err := db.Model(&model.User{}).Where("id = ?", first.User.ID).Update("status", "disabled").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CompleteSub2APISSO(payload); err == nil {
		t.Fatal("disabled user received an SSO session")
	}
	var sessions int64
	if err := db.Model(&model.AuthSession{}).Count(&sessions).Error; err != nil || sessions != 2 {
		t.Fatalf("disabled user created another session: %d, %v", sessions, err)
	}
}

func TestSub2APIDuplicateIdentityCreationRollsBackUser(t *testing.T) {
	svc, db := newPasswordResetTestService(t)
	if err := db.AutoMigrate(&model.UserIdentity{}, &model.CreditAccount{}); err != nil {
		t.Fatal(err)
	}
	payload := Sub2APISSOPayload{Subject: "same-subject", Username: "first_user"}
	user, identity, err := svc.createSub2APIUser(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.repo.CreateOAuthUser(user, identity); err != nil {
		t.Fatal(err)
	}
	payload.Username = "concurrent_user"
	user, identity, err = svc.createSub2APIUser(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.repo.CreateOAuthUser(user, identity); err == nil {
		t.Fatal("duplicate external identity was accepted")
	}
	for _, table := range []any{&model.User{}, &model.UserIdentity{}, &model.CreditAccount{}} {
		var count int64
		if err := db.Model(table).Count(&count).Error; err != nil || count != 1 {
			t.Fatalf("failed identity write left partial rows: %T count %d, err %v", table, count, err)
		}
	}
}

func TestSub2APIPersistentReplayAndHandoff(t *testing.T) {
	svc, db := newPasswordResetTestService(t)
	if err := db.AutoMigrate(&model.UserIdentity{}, &model.CreditAccount{}, &model.SSOTicket{}, &model.SSOHandoff{}, &model.SSORevocation{}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	payload := Sub2APISSOPayload{Issuer: "sub2api", Audience: "ju", Subject: "persistent-user", Nonce: kernel.NewID(), IssuedAt: now.Unix(), ExpiresAt: now.Add(time.Minute).Unix(), Next: "/projects?tab=recent"}
	secret := strings.Repeat("s", 32)
	raw, err := signSub2APITicket(payload, secret)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = VerifySub2APITicketWithRepository(raw, secret, now, svc.repo); err != nil {
		t.Fatal(err)
	}
	// A new repository has no process-local replay state.
	if _, err = VerifySub2APITicketWithRepository(raw, secret, now, repository.New(db)); err == nil {
		t.Fatal("persistent ticket was replayed")
	}
	var stored model.SSOTicket
	if err = db.First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.ID == payload.Nonce || len(stored.ID) != 64 {
		t.Fatal("nonce was not hashed")
	}
	handoff, err := svc.CreateSSOHandoff(payload)
	if err != nil {
		t.Fatal(err)
	}
	var sessions int64
	db.Model(&model.AuthSession{}).Count(&sessions)
	if sessions != 0 {
		t.Fatal("callback created a session before exchange")
	}
	result, next, err := svc.ExchangeSSOHandoff(handoff)
	if err != nil {
		t.Fatal(err)
	}
	if result.User.Role != model.UserRoleUser || result.User.IdentityProvider != "sub2api" || next != payload.Next {
		t.Fatal("handoff lost identity or destination")
	}
	if _, _, err = svc.ExchangeSSOHandoff(handoff); err == nil {
		t.Fatal("handoff replay accepted")
	}
	handoff, err = svc.CreateSSOHandoff(payload)
	if err != nil {
		t.Fatal(err)
	}
	db.Model(&model.SSOHandoff{}).Where("id = ?", HashToken(handoff)).Update("expires_at", now.Add(-time.Minute))
	if _, _, err = svc.ExchangeSSOHandoff(handoff); err == nil {
		t.Fatal("expired handoff accepted")
	}
	t.Setenv("SUB2API_SSO_SECRET", secret)
	pending, err := svc.CreateSSOHandoff(payload)
	if err != nil {
		t.Fatal(err)
	}
	logout := payload
	logout.Audience, logout.Nonce = "ju:logout", kernel.NewID()
	logoutRaw, err := signSub2APITicket(logout, secret)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = VerifySub2APITicketWithRepository(logoutRaw, secret, now, svc.repo); err == nil {
		t.Fatal("logout assertion accepted as login")
	}
	if err = svc.RevokeSub2APISessions(raw); err == nil {
		t.Fatal("login assertion accepted as logout")
	}
	legacy := model.AuthSession{ID: kernel.NewID(), UserID: result.User.ID, TokenHash: "legacy", ExpiresAt: now.Add(time.Hour)}
	if err = db.Create(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Model(&legacy).Update("sso_issued_at", nil).Error; err != nil {
		t.Fatal(err)
	}
	if err = svc.RevokeSub2APISessions(logoutRaw); err != nil {
		t.Fatal(err)
	}
	var legacyCount int64
	if err = db.Model(&model.AuthSession{}).Where("id = ?", legacy.ID).Count(&legacyCount).Error; err != nil || legacyCount != 0 {
		t.Fatal("legacy session survived global logout")
	}
	if _, err = svc.CurrentUser(result.Session); err == nil {
		t.Fatal("global logout left session active")
	}
	if _, _, err = svc.ExchangeSSOHandoff(pending); err == nil {
		t.Fatal("pending handoff bypassed logout")
	}
	if err = svc.RevokeSub2APISessions(logoutRaw); err == nil {
		t.Fatal("logout assertion replay accepted")
	}
	payload.IssuedAt = logout.IssuedAt + 1
	if _, err = svc.CompleteSub2APISSO(payload); err != nil {
		t.Fatalf("fresh login after logout failed: %v", err)
	}
}
