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
	"github.com/Wei-Shaw/sub2api/pkg/satellite"
	"github.com/gin-gonic/gin"
)

// IntegrationSSOStart is the single SSO entry: GET /auth/integrations/:slug/start
func (h *AuthHandler) IntegrationSSOStart(c *gin.Context) {
	h.startSatelliteSSO(c, c.Param("slug"))
}

func (h *AuthHandler) CanvasSSOStart(c *gin.Context)      { h.startSatelliteSSO(c, "canvas") }
func (h *AuthHandler) JuSSOStart(c *gin.Context)          { h.startSatelliteSSO(c, "ju") }
func (h *AuthHandler) LivartSSOStart(c *gin.Context)      { h.startSatelliteSSO(c, "livart") }
func (h *AuthHandler) PPTSSOStart(c *gin.Context)         { h.startSatelliteSSO(c, "ppt") }
func (h *AuthHandler) AicutSSOStart(c *gin.Context)       { h.startSatelliteSSO(c, "aicut") }
func (h *AuthHandler) Screen2CodeSSOStart(c *gin.Context) { h.startSatelliteSSO(c, "screen2code") }
func (h *AuthHandler) AIExcelSSOStart(c *gin.Context)     { h.startSatelliteSSO(c, "aiexcel") }
func (h *AuthHandler) QrcodeSSOStart(c *gin.Context)      { h.startSatelliteSSO(c, "qrcode") }

func (h *AuthHandler) startSatelliteSSO(c *gin.Context, slug string) {
	c.Header("Cache-Control", "no-store")
	c.Header("Referrer-Policy", "no-referrer")
	app, ok := satellite.Lookup(strings.TrimSpace(slug))
	if !ok {
		response.Error(c, http.StatusNotFound, "Unknown satellite app")
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	secret := strings.TrimSpace(os.Getenv("SUB2API_SSO_SECRET"))
	if len(secret) < 32 {
		response.Error(c, http.StatusServiceUnavailable, app.Slug+" SSO is not configured")
		return
	}
	if h == nil || h.userService == nil {
		response.Error(c, http.StatusServiceUnavailable, app.Slug+" SSO is unavailable")
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
	callback := ssoCallback(app.CallbackEnv, app.Slug, app.DefaultOrigin, app.CallbackPath)
	parsed, err := url.Parse(callback)
	if err != nil || !validSatelliteCallback(parsed, app.CallbackPath) {
		response.Error(c, http.StatusServiceUnavailable, app.CallbackEnv+" is invalid")
		return
	}
	ttl := app.TicketTTL
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}
	now := time.Now()
	payload := juSSOTicket{
		Issuer: "sub2api", Audience: app.Audience, Subject: strconvInt64(subject.UserID),
		Email: user.Email, Username: user.Username, DisplayName: user.Username, AvatarURL: user.AvatarURL,
		IssuedAt: now.Unix(), ExpiresAt: now.Add(ttl).Unix(),
		Nonce: randomNonce(), Next: safeSatelliteNext(c.Query("next"), app.DefaultNext),
	}
	raw, err := signJuTicket(payload, secret)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Unable to start "+app.Slug+" SSO")
		return
	}
	query := parsed.Query()
	query.Set("ticket", raw)
	parsed.RawQuery = query.Encode()
	response.Success(c, gin.H{"redirect_url": parsed.String()})
}

func validSatelliteCallback(u *url.URL, path string) bool {
	if u == nil || u.Hostname() == "" || u.User != nil || u.Opaque != "" ||
		u.RawQuery != "" || u.Fragment != "" || u.ForceQuery || u.Path != path {
		return false
	}
	local := u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"
	return u.Scheme == "https" || (u.Scheme == "http" && local)
}

func safeSatelliteNext(raw, fallback string) string {
	if fallback == "" {
		fallback = "/"
	}
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 2048 || !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") ||
		strings.Contains(raw, "\\") || strings.IndexFunc(raw, unicode.IsControl) >= 0 {
		return fallback
	}
	return raw
}
