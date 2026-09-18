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

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

// RevokeScreen2CodeSessions propagates the identity logout to the application.
func revokeScreen2CodeSessions(ctx context.Context, userID int64) error {
	callback := strings.TrimSpace(os.Getenv("SCREEN2CODE_SSO_CALLBACK_URL"))
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

// Screen2CodeSSOStart issues a short-lived identity handoff. Provider keys and
// the sub2api JWT never leave this server.
func (h *AuthHandler) Screen2CodeSSOStart(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Referrer-Policy", "no-referrer")
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	secret := strings.TrimSpace(os.Getenv("SUB2API_SSO_SECRET"))
	if len(secret) < 32 {
		response.Error(c, http.StatusServiceUnavailable, "Screen2Code SSO is not configured")
		return
	}
	if h == nil || h.userService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Screen2Code SSO is unavailable")
		return
	}
	if h.apiKeyService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Screen2Code relay key service is unavailable")
		return
	}
	user, err := h.userService.GetByID(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if user == nil || !user.IsActive() {
		response.Forbidden(c, "User is disabled")
		return
	}
	callback := strings.TrimSpace(os.Getenv("SCREEN2CODE_SSO_CALLBACK_URL"))
	if callback == "" {
		callback = projectLink("screen2code", "http://localhost:5173") + "/api/auth/sso/callback"
	}
	parsed, err := parseScreen2CodeCallback(callback)
	if err != nil {
		response.Error(c, http.StatusServiceUnavailable, "SCREEN2CODE_SSO_CALLBACK_URL is invalid")
		return
	}
	now := time.Now()
	payload := juSSOTicket{
		Issuer: "sub2api", Audience: "screen2code", Subject: strconvInt64(subject.UserID),
		Email: user.Email, Username: user.Username, DisplayName: user.Username,
		AvatarURL: user.AvatarURL, IssuedAt: now.Unix(), ExpiresAt: now.Add(2 * time.Minute).Unix(),
		Nonce: randomNonce(), Next: safeScreen2CodeNext(c.Query("next")),
	}
	relayKey, err := h.apiKeyService.GetOrCreateScreen2CodeAPIKey(c.Request.Context(), subject.UserID)
	if err != nil || relayKey == nil || strings.TrimSpace(relayKey.Key) == "" {
		response.Error(c, http.StatusServiceUnavailable, "Unable to provision Screen2Code relay key")
		return
	}
	payload.RelayKeyCiphertext, err = encryptRelayKey(relayKey.Key, secret)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Unable to protect Screen2Code relay key")
		return
	}
	raw, err := signJuTicket(payload, secret)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Unable to start Screen2Code SSO")
		return
	}
	query := parsed.Query()
	query.Set("ticket", raw)
	parsed.RawQuery = query.Encode()
	response.Success(c, gin.H{"redirect_url": parsed.String()})
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
