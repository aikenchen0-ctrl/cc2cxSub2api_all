package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const browserSessionCookieDefaultAge = int(service.BrowserSessionTTL / time.Second)

const legacyBrowserAccessTokenCookieName = "sub2api_access_token"

// setBrowserSessionCookie stores only an opaque random id in the browser. The
// corresponding access JWT remains in the server-side Redis session store.
func setBrowserSessionCookie(c *gin.Context, authService *service.AuthService, token string, _ int) {
	if c == nil {
		return
	}
	clearLegacyBrowserAccessTokenCookie(c)
	if authService == nil || strings.TrimSpace(token) == "" {
		return
	}
	sessionID, issuedTTL, err := authService.IssueBrowserSession(c.Request.Context(), token, service.BrowserSessionTTL)
	if err != nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	maxAge := int(issuedTTL / time.Second)
	if maxAge <= 0 {
		maxAge = browserSessionCookieDefaultAge
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     middleware.BrowserSessionCookieName,
		Value:    sessionID,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   isRequestHTTPS(c),
		SameSite: http.SameSiteLaxMode,
	})
}

func clearBrowserSessionCookie(c *gin.Context, authService *service.AuthService) {
	if c == nil {
		return
	}
	clearLegacyBrowserAccessTokenCookie(c)
	if cookie, err := c.Cookie(middleware.BrowserSessionCookieName); err == nil && authService != nil {
		_ = authService.DeleteBrowserSession(c.Request.Context(), cookie)
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     middleware.BrowserSessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   isRequestHTTPS(c),
		SameSite: http.SameSiteLaxMode,
	})
}

func clearLegacyBrowserAccessTokenCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     legacyBrowserAccessTokenCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   isRequestHTTPS(c),
		SameSite: http.SameSiteLaxMode,
	})
}
