package main

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	store, err := OpenStore(":memory:", "test-secret")
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{AgentID: "agent-test", AgentName: "Test", SiteName: "Test", InitialBalanceCents: 1000}
	if err := store.UpsertAgent(cfg); err != nil {
		store.Close()
		t.Fatal(err)
	}
	if _, err := store.UpsertUser(cfg.AgentID, "42", "u@example.com", "User"); err != nil {
		store.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestUsersPageReturnsStablePagesAndAgentScopedTotals(t *testing.T) {
	store := testStore(t)
	for id := 43; id <= 52; id++ {
		userID := strconv.Itoa(id)
		if _, err := store.UpsertUser("agent-test", userID, userID+"@example.com", "User "+userID); err != nil {
			t.Fatal(err)
		}
	}

	items, total, err := store.UsersPage("agent-test", 5, 5, "")
	if err != nil {
		t.Fatal(err)
	}
	if total != 11 || len(items) != 5 {
		t.Fatalf("unexpected second user page size or total: total=%d len=%d", total, len(items))
	}
	if items[0].MainUserID != "47" || items[4].MainUserID != "43" {
		t.Fatalf("user page order or offset is incorrect: first=%q last=%q", items[0].MainUserID, items[4].MainUserID)
	}

	otherAgentItems, otherAgentTotal, err := store.UsersPage("other-agent", 5, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(otherAgentItems) != 0 || otherAgentTotal != 0 {
		t.Fatalf("user page escaped its Agent scope: items=%+v total=%d", otherAgentItems, otherAgentTotal)
	}

	searchItems, searchTotal, err := store.UsersPage("agent-test", 5, 0, "uSeR 4")
	if err != nil {
		t.Fatal(err)
	}
	if searchTotal != 7 || len(searchItems) != 5 || searchItems[0].MainUserID != "49" {
		t.Fatalf("case-insensitive search did not filter before pagination: items=%+v total=%d", searchItems, searchTotal)
	}
	searchItems, searchTotal, err = store.UsersPage("agent-test", 5, 5, "52")
	if err != nil {
		t.Fatal(err)
	}
	if searchTotal != 1 || len(searchItems) != 0 {
		t.Fatalf("search total/page did not use the same filter: items=%+v total=%d", searchItems, searchTotal)
	}
	searchItems, searchTotal, err = store.UsersPage("agent-test", 5, 0, "52")
	if err != nil || searchTotal != 1 || len(searchItems) != 1 || searchItems[0].MainUserID != "52" {
		t.Fatalf("identifier search failed: items=%+v total=%d err=%v", searchItems, searchTotal, err)
	}
}

func TestAgentModelPolicyDefaultsToPublicCatalogAndPersistsAValidatedSubset(t *testing.T) {
	store := testStore(t)
	policy, err := store.AgentModelPolicy("agent-test")
	if err != nil {
		t.Fatal(err)
	}
	if policy.Customized || strings.Join(policy.Enabled, ",") != strings.Join(publicModelCatalog, ",") {
		t.Fatalf("unexpected default model policy: customized=%v enabled=%v", policy.Customized, policy.Enabled)
	}

	policy, err = store.UpdateAgentModelPolicy("agent-test", []string{"gpt-image-2", "gpt-5.5"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"gpt-5.5", "gpt-image-2"}
	if !policy.Customized || strings.Join(policy.Enabled, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected saved model policy: customized=%v enabled=%v", policy.Customized, policy.Enabled)
	}

	policy, err = store.AgentModelPolicy("agent-test")
	if err != nil || !policy.Customized || strings.Join(policy.Enabled, ",") != strings.Join(want, ",") {
		t.Fatalf("model policy was not persisted: policy=%+v err=%v", policy, err)
	}
	if _, err := store.UpdateAgentModelPolicy("agent-test", []string{"private-provider-model"}); err == nil {
		t.Fatal("model outside the public catalog was accepted")
	}
	if _, err := store.UpdateAgentModelPolicy("agent-test", []string{"gpt-5.5", "gpt-5.5"}); err == nil {
		t.Fatal("duplicate models were accepted")
	}
	policy, err = store.UpdateAgentModelPolicy("agent-test", []string{})
	if err != nil || !policy.Customized || len(policy.Enabled) != 0 {
		t.Fatalf("explicit empty policy should disable all models: policy=%+v err=%v", policy, err)
	}
	if _, err := store.UpdateAgentModelPolicy("missing-agent", []string{"gpt-5.5"}); !errors.Is(err, errNotFound) {
		t.Fatalf("unknown Agent policy update error=%v, want not found", err)
	}
}

func TestLegacySettlementTableUpgradesForUsageSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE settlements (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		agent_id TEXT NOT NULL,
		proxy_main_user_id TEXT NOT NULL,
		billing_main_user_id TEXT NOT NULL,
		request_id TEXT NOT NULL,
		usage_id TEXT NOT NULL,
		reserved_cents INTEGER NOT NULL,
		actual_cents INTEGER NOT NULL DEFAULT 0,
		status TEXT NOT NULL DEFAULT 'pending',
		error TEXT NOT NULL DEFAULT '',
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL,
		UNIQUE(agent_id, request_id)
	)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO settlements(agent_id, proxy_main_user_id, billing_main_user_id, request_id, usage_id, reserved_cents, status, created_at, updated_at) VALUES ('agent-test', '42', 'owner', 'legacy-request', 'legacy-request', 100, 'confirmed', 1, 1)`)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := OpenStore(path, "test-secret")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	items, err := store.Usage("agent-test", "42", 10)
	if err != nil || len(items) != 1 || items[0].RequestID != "legacy-request" || items[0].Source != "" || items[0].InputTokens != 0 {
		t.Fatalf("legacy settlement did not survive snapshot migration: items=%+v err=%v", items, err)
	}
}

func TestUsagePageCountsAndScopesRowsWithoutSilentTruncation(t *testing.T) {
	store := testStore(t)
	if _, err := store.UpsertUser("agent-test", "43", "other@example.com", "Other"); err != nil {
		t.Fatal(err)
	}
	if err := store.Allocate("agent-test", "42", 600, "usage-page-alloc-42", "order-42", "test"); err != nil {
		t.Fatal(err)
	}
	if err := store.Allocate("agent-test", "43", 100, "usage-page-alloc-43", "order-43", "test"); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 5; i++ {
		requestID := "page-user-42-" + strconv.Itoa(i)
		if _, created, err := store.PrepareSettlement("agent-test", "42", "owner", requestID, requestID, 10); err != nil || !created {
			t.Fatalf("create paged settlement %d: created=%v err=%v", i, created, err)
		}
	}
	if _, created, err := store.PrepareSettlement("agent-test", "43", "owner", "page-user-43", "page-user-43", 10); err != nil || !created {
		t.Fatalf("create second user's settlement: created=%v err=%v", created, err)
	}

	items, total, err := store.UsagePage("agent-test", "42", 2, 2)
	if err != nil || total != 5 || len(items) != 2 || items[0].RequestID != "page-user-42-3" || items[1].RequestID != "page-user-42-2" {
		t.Fatalf("unexpected scoped usage page: items=%+v total=%d err=%v", items, total, err)
	}
	adminItems, adminTotal, err := store.UsagePage("agent-test", "", 10, 0)
	if err != nil || adminTotal != 6 || len(adminItems) != 6 {
		t.Fatalf("agent admin page missed records: items=%+v total=%d err=%v", adminItems, adminTotal, err)
	}
}

func TestWalletAllocationConsumptionAndRefundAreIdempotent(t *testing.T) {
	store := testStore(t)
	if err := store.Allocate("agent-test", "42", 400, "alloc-1", "order-1", "test"); err != nil {
		t.Fatal(err)
	}
	if err := store.Allocate("agent-test", "42", 400, "alloc-1", "order-1", "retry"); err != nil {
		t.Fatalf("idempotent allocation: %v", err)
	}
	if err := store.Consume("agent-test", "42", 150, "req-1", "test"); err != nil {
		t.Fatal(err)
	}
	if err := store.Consume("agent-test", "42", 150, "req-1", "retry"); err != nil {
		t.Fatalf("idempotent consumption: %v", err)
	}
	if err := store.Credit("agent-test", "42", 50, "refund-1", "test"); err != nil {
		t.Fatal(err)
	}
	if err := store.Credit("agent-test", "42", 50, "refund-1", "retry"); err != nil {
		t.Fatalf("idempotent refund: %v", err)
	}

	agent, err := store.Agent("agent-test")
	if err != nil {
		t.Fatal(err)
	}
	// Initial 1000 - allocation 400; consumption releases 150 from the
	// allocated amount, while a refund restores 50 to the user sub-wallet.
	if agent.WalletAvailable != 600 || agent.WalletAllocated != 300 {
		t.Fatalf("unexpected agent wallet: %+v", agent)
	}
	user, err := store.User("agent-test", "42")
	if err != nil {
		t.Fatal(err)
	}
	if user.BalanceCents != 300 {
		t.Fatalf("unexpected user balance: %+v", user)
	}
}

func TestWalletRejectsOverspend(t *testing.T) {
	store := testStore(t)
	if err := store.Consume("agent-test", "42", 1, "req", "test"); !errors.Is(err, errInsufficientBalance) {
		t.Fatalf("want insufficient balance, got %v", err)
	}
	if err := store.Allocate("agent-test", "42", 1001, "alloc", "order", "test"); !errors.Is(err, errInsufficientBalance) {
		t.Fatalf("want insufficient allocation balance, got %v", err)
	}
}

func TestWalletIdempotencyKeyCannotBeReusedAcrossUsersOrAmounts(t *testing.T) {
	store := testStore(t)
	if _, err := store.UpsertUser("agent-test", "43", "other@example.com", "Other"); err != nil {
		t.Fatal(err)
	}
	if err := store.Allocate("agent-test", "42", 100, "same-key", "order", "test"); err != nil {
		t.Fatal(err)
	}
	if err := store.Allocate("agent-test", "43", 100, "same-key", "order", "test"); !errors.Is(err, errIdempotencyConflict) {
		t.Fatalf("cross-user allocation reused key: %v", err)
	}
	if err := store.Consume("agent-test", "42", 25, "consume-key", "test"); err != nil {
		t.Fatal(err)
	}
	if err := store.Consume("agent-test", "42", 30, "consume-key", "test"); !errors.Is(err, errIdempotencyConflict) {
		t.Fatalf("different amount reused key: %v", err)
	}
}

func TestPrepareSettlementAtomicallyReservesAndReplays(t *testing.T) {
	store := testStore(t)
	if err := store.Allocate("agent-test", "42", 400, "prepare-alloc", "order", "test"); err != nil {
		t.Fatal(err)
	}
	record, created, err := store.PrepareSettlement("agent-test", "42", "owner", "model-1", "model-1", 150)
	if err != nil || !created || record.Status != "pending" {
		t.Fatalf("prepare settlement: record=%+v created=%v err=%v", record, created, err)
	}
	replayed, created, err := store.PrepareSettlement("agent-test", "42", "owner", "model-1", "model-1", 150)
	if err != nil || created || replayed.ID != record.ID {
		t.Fatalf("settlement replay was not idempotent: record=%+v created=%v err=%v", replayed, created, err)
	}
	user, err := store.User("agent-test", "42")
	if err != nil {
		t.Fatal(err)
	}
	if user.BalanceCents != 250 {
		t.Fatalf("replay changed user balance: %+v", user)
	}
	if err := store.SettleSettlementWithUsage("agent-test", "model-1", "usage-1", "confirmed", 100, ""); err != nil {
		t.Fatal(err)
	}
	if err := store.SettleSettlementWithUsage("agent-test", "model-1", "usage-1", "confirmed", 100, ""); err != nil {
		t.Fatalf("terminal settlement should be idempotent: %v", err)
	}
	if err := store.SettleSettlementWithUsage("agent-test", "model-1", "usage-2", "confirmed", 100, ""); !errors.Is(err, errSettlementStateConflict) {
		t.Fatalf("expected terminal state conflict, got %v", err)
	}
}

func TestFinalizeSettlementAtomicallyRefundsUnusedReservation(t *testing.T) {
	store := testStore(t)
	if err := store.Allocate("agent-test", "42", 400, "finalize-alloc", "order", "test"); err != nil {
		t.Fatal(err)
	}
	if _, created, err := store.PrepareSettlement("agent-test", "42", "owner", "finalize-request", "finalize-request", 150); err != nil || !created {
		t.Fatalf("prepare settlement: created=%v err=%v", created, err)
	}
	if err := store.FinalizeSettlement("agent-test", "finalize-request", "usage-1", "confirmed", 100, ""); err != nil {
		t.Fatal(err)
	}
	// A repeated finalization must not write another refund or change either
	// balance a second time.
	if err := store.FinalizeSettlement("agent-test", "finalize-request", "usage-1", "confirmed", 100, ""); err != nil {
		t.Fatalf("idempotent finalization: %v", err)
	}
	user, err := store.User("agent-test", "42")
	if err != nil {
		t.Fatal(err)
	}
	if user.BalanceCents != 300 {
		t.Fatalf("unused reservation was not refunded exactly once: %+v", user)
	}
	agent, err := store.Agent("agent-test")
	if err != nil {
		t.Fatal(err)
	}
	if agent.WalletAvailable != 600 || agent.WalletAllocated != 300 {
		t.Fatalf("unexpected wallet after finalization: %+v", agent)
	}
}

func TestOwnerChangeRejectsPendingSettlement(t *testing.T) {
	store := testStore(t)
	if err := store.Allocate("agent-test", "42", 400, "owner-change-alloc", "order", "test"); err != nil {
		t.Fatal(err)
	}
	if _, created, err := store.PrepareSettlement("agent-test", "42", "old-owner", "owner-change-request", "owner-change-request", 100); err != nil || !created {
		t.Fatalf("prepare settlement: created=%v err=%v", created, err)
	}
	cfg := Config{AgentID: "agent-test", AgentName: "Test", SiteName: "Test", OwnerMainUserID: "new-owner"}
	if err := store.UpsertAgent(cfg); err == nil || !strings.Contains(err.Error(), "pending=100") {
		t.Fatalf("owner change with pending settlement was accepted: %v", err)
	}
}

func TestDisabledAgentStartsSuspended(t *testing.T) {
	store, err := OpenStore(":memory:", "test-secret")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	cfg := Config{
		AgentID: "disabled-agent", AgentName: "Disabled", SiteName: "Disabled",
		RuntimeControlCredential: "agt_ctl_test-control-secret", AppCredential: "app", OwnerMainUserID: "owner", BillingMode: "owner_upstream", AgentDisabled: true,
	}
	if err := store.UpsertAgent(cfg); err != nil {
		t.Fatal(err)
	}
	agent, err := store.Agent(cfg.AgentID)
	if err != nil {
		t.Fatal(err)
	}
	if agent.Status != "suspended" {
		t.Fatalf("disabled agent status=%q, want suspended", agent.Status)
	}
}

func TestRuntimeControlCredentialIsRequiredOnlyForProvisionedAgents(t *testing.T) {
	cfg := Config{
		AgentID: "agent-local", AppCredential: "app", OwnerMainUserID: "owner",
		BillingMode: "owner_upstream",
	}
	if got := agentConfigStatus(cfg); got != "active" {
		t.Fatalf("local agent without provisioning control status=%q, want active", got)
	}

	cfg.AgentID = "agt_test"
	cfg.ProvisioningControlEnabled = true
	if got := agentConfigStatus(cfg); got != "suspended" {
		t.Fatalf("managed agent without runtime control credential status=%q, want suspended", got)
	}
	cfg.RuntimeControlCredential = "agt_ctl_test-runtime-control-secret"
	if got := agentConfigStatus(cfg); got != "active" {
		t.Fatalf("managed agent with runtime control credential status=%q, want active", got)
	}
}

func TestBrandingUpdateSurvivesRestartUnlessEnvSyncIsExplicit(t *testing.T) {
	store, err := OpenStore(":memory:", "test-secret")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	cfg := Config{AgentID: "brand-agent", AgentName: "Initial", SiteName: "Initial Site", SiteLogo: "/initial.svg"}
	if err := store.UpsertAgent(cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateBranding(cfg.AgentID, "Edited", "Edited Site", "https://cdn.example.com/edited.svg"); err != nil {
		t.Fatal(err)
	}
	// A normal restart keeps the admin-edited branding instead of silently
	// replacing it with the values from the environment.
	if err := store.UpsertAgent(Config{AgentID: cfg.AgentID, AgentName: "Env Name", SiteName: "Env Site", SiteLogo: "/env.svg"}); err != nil {
		t.Fatal(err)
	}
	agent, err := store.Agent(cfg.AgentID)
	if err != nil {
		t.Fatal(err)
	}
	if agent.Name != "Edited" || agent.SiteName != "Edited Site" || agent.SiteLogo != "https://cdn.example.com/edited.svg" {
		t.Fatalf("normal restart overwrote branding: %+v", agent)
	}
	if err := store.UpsertAgent(Config{AgentID: cfg.AgentID, AgentName: "Env Name", SiteName: "Env Site", SiteLogo: "/env.svg", BrandSync: true}); err != nil {
		t.Fatal(err)
	}
	agent, err = store.Agent(cfg.AgentID)
	if err != nil {
		t.Fatal(err)
	}
	if agent.Name != "Env Name" || agent.SiteName != "Env Site" || agent.SiteLogo != "/env.svg" {
		t.Fatalf("explicit env branding sync did not apply: %+v", agent)
	}
}

func TestParseCents(t *testing.T) {
	for _, test := range []struct {
		raw  string
		want int64
	}{
		{"1", 100}, {"1.2", 120}, {"1.23", 123}, {"0.01", 1}, {"-2.50", -250},
	} {
		got, err := parseCents(test.raw)
		if err != nil || got != test.want {
			t.Errorf("parseCents(%q) = %d, %v; want %d", test.raw, got, err, test.want)
		}
	}
}

func TestAgentAPIKeyIsOneTimeVisibleAndRevocable(t *testing.T) {
	store := testStore(t)
	view, raw, err := store.CreateAPIKey("agent-test", "42", "desktop client")
	if err != nil {
		t.Fatal(err)
	}
	if raw == "" || view.Prefix == "" || view.Status != "active" {
		t.Fatalf("unexpected key creation result: view=%+v raw=%q", view, raw)
	}
	keys, err := store.APIKeys("agent-test", "42")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0].Prefix != view.Prefix || keys[0].Name != "desktop client" {
		t.Fatalf("unexpected key list: %+v", keys)
	}
	if _, err := store.ResolveAPIKey("agent-test", raw); err != nil {
		t.Fatalf("created key did not resolve: %v", err)
	}
	if err := store.RevokeAPIKey("agent-test", "42", view.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ResolveAPIKey("agent-test", raw); !errors.Is(err, errNotFound) {
		t.Fatalf("revoked key resolved: %v", err)
	}
}

func TestAuditEventsAreAgentScopedAndNewestFirst(t *testing.T) {
	store := testStore(t)
	store.clock = func() time.Time { return time.Unix(1700000000, 0) }
	for _, event := range []AuditEvent{
		{ActorType: "agent_admin", ActorID: "owner-1", AgentID: "agent-test", Operation: "branding.update", TargetType: "agent", TargetID: "agent-test", RequestID: "req-1", Result: "success"},
		{ActorType: "agent_admin", ActorID: "owner-1", AgentID: "agent-test", Operation: "wallet.sync", TargetType: "agent_wallet", TargetID: "agent-test", RequestID: "req-2", Result: "success"},
		{ActorType: "agent_admin", ActorID: "owner-2", AgentID: "agent-other", Operation: "wallet.sync", TargetType: "agent_wallet", TargetID: "agent-other", RequestID: "req-3", Result: "success"},
	} {
		if err := store.RecordAuditEvent(event); err != nil {
			t.Fatalf("record audit event: %v", err)
		}
	}
	items, err := store.AuditEvents("agent-test", 10)
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	if len(items) != 2 || items[0].RequestID != "req-2" || items[1].RequestID != "req-1" {
		t.Fatalf("unexpected audit events: %+v", items)
	}
	if items[0].CreatedAt != "2023-11-14T22:13:20Z" {
		t.Fatalf("unexpected created_at: %q", items[0].CreatedAt)
	}
}

func TestVideoTaskMappingIsScopedAndIdempotent(t *testing.T) {
	store := testStore(t)
	task, err := store.RecordVideoTask("agent-test", "42", "video-1", "request-1", "queued")
	if err != nil || task.TaskID != "video-1" || task.MainUserID != "42" {
		t.Fatalf("record video task: task=%+v err=%v", task, err)
	}
	if _, err := store.RecordVideoTask("agent-test", "42", "video-1", "request-1", "processing"); err != nil {
		t.Fatalf("idempotent video update failed: %v", err)
	}
	if _, err := store.RecordVideoTask("agent-test", "other", "video-1", "request-other", "queued"); !errors.Is(err, errIdempotencyConflict) {
		t.Fatalf("video task ownership conflict was accepted: %v", err)
	}
	if err := store.UpdateVideoTaskStatus("agent-test", "video-1", "completed"); err != nil {
		t.Fatal(err)
	}
	updated, err := store.VideoTask("agent-test", "video-1")
	if err != nil || updated.Status != "completed" {
		t.Fatalf("unexpected video task: %+v err=%v", updated, err)
	}
}

func TestPendingVideoTasksTreatsDoneAsTerminal(t *testing.T) {
	store := testStore(t)
	for _, item := range []struct {
		id     string
		status string
	}{
		{id: "video-queued", status: "queued"},
		{id: "video-done", status: "done"},
		{id: "video-completed", status: "completed"},
		{id: "video-failed-pending", status: "failed"},
	} {
		if item.id == "video-failed-pending" {
			if err := store.Allocate("agent-test", "42", 100, "allocate-pending-video-task", "order", "test"); err != nil {
				t.Fatal(err)
			}
			if _, _, err := store.PrepareSettlement("agent-test", "42", "owner", "request-"+item.id, "request-"+item.id, 50); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := store.RecordVideoTask("agent-test", "42", item.id, "request-"+item.id, item.status); err != nil {
			t.Fatalf("record %s video task: %v", item.status, err)
		}
	}

	pending, err := store.PendingVideoTasks("agent-test", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 2 {
		t.Fatalf("terminal tasks should only be re-polled while their settlement is pending: %+v", pending)
	}
	found := map[string]bool{}
	for _, task := range pending {
		found[task.TaskID] = true
	}
	if !found["video-queued"] || !found["video-failed-pending"] || found["video-done"] || found["video-completed"] {
		t.Fatalf("unexpected pending video tasks: %+v", pending)
	}
}

func TestImageTaskMappingIsScopedIdempotentAndTerminalAware(t *testing.T) {
	store := testStore(t)
	task, err := store.RecordImageTask("agent-test", "42", "image-1", "request-image-1", "processing")
	if err != nil || task.TaskID != "image-1" || task.MainUserID != "42" {
		t.Fatalf("record image task: task=%+v err=%v", task, err)
	}
	if _, err := store.RecordImageTask("agent-test", "42", "image-1", "request-image-1", "completed"); err != nil {
		t.Fatalf("idempotent image task update failed: %v", err)
	}
	if _, err := store.RecordImageTask("agent-test", "other", "image-1", "request-other", "processing"); !errors.Is(err, errIdempotencyConflict) {
		t.Fatalf("image task ownership conflict was accepted: %v", err)
	}
	if err := store.UpdateImageTaskStatus("agent-test", "image-1", "done"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RecordImageTask("agent-test", "42", "image-pending", "request-image-pending", "queued"); err != nil {
		t.Fatal(err)
	}
	if err := store.Allocate("agent-test", "42", 100, "allocate-pending-image-task", "order", "test"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.PrepareSettlement("agent-test", "42", "owner", "request-image-failed-pending", "request-image-failed-pending", 50); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RecordImageTask("agent-test", "42", "image-failed-pending", "request-image-failed-pending", "failed"); err != nil {
		t.Fatal(err)
	}
	pending, err := store.PendingImageTasks("agent-test", 10)
	if err != nil || len(pending) != 2 {
		t.Fatalf("terminal image task should only be re-polled while its settlement is pending: pending=%+v err=%v", pending, err)
	}
	found := map[string]bool{}
	for _, task := range pending {
		found[task.TaskID] = true
	}
	if !found["image-pending"] || !found["image-failed-pending"] || found["image-1"] {
		t.Fatalf("unexpected pending image tasks: %+v", pending)
	}
}
