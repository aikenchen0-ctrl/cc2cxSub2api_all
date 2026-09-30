package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRegistrationDoesNotExposeAdministratorDiagnostics(t *testing.T) {
	for _, tc := range []struct{ upstream, want int }{{200, 503}, {400, 400}, {401, 503}, {403, 503}, {409, 409}, {422, 400}, {429, 429}, {500, 503}} {
		t.Run(http.StatusText(tc.upstream), func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/admin/users" {
					t.Error("failed creation must not continue to login")
				}
				w.WriteHeader(tc.upstream)
				_, _ = w.Write([]byte(`{"code":"private-code","message":"private-key internal-host","data":{"id":43}}`))
			}))
			defer upstream.Close()
			s := testServer(t, upstream)
			req := httptest.NewRequest("POST", "/api/v1/auth/register", strings.NewReader(`{"email":"new@example.com","password":"password"}`))
			rec := httptest.NewRecorder()
			s.ServeHTTP(rec, req)
			if rec.Code != tc.want || strings.Contains(rec.Body.String(), "private-") || strings.Contains(rec.Body.String(), "internal-host") || len(rec.Result().Cookies()) != 0 {
				t.Fatalf("unsafe registration error: %d %s", rec.Code, rec.Body.String())
			}
			if _, err := s.store.User(s.cfg.AgentID, "43"); err != errNotFound {
				t.Fatalf("failed creation mapped user: %v", err)
			}
		})
	}
}
