package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRegistrationRejectsUnexpectedCreatedIdentity(t *testing.T) {
	for _, profile := range []string{
		`{"id":43,"email":"other@example.com","role":"user"}`,
		`{"id":43,"email":"new@example.com","role":"admin"}`,
		`{"id":0,"email":"new@example.com","role":"user"}`,
		`{"id":-1,"email":"new@example.com","role":"user"}`,
		`{"id":43.5,"email":"new@example.com","role":"user"}`,
		`{"id":"../42","email":"new@example.com","role":"user"}`,
		`{"id":43,"role":"user"}`,
	} {
		t.Run(profile, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/admin/users" {
					t.Error("unexpected login after invalid identity")
				}
				_ = json.NewEncoder(w).Encode(envelope(json.RawMessage(profile)))
			}))
			defer upstream.Close()
			s := testServer(t, upstream)
			req := httptest.NewRequest("POST", "/api/v1/auth/register", strings.NewReader(`{"email":"new@example.com","password":"password"}`))
			rec := httptest.NewRecorder()
			s.ServeHTTP(rec, req)
			if rec.Code != 502 || !strings.Contains(rec.Body.String(), "UPSTREAM_IDENTITY_MISMATCH") || len(rec.Result().Cookies()) != 0 {
				t.Fatalf("identity accepted: %d %s", rec.Code, rec.Body.String())
			}
			if _, err := s.store.User(s.cfg.AgentID, "43"); err != errNotFound {
				t.Fatalf("invalid response created membership: %v", err)
			}
		})
	}
}

func TestRecoveryRejectsEmailMismatchDespiteMatchingMarker(t *testing.T) {
	marker := ""
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 43, "email": "other@example.com", "role": "user", "notes": marker}))
	}))
	defer upstream.Close()
	s := testServer(t, upstream)
	var err error
	marker, err = s.store.RegistrationMarker(s.cfg.AgentID, "new@example.com")
	if err != nil {
		t.Fatal(err)
	}
	ok, err := s.recoverRegistrationForTenant(t.Context(), s.cfg.AgentID, "43", "new@example.com", "New")
	if err != nil || ok {
		t.Fatalf("mismatched recovery: %v %v", ok, err)
	}
	if _, err := s.store.User(s.cfg.AgentID, "43"); err != errNotFound {
		t.Fatalf("mismatched account mapped: %v", err)
	}
}

func TestMainUserIDPreservesIntegerPrecision(t *testing.T) {
	for _, data := range []string{`{"id":9007199254740993}`, `{"id":"9007199254740993"}`, `{"user_id":9007199254740993}`, `{"uid":9007199254740993}`} {
		if got := mainUserIDFromJSON([]byte(data)); got != "9007199254740993" {
			t.Fatalf("identity rounded: %s => %s", data, got)
		}
	}
}

func TestRegistrationPreservesLargeMainIdentityThroughSession(t *testing.T) {
	const id = "9007199254740993"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/admin/users":
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":9007199254740993,"email":"new@example.com","role":"user"}}`))
		case "/api/v1/auth/login":
			_, _ = w.Write([]byte(`{"code":0,"data":{"access_token":"test-token","user":{"id":9007199254740993,"email":"new@example.com","role":"user"}}}`))
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer upstream.Close()
	s := testServer(t, upstream)
	req := httptest.NewRequest("POST", "/api/v1/auth/register", strings.NewReader(`{"email":"new@example.com","password":"password"}`))
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != 200 || len(rec.Result().Cookies()) == 0 {
		t.Fatalf("registration failed: %d %s", rec.Code, rec.Body.String())
	}
	if _, err := s.store.User(s.cfg.AgentID, id); err != nil {
		t.Fatalf("exact identity missing: %v", err)
	}
	if _, err := s.store.User(s.cfg.AgentID, "9007199254740992"); err != errNotFound {
		t.Fatalf("rounded identity was mapped: %v", err)
	}
	var cookie *http.Cookie
	for _, candidate := range rec.Result().Cookies() {
		if candidate.Name == s.cfg.CookieName {
			cookie = candidate
		}
	}
	if cookie == nil {
		t.Fatal("session cookie missing")
	}
	session, err := s.store.LoadSession(cookie.Value)
	if err != nil || session.MainUserID != id {
		t.Fatalf("session identity mismatch: %s %v", session.MainUserID, err)
	}
}
