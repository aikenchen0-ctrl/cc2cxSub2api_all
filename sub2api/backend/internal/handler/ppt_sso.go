package handler

import (
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

// PPTSSOStart hands off identity only; provider credentials stay on the server.
func (h *AuthHandler) PPTSSOStart(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Referrer-Policy", "no-referrer")
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	secret := strings.TrimSpace(os.Getenv("SUB2API_SSO_SECRET"))
	if len(secret) < 32 {
		response.Error(c, http.StatusServiceUnavailable, "PPT SSO is not configured")
		return
	}
	if h == nil || h.userService == nil {
		response.Error(c, http.StatusServiceUnavailable, "PPT SSO is unavailable")
		return
	}
	if _, err := h.userService.GetByID(c.Request.Context(), subject.UserID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	callback := strings.TrimSpace(os.Getenv("PPT_SSO_CALLBACK_URL"))
	if callback == "" {
		callback = "http://localhost:8341/api/v1/auth/sso/callback"
	}
	parsed, err := url.Parse(callback)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.ForceQuery {
		response.Error(c, http.StatusServiceUnavailable, "PPT_SSO_CALLBACK_URL is invalid")
		return
	}
	next := safeCanvasNext(c.Query("next"))
	if next == "/" || len(next) > 2048 {
		next = "/upload"
	}
	now := time.Now()
	raw, err := signJuTicket(juSSOTicket{
		Audience: "presenton", Subject: strconvInt64(subject.UserID),
		IssuedAt: now.Unix(), ExpiresAt: now.Add(2 * time.Minute).Unix(),
		Nonce: randomNonce(), Next: next,
	}, secret)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Unable to start PPT SSO")
		return
	}
	query := parsed.Query()
	query.Set("ticket", raw)
	parsed.RawQuery = query.Encode()
	response.Success(c, gin.H{"redirect_url": parsed.String()})
}
