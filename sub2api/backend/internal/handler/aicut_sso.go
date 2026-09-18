package handler

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

// AicutSSOStart issues a short-lived identity handoff for OpenChatCut.
// The browser receives only the signed one-time ticket; Sub2API tokens and
// relay keys remain server-side.
func (h *AuthHandler) AicutSSOStart(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Referrer-Policy", "no-referrer")
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	secret := strings.TrimSpace(os.Getenv("SUB2API_SSO_SECRET"))
	if len(secret) < 32 {
		response.Error(c, http.StatusServiceUnavailable, "OpenChatCut SSO is not configured")
		return
	}
	if h == nil || h.userService == nil {
		response.Error(c, http.StatusServiceUnavailable, "OpenChatCut SSO is unavailable")
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
	callback := strings.TrimSpace(os.Getenv("OPENCHATCUT_SSO_CALLBACK_URL"))
	if callback == "" {
		callback = "http://localhost:5199/api/auth/sso/callback"
	}
	parsed, err := url.Parse(callback)
	if err != nil || !validAicutCallback(parsed) {
		response.Error(c, http.StatusServiceUnavailable, "OPENCHATCUT_SSO_CALLBACK_URL is invalid")
		return
	}
	if h.apiKeyService == nil {
		response.Error(c, http.StatusServiceUnavailable, "OpenChatCut relay key service is unavailable")
		return
	}
	relayKey, err := h.apiKeyService.GetOrCreateSuperAPIKey(c.Request.Context(), subject.UserID)
	if err != nil || relayKey == nil || strings.TrimSpace(relayKey.Key) == "" {
		response.Error(c, http.StatusServiceUnavailable, "Unable to provision OpenChatCut relay key")
		return
	}

	now := time.Now()
	payload := juSSOTicket{
		Issuer: "sub2api", Audience: "openchatcut", Subject: strconvInt64(subject.UserID),
		Email: user.Email, Username: user.Username, DisplayName: user.Username, AvatarURL: user.AvatarURL,
		IssuedAt: now.Unix(), ExpiresAt: now.Add(2 * time.Minute).Unix(),
		Nonce: randomNonce(), Next: safeJuNext(c.Query("next")),
	}
	payload.RelayKeyCiphertext, err = encryptRelayKey(relayKey.Key, secret)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Unable to protect OpenChatCut relay key")
		return
	}
	raw, err := signJuTicket(payload, secret)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Unable to start OpenChatCut SSO")
		return
	}
	query := parsed.Query()
	query.Set("ticket", raw)
	parsed.RawQuery = query.Encode()
	response.Success(c, gin.H{"redirect_url": parsed.String()})
}

func encryptRelayKey(relayKey, secret string) (string, error) {
	key := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nil, nonce, []byte(relayKey), nil)
	return base64.RawURLEncoding.EncodeToString(append(nonce, sealed...)), nil
}

func validAicutCallback(u *url.URL) bool {
	if u == nil || u.Hostname() == "" || u.User != nil || u.Opaque != "" ||
		u.RawQuery != "" || u.Fragment != "" || u.ForceQuery ||
		u.Path != "/api/auth/sso/callback" || u.RawPath != "" {
		return false
	}
	local := u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"
	return u.Scheme == "https" || (u.Scheme == "http" && local)
}
