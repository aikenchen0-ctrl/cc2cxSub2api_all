package handler

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"

	"github.com/gin-gonic/gin"
)

type juSSOTicket struct {
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

// JuSSOStart issues a short-lived, signed handoff for the sibling Yingce app.
// The browser only ever sees this one-time ticket, never a sub2api JWT/API key.
func (h *AuthHandler) JuSSOStart(c *gin.Context) {
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
	now := time.Now()
	next := safeJuNext(c.Query("next"))
	payload := juSSOTicket{Subject: strconvInt64(subject.UserID), Email: user.Email, Username: user.Username, DisplayName: user.Username, AvatarURL: user.AvatarURL, IssuedAt: now.Unix(), ExpiresAt: now.Add(2 * time.Minute).Unix(), Nonce: randomNonce(), Next: next}
	raw, err := signJuTicket(payload, secret)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Unable to start Yingce SSO")
		return
	}
	callback := strings.TrimSpace(os.Getenv("JU_SSO_CALLBACK_URL"))
	if callback == "" {
		callback = "http://localhost:3000/api/auth/sso/callback"
	}
	parsed, err := url.Parse(callback)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
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
	raw = strings.TrimSpace(raw)
	if raw == "" || !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") || strings.Contains(raw, "\\") || strings.ContainsAny(raw, "\r\n") {
		return "/projects"
	}
	return raw
}
