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

func TestAgentAdminAffiliateRecordsAreTenantScopedAndUseOBO(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]int{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer app-secret" || r.Header.Get("X-Sub2API-Satellite") != "agentapi" {
			t.Errorf("unexpected satellite headers: %#v", r.Header)
		}
		if r.Header.Get("X-Admin-Key") != "" || r.Header.Get("X-API-Key") != "" {
			t.Errorf("administrator credential leaked upstream: %#v", r.Header)
		}
		userID := r.Header.Get("X-Sub2API-On-Behalf-Of")
		mu.Lock()
		seen[userID]++
		mu.Unlock()
		items := []any{}
		if r.URL.Path == "/v1/sub2api/affiliate/invites" && userID == "42" {
			items = []any{
				map[string]any{"inviter_id": 42, "inviter_email": "upstream-owner@example.com", "inviter_username": "Upstream Owner", "invitee_id": 84, "invitee_email": "upstream-user@example.com", "invitee_username": "Upstream User", "aff_code": "TENANT", "total_rebate": 2.5, "created_at": "2026-09-28T00:00:00Z", "secret": "must-not-leak"},
				map[string]any{"inviter_id": 42, "inviter_email": "owner@example.com", "invitee_id": 999, "invitee_email": "other-tenant@example.com", "aff_code": "CROSS", "total_rebate": 99, "created_at": "2026-09-29T00:00:00Z"},
			}
		}
		_ = json.NewEncoder(w).Encode(envelope(map[string]any{"items": items, "total": len(items), "page": 1, "page_size": 100, "pages": 1}))
	}))
	defer upstream.Close()

	server := testServer(t, upstream)
	if _, err := server.store.UpsertUser(server.cfg.AgentID, "84", "tenant-user@example.com", "Tenant User"); err != nil {
		t.Fatal(err)
	}
	sessionID, err := server.store.CreateSession("42", json.RawMessage(`{"id":"42"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "http://agent.local/api/v1/agent/admin/affiliates/invites?page=1&page_size=20&sort_by=created_at&sort_order=desc", nil)
	req.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, expected := range []string{`"aff_code":"TENANT"`, `"inviter_email":"u@example.com"`, `"invitee_email":"tenant-user@example.com"`, `"total":1`} {
		if !strings.Contains(body, expected) {
			t.Fatalf("missing %s in %s", expected, body)
		}
	}
	for _, forbidden := range []string{"other-tenant@example.com", "CROSS", "must-not-leak", "secret", "upstream-owner@example.com"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("tenant or sensitive value %q leaked: %s", forbidden, body)
		}
	}
	mu.Lock()
	if seen["42"] != 1 || seen["84"] != 1 || len(seen) != 2 {
		t.Fatalf("unexpected OBO calls: %#v", seen)
	}
	mu.Unlock()
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("affiliate administration response must not be cached")
	}
}

func TestAgentAdminAffiliateRecordsRejectOrdinaryUsersAndUnmappedSelection(t *testing.T) {
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
	ordinarySession, _ := server.store.CreateSession("84", json.RawMessage(`{"id":"84"}`), "", "", time.Now().Add(sessionTTL))
	ordinaryReq := httptest.NewRequest(http.MethodGet, "http://agent.local/api/v1/agent/admin/affiliates/invites", nil)
	ordinaryReq.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: ordinarySession})
	ordinaryRec := httptest.NewRecorder()
	server.ServeHTTP(ordinaryRec, ordinaryReq)
	if ordinaryRec.Code != http.StatusForbidden {
		t.Fatalf("ordinary status=%d body=%s", ordinaryRec.Code, ordinaryRec.Body.String())
	}

	adminSession, _ := server.store.CreateSession("42", json.RawMessage(`{"id":"42"}`), "", "", time.Now().Add(sessionTTL))
	unmappedReq := httptest.NewRequest(http.MethodGet, "http://agent.local/api/v1/agent/admin/affiliates/rebates?main_user_id=999", nil)
	unmappedReq.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: adminSession})
	unmappedRec := httptest.NewRecorder()
	server.ServeHTTP(unmappedRec, unmappedReq)
	if unmappedRec.Code != http.StatusNotFound || !strings.Contains(unmappedRec.Body.String(), "MAPPED_USER_NOT_FOUND") {
		t.Fatalf("unmapped status=%d body=%s", unmappedRec.Code, unmappedRec.Body.String())
	}
	if upstreamCalls != 0 {
		t.Fatalf("rejected requests reached upstream: %d", upstreamCalls)
	}
}
