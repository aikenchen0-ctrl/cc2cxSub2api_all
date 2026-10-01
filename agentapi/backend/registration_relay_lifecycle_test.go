package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// This is an HTTP contract test with a simulated authoritative main site,
// not evidence of deployment or real upstream billing.
func TestRegisteredUserKeyBillingAndRevocationLifecycle(t *testing.T) {
	var mu sync.Mutex
	created, modelCalls := false, 0
	records := map[string]bool{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if strings.HasPrefix(r.URL.Path, "/v1/") {
			if r.Header.Get("Authorization") != "Bearer app-secret" || r.Header.Get("X-Sub2API-On-Behalf-Of") != "43" || r.Header.Get("X-Sub2API-Satellite") != "agentapi" || r.Header.Get("x-api-key") != "" {
				t.Error("model/usage request escaped the registered user's satellite identity")
				w.WriteHeader(403)
				return
			}
		}
		switch r.URL.Path {
		case "/api/v1/admin/users":
			if created || r.Header.Get("x-api-key") != "test-main-admin-key" {
				t.Error("duplicate or unauthorized creation")
			}
			created = true
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 43, "role": "user", "email": "lifecycle@example.com"}))
		case "/api/v1/auth/login":
			if !created {
				t.Error("login preceded main creation")
			}
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"access_token": "private-main-token", "user": map[string]any{"id": 43, "role": "user", "email": "lifecycle@example.com"}}))
		case "/v1/sub2api/balance":
			_ = json.NewEncoder(w).Encode(map[string]any{"balance": 10})
		case "/v1/chat/completions":
			modelCalls++
			requestID := r.Header.Get("X-Request-ID")
			if requestID == "" {
				t.Error("missing main request identifier")
			}
			records[requestID] = true
			_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{}})
		case "/v1/sub2api/usage":
			id := r.URL.Query().Get("request_id")
			if !records[id] {
				t.Error("usage was queried without corresponding main request")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{map[string]any{"id": 321, "request_id": id, "actual_cost": 2.5, "total_cost": 3, "model": "gpt-5.5"}}})
		default:
			t.Errorf("unexpected upstream route %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer upstream.Close()
	s := testServer(t, upstream)
	setTenantBillingModeForTest(t, s, "user_upstream")
	s.cfg.MainUsageAPI = true
	s.main = NewMainClient(s.cfg)
	request := func(method, path, body string, cookie *http.Cookie, key string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if cookie != nil {
			r.AddCookie(cookie)
		}
		if key != "" {
			r.Header.Set("Authorization", "Bearer "+key)
			r.Header.Set("Idempotency-Key", "lifecycle-request")
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	signup := request("POST", "/api/v1/auth/register", `{"email":"lifecycle@example.com","password":"password123"}`, nil, "")
	if signup.Code != 200 {
		t.Fatalf("registration: %d %s", signup.Code, signup.Body.String())
	}
	var cookie *http.Cookie
	for _, c := range signup.Result().Cookies() {
		if c.Name == s.cfg.CookieName {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("missing authenticated session")
	}
	if strings.Contains(signup.Body.String(), "private-main-token") {
		t.Fatal("main credential exposed")
	}
	keyResponse := request("POST", "/api/v1/api-keys", `{"name":"lifecycle"}`, cookie, "")
	if keyResponse.Code != 201 {
		t.Fatalf("create key: %d %s", keyResponse.Code, keyResponse.Body.String())
	}
	var keyBody struct {
		Data struct {
			Key  string `json:"key"`
			Item struct {
				ID int64 `json:"id"`
			} `json:"item"`
		} `json:"data"`
	}
	if err := json.Unmarshal(keyResponse.Body.Bytes(), &keyBody); err != nil {
		t.Fatal(err)
	}
	if keyBody.Data.Key == "" || keyBody.Data.Item.ID == 0 {
		t.Fatal("missing created key")
	}
	listed := request("GET", "/api/v1/api-keys", "", cookie, "")
	if listed.Code != 200 || !strings.Contains(listed.Body.String(), keyBody.Data.Key) || !strings.Contains(listed.Header().Get("Cache-Control"), "no-store") {
		t.Fatal("key list failed to return the current user's recoverable key with no-store")
	}
	model := `{"model":"gpt-5.5","messages":[]}`
	if rec := request("POST", "/v1/chat/completions", model, nil, keyBody.Data.Key); rec.Code != 200 {
		t.Fatalf("relay: %d %s", rec.Code, rec.Body.String())
	}
	usage, err := s.store.Usage(s.cfg.AgentID, "43", 10)
	if err != nil || len(usage) != 1 || usage[0].ActualCents != 250 || usage[0].SettlementStatus != "confirmed" {
		t.Fatalf("main cost replica: %+v %v", usage, err)
	}
	ownerUsage, err := s.store.Usage(s.cfg.AgentID, "42", 10)
	if err != nil || len(ownerUsage) != 0 {
		t.Fatal("member usage attributed to owner")
	}
	if rec := request("POST", "/v1/chat/completions", model, nil, keyBody.Data.Key); rec.Code != 409 {
		t.Fatalf("replay status: %d", rec.Code)
	}
	if rec := request("DELETE", fmt.Sprintf("/api/v1/api-keys/%d", keyBody.Data.Item.ID), "", cookie, ""); rec.Code != 200 {
		t.Fatalf("revoke: %d", rec.Code)
	}
	if rec := request("POST", "/v1/chat/completions", model, nil, keyBody.Data.Key); rec.Code != 401 {
		t.Fatalf("revoked key status: %d", rec.Code)
	}
	mu.Lock()
	defer mu.Unlock()
	if modelCalls != 1 || len(records) != 1 {
		t.Fatalf("expected one main billable request, got %d", modelCalls)
	}
}
