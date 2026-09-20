package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// RevokeScreen2CodeSessions propagates the identity logout to the application.
func revokeScreen2CodeSessions(ctx context.Context, userID int64) error {
	callback := ssoCallback("SCREEN2CODE_SSO_CALLBACK_URL", "screen2code", "", "/api/auth/sso/callback")
	if callback == "" {
		return nil
	}
	target, err := parseScreen2CodeCallback(callback)
	if err != nil {
		return err
	}
	secret := strings.TrimSpace(os.Getenv("SUB2API_SSO_SECRET"))
	if len(secret) < 32 {
		return fmt.Errorf("Screen2Code SSO is not configured")
	}
	now := time.Now()
	ticket, err := signJuTicket(juSSOTicket{Issuer: "sub2api", Audience: "screen2code:logout", Subject: strconvInt64(userID), IssuedAt: now.Unix(), ExpiresAt: now.Add(time.Minute).Unix(), Nonce: randomNonce()}, secret)
	if err != nil {
		return err
	}
	body, err := json.Marshal(map[string]string{"ticket": ticket})
	if err != nil {
		return err
	}
	target.Path = "/api/auth/sso/logout"
	requestContext, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(requestContext, http.MethodPost, target.String(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Screen2Code logout delivery failed")
	}
	defer resp.Body.Close()
	var result struct {
		Revoked bool `json:"revoked"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&result) != nil || !result.Revoked {
		return fmt.Errorf("Screen2Code logout was not acknowledged")
	}
	return nil
}

func parseScreen2CodeCallback(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	local := u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"
	if u.Hostname() == "" || (u.Scheme != "https" && !(u.Scheme == "http" && local)) ||
		u.User != nil || u.Opaque != "" || u.Fragment != "" || u.RawQuery != "" || u.ForceQuery ||
		u.Path != "/api/auth/sso/callback" || u.RawPath != "" {
		return nil, fmt.Errorf("invalid Screen2Code SSO callback")
	}
	return u, nil
}

func safeScreen2CodeNext(raw string) string {
	if raw == "" || !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") || strings.Contains(raw, "\\") || len(raw) > 2048 {
		return "/"
	}
	return raw
}
