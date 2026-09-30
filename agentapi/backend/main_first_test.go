package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMainFirstRegistrationWhitelistAndMembership(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/admin/users":
			calls++
			if r.Header.Get("x-api-key") != "test-main-admin-key" {
				t.Error("missing administrator credential")
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body["role"] != "user" || body["balance"] != float64(0) || body["allowed_groups"] != nil {
				t.Errorf("unsafe signup payload: %+v", body)
			}
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 43, "email": "new@example.com", "role": "user"}))
		case "/api/v1/auth/login":
			if calls != 1 || r.Header.Get("x-api-key") != "" {
				t.Error("invalid login ordering or leaked admin key")
			}
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"access_token": "main-token", "user": map[string]any{"id": 43, "email": "new@example.com", "role": "user"}}))
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer upstream.Close()
	s := testServer(t, upstream)
	s.main = NewMainClient(s.cfg)
	req := httptest.NewRequest("POST", "/api/v1/auth/register", strings.NewReader(`{"email":"new@example.com","password":"password","role":"admin","balance":99999,"allowed_groups":[1]}`))
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("signup %d %s", rec.Code, rec.Body.String())
	}
	if _, err := s.store.User(s.cfg.AgentID, "43"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rec.Body.String(), "main-token") || strings.Contains(rec.Body.String(), "test-main-admin-key") {
		t.Fatal("credentials leaked")
	}
}

func TestMainFirstRegistrationBindsAffiliateThroughMappedUserIdentity(t *testing.T) {
	createCalls := 0
	bindCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/admin/users":
			createCalls++
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 43, "email": "new@example.com", "role": "user"}))
		case "/v1/sub2api/affiliate/bind":
			bindCalls++
			if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer app-secret" || r.Header.Get("X-Sub2API-On-Behalf-Of") != "43" || r.Header.Get("X-Sub2API-Satellite") != "agentapi" {
				t.Fatalf("unexpected affiliate bind request: method=%s headers=%#v", r.Method, r.Header)
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["aff_code"] != "INVITER-123" || len(body) != 1 {
				t.Fatalf("unexpected affiliate bind body: %#v err=%v", body, err)
			}
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"bound": true}))
		case "/api/v1/auth/login":
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"access_token": "main-token", "user": map[string]any{"id": 43, "email": "new@example.com", "role": "user"}}))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer upstream.Close()

	s := testServer(t, upstream)
	s.main = NewMainClient(s.cfg)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(`{"email":"new@example.com","password":"password","aff_code":" inviter-123 ","main_user_id":"99"}`))
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("signup status=%d body=%s", rec.Code, rec.Body.String())
	}
	if createCalls != 1 || bindCalls != 1 {
		t.Fatalf("create=%d bind=%d", createCalls, bindCalls)
	}
	if _, err := s.store.User(s.cfg.AgentID, "43"); err != nil {
		t.Fatalf("local tenant membership missing: %v", err)
	}
}

func TestRegistrationRejectsMalformedAffiliateCodeBeforeMainUserCreation(t *testing.T) {
	upstreamCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	defer upstream.Close()
	s := testServer(t, upstream)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(`{"email":"new@example.com","password":"password","aff_code":"bad code!"}`))
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "INVALID_AFFILIATE_CODE") || upstreamCalls != 0 {
		t.Fatalf("status=%d calls=%d body=%s", rec.Code, upstreamCalls, rec.Body.String())
	}
}

func TestOwnerKeyManagementCannotEscapeMembership(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("local key operations must not call upstream") }))
	defer upstream.Close()
	s := testServer(t, upstream)
	if _, err := s.store.UpsertUser(s.cfg.AgentID, "43", "member@example.com", "Member"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		actor, target string
		code          int
	}{{"42", "43", 201}, {"43", "42", 403}, {"42", "999", 404}} {
		session, err := s.store.CreateSession(tc.actor, []byte(`{"id":"`+tc.actor+`"}`), "", "", time.Now().Add(sessionTTL))
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest("POST", "/api/v1/api-keys?main_user_id="+tc.target, strings.NewReader(`{"name":"managed"}`))
		req.AddCookie(&http.Cookie{Name: s.cfg.CookieName, Value: session})
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, req)
		if rec.Code != tc.code {
			t.Fatalf("%s -> %s: %d %s", tc.actor, tc.target, rec.Code, rec.Body.String())
		}
	}
	keys, err := s.store.APIKeys(s.cfg.AgentID, "43")
	if err != nil || len(keys) != 1 {
		t.Fatalf("wrong key owner: %+v %v", keys, err)
	}
	events, err := s.store.AuditEvents(s.cfg.AgentID, 10)
	if err != nil || len(events) != 1 || events[0].ActorType != "agent_admin" || events[0].ActorID != "42" || events[0].Reason != "key_user_id=43" {
		t.Fatalf("missing delegated key audit: %+v %v", events, err)
	}
}

func TestDirectRelayUsesMainRecordWithoutReservationLimit(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "" {
			t.Error("administrator key leaked into model bridge")
		}
		switch r.URL.Path {
		case "/v1/sub2api/balance":
			_ = json.NewEncoder(w).Encode(map[string]any{"balance": 0.0001})
		case "/v1/chat/completions":
			calls++
			_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{}})
		case "/v1/sub2api/usage":
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{map[string]any{"id": 123, "request_id": r.URL.Query().Get("request_id"), "actual_cost": 2.5, "total_cost": 3, "model": "gpt-5.5"}}})
		default:
			w.WriteHeader(404)
		}
	}))
	defer upstream.Close()
	s := testServer(t, upstream)
	s.cfg.BillingMode = "user_upstream"
	s.cfg.MainUsageAPI = true
	s.main = NewMainClient(s.cfg)
	for _, id := range []string{"42", "43"} {
		if _, err := s.store.UpsertUser(s.cfg.AgentID, id, id+"@example.com", id); err != nil {
			t.Fatal(err)
		}
		_, key, err := s.store.CreateAPIKey(s.cfg.AgentID, id, "client")
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.5","messages":[]}`))
		req.Header.Set("Authorization", "Bearer "+key)
		req.Header.Set("Idempotency-Key", "same-client-key")
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("relay %d %s", rec.Code, rec.Body.String())
		}
		usage, err := s.store.Usage(s.cfg.AgentID, id, 10)
		if err != nil || len(usage) != 1 || usage[0].ActualCents != 250 || usage[0].SettlementStatus != "confirmed" {
			t.Fatalf("main record not synced: %+v %v", usage, err)
		}
		replay := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.5","messages":[]}`))
		replay.Header.Set("Authorization", "Bearer "+key)
		replay.Header.Set("Idempotency-Key", "same-client-key")
		s.ServeHTTP(httptest.NewRecorder(), replay)
	}
	if calls != 2 {
		t.Fatalf("expected one main call per user, got %d", calls)
	}
}
