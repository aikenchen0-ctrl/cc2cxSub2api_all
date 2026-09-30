package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestTenantBackupsRoundTripOnlyAgentOwnedConfiguration(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("tenant backup unexpectedly called Sub2API: %s", r.URL.Path)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)

	home := AgentHomeSettings{SiteSubtitle: "本站副标题", CompactHomeEnabled: true, HomeContent: "<h1>本站首页</h1>"}
	if _, err := server.store.UpdateBranding(server.cfg.AgentID, "本站名称", "本站标题", "https://cdn.example/logo.png", "https://docs.example.com", "客服微信 tenant-support", home); err != nil {
		t.Fatal(err)
	}
	if _, err := server.store.UpdateAgentModelPolicy(server.cfg.AgentID, []string{"gpt-5.5"}); err != nil {
		t.Fatal(err)
	}
	if _, err := server.store.CreateAnnouncement(server.cfg.AgentID, "本站公告", "仅本站配置", "active", "popup", time.Time{}, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if _, err := server.store.CreateContentPage(server.cfg.AgentID, "guide", "custom", "本站指南", "本站内容", "active", 3); err != nil {
		t.Fatal(err)
	}
	if _, err := server.store.UpsertAgentPlanPolicy(server.cfg.AgentID, AgentPlanPolicy{PlanID: 7, Enabled: true, SortOrder: 4, DisplayName: "本站套餐", Description: "本站说明", Features: []string{"功能一"}}); err != nil {
		t.Fatal(err)
	}

	sessionID, err := server.store.CreateSession("42", json.RawMessage(`{"id":"42"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path, body, origin string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "http://agent.local"+path, strings.NewReader(body))
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		req.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		return rec
	}

	created := request(http.MethodPost, "/api/v1/agent/admin/backups", "", "http://agent.local")
	if created.Code != http.StatusCreated {
		t.Fatalf("create backup status=%d body=%s", created.Code, created.Body.String())
	}
	var createdEnvelope struct {
		Data TenantBackupRecord `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &createdEnvelope); err != nil || createdEnvelope.Data.ID <= 0 {
		t.Fatalf("decode backup record: id=%d err=%v body=%s", createdEnvelope.Data.ID, err, created.Body.String())
	}
	id := createdEnvelope.Data.ID

	download := request(http.MethodGet, "/api/v1/agent/admin/backups/"+strconv.FormatInt(id, 10)+"/download", "", "")
	if download.Code != http.StatusOK {
		t.Fatalf("download backup status=%d body=%s", download.Code, download.Body.String())
	}
	body := download.Body.String()
	for _, expected := range []string{"本站名称", "本站标题", "https://docs.example.com", "客服微信 tenant-support", "gpt-5.5", "本站公告", "本站指南", "本站套餐"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("backup omitted %q: %s", expected, body)
		}
	}
	for _, forbidden := range []string{"u@example.com", "test-main-admin-key", "app-secret", "super_key", "access_token", "refresh_token", "main_balance", "wallet_available", "out_trade_no", "settlement_status"} {
		if strings.Contains(strings.ToLower(body), strings.ToLower(forbidden)) {
			t.Fatalf("backup leaked forbidden field/value %q: %s", forbidden, body)
		}
	}

	if _, err := server.store.UpdateBranding(server.cfg.AgentID, "已修改", "已修改", "", "", "", AgentHomeSettings{}); err != nil {
		t.Fatal(err)
	}
	restored := request(http.MethodPost, "/api/v1/agent/admin/backups/"+strconv.FormatInt(id, 10)+"/restore", "", "http://agent.local")
	if restored.Code != http.StatusOK {
		t.Fatalf("restore backup status=%d body=%s", restored.Code, restored.Body.String())
	}
	agent, err := server.store.Agent(server.cfg.AgentID)
	if err != nil || agent.Name != "本站名称" || agent.SiteName != "本站标题" || agent.DocURL != "https://docs.example.com" || agent.ContactInfo != "客服微信 tenant-support" || agent.AgentHomeSettings != home {
		t.Fatalf("branding was not restored: agent=%+v err=%v", agent, err)
	}
	list := request(http.MethodGet, "/api/v1/agent/admin/backups", "", "")
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"restored_at":"`) || list.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("backup list missing restored status/no-store: status=%d headers=%v body=%s", list.Code, list.Header(), list.Body.String())
	}
}

func TestTenantBackupsRejectOrdinaryCrossOriginAndCrossTenantOperations(t *testing.T) {
	upstream := httptest.NewServer(http.NotFoundHandler())
	defer upstream.Close()
	server := testServer(t, upstream)
	if _, err := server.store.UpsertUser(server.cfg.AgentID, "99", "ordinary@example.com", "Ordinary"); err != nil {
		t.Fatal(err)
	}
	ownerSession, err := server.store.CreateSession("42", json.RawMessage(`{"id":"42"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	ordinarySession, err := server.store.CreateSession("99", json.RawMessage(`{"id":"99"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}

	request := func(method, path, body, session, origin string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "http://agent.local"+path, strings.NewReader(body))
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		if session != "" {
			req.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: session})
		}
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		return rec
	}

	if got := request(http.MethodGet, "/api/v1/agent/admin/backups", "", ordinarySession, ""); got.Code != http.StatusForbidden {
		t.Fatalf("ordinary user listed backups: status=%d body=%s", got.Code, got.Body.String())
	}
	if got := request(http.MethodPost, "/api/v1/agent/admin/backups", "", ownerSession, "https://attacker.example"); got.Code != http.StatusForbidden {
		t.Fatalf("cross-origin backup create: status=%d body=%s", got.Code, got.Body.String())
	}

	snapshot, err := server.store.buildTenantBackupSnapshot(server.cfg.AgentID)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.SourceAgentID = "another-agent"
	payload, _ := json.Marshal(snapshot)
	if got := request(http.MethodPost, "/api/v1/agent/admin/backups/import", string(payload), ownerSession, "http://agent.local"); got.Code != http.StatusBadRequest || !strings.Contains(got.Body.String(), "different agent") {
		t.Fatalf("cross-tenant backup import accepted: status=%d body=%s", got.Code, got.Body.String())
	}
	if got := request(http.MethodGet, "/api/v1/agent/admin/backups/999/download", "", ownerSession, ""); got.Code != http.StatusNotFound {
		t.Fatalf("missing/cross-tenant backup was visible: status=%d body=%s", got.Code, got.Body.String())
	}
}

func TestTenantBackupRestoreSynchronizesManagedMainModelScope(t *testing.T) {
	var runtimeModels []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/v1/agent-runtime/model-policy" || r.Header.Get("X-AgentAPI-Runtime-Control") != testRuntimeControlCredential {
			t.Fatalf("unexpected runtime sync: method=%s path=%s headers=%v", r.Method, r.URL.Path, r.Header)
		}
		var payload struct {
			Enabled []string `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		runtimeModels = append([]string(nil), payload.Enabled...)
		_ = json.NewEncoder(w).Encode(envelope(map[string]any{"agent_id": "agent-test", "enabled": payload.Enabled}))
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	if _, err := server.store.UpdateAgentModelPolicy(server.cfg.AgentID, []string{"gpt-5.5"}); err != nil {
		t.Fatal(err)
	}
	backup, err := server.store.CreateTenantBackup(server.cfg.AgentID, "42")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.store.UpdateAgentModelPolicy(server.cfg.AgentID, []string{"gpt-image-2"}); err != nil {
		t.Fatal(err)
	}
	server.cfg.AppCredential = "agt_model_test-app-credential-0123456789abcdef"
	server.cfg.RuntimeControlCredential = testRuntimeControlCredential
	server.main = NewMainClient(server.cfg)
	sessionID, err := server.store.CreateSession("42", json.RawMessage(`{"id":"42"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "http://agent.local/api/v1/agent/admin/backups/"+strconv.FormatInt(backup.ID, 10)+"/restore", nil)
	req.Header.Set("Origin", "http://agent.local")
	req.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("managed restore status=%d body=%s", rec.Code, rec.Body.String())
	}
	if len(runtimeModels) != 1 || runtimeModels[0] != "gpt-5.5" {
		t.Fatalf("restored model scope was not synchronized to Sub2API: %v", runtimeModels)
	}
	policy, err := server.store.AgentModelPolicy(server.cfg.AgentID)
	if err != nil || len(policy.Enabled) != 1 || policy.Enabled[0] != "gpt-5.5" {
		t.Fatalf("local model policy diverged after restore: policy=%+v err=%v", policy, err)
	}
}
