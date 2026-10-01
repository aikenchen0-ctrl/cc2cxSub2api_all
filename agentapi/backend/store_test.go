package main

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func pragmaIndexColumns(t *testing.T, db *sql.DB, index string) []string {
	t.Helper()
	rows, err := db.Query(fmt.Sprintf(`PRAGMA index_info(%q)`, index))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var columns []string
	for rows.Next() {
		var sequence, columnID int
		var name string
		if err := rows.Scan(&sequence, &columnID, &name); err != nil {
			t.Fatal(err)
		}
		columns = append(columns, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return columns
}

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

func TestFreshStoreDoesNotCreateRemovedManagementTables(t *testing.T) {
	store := testStore(t)
	for _, table := range []string{
		"agent_content_pages",
		"agent_plan_policies",
		"agent_model_policies",
		"agent_group_policies",
		"agent_config_backups",
		"agent_prompt_audit_events",
		"agent_prompt_audit_policies",
		"agent_risk_events",
		"agent_risk_policies",
		"agent_risk_rate_buckets",
	} {
		var name string
		err := store.db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&name)
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("removed management table %q still exists: name=%q err=%v", table, name, err)
		}
	}
}

func TestTenantSchemaContracts(t *testing.T) {
	store := testStore(t)
	tenantTables := []string{
		"registration_intents", "registration_limits", "agent_config", "agent_users", "sessions",
		"agent_wallets", "agent_user_wallets", "wallet_ledger", "agent_api_keys", "agent_balance_snapshots",
		"settlements", "recharge_orders", "payment_events", "payment_config", "audit_events",
		"agent_announcements", "agent_announcement_reads", "video_tasks", "image_tasks",
	}
	for _, table := range tenantTables {
		rows, err := store.db.Query(fmt.Sprintf(`PRAGMA table_info(%q)`, table))
		if err != nil {
			t.Fatalf("inspect %s: %v", table, err)
		}
		foundAgentID, agentIDNotNull := false, false
		for rows.Next() {
			var position, notNull, primaryKey int
			var name, dataType string
			var defaultValue any
			if err := rows.Scan(&position, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
				_ = rows.Close()
				t.Fatalf("inspect %s column: %v", table, err)
			}
			if name == "agent_id" {
				foundAgentID = true
				agentIDNotNull = notNull == 1 || primaryKey > 0
			}
		}
		if err := rows.Close(); err != nil {
			t.Fatalf("close %s schema rows: %v", table, err)
		}
		if !foundAgentID || !agentIDNotNull {
			t.Fatalf("tenant table %s must have a non-null agent_id column", table)
		}
	}

	expectedIndexes := map[string][]string{
		"idx_agent_api_keys_user":           {"agent_id", "main_user_id", "status"},
		"idx_agent_balance_snapshots_agent": {"agent_id", "checked_at"},
		"idx_settlements_agent_user":        {"agent_id", "proxy_main_user_id", "created_at"},
		"idx_settlements_agent_key":         {"agent_id", "agent_api_key_id", "created_at"},
		"idx_recharge_orders_user":          {"agent_id", "main_user_id", "created_at"},
		"idx_recharge_orders_status":        {"agent_id", "status", "updated_at"},
		"idx_audit_events_agent":            {"agent_id", "id"},
		"idx_agent_announcements_visible":   {"agent_id", "status", "starts_at", "ends_at", "id"},
		"idx_agent_announcement_reads_user": {"agent_id", "main_user_id", "read_at"},
		"idx_video_tasks_user":              {"agent_id", "main_user_id", "updated_at"},
		"idx_image_tasks_user":              {"agent_id", "main_user_id", "updated_at"},
	}
	for index, expected := range expectedIndexes {
		actual := pragmaIndexColumns(t, store.db, index)
		if strings.Join(actual, ",") != strings.Join(expected, ",") {
			t.Fatalf("index %s columns = %v, want %v", index, actual, expected)
		}
	}
}

func TestAnnouncementReadSchemaRejectsCrossTenantParent(t *testing.T) {
	store := testStore(t)
	other := Config{AgentID: "agent-other", AgentName: "Other", SiteName: "Other"}
	if err := store.UpsertAgent(other); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertUser(other.AgentID, "42", "other@example.com", "Other User"); err != nil {
		t.Fatal(err)
	}
	announcement, err := store.CreateAnnouncement("agent-test", "Tenant A", "Only tenant A may read this", "active", "silent", time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.db.Exec(`INSERT INTO agent_announcement_reads(agent_id, announcement_id, main_user_id, read_at) VALUES (?, ?, ?, ?)`, other.AgentID, announcement.ID, "42", time.Now().UTC().Unix())
	if err == nil || !strings.Contains(err.Error(), "announcement tenant mismatch") {
		t.Fatalf("cross-tenant announcement read insert error = %v, want tenant mismatch", err)
	}
}

func TestStoreStartupRejectsLegacyCrossTenantAnnouncementRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-cross-tenant-read.sqlite")
	store, err := OpenStore(path, "test-secret")
	if err != nil {
		t.Fatal(err)
	}
	for _, cfg := range []Config{
		{AgentID: "agent-a", AgentName: "Agent A", SiteName: "Agent A"},
		{AgentID: "agent-b", AgentName: "Agent B", SiteName: "Agent B"},
	} {
		if err := store.UpsertAgent(cfg); err != nil {
			t.Fatal(err)
		}
		if _, err := store.UpsertUser(cfg.AgentID, "42", cfg.AgentID+"@example.com", cfg.AgentName); err != nil {
			t.Fatal(err)
		}
	}
	announcement, err := store.CreateAnnouncement("agent-a", "Agent A", "Tenant-owned", "active", "silent", time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	for _, trigger := range []string{
		"trg_agent_announcement_reads_tenant_insert",
		"trg_agent_announcement_reads_tenant_update",
	} {
		if _, err := store.db.Exec(`DROP TRIGGER ` + trigger); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.db.Exec(`INSERT INTO agent_announcement_reads(agent_id, announcement_id, main_user_id, read_at) VALUES ('agent-b', ?, '42', ?)`, announcement.ID, time.Now().UTC().Unix()); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenStore(path, "test-secret")
	if reopened != nil {
		_ = reopened.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "cross-tenant or orphaned rows") {
		t.Fatalf("OpenStore error = %v, want legacy tenant audit failure", err)
	}
}

func TestStoreMigrationDropsRetiredManagementTables(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-management.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	removedTables := []string{
		"agent_content_pages",
		"agent_plan_policies",
		"agent_model_policies",
		"agent_group_policies",
		"agent_config_backups",
		"agent_prompt_audit_events",
		"agent_prompt_audit_policies",
		"agent_risk_events",
		"agent_risk_policies",
		"agent_risk_rate_buckets",
	}
	for _, table := range removedTables {
		if _, err := db.Exec(`CREATE TABLE ` + table + ` (id INTEGER PRIMARY KEY)`); err != nil {
			_ = db.Close()
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := OpenStore(path, "test-secret")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	for _, table := range removedTables {
		var name string
		err := store.db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&name)
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("retired table %q survived migration: name=%q err=%v", table, name, err)
		}
	}
}

func TestPaymentConfigIsInstanceScopedAndEncryptsWebhookSecret(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentapi.db")
	store, err := OpenStore(path, "instance-encryption-secret")
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		AgentID: "agent-a", AgentName: "Agent A", SiteName: "Agent A",
		PaymentProvider: "manual", PaymentCurrency: "CNY", PaymentMinCents: 100,
		PaymentMaxCents: 100000, PaymentOrderTTL: 30 * time.Minute,
	}
	if err := store.UpsertAgent(cfg); err != nil {
		t.Fatal(err)
	}
	wantSecret := "provider-webhook-secret-a"
	updated, err := store.UpdatePaymentConfig(cfg.AgentID, PaymentConfig{
		Enabled: true, Provider: "stripe-cn", Currency: "CNY", MerchantID: "merchant-a",
		WebhookSecret: wantSecret, MinAmountCents: 200, MaxAmountCents: 200000,
		OrderTTLSeconds: 900, CheckoutURLTemplate: "https://pay.example/{merchant_id}/{order_no}",
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Enabled || !updated.WebhookSecretSet || updated.WebhookSecret != wantSecret || updated.MerchantID != "merchant-a" {
		t.Fatalf("unexpected payment config: %+v", updated)
	}
	var storedCipher string
	if err := store.db.QueryRow(`SELECT webhook_secret_cipher FROM payment_config WHERE agent_id=?`, cfg.AgentID).Scan(&storedCipher); err != nil {
		t.Fatal(err)
	}
	if storedCipher == "" || storedCipher == wantSecret || strings.Contains(storedCipher, wantSecret) {
		t.Fatalf("payment secret was not encrypted at rest: %q", storedCipher)
	}
	if _, err := store.PaymentConfig("agent-b"); !errors.Is(err, errNotFound) {
		t.Fatalf("another instance unexpectedly read agent-a payment config: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenStore(path, "instance-encryption-secret")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	loaded, err := reopened.PaymentConfig(cfg.AgentID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.WebhookSecret != wantSecret || loaded.Provider != "stripe-cn" || loaded.MerchantID != "merchant-a" {
		t.Fatalf("payment config did not persist per instance: %+v", loaded)
	}
}

func TestPaymentConfigBootstrapRequiresSecretButStoredConfigSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentapi.db")
	store, err := OpenStore(path, "instance-encryption-secret")
	if err != nil {
		t.Fatal(err)
	}
	bootstrap := Config{
		AgentID: "agent-payment-restart", AgentName: "Agent", SiteName: "Agent",
		PaymentEnabled: true, PaymentProvider: "manual", PaymentCurrency: "CNY",
		PaymentMinCents: 100, PaymentMaxCents: 100000, PaymentOrderTTL: 30 * time.Minute,
	}
	if err := store.UpsertAgent(bootstrap); err == nil || !strings.Contains(err.Error(), "required when seeding") {
		t.Fatalf("enabled payment bootstrap without secret error = %v, want rejection", err)
	}

	bootstrap.PaymentWebhookSecret = "bootstrap-payment-secret"
	if err := store.UpsertAgent(bootstrap); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	restarted, err := OpenStore(path, "instance-encryption-secret")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restarted.Close() })
	bootstrap.PaymentWebhookSecret = ""
	if err := restarted.UpsertAgent(bootstrap); err != nil {
		t.Fatalf("restart rejected persisted payment configuration: %v", err)
	}
	loaded, err := restarted.PaymentConfig(bootstrap.AgentID)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.Enabled || loaded.WebhookSecret != "bootstrap-payment-secret" {
		t.Fatalf("persisted payment configuration changed during restart: %+v", loaded)
	}
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

	if _, err := store.SetUserStatus("agent-test", "44", "disabled"); err != nil {
		t.Fatal(err)
	}
	disabledItems, disabledTotal, err := store.UsersPageFiltered("agent-test", 5, 0, "", "disabled")
	if err != nil || disabledTotal != 1 || len(disabledItems) != 1 || disabledItems[0].MainUserID != "44" {
		t.Fatalf("local status filter escaped or returned the wrong users: items=%+v total=%d err=%v", disabledItems, disabledTotal, err)
	}
	activeItems, activeTotal, err := store.UsersPageFiltered("agent-test", 20, 0, "", "active")
	if err != nil || activeTotal != 10 || len(activeItems) != 10 {
		t.Fatalf("active status filter returned the wrong page: items=%+v total=%d err=%v", activeItems, activeTotal, err)
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

func TestRechargePaymentValidationAndLedgerReconciliation(t *testing.T) {
	store := testStore(t)
	order, created, err := store.CreateRechargeOrder(
		"agent-test",
		"42",
		200,
		"CNY",
		"manual",
		"",
		"recharge-validation",
		time.Now().Add(time.Hour),
	)
	if err != nil || !created {
		t.Fatalf("create recharge order: order=%+v created=%v err=%v", order, created, err)
	}

	rejected := []struct {
		name        string
		eventID     string
		status      string
		amountCents int64
		currency    string
		provider    string
		providerTxn string
		payloadHash string
	}{
		{name: "amount", eventID: "evt-wrong-amount", status: "paid", amountCents: 201, currency: "CNY", provider: "manual", providerTxn: "trade-wrong-amount", payloadHash: "hash-wrong-amount"},
		{name: "currency", eventID: "evt-wrong-currency", status: "paid", amountCents: 200, currency: "USD", provider: "manual", providerTxn: "trade-wrong-currency", payloadHash: "hash-wrong-currency"},
		{name: "provider", eventID: "evt-wrong-provider", status: "paid", amountCents: 200, currency: "CNY", provider: "other", providerTxn: "trade-wrong-provider", payloadHash: "hash-wrong-provider"},
		{name: "status", eventID: "evt-wrong-status", status: "processing", amountCents: 200, currency: "CNY", provider: "manual", providerTxn: "trade-wrong-status", payloadHash: "hash-wrong-status"},
	}
	for _, tc := range rejected {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := store.RecordPaymentEvent("agent-test", tc.provider, tc.eventID, order.OrderNo, tc.status, tc.amountCents, tc.currency, tc.providerTxn, tc.payloadHash); err == nil {
				t.Fatalf("invalid %s payment event was accepted", tc.name)
			}
			stored, lookupErr := store.RechargeOrder("agent-test", order.OrderNo)
			if lookupErr != nil {
				t.Fatal(lookupErr)
			}
			if stored.Status != "pending" {
				t.Fatalf("invalid %s payment event changed order state: %+v", tc.name, stored)
			}
			user, userErr := store.User("agent-test", "42")
			if userErr != nil {
				t.Fatal(userErr)
			}
			if user.BalanceCents != 0 {
				t.Fatalf("invalid %s payment event credited user: %+v", tc.name, user)
			}
		})
	}

	paid, err := store.RecordPaymentEvent("agent-test", "manual", "evt-paid", order.OrderNo, "paid", 200, "CNY", "trade-paid", "hash-paid")
	if err != nil || !paid.Created || paid.Order.Status != "paid_pending_allocation" {
		t.Fatalf("record verified payment: result=%+v err=%v", paid, err)
	}
	allocated, err := store.AllocateRechargeOrder("agent-test", order.OrderNo)
	if err != nil || allocated.Status != "allocated" {
		t.Fatalf("allocate verified recharge: order=%+v err=%v", allocated, err)
	}
	if _, err := store.AllocateRechargeOrder("agent-test", order.OrderNo); err != nil {
		t.Fatalf("replay verified recharge allocation: %v", err)
	}

	var ledgerRows int
	var userAllocation, agentAllocation int64
	if err := store.db.QueryRow(`
		SELECT COUNT(*),
			COALESCE(SUM(CASE WHEN kind='user_allocate' THEN amount_cents ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN kind='agent_allocate' THEN amount_cents ELSE 0 END), 0)
		FROM wallet_ledger
		WHERE agent_id=? AND order_id=?
	`, "agent-test", order.OrderNo).Scan(&ledgerRows, &userAllocation, &agentAllocation); err != nil {
		t.Fatal(err)
	}
	if ledgerRows != 2 || userAllocation != 200 || agentAllocation != -200 {
		t.Fatalf("recharge ledger does not reconcile: rows=%d user=%d agent=%d", ledgerRows, userAllocation, agentAllocation)
	}

	agent, err := store.Agent("agent-test")
	if err != nil {
		t.Fatal(err)
	}
	user, err := store.User("agent-test", "42")
	if err != nil {
		t.Fatal(err)
	}
	var userWalletTotal int64
	if err := store.db.QueryRow(`SELECT COALESCE(SUM(balance_cents), 0) FROM agent_user_wallets WHERE agent_id=?`, "agent-test").Scan(&userWalletTotal); err != nil {
		t.Fatal(err)
	}
	if user.BalanceCents < 0 || user.BalanceCents != 200 || userWalletTotal != agent.WalletAllocated || agent.WalletAvailable != 800 {
		t.Fatalf("wallet invariant failed: agent=%+v user=%+v user_total=%d", agent, user, userWalletTotal)
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

func TestSettlementRefundUsesActualLocalDebitNotIdentityEquality(t *testing.T) {
	for _, direct := range []bool{false, true} {
		t.Run(strconv.FormatBool(direct), func(t *testing.T) {
			store := testStore(t)
			if err := store.Allocate("agent-test", "42", 400, "alloc", "order", "test"); err != nil {
				t.Fatal(err)
			}
			prepare := store.PrepareSettlement
			if direct {
				prepare = store.PrepareDirectSettlement
			}
			if _, _, err := prepare("agent-test", "42", "42", "self-request", "self-request", 150); err != nil {
				t.Fatal(err)
			}
			record, err := store.Settlement("agent-test", "self-request")
			if err != nil || record.HasLocalReservation == direct {
				t.Fatalf("incorrect local reservation classification: %+v err=%v", record, err)
			}
			pending, err := store.PendingSettlements("agent-test", 10)
			if err != nil || len(pending) != 1 || pending[0].HasLocalReservation == direct {
				t.Fatalf("batch classification differs: %+v err=%v", pending, err)
			}
			if !direct {
				if err := store.FinalizeSettlement("agent-test", "self-request", "main-usage", "confirmed", 200, ""); err == nil {
					t.Fatal("legacy self-billing must not bypass its actual local reservation")
				}
				unchanged, err := store.Settlement("agent-test", "self-request")
				if err != nil || unchanged.Status != "pending" {
					t.Fatalf("rejected settlement changed state: %+v %v", unchanged, err)
				}
			}
			for i := 0; i < 2; i++ {
				if err := store.FinalizeSettlement("agent-test", "self-request", "main-usage", "confirmed", 100, ""); err != nil {
					t.Fatal(err)
				}
			}
			user, err := store.User("agent-test", "42")
			if err != nil {
				t.Fatal(err)
			}
			want := int64(300)
			if direct {
				want = 400
			}
			if user.BalanceCents != want {
				t.Fatalf("direct=%v balance=%d want=%d", direct, user.BalanceCents, want)
			}
		})
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

func TestAgentOwnerIsImmutableWithoutFinancialActivity(t *testing.T) {
	store := testStore(t)
	const agentID = "owner-immutable-empty"
	if err := store.UpsertAgent(Config{
		AgentID:         agentID,
		AgentName:       "Owner Immutable",
		SiteName:        "Owner Immutable",
		OwnerMainUserID: "original-owner",
	}); err != nil {
		t.Fatal(err)
	}

	// Ownership identifies the tenant itself. It must remain immutable even
	// before the tenant has a wallet balance, allocation or settlement; those
	// financial rows are diagnostic context, not the security boundary.
	err := store.UpsertAgent(Config{
		AgentID:         agentID,
		AgentName:       "Test",
		SiteName:        "Test",
		OwnerMainUserID: "different-owner",
	})
	if err == nil || !strings.Contains(err.Error(), "agent owner is immutable") {
		t.Fatalf("owner change without financial activity was accepted: %v", err)
	}

	agent, getErr := store.Agent(agentID)
	if getErr != nil {
		t.Fatal(getErr)
	}
	if agent.OwnerMainUserID != "original-owner" {
		t.Fatalf("owner changed after rejected upsert: %q", agent.OwnerMainUserID)
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
		AppCredential: "app", OwnerMainUserID: "owner", BillingMode: "owner_upstream", AgentDisabled: true,
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
	if _, err := store.UpdateBranding(cfg.AgentID, "Edited", "Edited Site", "https://cdn.example.com/edited.svg", "https://docs.example.com/agent", "support@example.com", "https://agent.example.com/v1", AgentHomeSettings{SiteSubtitle: "Edited subtitle", CompactHomeEnabled: true, HomeContent: "<h1>Edited home</h1>"}); err != nil {
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
	if agent.Name != "Edited" || agent.SiteName != "Edited Site" || agent.SiteLogo != "https://cdn.example.com/edited.svg" || agent.DocURL != "https://docs.example.com/agent" || agent.ContactInfo != "support@example.com" {
		t.Fatalf("normal restart overwrote branding: %+v", agent)
	}
	if agent.SiteSubtitle != "Edited subtitle" || !agent.CompactHomeEnabled || agent.HomeContent != "<h1>Edited home</h1>" {
		t.Fatalf("restart overwrote homepage: %+v", agent.AgentHomeSettings)
	}
	if err := store.UpsertAgent(Config{AgentID: cfg.AgentID, AgentName: "Env Name", SiteName: "Env Site", SiteLogo: "/env.svg", BrandSync: true}); err != nil {
		t.Fatal(err)
	}
	agent, err = store.Agent(cfg.AgentID)
	if err != nil {
		t.Fatal(err)
	}
	if agent.Name != "Env Name" || agent.SiteName != "Env Site" || agent.SiteLogo != "/env.svg" || agent.DocURL != "https://docs.example.com/agent" || agent.ContactInfo != "support@example.com" {
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
	if view.Key != raw {
		t.Fatalf("created key view did not retain the recoverable key")
	}
	if !strings.HasPrefix(raw, "sk-") || strings.HasPrefix(raw, "sk-agent-") {
		t.Fatalf("new AgentAPI key has unexpected format: %q", raw)
	}
	keys, err := store.APIKeys("agent-test", "42")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0].Prefix != view.Prefix || keys[0].Name != "desktop client" {
		t.Fatalf("unexpected key list: %+v", keys)
	}
	if keys[0].Key != raw {
		t.Fatalf("listed key is not recoverable: %+v", keys[0])
	}
	var storedHash, storedCiphertext string
	if err := store.db.QueryRow(`SELECT key_hash,key_ciphertext FROM agent_api_keys WHERE id=?`, view.ID).Scan(&storedHash, &storedCiphertext); err != nil {
		t.Fatal(err)
	}
	if storedHash == raw || storedCiphertext == raw || storedCiphertext == "" {
		t.Fatalf("key was not protected at rest: hash=%q ciphertext_present=%v", storedHash, storedCiphertext != "")
	}
	if _, err := store.ResolveAPIKey("agent-test", raw); err != nil {
		t.Fatalf("created key did not resolve: %v", err)
	}
	if _, err := store.ResolveAPIKey("agent-other", raw); !errors.Is(err, errNotFound) {
		t.Fatalf("AgentAPI key escaped its issuing agent: %v", err)
	}
	if err := store.RevokeAPIKey("agent-test", "42", view.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ResolveAPIKey("agent-test", raw); !errors.Is(err, errNotFound) {
		t.Fatalf("revoked key resolved: %v", err)
	}
}

func TestLegacyAgentAPIKeyBecomesRecoverableWithoutBreakingExistingCredential(t *testing.T) {
	store := testStore(t)
	legacyKey := "sk-legacy-client-secret"
	now := time.Now().UTC().Unix()
	result, err := store.db.Exec(`INSERT INTO agent_api_keys(agent_id,main_user_id,name,prefix,key_hash,status,created_at,updated_at) VALUES(?,?,?,?,?,'active',?,?)`,
		"agent-test", "42", "legacy client", "sk-legacy-client", hashToken(legacyKey), now, now)
	if err != nil {
		t.Fatal(err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}

	keys, err := store.APIKeys("agent-test", "42")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0].ID != id || !strings.HasPrefix(keys[0].Key, "sk-") || keys[0].Key == legacyKey {
		t.Fatalf("legacy key was not upgraded to a recoverable value: %+v", keys)
	}
	var currentHash, legacyHash, ciphertext string
	if err := store.db.QueryRow(`SELECT key_hash,legacy_key_hash,key_ciphertext FROM agent_api_keys WHERE id=?`, id).Scan(&currentHash, &legacyHash, &ciphertext); err != nil {
		t.Fatal(err)
	}
	if currentHash != hashToken(keys[0].Key) || legacyHash != hashToken(legacyKey) || ciphertext == "" {
		t.Fatalf("legacy key upgrade was not persisted safely: current=%q legacy=%q ciphertext_present=%v", currentHash, legacyHash, ciphertext != "")
	}
	if _, err := store.ResolveAPIKey("agent-test", legacyKey); err != nil {
		t.Fatalf("existing credential stopped resolving after upgrade: %v", err)
	}
	if _, err := store.ResolveAPIKey("agent-test", keys[0].Key); err != nil {
		t.Fatalf("recoverable credential did not resolve after upgrade: %v", err)
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
