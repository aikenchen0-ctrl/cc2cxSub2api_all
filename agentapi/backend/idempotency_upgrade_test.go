package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLegacyIdempotencyBarrierSurvivesNamespacedUpgrade(t *testing.T) {
	for _, header := range []string{"Idempotency-Key", "X-Request-ID"} {
		t.Run(header, func(t *testing.T) {
			for _, state := range []string{"pending", "confirmed"} {
				t.Run(state, func(t *testing.T) {
					calls := 0
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls++
						switch r.URL.Path {
						case "/v1/sub2api/balance":
							_ = json.NewEncoder(w).Encode(map[string]any{"balance": 10})
						case "/v1/chat/completions":
							_, _ = w.Write([]byte(`{"choices":[]}`))
						case "/v1/sub2api/usage":
							_, _ = w.Write([]byte(`{"items":[]}`))
						default:
							w.WriteHeader(404)
						}
					}))
					defer upstream.Close()
					s := testServer(t, upstream)
					s.cfg.BillingMode = "user_upstream"
					s.main = NewMainClient(s.cfg)
					if _, _, err := s.store.PrepareDirectSettlement(s.cfg.AgentID, "42", "42", "old-key", "old-key", 100); err != nil {
						t.Fatal(err)
					}
					if state == "confirmed" {
						if err := s.store.FinalizeSettlement(s.cfg.AgentID, "old-key", "usage-old", "confirmed", 25, ""); err != nil {
							t.Fatal(err)
						}
					}
					for _, userID := range []string{"42", "43"} {
						if _, err := s.store.UpsertUser(s.cfg.AgentID, userID, userID+"@example.com", userID); err != nil {
							t.Fatal(err)
						}
						_, key, err := s.store.CreateAPIKey(s.cfg.AgentID, userID, "upgrade")
						if err != nil {
							t.Fatal(err)
						}
						req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.5","messages":[]}`))
						req.Header.Set("Authorization", "Bearer "+key)
						req.Header.Set(header, "old-key")
						rec := httptest.NewRecorder()
						s.ServeHTTP(rec, req)
						if userID == "42" && (rec.Code != 409 || calls != 0) {
							t.Fatalf("legacy retry forwarded: status=%d calls=%d", rec.Code, calls)
						}
						if userID == "43" && rec.Code != 200 {
							t.Fatalf("other user blocked by legacy key: %d %s", rec.Code, rec.Body.String())
						}
					}
					old, err := s.store.Settlement(s.cfg.AgentID, "old-key")
					if err != nil || old.Status != state {
						t.Fatalf("legacy record changed: %+v %v", old, err)
					}
				})
			}
		})
	}
}

func TestOversizedRetryIdentifierDoesNotReachMain(t *testing.T) {
	for _, header := range []string{"Idempotency-Key", "X-Request-ID"} {
		t.Run(header, func(t *testing.T) {
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer upstream.Close()
			s := testServer(t, upstream)
			s.cfg.BillingMode = "user_upstream"
			s.main = NewMainClient(s.cfg)
			if _, err := s.store.UpsertUser(s.cfg.AgentID, "42", "42@example.com", "42"); err != nil {
				t.Fatal(err)
			}
			_, key, err := s.store.CreateAPIKey(s.cfg.AgentID, "42", "identifier test")
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.5","messages":[]}`))
			req.Header.Set("Authorization", "Bearer "+key)
			req.Header.Set(header, strings.Repeat("x", 129))
			rec := httptest.NewRecorder()
			s.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest || calls != 0 || !strings.Contains(rec.Body.String(), "INVALID_REQUEST_IDENTIFIER") {
				t.Fatalf("status=%d upstream calls=%d body=%s", rec.Code, calls, rec.Body.String())
			}
		})
	}
}

func TestRequestIDIsScopedToUserAndReplaysDoNotRelay(t *testing.T) {
	modelIDs := map[string]string{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/sub2api/balance":
			_, _ = w.Write([]byte(`{"balance":10}`))
		case "/v1/chat/completions":
			user := r.Header.Get("X-Sub2API-On-Behalf-Of")
			if _, exists := modelIDs[user]; exists {
				t.Error("duplicate relay for user")
			}
			modelIDs[user] = r.Header.Get("X-Request-ID")
			_, _ = w.Write([]byte(`{"choices":[]}`))
		case "/v1/sub2api/usage":
			_, _ = w.Write([]byte(`{"items":[]}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer upstream.Close()
	s := testServer(t, upstream)
	s.cfg.BillingMode = "user_upstream"
	s.main = NewMainClient(s.cfg)
	for _, user := range []string{"42", "43"} {
		if _, err := s.store.UpsertUser(s.cfg.AgentID, user, user+"@example.com", user); err != nil {
			t.Fatal(err)
		}
		_, key, err := s.store.CreateAPIKey(s.cfg.AgentID, user, "request ID test")
		if err != nil {
			t.Fatal(err)
		}
		for attempt := 0; attempt < 2; attempt++ {
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.5","messages":[]}`))
			req.Header.Set("Authorization", "Bearer "+key)
			req.Header.Set("X-Request-ID", "shared-client-request")
			rec := httptest.NewRecorder()
			s.ServeHTTP(rec, req)
			want := 200
			if attempt == 1 {
				want = 409
			}
			if rec.Code != want {
				t.Fatalf("user=%s attempt=%d status=%d body=%s", user, attempt, rec.Code, rec.Body.String())
			}
		}
	}
	if len(modelIDs) != 2 || modelIDs["42"] == modelIDs["43"] {
		t.Fatalf("unscoped upstream request IDs: %v", modelIDs)
	}
}
