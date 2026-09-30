package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAgentAdminProvisioningReturnsOnlyCurrentAgentLifecycle(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("provisioning status page must not call upstream: %s", r.URL.Path)
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	server.cfg.ProvisioningControlEnabled = true
	server.cfg.ProvisioningControlStaleAfter = 2 * time.Minute
	server.cfg.RuntimeControlCredential = "runtime-control-secret-must-not-leak"
	server.cfg.MainAdminAPIKey = "main-admin-secret-must-not-leak"
	server.cfg.AppCredential = "satellite-app-secret-must-not-leak"
	checkedAt := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	server.setProvisioningState("active", checkedAt)
	server.store.clock = func() time.Time { return checkedAt.Add(time.Minute) }

	sessionID, err := server.store.CreateSession("42", json.RawMessage(`{"id":"42"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "http://agent.local/api/v1/agent/admin/agent-provisioning", nil)
	req.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, expected := range []string{
		`"agent_id":"agent-test"`, `"runtime_status":"active"`, `"control_enabled":true`,
		`"control_available":true`, `"ready":true`, `"lifecycle_authority":"sub2api_main"`,
		`"management_scope":"current_agent_read_only"`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("missing %s in %s", expected, body)
		}
	}
	for _, forbidden := range []string{"runtime-control-secret", "main-admin-secret", "satellite-app-secret", "relay_base_url", "database"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("sensitive field %q leaked in %s", forbidden, body)
		}
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("provisioning response must not be cached")
	}
}

func TestAgentAdminProvisioningRejectsOrdinaryUsersAndWrites(t *testing.T) {
	upstream := httptest.NewServer(http.NotFoundHandler())
	defer upstream.Close()
	server := testServer(t, upstream)
	if _, err := server.store.UpsertUser(server.cfg.AgentID, "84", "ordinary@example.com", "Ordinary"); err != nil {
		t.Fatal(err)
	}
	ordinarySession, _ := server.store.CreateSession("84", json.RawMessage(`{"id":"84"}`), "", "", time.Now().Add(sessionTTL))
	ordinaryReq := httptest.NewRequest(http.MethodGet, "/api/v1/agent/admin/agent-provisioning", nil)
	ordinaryReq.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: ordinarySession})
	ordinaryRec := httptest.NewRecorder()
	server.ServeHTTP(ordinaryRec, ordinaryReq)
	if ordinaryRec.Code != http.StatusForbidden {
		t.Fatalf("ordinary status=%d body=%s", ordinaryRec.Code, ordinaryRec.Body.String())
	}

	adminSession, _ := server.store.CreateSession("42", json.RawMessage(`{"id":"42"}`), "", "", time.Now().Add(sessionTTL))
	writeReq := httptest.NewRequest(http.MethodPost, "http://agent.local/api/v1/agent/admin/agent-provisioning", strings.NewReader(`{"status":"suspended"}`))
	writeReq.Header.Set("Origin", "http://agent.local")
	writeReq.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: adminSession})
	writeRec := httptest.NewRecorder()
	server.ServeHTTP(writeRec, writeReq)
	if writeRec.Code != http.StatusMethodNotAllowed || !strings.Contains(writeRec.Body.String(), "METHOD_NOT_ALLOWED") {
		t.Fatalf("write status=%d body=%s", writeRec.Code, writeRec.Body.String())
	}
}
