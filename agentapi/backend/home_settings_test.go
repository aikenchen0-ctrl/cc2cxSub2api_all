package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAgentHomeSettingsPersistPatchAndStayTenantLocal(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("homepage configuration must never call the main site: %s", r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	other := testServer(t, upstream) // another instance/database remains independent
	owner, err := server.store.CreateSession("42", []byte(`{"id":"42"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.store.UpsertUser(server.cfg.AgentID, "99", "user@example.com", "User"); err != nil {
		t.Fatal(err)
	}
	ordinary, err := server.store.CreateSession("99", []byte(`{"id":"99","role":"admin"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path, body, session, origin string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "https://agent.example.com"+path, strings.NewReader(body))
		if session != "" {
			req.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: session})
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		return rec
	}
	const endpoint = "/api/v1/agent/admin/branding"
	const origin = "https://agent.example.com"
	body := `{"site_subtitle":"Tenant home","compact_home_enabled":true,"home_content":"<h1>Tenant home</h1>"}`
	for _, c := range []struct {
		session, origin string
		status          int
	}{
		{"", origin, http.StatusUnauthorized}, {ordinary, origin, http.StatusForbidden},
		{owner, "https://foreign.example.com", http.StatusForbidden},
	} {
		rec := request(http.MethodPatch, endpoint, body, c.session, c.origin)
		if rec.Code != c.status {
			t.Fatalf("permission status=%d want=%d body=%s", rec.Code, c.status, rec.Body.String())
		}
	}
	rec := request(http.MethodPatch, endpoint, body, owner, origin)
	if rec.Code != http.StatusOK {
		t.Fatalf("save status=%d body=%s", rec.Code, rec.Body.String())
	}
	// Existing clients that patch just a name must not reset homepage choices.
	rec = request(http.MethodPatch, endpoint, `{"site_name":"Renamed tenant"}`, owner, origin)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch status=%d body=%s", rec.Code, rec.Body.String())
	}
	for _, path := range []string{endpoint, "/api/v1/settings/public"} {
		rec = request(http.MethodGet, path, "", owner, "")
		var envelope struct {
			Data AgentHomeSettings `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		if rec.Code != http.StatusOK || envelope.Data.SiteSubtitle != "Tenant home" || !envelope.Data.CompactHomeEnabled || envelope.Data.HomeContent != "<h1>Tenant home</h1>" {
			t.Fatalf("homepage missing from %s: %s", path, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "test-main-admin-key") || strings.Contains(rec.Body.String(), "app-secret") {
			t.Fatal("public credentials leak")
		}
	}
	otherAgent, err := other.store.Agent(other.cfg.AgentID)
	if err != nil || otherAgent.CompactHomeEnabled || otherAgent.HomeContent != "" {
		t.Fatalf("other tenant changed: %+v %v", otherAgent, err)
	}
	rec = request(http.MethodPatch, endpoint, `{"compact_home_enabled":false,"home_content":""}`, owner, origin)
	agent, err := server.store.Agent(server.cfg.AgentID)
	if rec.Code != http.StatusOK || err != nil || agent.CompactHomeEnabled || agent.HomeContent != "" || agent.SiteSubtitle != "Tenant home" {
		t.Fatalf("explicit clear failed: agent=%+v err=%v body=%s", agent, err, rec.Body.String())
	}
}

func TestAgentHomeSettingsValidation(t *testing.T) {
	for _, content := range []string{"", "<html><style>body{color:red}</style><h1>Home</h1></html>", "https://example.com/home", "http://localhost:8080/home"} {
		if err := validateAgentHomeSettings(AgentHomeSettings{HomeContent: content, SiteSubtitle: "Line 1\nLine 2"}); err != nil {
			t.Fatalf("valid home rejected: %v", err)
		}
	}
	for _, home := range []AgentHomeSettings{
		{HomeContent: "javascript:alert(1)"}, {HomeContent: "data:text/html,hello"},
		{HomeContent: "https://user:secret@example.com"}, {HomeContent: "//example.com"},
		{HomeContent: "<h1>\x00</h1>"}, {HomeContent: "<" + strings.Repeat("a", 100000)},
		{SiteSubtitle: strings.Repeat("中", 501)}, {SiteSubtitle: "a\x00b"},
	} {
		if err := validateAgentHomeSettings(home); err == nil {
			t.Fatal("invalid home accepted")
		}
	}
}

func TestAgentHomeSettingsUpgradeExistingDatabase(t *testing.T) {
	store := testStore(t)
	// Model the previous schema and its persisted branding, then run the real migration.
	for _, column := range []string{"site_subtitle", "compact_home_enabled", "home_content"} {
		if _, err := store.db.Exec("ALTER TABLE agent_config DROP COLUMN " + column); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.migrate(); err != nil {
		t.Fatal(err)
	}
	agent, err := store.Agent("agent-test")
	if err != nil || agent.SiteSubtitle != "AI API Gateway Platform" || agent.CompactHomeEnabled || agent.HomeContent != "" {
		t.Fatalf("migration defaults failed: %+v %v", agent, err)
	}
}
