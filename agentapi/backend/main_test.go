package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

func makeSSOTicket(t *testing.T, secret string, payload map[string]any) string {
	t.Helper()
	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(encodedPayload)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(encoded))
	return encoded + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func testServer(t *testing.T, upstream *httptest.Server) *Server {
	t.Helper()
	cfg := Config{
		Addr:                ":0",
		DatabasePath:        ":memory:",
		MainAPIBaseURL:      upstream.URL + "/api/v1",
		MainModelBaseURL:    upstream.URL + "/v1",
		AdminKey:            "admin-secret",
		AppCredential:       "app-secret",
		SatelliteSlug:       "agentapi",
		SessionSecret:       "session-secret",
		CookieName:          "agentapi_session",
		AgentID:             "agent-test",
		AgentName:           "Agent Test",
		SiteName:            "Agent Test",
		OwnerMainUserID:     "42",
		MaxRequestCostCents: 100,
		MainRequestTimeout:  0,
	}
	// http.Client treats a zero timeout as no timeout; use the normal value for
	// the production client path.
	cfg.MainRequestTimeout = 10 * time.Second
	store, err := OpenStore(":memory:", cfg.SessionSecret)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAgent(cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertUser(cfg.AgentID, "42", "u@example.com", "User"); err != nil {
		t.Fatal(err)
	}
	s := &Server{cfg: cfg, store: store, main: NewMainClient(cfg), webDir: "", settleByUser: map[string]*sync.Mutex{}}
	t.Cleanup(func() { _ = store.Close() })
	return s
}

func envelope(data any) map[string]any {
	return map[string]any{"code": 0, "message": "success", "data": data}
}

func TestLoginUsesHttpOnlyAgentSession(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/login" {
			t.Fatalf("unexpected upstream path: %s", r.URL.Path)
		}
		w.Header().Set("Set-Cookie", "main_secret=must-not-leak")
		_ = json.NewEncoder(w).Encode(envelope(map[string]any{
			"access_token": "main-access-secret", "refresh_token": "main-refresh-secret", "expires_in": 3600,
			"token_type": "Bearer", "user": map[string]any{"id": 42, "email": "u@example.com", "role": "user"},
		}))
	}))
	defer upstream.Close()
	server := testServer(t, upstream)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"u@example.com","password":"password"}`))
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "main-access-secret") || strings.Contains(rec.Body.String(), "main-refresh-secret") {
		t.Fatalf("main credentials leaked: %s", rec.Body.String())
	}
	cookie := rec.Result().Cookies()
	if len(cookie) != 1 || !cookie[0].HttpOnly || cookie[0].Name != "agentapi_session" {
		t.Fatalf("unexpected session cookie: %#v", cookie)
	}

	// The upstream test server only implements login; /auth/me is not called by
	// this assertion, proving login itself does not expose a token.
}

func TestSSOCallbackCreatesLocalSessionAndConsumesTicketOnce(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("SSO session should not call the main auth API: %s", r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	server.cfg.SSOSecret = strings.Repeat("s", 32)
	server.cfg.SSOAudience = "agentapi"
	now := time.Now().UTC().Unix()
	ticket := makeSSOTicket(t, server.cfg.SSOSecret, map[string]any{
		"iss": "sub2api", "aud": "agentapi", "sub": "42", "jti": "sso-once",
		"iat": now, "exp": now + 60, "email": "u@example.com", "username": "SSO user", "next": "/dashboard",
	})
	request := httptest.NewRequest(http.MethodGet, "/api/auth/sso/callback?ticket="+url.QueryEscape(ticket), nil)
	first := httptest.NewRecorder()
	server.ServeHTTP(first, request)
	if first.Code != http.StatusFound || first.Header().Get("Location") != "/dashboard" {
		t.Fatalf("SSO callback status=%d headers=%v body=%s", first.Code, first.Header(), first.Body.String())
	}
	cookies := first.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].MaxAge != int(sessionTTL.Seconds()) {
		t.Fatalf("unexpected SSO session cookie: %#v", cookies)
	}
	me := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	me.AddCookie(cookies[0])
	meResponse := httptest.NewRecorder()
	server.ServeHTTP(meResponse, me)
	if meResponse.Code != http.StatusOK || strings.Contains(meResponse.Body.String(), "access_token") {
		t.Fatalf("SSO auth/me leaked credentials or failed: status=%d body=%s", meResponse.Code, meResponse.Body.String())
	}
	replay := httptest.NewRecorder()
	server.ServeHTTP(replay, httptest.NewRequest(http.MethodGet, "/api/auth/sso/callback?ticket="+url.QueryEscape(ticket), nil))
	if replay.Code != http.StatusConflict || !strings.Contains(replay.Body.String(), "SSO_TICKET_REPLAYED") {
		t.Fatalf("SSO replay status=%d body=%s", replay.Code, replay.Body.String())
	}
}

func TestReadyEndpointChecksAgentAndModelCredential(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)

	ready := httptest.NewRecorder()
	server.ServeHTTP(ready, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if ready.Code != http.StatusOK {
		t.Fatalf("ready status=%d body=%s", ready.Code, ready.Body.String())
	}
	server.cfg.AppCredential = ""
	notReady := httptest.NewRecorder()
	server.ServeHTTP(notReady, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if notReady.Code != http.StatusServiceUnavailable {
		t.Fatalf("missing credential ready status=%d body=%s", notReady.Code, notReady.Body.String())
	}
}

func TestModelRelayAddsSatelliteHeadersAndNeverCopiesCookies(t *testing.T) {
	var gotHeaders http.Header
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/auth/login" {
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{
				"access_token": "main-access", "refresh_token": "main-refresh", "expires_in": 3600,
				"user": map[string]any{"id": 42, "email": "u@example.com", "role": "user"},
			}))
			return
		}
		if r.URL.Path == "/api/v1/admin/users/42" {
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 42, "email": "u@example.com", "balance": 10.0}))
			return
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected model path: %s", r.URL.Path)
		}
		gotHeaders = r.Header.Clone()
		w.Header().Set("Set-Cookie", "upstream=secret")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"completion-1"}`)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	if err := server.store.CreditAgent(server.cfg.AgentID, 1000, "seed", "seed", "test"); err != nil {
		t.Fatal(err)
	}
	if err := server.store.Allocate(server.cfg.AgentID, "42", 500, "allocate", "order", "test"); err != nil {
		t.Fatal(err)
	}

	login := httptest.NewRecorder()
	server.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"u@example.com","password":"password"}`)))
	if login.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", login.Code, login.Body.String())
	}
	cookie := login.Result().Cookies()[0]
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.5","messages":[]}`))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || strings.Contains(strings.ToLower(rec.Header().Get("Set-Cookie")), "upstream") {
		t.Fatalf("relay status=%d headers=%v body=%s", rec.Code, rec.Header(), rec.Body.String())
	}
	if gotHeaders.Get("Authorization") != "Bearer app-secret" || gotHeaders.Get("X-Sub2API-On-Behalf-Of") != "42" || gotHeaders.Get("X-Sub2API-Satellite") != "agentapi" {
		t.Fatalf("missing relay headers: %v", gotHeaders)
	}
	user, err := server.store.User(server.cfg.AgentID, "42")
	if err != nil {
		t.Fatal(err)
	}
	if user.BalanceCents != 400 {
		t.Fatalf("reservation was not charged: %+v", user)
	}
}

func TestModelsEndpointExposesOnlyTheAgentPublicCatalog(t *testing.T) {
	upstreamCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		t.Fatalf("models listing must not call upstream: %s", r.URL.Path)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	sessionID, err := server.store.CreateSession("42", []byte(`{"id":"42","email":"u@example.com"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("models status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Object string `json:"object"`
		Data   []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Object != "list" || len(payload.Data) != len(publicModelCatalog) {
		t.Fatalf("unexpected public model response: object=%q count=%d", payload.Object, len(payload.Data))
	}
	for _, item := range payload.Data {
		if _, ok := publicModelNames[item.ID]; !ok {
			t.Fatalf("private or unknown model leaked: %q", item.ID)
		}
	}
	if upstreamCalls != 0 {
		t.Fatalf("unexpected upstream calls: %d", upstreamCalls)
	}
}

func TestRechargeWebhookRequiresSignatureAndAllocatesVerifiedOrder(t *testing.T) {
	adminReads := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/admin/users/42" {
			t.Fatalf("unexpected upstream path: %s", r.URL.Path)
		}
		adminReads++
		_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 42, "email": "u@example.com", "balance": 100.0}))
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	server.cfg.PaymentEnabled = true
	server.cfg.PaymentProvider = "manual"
	server.cfg.PaymentCurrency = "CNY"
	server.cfg.PaymentWebhookSecret = "payment-secret"
	server.cfg.PaymentMinCents = 100
	server.cfg.PaymentMaxCents = 100000
	server.cfg.PaymentOrderTTL = 30 * time.Minute

	sessionID, err := server.store.CreateSession("42", []byte(`{"id":"42","email":"u@example.com"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	create := httptest.NewRequest(http.MethodPost, "/api/v1/agent/recharge/orders", strings.NewReader(`{"amount":"1.00"}`))
	create.Header.Set("Idempotency-Key", "recharge-test-1")
	create.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	created := httptest.NewRecorder()
	server.ServeHTTP(created, create)
	if created.Code != http.StatusCreated {
		t.Fatalf("create order status=%d body=%s", created.Code, created.Body.String())
	}
	var createdEnvelope struct {
		Data struct {
			Order RechargeOrder `json:"order"`
		} `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &createdEnvelope); err != nil {
		t.Fatal(err)
	}
	order := createdEnvelope.Data.Order
	if order.OrderNo == "" || order.Status != "pending" || order.AmountCents != 100 {
		t.Fatalf("unexpected order: %+v", order)
	}

	body := []byte(fmt.Sprintf(`{"order_no":%q,"status":"paid","amount_cents":100,"currency":"CNY","provider_trade_no":"trade-1"}`, order.OrderNo))
	bad := httptest.NewRequest(http.MethodPost, "/api/v1/payments/webhook", bytes.NewReader(body))
	bad.Header.Set("X-Agent-Payment-Event-ID", "evt-bad")
	bad.Header.Set("X-Agent-Payment-Signature", "sha256=00")
	badResponse := httptest.NewRecorder()
	server.ServeHTTP(badResponse, bad)
	if badResponse.Code != http.StatusUnauthorized {
		t.Fatalf("bad signature status=%d body=%s", badResponse.Code, badResponse.Body.String())
	}

	mac := hmac.New(sha256.New, []byte(server.cfg.PaymentWebhookSecret))
	_, _ = mac.Write(body)
	valid := httptest.NewRequest(http.MethodPost, "/api/v1/payments/webhook", bytes.NewReader(body))
	valid.Header.Set("X-Agent-Payment-Event-ID", "evt-1")
	valid.Header.Set("X-Agent-Payment-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	validResponse := httptest.NewRecorder()
	server.ServeHTTP(validResponse, valid)
	if validResponse.Code != http.StatusOK {
		t.Fatalf("valid webhook status=%d body=%s", validResponse.Code, validResponse.Body.String())
	}
	allocated, err := server.store.RechargeOrder(server.cfg.AgentID, order.OrderNo)
	if err != nil {
		t.Fatal(err)
	}
	if allocated.Status != "allocated" || adminReads == 0 {
		t.Fatalf("order was not allocated after owner sync: %+v reads=%d", allocated, adminReads)
	}
	user, err := server.store.User(server.cfg.AgentID, "42")
	if err != nil {
		t.Fatal(err)
	}
	if user.BalanceCents != 100 {
		t.Fatalf("verified recharge did not credit user sub-balance: %+v", user)
	}

	duplicate := httptest.NewRequest(http.MethodPost, "/api/v1/payments/webhook", bytes.NewReader(body))
	duplicate.Header.Set("X-Agent-Payment-Event-ID", "evt-1")
	duplicate.Header.Set("X-Agent-Payment-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	duplicateResponse := httptest.NewRecorder()
	server.ServeHTTP(duplicateResponse, duplicate)
	if duplicateResponse.Code != http.StatusOK || !strings.Contains(duplicateResponse.Body.String(), `"duplicate":true`) {
		t.Fatalf("duplicate webhook was not idempotent: status=%d body=%s", duplicateResponse.Code, duplicateResponse.Body.String())
	}
}

func TestPaidRechargeWaitsForOwnerCreditAndAdminCanRetryAllocation(t *testing.T) {
	adminReads := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/admin/users/42" {
			t.Fatalf("unexpected upstream path: %s", r.URL.Path)
		}
		adminReads++
		balance := 0.0
		if adminReads > 1 {
			balance = 100.0
		}
		_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 42, "email": "u@example.com", "balance": balance}))
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	server.cfg.PaymentEnabled = true
	server.cfg.PaymentProvider = "manual"
	server.cfg.PaymentCurrency = "CNY"
	server.cfg.PaymentWebhookSecret = "payment-secret"
	server.cfg.PaymentMinCents = 100
	server.cfg.PaymentMaxCents = 100000
	server.cfg.PaymentOrderTTL = 30 * time.Minute
	sessionID, err := server.store.CreateSession("42", []byte(`{"id":"42","email":"u@example.com"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	create := httptest.NewRequest(http.MethodPost, "/api/v1/agent/recharge/orders", strings.NewReader(`{"amount":"1.00"}`))
	create.Header.Set("Idempotency-Key", "recharge-pending-1")
	create.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	created := httptest.NewRecorder()
	server.ServeHTTP(created, create)
	if created.Code != http.StatusCreated {
		t.Fatalf("create order status=%d body=%s", created.Code, created.Body.String())
	}
	var createdEnvelope struct {
		Data struct {
			Order RechargeOrder `json:"order"`
		} `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &createdEnvelope); err != nil {
		t.Fatal(err)
	}
	order := createdEnvelope.Data.Order
	body := []byte(fmt.Sprintf(`{"order_no":%q,"status":"paid","amount_cents":100,"currency":"CNY","provider_trade_no":"trade-pending"}`, order.OrderNo))
	mac := hmac.New(sha256.New, []byte(server.cfg.PaymentWebhookSecret))
	_, _ = mac.Write(body)
	webhook := httptest.NewRequest(http.MethodPost, "/api/v1/payments/webhook", bytes.NewReader(body))
	webhook.Header.Set("X-Agent-Payment-Event-ID", "evt-pending")
	webhook.Header.Set("X-Agent-Payment-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, webhook)
	if response.Code != http.StatusAccepted {
		t.Fatalf("pending webhook status=%d body=%s", response.Code, response.Body.String())
	}
	pending, err := server.store.RechargeOrder(server.cfg.AgentID, order.OrderNo)
	if err != nil || pending.Status != "paid_pending_allocation" {
		t.Fatalf("expected durable pending allocation order: %+v err=%v", pending, err)
	}

	retry := httptest.NewRequest(http.MethodPost, "/api/v1/agent/admin/recharge/allocate", strings.NewReader(fmt.Sprintf(`{"order_no":%q}`, order.OrderNo)))
	retry.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	retried := httptest.NewRecorder()
	server.ServeHTTP(retried, retry)
	if retried.Code != http.StatusOK {
		t.Fatalf("admin allocation status=%d body=%s", retried.Code, retried.Body.String())
	}
	allocated, err := server.store.RechargeOrder(server.cfg.AgentID, order.OrderNo)
	if err != nil || allocated.Status != "allocated" {
		t.Fatalf("admin retry did not allocate order: %+v err=%v", allocated, err)
	}
}

func TestAgentAPIKeyCanRelayWithoutCookieAndIsNotForwarded(t *testing.T) {
	var gotHeaders http.Header
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/login":
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{
				"access_token": "main-access", "refresh_token": "main-refresh", "expires_in": 3600,
				"user": map[string]any{"id": 42, "email": "u@example.com", "role": "user"},
			}))
		case "/api/v1/admin/users/42":
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 42, "email": "u@example.com", "balance": 10.0}))
		case "/v1/chat/completions":
			gotHeaders = r.Header.Clone()
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":"completion-key"}`)
		default:
			t.Errorf("unexpected upstream path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	if err := server.store.CreditAgent(server.cfg.AgentID, 1000, "seed", "seed", "test"); err != nil {
		t.Fatal(err)
	}
	if err := server.store.Allocate(server.cfg.AgentID, "42", 500, "allocate", "order", "test"); err != nil {
		t.Fatal(err)
	}

	login := httptest.NewRecorder()
	server.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"u@example.com","password":"password"}`)))
	if login.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", login.Code, login.Body.String())
	}
	cookie := login.Result().Cookies()[0]
	create := httptest.NewRequest(http.MethodPost, "/api/v1/api-keys", strings.NewReader(`{"name":"CLI"}`))
	create.AddCookie(cookie)
	created := httptest.NewRecorder()
	server.ServeHTTP(created, create)
	if created.Code != http.StatusCreated {
		t.Fatalf("create key status=%d body=%s", created.Code, created.Body.String())
	}
	var createdEnvelope struct {
		Data struct {
			Key string `json:"key"`
		} `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &createdEnvelope); err != nil {
		t.Fatal(err)
	}
	if createdEnvelope.Data.Key == "" {
		t.Fatalf("one-time key missing: %s", created.Body.String())
	}

	relay := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.5","messages":[]}`))
	relay.Header.Set("Authorization", "Bearer "+createdEnvelope.Data.Key)
	relayResponse := httptest.NewRecorder()
	server.ServeHTTP(relayResponse, relay)
	if relayResponse.Code != http.StatusOK {
		t.Fatalf("key relay status=%d body=%s", relayResponse.Code, relayResponse.Body.String())
	}
	if gotHeaders.Get("Authorization") != "Bearer app-secret" || gotHeaders.Get("X-Sub2API-On-Behalf-Of") != "42" || gotHeaders.Get("X-Sub2API-Satellite") != "agentapi" {
		t.Fatalf("unexpected upstream identity headers: %v", gotHeaders)
	}
	if strings.Contains(gotHeaders.Get("Authorization"), createdEnvelope.Data.Key) {
		t.Fatalf("local AgentAPI key leaked upstream: %v", gotHeaders)
	}
	user, err := server.store.User(server.cfg.AgentID, "42")
	if err != nil {
		t.Fatal(err)
	}
	if user.BalanceCents != 400 {
		t.Fatalf("key relay did not charge local wallet: %+v", user)
	}
}

func TestModelRelayUsesOwnerForUpstreamChargeButProxyForLocalLedger(t *testing.T) {
	var modelHeader http.Header
	adminReads := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/admin/users/99":
			adminReads++
			balance := 10.0
			if adminReads > 1 {
				balance = 9.0
			}
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 99, "email": "owner@example.com", "balance": balance}))
		case "/v1/chat/completions":
			modelHeader = r.Header.Clone()
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":"owner-billed"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	server.cfg.OwnerMainUserID = "99"
	server.main = NewMainClient(server.cfg)
	if err := server.store.CreditAgent(server.cfg.AgentID, 1000, "seed-owner-test", "seed", "test"); err != nil {
		t.Fatal(err)
	}
	if err := server.store.Allocate(server.cfg.AgentID, "42", 500, "allocate-owner-test", "order", "test"); err != nil {
		t.Fatal(err)
	}
	_, key, err := server.store.CreateAPIKey(server.cfg.AgentID, "42", "owner billing")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.5","messages":[]}`))
	req.Header.Set("Authorization", "Bearer "+key)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("relay status=%d body=%s", response.Code, response.Body.String())
	}
	if modelHeader.Get("X-Sub2API-On-Behalf-Of") != "99" {
		t.Fatalf("upstream was not charged to owner: %v", modelHeader)
	}
	user, err := server.store.User(server.cfg.AgentID, "42")
	if err != nil {
		t.Fatal(err)
	}
	if user.BalanceCents != 400 {
		t.Fatalf("proxy user's local reservation was not charged: %+v", user)
	}
	usage, err := server.store.Usage(server.cfg.AgentID, "42", 10)
	if err != nil || len(usage) != 1 || usage[0].SettlementStatus != "confirmed" || usage[0].ActualCents != 100 {
		t.Fatalf("unexpected owner settlement: %+v err=%v", usage, err)
	}
}

func TestStreamingModelRelayForwardsChunksAndSettlesAfterEOF(t *testing.T) {
	adminReads := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/admin/users/42":
			adminReads++
			balance := 10.0
			if adminReads > 1 {
				balance = 9.0
			}
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 42, "balance": balance}))
		case "/v1/chat/completions":
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, _ := w.(http.Flusher)
			_, _ = io.WriteString(w, "data: one\n\n")
			if flusher != nil {
				flusher.Flush()
			}
			_, _ = io.WriteString(w, "data: [DONE]\n\n")
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	if err := server.store.CreditAgent(server.cfg.AgentID, 1000, "seed-stream", "seed", "test"); err != nil {
		t.Fatal(err)
	}
	if err := server.store.Allocate(server.cfg.AgentID, "42", 500, "allocate-stream", "order", "test"); err != nil {
		t.Fatal(err)
	}
	_, key, err := server.store.CreateAPIKey(server.cfg.AgentID, "42", "stream")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.5","stream":true,"messages":[]}`))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "text/event-stream")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, req)
	if response.Code != http.StatusOK || response.Body.String() != "data: one\n\ndata: [DONE]\n\n" {
		t.Fatalf("stream relay status=%d body=%q", response.Code, response.Body.String())
	}
	usage, err := server.store.Usage(server.cfg.AgentID, "42", 10)
	if err != nil || len(usage) != 1 || usage[0].SettlementStatus != "confirmed" || usage[0].ActualCents != 100 {
		t.Fatalf("stream settlement=%+v err=%v", usage, err)
	}
}

func TestVideoTaskPollIsOwnerScopedAndSettlesOriginalRequest(t *testing.T) {
	adminReads := 0
	videoPolls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/admin/users/42":
			adminReads++
			balance := 10.0
			if adminReads >= 4 {
				balance = 9.0
			}
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 42, "balance": balance}))
		case "/v1/videos":
			_, _ = io.WriteString(w, `{"id":"video-task-1","status":"queued"}`)
		case "/v1/videos/video-task-1":
			videoPolls++
			_, _ = io.WriteString(w, `{"id":"video-task-1","status":"completed"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	if err := server.store.CreditAgent(server.cfg.AgentID, 1000, "seed-video", "seed", "test"); err != nil {
		t.Fatal(err)
	}
	if err := server.store.Allocate(server.cfg.AgentID, "42", 500, "allocate-video", "order", "test"); err != nil {
		t.Fatal(err)
	}
	_, key, err := server.store.CreateAPIKey(server.cfg.AgentID, "42", "video")
	if err != nil {
		t.Fatal(err)
	}
	create := httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{"model":"grok-imagine-video-1.5"}`))
	create.Header.Set("Authorization", "Bearer "+key)
	create.Header.Set("Idempotency-Key", "video-request-1")
	created := httptest.NewRecorder()
	server.ServeHTTP(created, create)
	if created.Code != http.StatusOK {
		t.Fatalf("video create status=%d body=%s", created.Code, created.Body.String())
	}
	if _, err := server.store.UpsertUser(server.cfg.AgentID, "43", "other@example.com", "Other"); err != nil {
		t.Fatal(err)
	}
	_, otherKey, err := server.store.CreateAPIKey(server.cfg.AgentID, "43", "other")
	if err != nil {
		t.Fatal(err)
	}
	deniedRequest := httptest.NewRequest(http.MethodGet, "/v1/videos/video-task-1", nil)
	deniedRequest.Header.Set("Authorization", "Bearer "+otherKey)
	denied := httptest.NewRecorder()
	server.ServeHTTP(denied, deniedRequest)
	if denied.Code != http.StatusNotFound || videoPolls != 0 {
		t.Fatalf("foreign video poll was not isolated: status=%d polls=%d body=%s", denied.Code, videoPolls, denied.Body.String())
	}
	poll := httptest.NewRequest(http.MethodGet, "/v1/videos/video-task-1", nil)
	poll.Header.Set("Authorization", "Bearer "+key)
	polled := httptest.NewRecorder()
	server.ServeHTTP(polled, poll)
	if polled.Code != http.StatusOK || videoPolls != 1 {
		t.Fatalf("video poll status=%d polls=%d body=%s", polled.Code, videoPolls, polled.Body.String())
	}
	usage, err := server.store.Usage(server.cfg.AgentID, "42", 10)
	if err != nil || len(usage) != 1 || usage[0].RequestID != "video-request-1" || usage[0].SettlementStatus != "confirmed" || usage[0].ActualCents != 100 {
		t.Fatalf("unexpected video settlement: %+v err=%v", usage, err)
	}
	task, err := server.store.VideoTask(server.cfg.AgentID, "video-task-1")
	if err != nil || task.Status != "completed" || task.RequestID != "video-request-1" {
		t.Fatalf("unexpected video task mapping: %+v err=%v", task, err)
	}
}

func TestStaleVideoTaskReconcilerReleasesFailedTask(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/videos/video-stale" {
			_, _ = io.WriteString(w, `{"id":"video-stale","status":"failed"}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	server.cfg.OwnerMainUserID = "99"
	server.cfg.VideoTaskReconcileAge = time.Minute
	server.main = NewMainClient(server.cfg)
	if err := server.store.CreditAgent(server.cfg.AgentID, 1000, "seed-stale-video", "seed", "test"); err != nil {
		t.Fatal(err)
	}
	if err := server.store.Allocate(server.cfg.AgentID, "42", 500, "allocate-stale-video", "order", "test"); err != nil {
		t.Fatal(err)
	}
	old := time.Now().UTC().Add(-2 * time.Hour)
	server.store.clock = func() time.Time { return old }
	if _, _, err := server.store.PrepareSettlement(server.cfg.AgentID, "42", "99", "stale-video-request", "stale-video-request", 100); err != nil {
		t.Fatal(err)
	}
	if _, err := server.store.RecordVideoTask(server.cfg.AgentID, "42", "video-stale", "stale-video-request", "processing"); err != nil {
		t.Fatal(err)
	}
	server.store.clock = time.Now
	server.reconcileStaleVideoTasks(context.Background())
	usage, err := server.store.Usage(server.cfg.AgentID, "42", 10)
	if err != nil || len(usage) != 1 || usage[0].SettlementStatus != "released" {
		t.Fatalf("stale failed video was not released: %+v err=%v", usage, err)
	}
	task, err := server.store.VideoTask(server.cfg.AgentID, "video-stale")
	if err != nil || task.Status != "failed" {
		t.Fatalf("stale task status was not updated: %+v err=%v", task, err)
	}
}

func TestUncertainRelayStaysPendingInsteadOfRefunding(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/v1/admin/users/") {
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 42, "balance": 10.0}))
			return
		}
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	if err := server.store.CreditAgent(server.cfg.AgentID, 1000, "seed-pending", "seed", "test"); err != nil {
		t.Fatal(err)
	}
	if err := server.store.Allocate(server.cfg.AgentID, "42", 500, "allocate-pending", "order", "test"); err != nil {
		t.Fatal(err)
	}
	_, key, err := server.store.CreateAPIKey(server.cfg.AgentID, "42", "pending")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.5","messages":[]}`))
	req.Header.Set("Authorization", "Bearer "+key)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, req)
	if response.Code != http.StatusBadGateway {
		t.Fatalf("relay status=%d body=%s", response.Code, response.Body.String())
	}
	user, err := server.store.User(server.cfg.AgentID, "42")
	if err != nil {
		t.Fatal(err)
	}
	if user.BalanceCents != 400 {
		t.Fatalf("uncertain charge was incorrectly refunded: %+v", user)
	}
	usage, err := server.store.Usage(server.cfg.AgentID, "42", 10)
	if err != nil || len(usage) != 1 || usage[0].SettlementStatus != "pending" {
		t.Fatalf("unexpected pending settlement: %+v err=%v", usage, err)
	}
}

func TestDuplicateModelRequestIsNotForwardedOrChargedTwice(t *testing.T) {
	modelCalls := 0
	adminReads := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/admin/users/42":
			adminReads++
			balance := 10.0
			if adminReads > 1 {
				balance = 9.0
			}
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 42, "balance": balance}))
		case "/v1/chat/completions":
			modelCalls++
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":"idempotent"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	if err := server.store.CreditAgent(server.cfg.AgentID, 1000, "seed-idempotent", "seed", "test"); err != nil {
		t.Fatal(err)
	}
	if err := server.store.Allocate(server.cfg.AgentID, "42", 500, "allocate-idempotent", "order", "test"); err != nil {
		t.Fatal(err)
	}
	_, key, err := server.store.CreateAPIKey(server.cfg.AgentID, "42", "idempotent")
	if err != nil {
		t.Fatal(err)
	}
	newRequest := func() *http.Request {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.5","messages":[]}`))
		req.Header.Set("Authorization", "Bearer "+key)
		req.Header.Set("Idempotency-Key", "same-model-request")
		return req
	}
	first := httptest.NewRecorder()
	server.ServeHTTP(first, newRequest())
	if first.Code != http.StatusOK {
		t.Fatalf("first relay status=%d body=%s", first.Code, first.Body.String())
	}
	second := httptest.NewRecorder()
	server.ServeHTTP(second, newRequest())
	if second.Code != http.StatusConflict || !strings.Contains(second.Body.String(), "IDEMPOTENCY_REPLAY") {
		t.Fatalf("duplicate relay status=%d body=%s", second.Code, second.Body.String())
	}
	if modelCalls != 1 {
		t.Fatalf("duplicate request reached upstream %d times", modelCalls)
	}
	user, err := server.store.User(server.cfg.AgentID, "42")
	if err != nil {
		t.Fatal(err)
	}
	if user.BalanceCents != 400 {
		t.Fatalf("duplicate request changed local wallet twice: %+v", user)
	}
}

func TestAgentAPIRejectsForeignCredentialedOriginAndMainKeyProxy(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected upstream request: %s", r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)

	options := httptest.NewRequest(http.MethodOptions, "/api/v1/api-keys", nil)
	options.Host = "agent.example.com"
	options.Header.Set("Origin", "https://evil.example.com")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, options)
	if response.Code != http.StatusForbidden {
		t.Fatalf("foreign preflight status=%d body=%s", response.Code, response.Body.String())
	}

	mainKeys := httptest.NewRequest(http.MethodGet, "/api/v1/keys", nil)
	mainKeys.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: "not-a-real-session"})
	mainResponse := httptest.NewRecorder()
	server.ServeHTTP(mainResponse, mainKeys)
	if mainResponse.Code != http.StatusForbidden {
		t.Fatalf("main key proxy status=%d body=%s", mainResponse.Code, mainResponse.Body.String())
	}
	for _, route := range []string{"/api/v1/users", "/api/v1/balance", "/api/v1/payment/orders"} {
		probe := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, route, nil)
		req.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: "not-a-real-session"})
		server.ServeHTTP(probe, req)
		if probe.Code != http.StatusForbidden || !strings.Contains(probe.Body.String(), "AGENT_ROUTE_FORBIDDEN") {
			t.Fatalf("main route %s was not rejected: status=%d body=%s", route, probe.Code, probe.Body.String())
		}
	}
	for _, route := range []string{"/v1/admin/users", "/v1/private/completions"} {
		probe := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, route, nil)
		req.Header.Set("Authorization", "Bearer sk-agent-not-used")
		server.ServeHTTP(probe, req)
		if probe.Code != http.StatusForbidden || !strings.Contains(probe.Body.String(), "MODEL_ROUTE_FORBIDDEN") {
			t.Fatalf("model route %s was not rejected: status=%d body=%s", route, probe.Code, probe.Body.String())
		}
	}
}

func TestAgentAdminAllocationRejectsForeignOrigin(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("CSRF-rejected allocation must not call upstream: %s", r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	sessionID, err := server.store.CreateSession("42", []byte(`{"id":"42","email":"u@example.com"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/admin/users/42/allocate", strings.NewReader(`{"amount":"1.00"}`))
	req.Host = "agent.example.com"
	req.Header.Set("Origin", "https://evil.example.com")
	req.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "CSRF_ORIGIN_REJECTED") {
		t.Fatalf("foreign allocation origin status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAgentAdminCanPersistBrandingWithoutExposingOwnerFields(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("branding update must not call upstream: %s", r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	sessionID, err := server.store.CreateSession("42", []byte(`{"id":"42","email":"u@example.com"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPut, "/api/v1/agent/admin/branding", strings.NewReader(`{"name":"Brand Agent","site_name":"Brand Site","site_logo":"https://cdn.example.com/logo.svg"}`))
	req.Host = "agent.example.com"
	req.Header.Set("Origin", "https://agent.example.com")
	req.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "owner_main_user_id") {
		t.Fatalf("branding update status=%d body=%s", rec.Code, rec.Body.String())
	}
	agent, err := server.store.Agent(server.cfg.AgentID)
	if err != nil {
		t.Fatal(err)
	}
	if agent.Name != "Brand Agent" || agent.SiteName != "Brand Site" || agent.SiteLogo != "https://cdn.example.com/logo.svg" {
		t.Fatalf("branding was not persisted: %+v", agent)
	}
	settings := httptest.NewRecorder()
	server.ServeHTTP(settings, httptest.NewRequest(http.MethodGet, "/api/v1/settings/public", nil))
	if settings.Code != http.StatusOK || !strings.Contains(settings.Body.String(), "Brand Site") || !strings.Contains(settings.Body.String(), "cdn.example.com/logo.svg") {
		t.Fatalf("public branding did not reflect persisted value: status=%d body=%s", settings.Code, settings.Body.String())
	}
	bad := httptest.NewRequest(http.MethodPut, "/api/v1/agent/admin/branding", strings.NewReader(`{"site_logo":"javascript:alert(1)"}`))
	bad.Host = "agent.example.com"
	bad.Header.Set("Origin", "https://agent.example.com")
	bad.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	badResponse := httptest.NewRecorder()
	server.ServeHTTP(badResponse, bad)
	if badResponse.Code != http.StatusBadRequest || !strings.Contains(badResponse.Body.String(), "INVALID_BRANDING") {
		t.Fatalf("unsafe branding URL status=%d body=%s", badResponse.Code, badResponse.Body.String())
	}
}

func TestAgentAdminAllocationSynchronizesOwnerAndAllocatesUnderOneLock(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/admin/users/42" {
			t.Errorf("unexpected owner lookup path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 42, "email": "owner@example.com", "balance": 10.0}))
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	sessionID, err := server.store.CreateSession("42", []byte(`{"id":"42","email":"owner@example.com"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/admin/users/42/allocate", strings.NewReader(`{"amount_cents":100}`))
	req.Host = "agent.example.com"
	req.Header.Set("Origin", "https://agent.example.com")
	req.Header.Set("Idempotency-Key", "admin-allocate-lock")
	req.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("allocation status=%d body=%s", rec.Code, rec.Body.String())
	}
	user, err := server.store.User(server.cfg.AgentID, "42")
	if err != nil {
		t.Fatal(err)
	}
	if user.BalanceCents != 100 {
		t.Fatalf("allocation did not credit user: %+v", user)
	}
	agent, err := server.store.Agent(server.cfg.AgentID)
	if err != nil {
		t.Fatal(err)
	}
	if agent.WalletAvailable != 900 || agent.WalletAllocated != 100 {
		t.Fatalf("unexpected synchronized wallet: %+v", agent)
	}
}

func TestAgentRejectsUnknownHostButKeepsHealthProbeAvailable(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unknown host must be rejected before upstream access: %s", r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	server.cfg.AgentDomain = "agent.example.com"
	unknown := httptest.NewRequest(http.MethodGet, "/api/v1/settings/public", nil)
	unknown.Host = "other.example.com"
	unknownResponse := httptest.NewRecorder()
	server.ServeHTTP(unknownResponse, unknown)
	if unknownResponse.Code != http.StatusMisdirectedRequest || !strings.Contains(unknownResponse.Body.String(), "HOST_NOT_ALLOWED") {
		t.Fatalf("unknown host status=%d body=%s", unknownResponse.Code, unknownResponse.Body.String())
	}
	valid := httptest.NewRequest(http.MethodGet, "/api/v1/settings/public", nil)
	valid.Host = "agent.example.com:443"
	validResponse := httptest.NewRecorder()
	server.ServeHTTP(validResponse, valid)
	if validResponse.Code != http.StatusOK {
		t.Fatalf("configured host with port status=%d body=%s", validResponse.Code, validResponse.Body.String())
	}
	health := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	health.Host = "other.example.com"
	healthResponse := httptest.NewRecorder()
	server.ServeHTTP(healthResponse, health)
	if healthResponse.Code != http.StatusOK {
		t.Fatalf("health probe on internal host status=%d body=%s", healthResponse.Code, healthResponse.Body.String())
	}
}

func TestModelRelayRejectsNonPublicModelBeforeCharging(t *testing.T) {
	called := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	if err := server.store.CreditAgent(server.cfg.AgentID, 1000, "seed", "seed", "test"); err != nil {
		t.Fatal(err)
	}
	if err := server.store.Allocate(server.cfg.AgentID, "42", 500, "allocate", "order", "test"); err != nil {
		t.Fatal(err)
	}
	keyView, key, err := server.store.CreateAPIKey(server.cfg.AgentID, "42", "test")
	if err != nil || keyView.ID == 0 {
		t.Fatalf("create key: view=%+v err=%v", keyView, err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"private-model","messages":[]}`))
	req.Header.Set("Authorization", "Bearer "+key)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, req)
	if response.Code != http.StatusBadRequest || called {
		t.Fatalf("non-public model status=%d called=%v body=%s", response.Code, called, response.Body.String())
	}
	user, err := server.store.User(server.cfg.AgentID, "42")
	if err != nil {
		t.Fatal(err)
	}
	if user.BalanceCents != 500 {
		t.Fatalf("model validation charged wallet: %+v", user)
	}
}

func TestMultipartModelValidationUsesThePublicCatalog(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("model", "private-model"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := validatePublicModel(writer.FormDataContentType(), body.Bytes()); err == nil {
		t.Fatal("private multipart model was accepted")
	}

	body.Reset()
	writer = multipart.NewWriter(&body)
	if err := writer.WriteField("model", "gpt-image-2"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := validatePublicModel(writer.FormDataContentType(), body.Bytes()); err != nil {
		t.Fatalf("public multipart model was rejected: %v", err)
	}
}
