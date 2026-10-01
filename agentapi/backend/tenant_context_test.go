package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func tenantTestConfig(agentID, ownerID, name string) Config {
	return Config{
		AgentID: agentID, AgentName: name, SiteName: name,
		OwnerMainUserID: ownerID, BillingMode: "user_upstream",
		AppCredential:   "agt_model_tenant-test-credential",
		PaymentCurrency: "CNY", PaymentOrderTTL: 30 * time.Minute,
	}
}

func TestStoreScopesReusableBusinessIdentifiersByTenant(t *testing.T) {
	store, err := OpenStore(":memory:", "tenant-business-id-secret")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	for _, cfg := range []Config{
		tenantTestConfig("agent-a", "owner-a", "Agent A"),
		tenantTestConfig("agent-b", "owner-b", "Agent B"),
	} {
		if err := store.UpsertAgent(cfg); err != nil {
			t.Fatalf("upsert %s: %v", cfg.AgentID, err)
		}
		if _, err := store.UpsertUser(cfg.AgentID, "shared-user", cfg.AgentID+"@example.com", cfg.AgentName+" User"); err != nil {
			t.Fatalf("upsert shared user for %s: %v", cfg.AgentID, err)
		}
	}

	const (
		sharedTaskID    = "task-shared-across-tenants"
		sharedRequestID = "request-shared-across-tenants"
	)
	if _, err := store.RecordVideoTask("agent-a", "shared-user", sharedTaskID, sharedRequestID, "pending"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RecordVideoTask("agent-b", "shared-user", sharedTaskID, sharedRequestID, "processing"); err != nil {
		t.Fatalf("same video identifiers were not reusable in another tenant: %v", err)
	}
	if err := store.UpdateVideoTaskStatus("agent-a", sharedTaskID, "completed"); err != nil {
		t.Fatal(err)
	}
	videoA, err := store.VideoTask("agent-a", sharedTaskID)
	if err != nil {
		t.Fatal(err)
	}
	videoB, err := store.VideoTask("agent-b", sharedTaskID)
	if err != nil {
		t.Fatal(err)
	}
	if videoA.Status != "completed" || videoB.Status != "processing" {
		t.Fatalf("video task status crossed tenant boundary: agent-a=%q agent-b=%q", videoA.Status, videoB.Status)
	}

	if _, err := store.RecordImageTask("agent-a", "shared-user", sharedTaskID, sharedRequestID, "processing"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RecordImageTask("agent-b", "shared-user", sharedTaskID, sharedRequestID, "queued"); err != nil {
		t.Fatalf("same image identifiers were not reusable in another tenant: %v", err)
	}
	if err := store.UpdateImageTaskStatus("agent-b", sharedTaskID, "completed"); err != nil {
		t.Fatal(err)
	}
	imageA, err := store.ImageTask("agent-a", sharedTaskID)
	if err != nil {
		t.Fatal(err)
	}
	imageB, err := store.ImageTask("agent-b", sharedTaskID)
	if err != nil {
		t.Fatal(err)
	}
	if imageA.Status != "processing" || imageB.Status != "completed" {
		t.Fatalf("image task status crossed tenant boundary: agent-a=%q agent-b=%q", imageA.Status, imageB.Status)
	}

	settlementA, created, err := store.PrepareDirectSettlement("agent-a", "shared-user", "shared-user", sharedRequestID, "usage-agent-a", 100)
	if err != nil || !created {
		t.Fatalf("prepare agent-a settlement: created=%v err=%v", created, err)
	}
	settlementB, created, err := store.PrepareDirectSettlement("agent-b", "shared-user", "shared-user", sharedRequestID, "usage-agent-b", 200)
	if err != nil || !created {
		t.Fatalf("same settlement request id was not reusable in another tenant: created=%v err=%v", created, err)
	}
	if settlementA.AgentID != "agent-a" || settlementA.UsageID != "usage-agent-a" || settlementA.ReservedCents != 100 ||
		settlementB.AgentID != "agent-b" || settlementB.UsageID != "usage-agent-b" || settlementB.ReservedCents != 200 {
		t.Fatalf("settlement lookup crossed tenant boundary: agent-a=%+v agent-b=%+v", settlementA, settlementB)
	}
	if err := store.SetSettlementModel("agent-a", sharedRequestID, "model-a"); err != nil {
		t.Fatal(err)
	}
	settlementA, err = store.Settlement("agent-a", sharedRequestID)
	if err != nil {
		t.Fatal(err)
	}
	settlementB, err = store.Settlement("agent-b", sharedRequestID)
	if err != nil {
		t.Fatal(err)
	}
	if settlementA.Model != "model-a" || settlementB.Model != "" {
		t.Fatalf("settlement update crossed tenant boundary: agent-a=%q agent-b=%q", settlementA.Model, settlementB.Model)
	}
}

func TestStoreScopesAnnouncementsAndReadStateByTenant(t *testing.T) {
	store, err := OpenStore(":memory:", "tenant-announcement-secret")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	for _, cfg := range []Config{
		tenantTestConfig("agent-a", "owner-a", "Agent A"),
		tenantTestConfig("agent-b", "owner-b", "Agent B"),
	} {
		if err := store.UpsertAgent(cfg); err != nil {
			t.Fatalf("upsert %s: %v", cfg.AgentID, err)
		}
		if _, err := store.UpsertUser(cfg.AgentID, "shared-user", cfg.AgentID+"@example.com", "Shared User"); err != nil {
			t.Fatalf("upsert shared user for %s: %v", cfg.AgentID, err)
		}
	}

	announcementA, err := store.CreateAnnouncement("agent-a", "Agent A notice", "A only", "active", "popup", time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	announcementB, err := store.CreateAnnouncement("agent-b", "Agent B notice", "B only", "active", "popup", time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}

	if err := store.MarkAnnouncementRead("agent-a", "shared-user", announcementB.ID); !errors.Is(err, errNotFound) {
		t.Fatalf("tenant A marked tenant B announcement as read: %v", err)
	}
	if err := store.MarkAnnouncementRead("agent-a", "shared-user", announcementA.ID); err != nil {
		t.Fatal(err)
	}

	itemsA, err := store.UserAnnouncements("agent-a", "shared-user")
	if err != nil {
		t.Fatal(err)
	}
	itemsB, err := store.UserAnnouncements("agent-b", "shared-user")
	if err != nil {
		t.Fatal(err)
	}
	if len(itemsA) != 1 || itemsA[0].ID != announcementA.ID || itemsA[0].Title != "Agent A notice" || itemsA[0].ReadAt == "" {
		t.Fatalf("tenant A announcement state is wrong: %+v", itemsA)
	}
	if len(itemsB) != 1 || itemsB[0].ID != announcementB.ID || itemsB[0].Title != "Agent B notice" || itemsB[0].ReadAt != "" {
		t.Fatalf("announcement or read state crossed tenant boundary: %+v", itemsB)
	}
}

func TestStoreSupportsMultipleAgentsAndResolvesKeyTenant(t *testing.T) {
	store, err := OpenStore(":memory:", "tenant-test-secret")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	for _, cfg := range []Config{
		tenantTestConfig("agent-a", "owner-a", "Agent A"),
		tenantTestConfig("agent-b", "owner-b", "Agent B"),
	} {
		if err := store.UpsertAgent(cfg); err != nil {
			t.Fatalf("upsert %s: %v", cfg.AgentID, err)
		}
		if _, err := store.UpsertUser(cfg.AgentID, "shared-user", cfg.AgentID+"@example.com", cfg.AgentName+" User"); err != nil {
			t.Fatalf("upsert user for %s: %v", cfg.AgentID, err)
		}
	}

	_, keyA, err := store.CreateAPIKey("agent-a", "shared-user", "key-a")
	if err != nil {
		t.Fatal(err)
	}
	_, keyB, err := store.CreateAPIKey("agent-b", "shared-user", "key-b")
	if err != nil {
		t.Fatal(err)
	}

	resolvedA, err := store.ResolveAPIKeyDetailsAnyTenant(keyA)
	if err != nil || resolvedA.AgentID != "agent-a" || resolvedA.MainUserID != "shared-user" {
		t.Fatalf("key A resolved to wrong tenant: %+v err=%v", resolvedA, err)
	}
	resolvedB, err := store.ResolveAPIKeyDetailsAnyTenant(keyB)
	if err != nil || resolvedB.AgentID != "agent-b" || resolvedB.MainUserID != "shared-user" {
		t.Fatalf("key B resolved to wrong tenant: %+v err=%v", resolvedB, err)
	}
	if _, err := store.ResolveAPIKeyDetails("agent-a", keyB); !errors.Is(err, errNotFound) {
		t.Fatalf("tenant-scoped resolver accepted another tenant's key: %v", err)
	}
	if agent, err := store.Agent("agent-b"); err != nil || agent.Name != "Agent B" {
		t.Fatalf("second agent config was not persisted independently: %+v err=%v", agent, err)
	}
}

func TestActiveAgentsReturnsEveryActiveTenantAndSkipsInactiveTenants(t *testing.T) {
	store, err := OpenStore(":memory:", "active-agent-test-secret")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	for _, cfg := range []Config{
		tenantTestConfig("agent-b", "owner-b", "Agent B"),
		tenantTestConfig("agent-a", "owner-a", "Agent A"),
		tenantTestConfig("agent-c", "owner-c", "Agent C"),
	} {
		if err := store.UpsertAgent(cfg); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.db.Exec(`UPDATE agent_config SET status='suspended' WHERE agent_id='agent-c'`); err != nil {
		t.Fatal(err)
	}

	agents, err := store.ActiveAgents()
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != 2 || agents[0].ID != "agent-a" || agents[1].ID != "agent-b" {
		t.Fatalf("active tenants = %+v, want agent-a and agent-b", agents)
	}
}

func TestReadinessUsesAllActiveTenantsInsteadOfBootstrapTenant(t *testing.T) {
	upstream := httptest.NewServer(http.NotFoundHandler())
	defer upstream.Close()
	server := testServer(t, upstream)

	shared := tenantTestConfig("agent-shared", "", "Shared Agent")
	shared.AgentDomain = "shared.example.test"
	if err := server.store.UpsertAgent(shared); err != nil {
		t.Fatal(err)
	}
	if _, err := server.store.db.Exec(`UPDATE agent_config SET status='suspended' WHERE agent_id=?`, server.cfg.AgentID); err != nil {
		t.Fatal(err)
	}

	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("shared readiness status=%d body=%s", response.Code, response.Body.String())
	}
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data["active_tenants"] != float64(1) {
		t.Fatalf("active_tenants=%v body=%s", envelope.Data["active_tenants"], response.Body.String())
	}
	if _, exists := envelope.Data["agent_id"]; exists {
		t.Fatalf("inactive bootstrap tenant leaked as readiness authority: %s", response.Body.String())
	}
}

func TestReadinessRejectsInvalidActiveTenantWithoutRequiringUserUpstreamOwner(t *testing.T) {
	upstream := httptest.NewServer(http.NotFoundHandler())
	defer upstream.Close()
	server := testServer(t, upstream)

	if _, err := server.store.db.Exec(`UPDATE agent_config SET billing_mode='user_upstream', billing_main_user_id='' WHERE agent_id=?`, server.cfg.AgentID); err != nil {
		t.Fatal(err)
	}
	ready := httptest.NewRecorder()
	server.ServeHTTP(ready, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if ready.Code != http.StatusOK {
		t.Fatalf("user-upstream tenant incorrectly required an owner: %d %s", ready.Code, ready.Body.String())
	}

	if _, err := server.store.db.Exec(`UPDATE agent_config SET billing_mode='owner_upstream', billing_main_user_id='' WHERE agent_id=?`, server.cfg.AgentID); err != nil {
		t.Fatal(err)
	}
	notReady := httptest.NewRecorder()
	server.ServeHTTP(notReady, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if notReady.Code != http.StatusServiceUnavailable || !strings.Contains(notReady.Body.String(), "MAIN_OWNER_MISSING") {
		t.Fatalf("owner-upstream tenant without owner status=%d body=%s", notReady.Code, notReady.Body.String())
	}
}

func TestRequestAgentUsesPersistedHostAndRejectsCrossTenantSession(t *testing.T) {
	store, err := OpenStore(":memory:", "tenant-host-secret")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	agentA := tenantTestConfig("agent-a", "owner-a", "Agent A")
	agentA.AgentDomain = "a.example.test"
	agentA.CookieName = "agentapi_session"
	agentB := tenantTestConfig("agent-b", "owner-b", "Agent B")
	agentB.AgentDomain = "b.example.test"
	for _, cfg := range []Config{agentA, agentB} {
		if err := store.UpsertAgent(cfg); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.UpsertUser("agent-a", "owner-a", "owner-a@example.test", "Owner A"); err != nil {
		t.Fatal(err)
	}

	server := &Server{cfg: agentA, store: store}
	req := httptest.NewRequest(http.MethodGet, "https://b.example.test/api/v1/settings/public?agent_id=agent-a", nil)
	req.Header.Set("X-Agent-ID", "agent-a")
	resolved, err := server.requestAgent(req)
	if err != nil || resolved.ID != "agent-b" {
		t.Fatalf("host did not resolve persisted tenant: agent=%+v err=%v", resolved, err)
	}

	sessionID, err := store.CreateTenantSession("agent-a", "owner-a", tenantRoleOwner, []byte(`{"id":"owner-a"}`), "", "", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(&http.Cookie{Name: agentA.CookieName, Value: sessionID})
	if _, err := server.requestAgent(req); !errors.Is(err, errNotFound) {
		t.Fatalf("cross-tenant session was accepted on another tenant host: %v", err)
	}
}

func TestRequestAgentRejectsUnboundRequestWhenMultipleTenantsAreActive(t *testing.T) {
	store, err := OpenStore(":memory:", "tenant-unbound-secret")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	bootstrap := tenantTestConfig("agent-bootstrap", "owner-bootstrap", "Bootstrap")
	bootstrap.CookieName = "agentapi_session"
	shared := tenantTestConfig("agent-shared", "owner-shared", "Shared")
	for _, cfg := range []Config{bootstrap, shared} {
		if err := store.UpsertAgent(cfg); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.UpsertUser("agent-shared", "owner-shared", "owner-shared@example.test", "Shared Owner"); err != nil {
		t.Fatal(err)
	}

	server := &Server{cfg: bootstrap, store: store}
	unbound := httptest.NewRequest(http.MethodGet, "https://shared-entry.example.test/api/v1/settings/public", nil)
	if _, err := server.requestAgent(unbound); !errors.Is(err, errNotFound) {
		t.Fatalf("unbound multi-tenant request fell back to bootstrap tenant: %v", err)
	}

	sessionID, err := store.CreateTenantSession("agent-shared", "owner-shared", tenantRoleOwner, []byte(`{"id":"owner-shared"}`), "", "", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	unbound.AddCookie(&http.Cookie{Name: bootstrap.CookieName, Value: sessionID})
	resolved, err := server.requestAgent(unbound)
	if err != nil || resolved.ID != "agent-shared" {
		t.Fatalf("trusted session did not resolve shared tenant: agent=%+v err=%v", resolved, err)
	}
}

func TestRequestAgentSharedHostUsesSessionDespiteLegacyDomainMapping(t *testing.T) {
	store, err := OpenStore(":memory:", "tenant-shared-host-secret")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	legacy := tenantTestConfig("agent-legacy", "owner-legacy", "Legacy")
	legacy.AgentDomain = "shared-entry.example.test"
	target := tenantTestConfig("agent-target", "owner-target", "Target")
	for _, cfg := range []Config{legacy, target} {
		if err := store.UpsertAgent(cfg); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.UpsertUser("agent-target", "owner-target", "owner-target@example.test", "Target Owner"); err != nil {
		t.Fatal(err)
	}

	server := &Server{
		cfg:   Config{CookieName: "agentapi_session", SharedHosts: []string{"shared-entry.example.test"}},
		store: store,
	}
	request := httptest.NewRequest(http.MethodGet, "https://shared-entry.example.test/api/v1/settings/public", nil)
	if _, err := server.requestAgent(request); !errors.Is(err, errNotFound) {
		t.Fatalf("unbound shared host resolved stale custom-domain row: %v", err)
	}

	sessionID, err := store.CreateTenantSession("agent-target", "owner-target", tenantRoleOwner, []byte(`{"id":"owner-target"}`), "", "", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	request.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	resolved, err := server.requestAgent(request)
	if err != nil || resolved.ID != "agent-target" {
		t.Fatalf("shared host did not resolve trusted session tenant: agent=%+v err=%v", resolved, err)
	}
}

func TestRequestAgentKeepsSingleTenantCompatibilityFallback(t *testing.T) {
	store, err := OpenStore(":memory:", "tenant-single-fallback-secret")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	bootstrap := tenantTestConfig("agent-bootstrap", "owner-bootstrap", "Bootstrap")
	if err := store.UpsertAgent(bootstrap); err != nil {
		t.Fatal(err)
	}
	server := &Server{cfg: bootstrap, store: store}
	request := httptest.NewRequest(http.MethodGet, "https://legacy-unbound.example.test/api/v1/settings/public", nil)
	resolved, err := server.requestAgent(request)
	if err != nil || resolved.ID != bootstrap.AgentID {
		t.Fatalf("single-tenant compatibility fallback failed: agent=%+v err=%v", resolved, err)
	}
}

func TestAgentDomainIsUniqueCaseInsensitive(t *testing.T) {
	store, err := OpenStore(":memory:", "tenant-domain-secret")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	first := tenantTestConfig("agent-a", "owner-a", "Agent A")
	first.AgentDomain = "Proxy.Example.Test"
	second := tenantTestConfig("agent-b", "owner-b", "Agent B")
	second.AgentDomain = "proxy.example.test"
	if err := store.UpsertAgent(first); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAgent(second); err == nil {
		t.Fatal("duplicate tenant domain was accepted")
	}
}

func TestTenantSessionBindingIgnoresCallerTenantHints(t *testing.T) {
	store, err := OpenStore(":memory:", "tenant-session-secret")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	for _, cfg := range []Config{
		tenantTestConfig("agent-a", "owner-a", "Agent A"),
		tenantTestConfig("agent-b", "owner-b", "Agent B"),
	} {
		if err := store.UpsertAgent(cfg); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.UpsertUser("agent-b", "owner-b", "owner-b@example.com", "Owner B"); err != nil {
		t.Fatal(err)
	}
	sessionID, err := store.CreateTenantSession("agent-b", "owner-b", tenantRoleOwner, []byte(`{"id":"owner-b"}`), "", "", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadSession(sessionID)
	if err != nil || loaded.AgentID != "agent-b" || loaded.Role != tenantRoleOwner {
		t.Fatalf("tenant session was not persisted: %+v err=%v", loaded, err)
	}

	server := &Server{cfg: Config{AgentID: "agent-a", OwnerMainUserID: "owner-a", CookieName: "agentapi_session"}, store: store}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me?agent_id=agent-a", nil)
	req.Header.Set("X-Agent-ID", "agent-a")
	req.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	tenant, _, user, ok := server.loadTenantSession(req)
	if !ok || tenant.AgentID != "agent-b" || tenant.MainUserID != "owner-b" || tenant.Role != tenantRoleOwner || user.AgentID != "agent-b" {
		t.Fatalf("caller hints changed the server-bound tenant: tenant=%+v user=%+v ok=%v", tenant, user, ok)
	}
	if !server.isAgentAdmin(loaded) {
		t.Fatal("owner of agent-b was not recognized as the administrator of its own tenant")
	}

	adminReq := httptest.NewRequest(http.MethodGet, "/api/v1/agent/admin/branding?agent_id=agent-a", nil)
	adminReq.Header.Set("X-Agent-ID", "agent-a")
	adminReq.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	recorder := httptest.NewRecorder()
	server.handleAgentAdmin(recorder, adminReq, "tenant-admin-branding")
	if recorder.Code != http.StatusOK {
		t.Fatalf("tenant owner could not read its own branding: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "Agent B") || strings.Contains(body, "Agent A") {
		t.Fatalf("tenant admin response crossed tenant boundary: %s", body)
	}
}

func TestLegacySessionFallsBackOnlyToBootstrapTenant(t *testing.T) {
	store, err := OpenStore(":memory:", "legacy-session-secret")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	cfg := tenantTestConfig("agent-a", "owner-a", "Agent A")
	cfg.CookieName = "agentapi_session"
	if err := store.UpsertAgent(cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertUser(cfg.AgentID, "member-a", "member-a@example.com", "Member A"); err != nil {
		t.Fatal(err)
	}
	sessionID, err := store.CreateSession("member-a", []byte(`{"id":"member-a"}`), "", "", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{cfg: cfg, store: store}
	req := httptest.NewRequest(http.MethodGet, "/?agent_id=agent-b", nil)
	req.Header.Set("X-Agent-ID", "agent-b")
	req.AddCookie(&http.Cookie{Name: cfg.CookieName, Value: sessionID})
	tenant, _, _, ok := server.loadTenantSession(req)
	if !ok || tenant.AgentID != "agent-a" || tenant.Source != tenantSourceLegacy {
		t.Fatalf("legacy session did not remain bootstrap-scoped: %+v ok=%v", tenant, ok)
	}
}

func TestLegacySessionIsRejectedWithoutCompatibilityTenant(t *testing.T) {
	store, err := OpenStore(":memory:", "legacy-session-rejection-secret")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	cfg := tenantTestConfig("agent-a", "owner-a", "Agent A")
	if err := store.UpsertAgent(cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertUser(cfg.AgentID, "member-a", "member-a@example.com", "Member A"); err != nil {
		t.Fatal(err)
	}
	sessionID, err := store.CreateSession("member-a", []byte(`{"id":"member-a"}`), "", "", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{cfg: Config{CookieName: "agentapi_session"}, store: store}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	if tenant, _, _, ok := server.loadTenantSession(req); ok {
		t.Fatalf("shared runtime accepted tenantless legacy session: %+v", tenant)
	}
}

func TestLegacyAgentConfigSchemaMigratesToMultipleRows(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE agent_config (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		agent_id TEXT NOT NULL UNIQUE,
		domain TEXT NOT NULL DEFAULT '',
		name TEXT NOT NULL,
		site_name TEXT NOT NULL,
		site_logo TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'active',
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL
	); INSERT INTO agent_config(id,agent_id,name,site_name,created_at,updated_at) VALUES(1,'legacy-agent','Legacy','Legacy',1,1)`)
	if err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := OpenStore(dbPath, "migration-secret")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.UpsertAgent(tenantTestConfig("second-agent", "owner-2", "Second")); err != nil {
		t.Fatalf("legacy schema still rejected a second tenant: %v", err)
	}
	var count int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM agent_config`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("agent config migration count=%d err=%v", count, err)
	}
}
