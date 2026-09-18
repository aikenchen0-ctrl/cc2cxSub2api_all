package handler

import (
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

// CanvasSSOStart issues a short-lived, signed handoff for the sibling canvas app.
// The browser only ever sees this one-time ticket, never a sub2api JWT/API key.
func (h *AuthHandler) CanvasSSOStart(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Referrer-Policy", "no-referrer")
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	secret := strings.TrimSpace(os.Getenv("SUB2API_SSO_SECRET"))
	if len(secret) < 32 {
		response.Error(c, http.StatusServiceUnavailable, "Canvas SSO is not configured")
		return
	}
	if h == nil || h.userService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Canvas SSO is unavailable")
		return
	}
	user, err := h.userService.GetByID(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	now := time.Now()
	next := safeCanvasNext(c.Query("next"))
	payload := juSSOTicket{Subject: strconvInt64(subject.UserID), Email: user.Email, Username: user.Username, DisplayName: user.Username, AvatarURL: user.AvatarURL, IssuedAt: now.Unix(), ExpiresAt: now.Add(2 * time.Minute).Unix(), Nonce: randomNonce(), Next: next}
	raw, err := signJuTicket(payload, secret)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Unable to start Canvas SSO")
		return
	}
	callback := strings.TrimSpace(os.Getenv("CANVAS_SSO_CALLBACK_URL"))
	if callback == "" {
		callback = "http://localhost:3522/api/auth/sso/callback"
	}
	parsed, err := url.Parse(callback)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.ForceQuery {
		response.Error(c, http.StatusServiceUnavailable, "CANVAS_SSO_CALLBACK_URL is invalid")
		return
	}
	query := parsed.Query()
	query.Set("ticket", raw)
	parsed.RawQuery = query.Encode()
	response.Success(c, gin.H{"redirect_url": parsed.String()})
}

func safeCanvasNext(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") || strings.Contains(raw, "\\") || strings.IndexFunc(raw, unicode.IsControl) >= 0 {
		return "/"
	}
	return raw
}
