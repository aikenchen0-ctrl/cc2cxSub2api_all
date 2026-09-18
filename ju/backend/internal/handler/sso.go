package handler

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

func validSSOExchangeOrigin(c *gin.Context) bool {
	if c.GetHeader("X-Ju-SSO") != "1" {
		return false
	}
	if site := c.GetHeader("Sec-Fetch-Site"); site != "" && site != "same-origin" {
		return false
	}
	origin, err := url.Parse(c.GetHeader("Origin"))
	scheme := "http"
	if c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	return err == nil && origin.Scheme == scheme && origin.Host == c.Request.Host && origin.User == nil && origin.Path == "" && origin.RawQuery == "" && origin.Fragment == ""
}

func setSSOHandoffCookie(c *gin.Context, value string, maxAge int) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name: "ju_sso_handoff", Value: value, Path: "/api/auth/sso/exchange",
		HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: maxAge,
		Secure: c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https"),
	})
}
