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

// YibiaoSSOStart 为易标招标监控生成短时效身份交接票据。
// 浏览器不会收到 Sub2API JWT、模型密钥或内部服务令牌。
func (h *AuthHandler) YibiaoSSOStart(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Referrer-Policy", "no-referrer")
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	secret := strings.TrimSpace(os.Getenv("SUB2API_SSO_SECRET"))
	if len(secret) < 32 {
		response.Error(c, http.StatusServiceUnavailable, "Yibiao SSO is not configured")
		return
	}
	if h == nil || h.userService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Yibiao SSO is unavailable")
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

	callback := strings.TrimSpace(os.Getenv("YIBIAO_SSO_CALLBACK_URL"))
	if callback == "" {
		callback = projectLink("yibiao", "http://localhost:5173") + "/api/auth/sso/callback"
	}
	parsed, err := url.Parse(callback)
	if err != nil || !validYibiaoCallback(parsed) {
		response.Error(c, http.StatusServiceUnavailable, "YIBIAO_SSO_CALLBACK_URL is invalid")
		return
	}

	now := time.Now().Unix()
	payload := juSSOTicket{
		Issuer:      "sub2api",
		Audience:    "yibiao-bidmonitor",
		Subject:     strconvInt64(subject.UserID),
		Email:       user.Email,
		Username:    user.Username,
		DisplayName: user.Username,
		AvatarURL:   user.AvatarURL,
		IssuedAt:    now,
		ExpiresAt:   now + 120,
		Nonce:       randomNonce(),
		Next:        safeYibiaoNext(c.Query("next")),
	}
	raw, err := signJuTicket(payload, secret)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Unable to start Yibiao SSO")
		return
	}
	query := parsed.Query()
	query.Set("ticket", raw)
	parsed.RawQuery = query.Encode()
	response.Success(c, gin.H{"redirect_url": parsed.String()})
}

func validYibiaoCallback(u *url.URL) bool {
	if u == nil || u.Hostname() == "" || u.User != nil || u.Opaque != "" || u.RawQuery != "" || u.Fragment != "" || u.ForceQuery || u.Path != "/api/auth/sso/callback" || u.RawPath != "" {
		return false
	}
	local := u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"
	return u.Scheme == "https" || (u.Scheme == "http" && local)
}

func safeYibiaoNext(raw string) string {
	if len(raw) > 2048 || strings.IndexFunc(raw, unicode.IsControl) >= 0 {
		return "/"
	}
	raw = strings.TrimSpace(raw)
	if raw == "" || !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") || strings.Contains(raw, "\\") || strings.ContainsAny(raw, "\r\n") {
		return "/"
	}
	return raw
}
