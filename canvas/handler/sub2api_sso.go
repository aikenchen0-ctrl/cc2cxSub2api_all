package handler

import (
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/tigerowo/infinite-canvas/model"
	"github.com/tigerowo/infinite-canvas/service"
)

const ssoCookieName = "canvas_sso_handoff"

func Sub2APISSOCallback(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	payload, err := service.VerifySub2APITicket(r.URL.Query().Get("ticket"), os.Getenv("SUB2API_SSO_SECRET"), time.Now())
	if err != nil {
		http.Redirect(w, r, "/login?error=SSO_failed", http.StatusSeeOther)
		return
	}
	session, err := service.CompleteSub2APISSO(payload)
	if err != nil {
		http.Redirect(w, r, "/login?error=SSO_failed", http.StatusSeeOther)
		return
	}
	setSSOHandoffCookie(w, r, session.Token, 60)
	next := service.SSONext(payload.Next)
	http.Redirect(w, r, "/login?sso=1&redirect="+url.QueryEscape(next), http.StatusSeeOther)
}

func setSSOHandoffCookie(w http.ResponseWriter, r *http.Request, token string, age int) {
	http.SetCookie(w, &http.Cookie{Name: ssoCookieName, Value: token, Path: "/api/auth/sso/exchange", MaxAge: age, HttpOnly: true, Secure: r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"), SameSite: http.SameSiteStrictMode})
}

// A same-origin POST exchanges the short-lived HttpOnly handoff for the normal bearer session.
func Sub2APISSOExchange(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if r.Header.Get("X-Canvas-SSO") != "1" || r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		FailWithStatus(w, http.StatusForbidden, "SSO exchange rejected")
		return
	}
	cookie, err := r.Cookie(ssoCookieName)
	setSSOHandoffCookie(w, r, "", -1)
	if err != nil {
		FailWithStatus(w, http.StatusUnauthorized, "SSO handoff expired")
		return
	}
	user, ok := service.CurrentAuthUser(cookie.Value)
	if !ok {
		FailWithStatus(w, http.StatusUnauthorized, "SSO handoff expired")
		return
	}
	OK(w, model.AuthSession{Token: cookie.Value, User: user})
}
