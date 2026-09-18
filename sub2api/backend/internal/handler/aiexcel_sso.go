package handler

import (
	"crypto/rand"
	"encoding/hex"
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

// AIExcelSSOStart issues a short-lived, one-time handoff for AIExcel.
// The browser never receives a Sub2API JWT or model API key.
func (h *AuthHandler) AIExcelSSOStart(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Referrer-Policy", "no-referrer")
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	secret := strings.TrimSpace(os.Getenv("SUB2API_SSO_SECRET"))
	if len(secret) < 32 {
		response.Error(c, http.StatusServiceUnavailable, "AIExcel SSO is not configured")
		return
	}
	if h == nil || h.userService == nil {
		response.Error(c, http.StatusServiceUnavailable, "AIExcel SSO is unavailable")
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
	callback := strings.TrimSpace(os.Getenv("AIEXCEL_SSO_CALLBACK_URL"))
	if callback == "" {
		callback = "http://localhost:4173/api/auth/sso/callback"
	}
	parsed, err := url.Parse(callback)
	if err != nil || !validAIExcelCallback(parsed) {
		response.Error(c, http.StatusServiceUnavailable, "AIEXCEL_SSO_CALLBACK_URL is invalid")
		return
	}
	var nonce [32]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		response.Error(c, http.StatusInternalServerError, "Unable to start AIExcel SSO")
		return
	}
	now := time.Now()
	payload := juSSOTicket{
		Issuer: "sub2api", Audience: "aiexcel", Subject: strconvInt64(subject.UserID),
		Email: user.Email, Username: user.Username, DisplayName: user.Username, AvatarURL: user.AvatarURL,
		IssuedAt: now.Unix(), ExpiresAt: now.Add(2 * time.Minute).Unix(), Nonce: hex.EncodeToString(nonce[:]),
		Next: safeAIExcelNext(c.Query("next")),
	}
	raw, err := signJuTicket(payload, secret)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Unable to start AIExcel SSO")
		return
	}
	query := parsed.Query()
	query.Set("ticket", raw)
	parsed.RawQuery = query.Encode()
	response.Success(c, gin.H{"redirect_url": parsed.String()})
}

func validAIExcelCallback(u *url.URL) bool {
	if u == nil || u.Hostname() == "" || u.User != nil || u.Opaque != "" || u.RawQuery != "" || u.Fragment != "" || u.ForceQuery || u.Path != "/api/auth/sso/callback" || u.RawPath != "" {
		return false
	}
	local := u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"
	return u.Scheme == "https" || (u.Scheme == "http" && local)
}

func safeAIExcelNext(raw string) string {
	if len(raw) > 2048 || strings.IndexFunc(raw, unicode.IsControl) >= 0 {
		return "/"
	}
	raw = strings.TrimSpace(raw)
	if raw == "" || !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") || strings.Contains(raw, "\\") || strings.ContainsAny(raw, "\r\n") {
		return "/"
	}
	return raw
}
