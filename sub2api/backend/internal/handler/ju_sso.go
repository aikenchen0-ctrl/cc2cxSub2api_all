package handler

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"

	"github.com/gin-gonic/gin"
)

type juSSOTicket struct {
	Issuer             string `json:"iss,omitempty"`
	Audience           string `json:"aud,omitempty"`
	Subject            string `json:"sub"`
	Email              string `json:"email,omitempty"`
	Username           string `json:"username,omitempty"`
	DisplayName        string `json:"displayName,omitempty"`
	AvatarURL          string `json:"avatarUrl,omitempty"`
	IssuedAt           int64  `json:"iat"`
	ExpiresAt          int64  `json:"exp"`
	Nonce              string `json:"jti"`
	Next               string `json:"next,omitempty"`
	RelayKeyCiphertext string `json:"rk,omitempty"`
}

// JuSSOStart issues a three-day, signed handoff for the sibling Yingce app.
// The browser only ever sees this one-time ticket, never a sub2api JWT/API key.
func (h *AuthHandler) JuSSOStart(c *gin.Context) {
	defer func() { slog.Info("ju_sso_start", "status", c.Writer.Status()) }()
	c.Header("Cache-Control", "no-store")
	c.Header("Referrer-Policy", "no-referrer")
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	secret := strings.TrimSpace(os.Getenv("SUB2API_SSO_SECRET"))
	if len(secret) < 32 {
		response.Error(c, http.StatusServiceUnavailable, "Yingce SSO is not configured")
		return
	}
	if h == nil || h.userService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Yingce SSO is unavailable")
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
	now := time.Now()
	next := safeJuNext(c.Query("next"))
	var nonce [32]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		response.Error(c, http.StatusInternalServerError, "Unable to start Yingce SSO")
		return
	}
	// Keep the signed login assertion valid for three days. It is still
	// one-time at the Ju exchange boundary, so extending this window does not
	// permit replay after a successful handoff.
	payload := juSSOTicket{Issuer: "sub2api", Audience: "ju", Subject: strconvInt64(subject.UserID), Email: user.Email, Username: user.Username, DisplayName: user.Username, AvatarURL: user.AvatarURL, IssuedAt: now.Unix(), ExpiresAt: now.Add(3 * 24 * time.Hour).Unix(), Nonce: hex.EncodeToString(nonce[:]), Next: next}
	raw, err := signJuTicket(payload, secret)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Unable to start Yingce SSO")
		return
	}
	callback := strings.TrimSpace(os.Getenv("JU_SSO_CALLBACK_URL"))
	if callback == "" {
		if link := projectLink("ju", ""); link != "" {
			callback = link + "/api/auth/sso/callback"
		}
	}
	parsed, err := parseJuCallback(callback)
	if err != nil {
		response.Error(c, http.StatusServiceUnavailable, "JU_SSO_CALLBACK_URL is invalid")
		return
	}
	query := parsed.Query()
	query.Set("ticket", raw)
	parsed.RawQuery = query.Encode()
	response.Success(c, gin.H{"redirect_url": parsed.String()})
}

func signJuTicket(payload juSSOTicket, secret string) (string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(body)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(encoded))
	return encoded + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func randomNonce() string {
	var b [18]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconvInt64(time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

func strconvInt64(value int64) string {
	const digits = "0123456789"
	if value == 0 {
		return "0"
	}
	negative := value < 0
	if negative {
		value = -value
	}
	var buf [24]byte
	i := len(buf)
	for value > 0 {
		i--
		buf[i] = digits[value%10]
		value /= 10
	}
	if negative {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func safeJuNext(raw string) string {
	if len(raw) > 2048 || strings.IndexFunc(raw, unicode.IsControl) >= 0 {
		return "/projects"
	}
	raw = strings.TrimSpace(raw)
	if raw == "" || !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") || strings.Contains(raw, "\\") || strings.ContainsAny(raw, "\r\n") {
		return "/projects"
	}
	return raw
}

func parseJuCallback(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	local := u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"
	if u.Hostname() == "" || (u.Scheme != "https" && !(u.Scheme == "http" && local)) || u.User != nil || u.Opaque != "" || u.Fragment != "" || u.RawQuery != "" || u.ForceQuery || u.Path != "/api/auth/sso/callback" || u.RawPath != "" {
		return nil, fmt.Errorf("invalid Ju SSO callback")
	}
	return u, nil
}

// Revoke Ju sessions through the registered server-to-server endpoint. A
// failed delivery must not be reported as a successful global logout.
func revokeJuSessions(ctx context.Context, userID int64) error {
	callback := strings.TrimSpace(os.Getenv("JU_SSO_CALLBACK_URL"))
	if callback == "" {
		return nil
	}
	target, err := parseJuCallback(callback)
	if err != nil {
		return err
	}
	secret := strings.TrimSpace(os.Getenv("SUB2API_SSO_SECRET"))
	if len(secret) < 32 {
		return fmt.Errorf("Ju SSO is not configured")
	}
	var nonce [32]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return err
	}
	now := time.Now()
	ticket, err := signJuTicket(juSSOTicket{Issuer: "sub2api", Audience: "ju:logout", Subject: strconvInt64(userID), IssuedAt: now.Unix(), ExpiresAt: now.Add(time.Minute).Unix(), Nonce: hex.EncodeToString(nonce[:])}, secret)
	if err != nil {
		return err
	}
	body, err := json.Marshal(map[string]string{"ticket": ticket})
	if err != nil {
		return err
	}
	target.Path = "/api/auth/sso/logout"
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("unable to construct Ju logout request")
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Ju logout delivery failed")
	}
	defer resp.Body.Close()
	var result struct {
		Code int `json:"code"`
		Data struct {
			Revoked bool `json:"revoked"`
		} `json:"data"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&result) != nil || result.Code != 0 || !result.Data.Revoked {
		return fmt.Errorf("Ju logout was not acknowledged")
	}
	return nil
}
