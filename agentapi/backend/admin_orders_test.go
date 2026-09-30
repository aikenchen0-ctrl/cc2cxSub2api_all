package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAgentAdminOrdersAggregateOnlyMappedTenantUsers(t *testing.T) {
	var mu sync.Mutex
	seenUsers := map[string]int{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer app-secret" || r.Header.Get("X-Sub2API-Satellite") != "agentapi" {
			t.Errorf("unexpected satellite credentials: %#v", r.Header)
		}
		mainUserID := r.Header.Get("X-Sub2API-On-Behalf-Of")
		mu.Lock()
		seenUsers[mainUserID]++
		mu.Unlock()
		if r.Method != http.MethodGet || r.URL.Path != "/v1/sub2api/orders" {
			t.Errorf("unexpected upstream request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("page") != "1" || r.URL.Query().Get("page_size") != "20" {
			t.Errorf("unexpected pagination: %s", r.URL.RawQuery)
		}
		id, created := 10, "2026-09-28T00:00:00Z"
		if mainUserID == "84" {
			id, created = 20, "2026-09-29T00:00:00Z"
		}
		_ = json.NewEncoder(w).Encode(envelope(map[string]any{
			"items": []any{map[string]any{
				"id": id, "user_id": mainUserID, "amount": 12.5, "pay_amount": 13,
				"currency": "USD", "status": "PENDING", "out_trade_no": "order-" + mainUserID,
				"created_at": created, "expires_at": "2026-09-30T00:00:00Z",
				"provider_secret": "must-not-leak",
			}},
			"total": 1, "page": 1, "page_size": 20, "pages": 1,
		}))
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	if _, err := server.store.UpsertUser(server.cfg.AgentID, "84", "second@example.com", "Second User"); err != nil {
		t.Fatal(err)
	}
	sessionID, err := server.store.CreateSession("42", json.RawMessage(`{"id":"42","email":"u@example.com"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "http://agent.local/api/v1/agent/admin/orders?page=1&page_size=20", nil)
	req.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, expected := range []string{`"id":20`, `"id":10`, `"main_user_id":"84"`, `"user_email":"second@example.com"`, `"total":2`} {
		if !strings.Contains(body, expected) {
			t.Fatalf("missing %s in %s", expected, body)
		}
	}
	for _, forbidden := range []string{"provider_secret", "must-not-leak", `"user_id"`} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("sensitive main-site field %q leaked: %s", forbidden, body)
		}
	}
	if strings.Index(body, `"id":20`) > strings.Index(body, `"id":10`) {
		t.Fatalf("orders were not globally sorted newest-first: %s", body)
	}
	mu.Lock()
	defer mu.Unlock()
	if seenUsers["42"] != 1 || seenUsers["84"] != 1 || len(seenUsers) != 2 {
		t.Fatalf("unexpected OBO users: %#v", seenUsers)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("tenant order administration response must not be cached")
	}
}

func TestAgentAdminOrdersRejectNonAdminAndUnmappedTargets(t *testing.T) {
	upstreamCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	if _, err := server.store.UpsertUser(server.cfg.AgentID, "84", "ordinary@example.com", "Ordinary"); err != nil {
		t.Fatal(err)
	}
	ordinarySession, err := server.store.CreateSession("84", json.RawMessage(`{"id":"84"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	ordinaryReq := httptest.NewRequest(http.MethodGet, "http://agent.local/api/v1/agent/admin/orders", nil)
	ordinaryReq.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: ordinarySession})
	ordinaryRec := httptest.NewRecorder()
	server.ServeHTTP(ordinaryRec, ordinaryReq)
	if ordinaryRec.Code != http.StatusForbidden {
		t.Fatalf("ordinary user status=%d body=%s", ordinaryRec.Code, ordinaryRec.Body.String())
	}

	adminSession, err := server.store.CreateSession("42", json.RawMessage(`{"id":"42"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	unmappedReq := httptest.NewRequest(http.MethodGet, "http://agent.local/api/v1/agent/admin/orders?main_user_id=999", nil)
	unmappedReq.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: adminSession})
	unmappedRec := httptest.NewRecorder()
	server.ServeHTTP(unmappedRec, unmappedReq)
	if unmappedRec.Code != http.StatusNotFound {
		t.Fatalf("unmapped target status=%d body=%s", unmappedRec.Code, unmappedRec.Body.String())
	}
	if upstreamCalls != 0 {
		t.Fatalf("rejected requests reached main site: %d", upstreamCalls)
	}
}

func TestAgentAdminOrderCancelUsesMappedUserOBOAndSameOrigin(t *testing.T) {
	upstreamCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		if r.Method != http.MethodPost || r.URL.Path != "/v1/sub2api/orders/7/cancel" || r.Header.Get("X-Sub2API-On-Behalf-Of") != "84" {
			t.Errorf("unexpected cancel request: %s %s %#v", r.Method, r.URL.Path, r.Header)
		}
		_ = json.NewEncoder(w).Encode(envelope(map[string]string{"message": "cancelled"}))
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	if _, err := server.store.UpsertUser(server.cfg.AgentID, "84", "second@example.com", "Second"); err != nil {
		t.Fatal(err)
	}
	sessionID, err := server.store.CreateSession("42", json.RawMessage(`{"id":"42"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	request := func(origin string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "http://agent.local/api/v1/agent/admin/orders/84/7/cancel", strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", origin)
		req.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		return rec
	}
	if rec := request("https://attacker.example"); rec.Code != http.StatusForbidden || upstreamCalls != 0 {
		t.Fatalf("cross-origin cancel status=%d calls=%d body=%s", rec.Code, upstreamCalls, rec.Body.String())
	}
	if rec := request("http://agent.local"); rec.Code != http.StatusOK || upstreamCalls != 1 {
		t.Fatalf("same-origin cancel status=%d calls=%d body=%s", rec.Code, upstreamCalls, rec.Body.String())
	}
}

func TestAgentAdminPaymentDashboardAggregatesMappedUsersWithPaidSince(t *testing.T) {
	var mu sync.Mutex
	seenUsers := map[string]int{}
	paidSinceSeen := 0
	today := time.Now().Format(time.RFC3339)
	yesterday := time.Now().Add(-24 * time.Hour).Format(time.RFC3339)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer app-secret" || r.Header.Get("X-Sub2API-Satellite") != "agentapi" {
			t.Errorf("unexpected satellite credentials: %#v", r.Header)
		}
		mainUserID := r.Header.Get("X-Sub2API-On-Behalf-Of")
		mu.Lock()
		seenUsers[mainUserID]++
		mu.Unlock()
		if r.Method != http.MethodGet || r.URL.Path != "/v1/sub2api/orders" {
			t.Errorf("unexpected upstream request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		status := r.URL.Query().Get("status")
		if status == "PENDING" {
			if r.URL.Query().Get("paid_since") != "" {
				t.Errorf("pending count must not be constrained by paid_since: %s", r.URL.RawQuery)
			}
			total := 2
			if mainUserID == "84" {
				total = 1
			}
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"items": []any{}, "total": total, "page": 1, "page_size": 1, "pages": total}))
			return
		}
		if r.URL.Query().Get("paid_since") == "" {
			t.Errorf("paid status %s missing paid_since", status)
		} else {
			mu.Lock()
			paidSinceSeen++
			mu.Unlock()
		}
		items := []any{}
		if status == "COMPLETED" {
			amount, paidAt, method := 12.5, today, "stripe"
			if mainUserID == "84" {
				amount, paidAt, method = 7.5, yesterday, "wxpay"
			}
			items = append(items, map[string]any{
				"id": 10, "amount": amount, "pay_amount": amount, "currency": "USD",
				"payment_type": method, "status": status, "paid_at": paidAt,
				"created_at": paidAt, "expires_at": paidAt, "main_admin_secret": "must-not-leak",
			})
		}
		_ = json.NewEncoder(w).Encode(envelope(map[string]any{"items": items, "total": len(items), "page": 1, "page_size": 100, "pages": 1}))
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	if _, err := server.store.UpsertUser(server.cfg.AgentID, "84", "second@example.com", "Second User"); err != nil {
		t.Fatal(err)
	}
	sessionID, err := server.store.CreateSession("42", json.RawMessage(`{"id":"42","email":"u@example.com"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "http://agent.local/api/v1/agent/admin/orders/dashboard?days=7", nil)
	req.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Data AgentAdminPaymentDashboard `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.TotalCount != 2 || body.Data.TotalAmount["USD"] != 20 || body.Data.PendingOrders != 3 {
		t.Fatalf("unexpected dashboard totals: %#v", body.Data)
	}
	if len(body.Data.DailySeries) != 7 || len(body.Data.PaymentMethods) != 2 || len(body.Data.TopUsers["USD"]) != 2 {
		t.Fatalf("unexpected dashboard breakdown: %#v", body.Data)
	}
	if body.Data.TopUsers["USD"][0].MainUserID != "42" || body.Data.TopUsers["USD"][1].MainUserID != "84" {
		t.Fatalf("top users must be tenant mapped users: %#v", body.Data.TopUsers)
	}
	if strings.Contains(rec.Body.String(), "main_admin_secret") || strings.Contains(rec.Body.String(), "must-not-leak") {
		t.Fatalf("upstream-only fields leaked: %s", rec.Body.String())
	}
	mu.Lock()
	defer mu.Unlock()
	if seenUsers["42"] != 4 || seenUsers["84"] != 4 || len(seenUsers) != 2 || paidSinceSeen != 6 {
		t.Fatalf("unexpected OBO dashboard requests: users=%#v paid_since=%d", seenUsers, paidSinceSeen)
	}
}

func TestAgentAdminPaymentDashboardRejectsUnsupportedWindowBeforeUpstream(t *testing.T) {
	upstreamCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	sessionID, err := server.store.CreateSession("42", json.RawMessage(`{"id":"42"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "http://agent.local/api/v1/agent/admin/orders/dashboard?days=14", nil)
	req.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || upstreamCalls != 0 {
		t.Fatalf("status=%d calls=%d body=%s", rec.Code, upstreamCalls, rec.Body.String())
	}
}
