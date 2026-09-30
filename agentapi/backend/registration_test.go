package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRegistrationLimitWindowAndAgentScope(t *testing.T) {
	s := testStore(t)
	now := time.Now()
	s.clock = func() time.Time { return now }
	for i := 0; i < 5; i++ {
		allowed, err := s.AllowRegistration("agent-test", "127.0.0.1", "A@example.com")
		if err != nil || !allowed {
			t.Fatalf("attempt %d: %v %v", i, allowed, err)
		}
	}
	if allowed, err := s.AllowRegistration("agent-test", "127.0.0.2", "a@example.com"); err != nil || allowed {
		t.Fatalf("email limit bypassed: %v %v", allowed, err)
	}
	if allowed, err := s.AllowRegistration("other", "127.0.0.1", "a@example.com"); err != nil || !allowed {
		t.Fatalf("agent scope: %v %v", allowed, err)
	}
	now = now.Add(time.Hour)
	if allowed, err := s.AllowRegistration("agent-test", "127.0.0.1", "a@example.com"); err != nil || !allowed {
		t.Fatalf("window did not reset: %v %v", allowed, err)
	}
}

func TestLostRegistrationResponseRecoversOnlyWithMainProvenance(t *testing.T) {
	for _, valid := range []bool{true, false} {
		t.Run(map[bool]string{true: "matching-marker", false: "other-account"}[valid], func(t *testing.T) {
			marker := ""
			creates := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v1/admin/users":
					creates++
					var body map[string]any
					_ = json.NewDecoder(r.Body).Decode(&body)
					marker, _ = body["notes"].(string)
					if !strings.HasPrefix(marker, "agentapi-registration:") {
						t.Error("missing durable provenance")
					}
					// Main creation committed but the response was lost/malformed.
					w.WriteHeader(502)
				case "/api/v1/auth/login":
					_ = json.NewEncoder(w).Encode(envelope(map[string]any{"access_token": "user-token", "user": map[string]any{"id": 43, "email": "new@example.com", "role": "user"}}))
				case "/api/v1/admin/users/43":
					if r.Header.Get("x-api-key") != "test-main-admin-key" {
						t.Error("recovery must be privileged server-side read")
					}
					notes := marker
					if !valid {
						notes = "another-site-marker"
					}
					_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 43, "email": "new@example.com", "role": "user", "notes": notes}))
				default:
					t.Errorf("unexpected %s", r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer upstream.Close()
			s := testServer(t, upstream)
			s.main = NewMainClient(s.cfg)
			registration := httptest.NewRequest("POST", "/api/v1/auth/register", strings.NewReader(`{"email":"new@example.com","password":"password"}`))
			rec := httptest.NewRecorder()
			s.ServeHTTP(rec, registration)
			if rec.Code == 200 {
				t.Fatal("lost response treated as success")
			}
			if _, err := s.store.User(s.cfg.AgentID, "43"); err != errNotFound {
				t.Fatal("mapping exists before proof")
			}
			login := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(`{"email":"new@example.com","password":"password"}`))
			rec = httptest.NewRecorder()
			s.ServeHTTP(rec, login)
			if valid && rec.Code != 200 {
				t.Fatalf("recovery failed %d %s", rec.Code, rec.Body.String())
			}
			if !valid && rec.Code != 403 {
				t.Fatalf("unrelated main account claimed: %d", rec.Code)
			}
			if creates != 1 {
				t.Fatal("recovery created another user")
			}
			if strings.Contains(rec.Body.String(), marker) || strings.Contains(rec.Body.String(), "user-token") {
				t.Fatal("recovery secret leaked")
			}
		})
	}
}

func TestRegistrationLimitRejectsBeforeMainCall(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("limited signup reached main") }))
	defer upstream.Close()
	s := testServer(t, upstream)
	for i := 0; i < 5; i++ {
		if _, err := s.store.AllowRegistration(s.cfg.AgentID, "192.0.2.1", "new@example.com"); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest("POST", "/api/v1/auth/register", strings.NewReader(`{"email":"new@example.com","password":"password"}`))
	req.Header.Set("X-Forwarded-For", "untrusted")
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != 429 || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("missing limit %d %s", rec.Code, rec.Body.String())
	}
}
