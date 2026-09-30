package service

import (
	"net/http"
	"strings"
)

const SessionCookieName = "canvas_session"

func SetSessionCookie(w http.ResponseWriter, r *http.Request, token string, age int) {
	http.SetCookie(w, &http.Cookie{Name: SessionCookieName, Value: token, Path: "/", MaxAge: age, HttpOnly: true, Secure: r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"), SameSite: http.SameSiteLaxMode})
}

func SameOriginSessionRequest(r *http.Request) bool {
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return false
	}
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	origin := r.Header.Get("Origin")
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	return origin == "" || origin == scheme+"://"+host
}
