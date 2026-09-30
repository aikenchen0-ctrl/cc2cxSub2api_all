package service

import (
	"strings"
	"testing"
	"time"
)

func TestWeChatPaymentHandoffTokenRoundTripAndTamperProtection(t *testing.T) {
	svc := NewPaymentResumeService([]byte("0123456789abcdef0123456789abcdef"))
	token, err := svc.CreateWeChatPaymentHandoffToken("agentapi")
	if err != nil {
		t.Fatal(err)
	}
	claims, err := svc.ParseWeChatPaymentHandoffToken(token)
	if err != nil {
		t.Fatal(err)
	}
	if claims.SatelliteSlug != "agentapi" || claims.ExpiresAt <= claims.IssuedAt {
		t.Fatalf("unexpected claims: %+v", claims)
	}
	if time.Unix(claims.ExpiresAt, 0).Sub(time.Unix(claims.IssuedAt, 0)) != wechatPaymentHandoffTokenTTL {
		t.Fatalf("unexpected handoff lifetime: %+v", claims)
	}

	tampered := token[:len(token)-1] + map[bool]string{true: "a", false: "b"}[strings.HasSuffix(token, "a")]
	if _, err := svc.ParseWeChatPaymentHandoffToken(tampered); err == nil {
		t.Fatal("tampered handoff token should be rejected")
	}
}

func TestWeChatPaymentHandoffTokenRejectsInvalidAndExpiredClaims(t *testing.T) {
	svc := NewPaymentResumeService([]byte("0123456789abcdef0123456789abcdef"))
	if _, err := svc.CreateWeChatPaymentHandoffToken("Agent API"); err == nil {
		t.Fatal("invalid satellite slug should be rejected")
	}

	expired, err := svc.createSignedToken(WeChatPaymentHandoffClaims{
		TokenType:     wechatPaymentHandoffTokenType,
		SatelliteSlug: "agentapi",
		IssuedAt:      time.Now().Add(-10 * time.Minute).Unix(),
		ExpiresAt:     time.Now().Add(-5 * time.Minute).Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ParseWeChatPaymentHandoffToken(expired); err == nil {
		t.Fatal("expired handoff token should be rejected")
	}
}

func TestOAuthBindingHandoffTokenIsUserAndProviderScoped(t *testing.T) {
	svc := NewPaymentResumeService([]byte("0123456789abcdef0123456789abcdef"))
	token, err := svc.CreateOAuthBindingHandoffToken(OAuthBindingHandoffClaims{
		SatelliteSlug: "agentapi",
		UserID:        42,
		Provider:      "OIDC",
		RedirectTo:    "/profile",
	})
	if err != nil {
		t.Fatal(err)
	}
	claims, err := svc.ParseOAuthBindingHandoffToken(token)
	if err != nil {
		t.Fatal(err)
	}
	if claims.SatelliteSlug != "agentapi" || claims.UserID != 42 || claims.Provider != "oidc" || claims.RedirectTo != "/profile" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
	if time.Unix(claims.ExpiresAt, 0).Sub(time.Unix(claims.IssuedAt, 0)) != oauthBindingHandoffTokenTTL {
		t.Fatalf("unexpected handoff lifetime: %+v", claims)
	}

	tampered := token[:len(token)-1] + map[bool]string{true: "a", false: "b"}[strings.HasSuffix(token, "a")]
	if _, err := svc.ParseOAuthBindingHandoffToken(tampered); err == nil {
		t.Fatal("tampered oauth binding handoff should be rejected")
	}
	for _, invalid := range []OAuthBindingHandoffClaims{
		{SatelliteSlug: "agentapi", UserID: 0, Provider: "oidc"},
		{SatelliteSlug: "agentapi", UserID: 42, Provider: "github"},
		{SatelliteSlug: "Agent API", UserID: 42, Provider: "oidc"},
		{SatelliteSlug: "agentapi", UserID: 42, Provider: "oidc", RedirectTo: "https://attacker.example"},
	} {
		if _, err := svc.CreateOAuthBindingHandoffToken(invalid); err == nil {
			t.Fatalf("invalid claims should be rejected: %+v", invalid)
		}
	}
}
