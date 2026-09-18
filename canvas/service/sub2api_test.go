package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/tigerowo/infinite-canvas/config"
	"github.com/tigerowo/infinite-canvas/model"
	"github.com/tigerowo/infinite-canvas/repository"
)

func TestSub2APIIntegration(t *testing.T) {
	const marker = "CANVAS_SUB2API_TEST_CHILD"
	if os.Getenv(marker) != "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSub2APIIntegration$", "-test.v")
		cmd.Env = append(os.Environ(), marker+"=1")
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("isolated integration: %v\n%s", err, output)
		}
		return
	}
	config.Cfg = config.Config{StorageDriver: "sqlite", DatabaseDSN: t.TempDir() + "/test.db", JWTSecret: strings.Repeat("j", 32)}
	db, err := repository.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("SUB2API_RELAY_BASE_URL", "http://sub2api:8080")
	t.Setenv("SUB2API_RELAY_API_KEY", "sk-test-not-real")
	t.Setenv("SUB2API_RELAY_MODELS", "gpt-5.5,gpt-5.5")
	t.Setenv("SUB2API_RELAY_IMAGE_MODELS", "grok-imagine-image")
	t.Setenv("SUB2API_RELAY_VIDEO_MODELS", "")
	if err := EnsureSub2APIRelayChannel(); err != nil {
		t.Fatal(err)
	}
	channel, err := SelectModelChannelForModel("gpt-5.5", sub2APIRelayChannelID)
	if err != nil || channel.BaseURL != "http://sub2api:8080/v1" {
		t.Fatalf("channel: %+v %v", channel, err)
	}
	public, err := PublicSettings()
	if err != nil {
		t.Fatal(err)
	}
	serialized, _ := json.Marshal(public)
	if strings.Contains(string(serialized), channel.APIKey) || len(public.ModelChannel.AvailableModels) != 2 || !*public.ModelChannel.AllowUserRemoteChannel {
		t.Fatal("public settings invalid or secret exposed")
	}
	if err := EnsureSub2APIRelayChannel(); err != nil {
		t.Fatal(err)
	}
	settings, _ := repository.GetSettings()
	if len(settings.Private.Channels) != 1 {
		t.Fatal("bootstrap is not idempotent")
	}
	t.Setenv("SUB2API_RELAY_API_KEY", "sk-rotated-test")
	t.Setenv("SUB2API_RELAY_IMAGE_MODELS", "")
	if err := EnsureSub2APIRelayChannel(); err != nil {
		t.Fatal(err)
	}
	channel, err = SelectModelChannel("gpt-5.5")
	if err != nil || channel.APIKey != "sk-rotated-test" {
		t.Fatal("key rotation failed")
	}
	if _, err := SelectModelChannel("grok-imagine-image"); err == nil {
		t.Fatal("removed model remains enabled")
	}
	for _, address := range []string{"http://sub2api/api/v1", "http://sub2api:99999", "http://user:pass@sub2api", "http://sub2api/v1?", "http://sub2api/v1?secret=1", "http://sub2api/%76%31"} {
		t.Setenv("SUB2API_RELAY_BASE_URL", address)
		if err := EnsureSub2APIRelayChannel(); err == nil {
			t.Fatalf("accepted invalid URL %s", address)
		}
	}
	t.Setenv("SUB2API_RELAY_BASE_URL", "")
	t.Setenv("SUB2API_RELAY_API_KEY", "")
	if err := EnsureSub2APIRelayChannel(); err != nil {
		t.Fatal(err)
	}
	settings, _ = repository.GetSettings()
	if settings.Private.Channels[0].APIKey != "" || settings.Private.Channels[0].Enabled {
		t.Fatal("disabled channel retained secret")
	}

	at := time.Now()
	secret := strings.Repeat("s", 32)
	payload := Sub2APISSOPayload{Subject: "external-1", Username: "admin", Nonce: "nonce-1", IssuedAt: at.Unix(), ExpiresAt: at.Add(time.Minute).Unix()}
	sign := func(p Sub2APISSOPayload) string {
		body, _ := json.Marshal(p)
		encoded := base64.RawURLEncoding.EncodeToString(body)
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write([]byte(encoded))
		return encoded + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	}
	ticket := sign(payload)
	if _, err := VerifySub2APITicket(ticket, strings.Repeat("x", 32), at); err == nil {
		t.Fatal("accepted wrong signature")
	}
	verified, err := VerifySub2APITicket(ticket, secret, at)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifySub2APITicket(ticket, secret, at); err == nil {
		t.Fatal("accepted replay")
	}
	var consumed int64
	db.Model(&model.SSOTicket{}).Count(&consumed)
	if consumed != 1 {
		t.Fatal("replay state not durable")
	}
	for _, invalid := range []Sub2APISSOPayload{
		{Subject: "external-1", Nonce: "bad-time", IssuedAt: at.Unix() + 30, ExpiresAt: at.Unix() + 20},
		{Subject: "external-1", Nonce: "expired", IssuedAt: at.Unix() - 120, ExpiresAt: at.Unix() - 1},
		{Subject: "external-1", Nonce: "too-long", IssuedAt: at.Unix(), ExpiresAt: at.Unix() + 121},
		{Subject: "external-1", Nonce: "control\n", IssuedAt: at.Unix(), ExpiresAt: at.Unix() + 60},
	} {
		if _, err := VerifySub2APITicket(sign(invalid), secret, at); err == nil {
			t.Fatal("accepted invalid ticket")
		}
	}
	session, err := CompleteSub2APISSO(verified)
	if err != nil {
		t.Fatal(err)
	}
	again, err := CompleteSub2APISSO(verified)
	if err != nil {
		t.Fatal(err)
	}
	if session.User.ID != again.User.ID || session.User.Role != model.UserRoleUser || session.User.Username == "admin" {
		t.Fatal("identity isolation failed")
	}
	if _, ok := CurrentAuthUser(session.Token); !ok {
		t.Fatal("SSO session invalid")
	}
	db.Model(&model.User{}).Where("id = ?", session.User.ID).Update("status", model.UserStatusBan)
	if _, err := CompleteSub2APISSO(verified); err == nil {
		t.Fatal("banned SSO user admitted")
	}
	for _, next := range []string{"//evil.example", "/\\evil.example", "/\nevil", "https://evil.example"} {
		if SSONext(next) != "/" {
			t.Fatalf("unsafe redirect %q", next)
		}
	}
}
