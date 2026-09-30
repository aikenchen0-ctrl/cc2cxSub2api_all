package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRegistrationChallengeRequiresVerificationBeforeSession(t *testing.T) {
	creates := 0
	verified := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/admin/users":
			creates++
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 43, "email": "new@example.com", "role": "user"}))
		case "/api/v1/auth/login":
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"requires_2fa": true, "temp_token": "temporary-proof"}))
		case "/api/v1/auth/login/2fa":
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Error(err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if payload["temp_token"] != "temporary-proof" {
				t.Error("verification proof not forwarded")
			}
			if payload["totp_code"] != "123456" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"code":"INVALID_TOTP","message":"invalid verification code"}`))
				return
			}
			verified = true
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"access_token": "private-access", "refresh_token": "private-refresh", "user": map[string]any{"id": 43, "email": "new@example.com", "role": "user"}}))
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer upstream.Close()
	s := testServer(t, upstream)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/auth/register", strings.NewReader(`{"email":"new@example.com","password":"password"}`)))
	if rec.Code != 200 || len(rec.Result().Cookies()) != 0 || !strings.Contains(rec.Body.String(), `"requires_2fa":true`) {
		t.Fatalf("challenge incorrectly authenticated: %d %s", rec.Code, rec.Body.String())
	}
	if _, err := s.store.User(s.cfg.AgentID, "43"); err != nil {
		t.Fatalf("main-created membership missing: %v", err)
	}
	rec = httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/auth/login/2fa", strings.NewReader(`{"temp_token":"temporary-proof","totp_code":"000000"}`)))
	if rec.Code != http.StatusUnauthorized || verified || len(rec.Result().Cookies()) != 0 || creates != 1 {
		t.Fatal("incorrect verification authenticated or recreated the account")
	}
	rec = httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/auth/login/2fa", strings.NewReader(`{"temp_token":"temporary-proof","totp_code":"123456"}`)))
	if rec.Code != 200 || !verified || creates != 1 {
		t.Fatalf("verification failed or account recreated: status=%d creates=%d", rec.Code, creates)
	}
	if strings.Contains(rec.Body.String(), "private-") {
		t.Fatal("main credentials leaked")
	}
	var sessionID string
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == s.cfg.CookieName {
			sessionID = cookie.Value
		}
	}
	session, err := s.store.LoadSession(sessionID)
	if err != nil || session.MainUserID != "43" {
		t.Fatalf("verified session not bound: %v", err)
	}
}
