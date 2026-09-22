package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var errNotFound = errors.New("not found")
var errInsufficientBalance = errors.New("insufficient balance")
var errIdempotencyConflict = errors.New("idempotency key belongs to a different operation")
var errSettlementStateConflict = errors.New("settlement is already in a terminal state")
var errRechargeExpired = errors.New("recharge order has expired")
var errRechargeStateConflict = errors.New("recharge order is already in a terminal state")

type Store struct {
	db    *sql.DB
	aead  cipher.AEAD
	clock func() time.Time
}

type Session struct {
	ID           string
	MainUserID   string
	UserJSON     []byte
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
}

type AgentView struct {
	ID              string `json:"agent_id"`
	Domain          string `json:"domain"`
	Name            string `json:"name"`
	SiteName        string `json:"site_name"`
	SiteLogo        string `json:"site_logo,omitempty"`
	Status          string `json:"status"`
	BillingMode     string `json:"billing_mode"`
	OwnerMainUserID string `json:"owner_main_user_id,omitempty"`
	MainBalance     int64  `json:"main_balance_cents"`
	BalanceChecked  string `json:"main_balance_checked_at,omitempty"`
	BillingStatus   string `json:"billing_status"`
	WalletAvailable int64  `json:"wallet_available_cents"`
	WalletAllocated int64  `json:"wallet_allocated_cents"`
}

type AgentUserView struct {
	AgentID      string `json:"agent_id"`
	MainUserID   string `json:"main_user_id"`
	Email        string `json:"email,omitempty"`
	DisplayName  string `json:"display_name,omitempty"`
	Status       string `json:"status"`
	BalanceCents int64  `json:"balance_cents"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

type AgentAPIKeyView struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Prefix     string `json:"prefix"`
	Status     string `json:"status"`
	CreatedAt  string `json:"created_at"`
	LastUsedAt string `json:"last_used_at,omitempty"`
}

type AgentUsageView struct {
	RequestID         string `json:"request_id"`
	ProxyMainUserID   string `json:"proxy_main_user_id"`
	BillingMainUserID string `json:"billing_main_user_id"`
	ReservedCents     int64  `json:"reserved_cents"`
	ActualCents       int64  `json:"actual_cents"`
	SettlementStatus  string `json:"settlement_status"`
	Model             string `json:"model,omitempty"`
	CreatedAt         string `json:"created_at"`
	Error             string `json:"error,omitempty"`
}

type RechargeOrder struct {
	ID              int64     `json:"id"`
	OrderNo         string    `json:"order_no"`
	AgentID         string    `json:"agent_id"`
	MainUserID      string    `json:"main_user_id"`
	AmountCents     int64     `json:"amount_cents"`
	Currency        string    `json:"currency"`
	Provider        string    `json:"provider"`
	Status          string    `json:"status"`
	ProviderTradeNo string    `json:"provider_trade_no,omitempty"`
	PaymentURL      string    `json:"payment_url,omitempty"`
	RequestID       string    `json:"request_id,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	ExpiresAt       time.Time `json:"expires_at"`
	PaidAt          time.Time `json:"paid_at,omitempty"`
	AllocatedAt     time.Time `json:"allocated_at,omitempty"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type PaymentEventResult struct {
	Order   RechargeOrder
	Created bool
}

type AuditEvent struct {
	ID         int64  `json:"id"`
	ActorType  string `json:"actor_type"`
	ActorID    string `json:"actor_id"`
	AgentID    string `json:"agent_id"`
	Operation  string `json:"operation"`
	TargetType string `json:"target_type"`
	TargetID   string `json:"target_id,omitempty"`
	RequestID  string `json:"request_id"`
	Result     string `json:"result"`
	Reason     string `json:"reason,omitempty"`
	CreatedAt  string `json:"created_at"`
}

type VideoTask struct {
	AgentID    string `json:"agent_id"`
	MainUserID string `json:"main_user_id"`
	TaskID     string `json:"task_id"`
	RequestID  string `json:"request_id"`
	Status     string `json:"status"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

// SettlementRecord is the internal representation of a model request's
// local reservation. A settlement is created before the upstream request is
// sent and is the durable idempotency record for that request.
type SettlementRecord struct {
	ID                int64
	AgentID           string
	ProxyMainUserID   string
	BillingMainUserID string
	RequestID         string
	UsageID           string
	ReservedCents     int64
	ActualCents       int64
	Status            string
	Error             string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func OpenStore(path string, secret string) (*Store, error) {
	if path == ":memory:" {
		// Keep the special SQLite name intact.
	} else if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	for _, statement := range []string{
		`PRAGMA busy_timeout = 5000`,
		`PRAGMA journal_mode = WAL`,
		`PRAGMA foreign_keys = ON`,
	} {
		if _, err := db.Exec(statement); err != nil {
			db.Close()
			return nil, fmt.Errorf("sqlite pragma: %w", err)
		}
	}

	store := &Store{db: db, clock: time.Now}
	block, err := aes.NewCipher(sessionKey(secret))
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("create session cipher: %w", err)
	}
	store.aead, err = cipher.NewGCM(block)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("create session AEAD: %w", err)
	}
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS agent_config (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			agent_id TEXT NOT NULL UNIQUE,
			domain TEXT NOT NULL DEFAULT '',
			name TEXT NOT NULL,
			site_name TEXT NOT NULL,
			site_logo TEXT NOT NULL DEFAULT '',
			billing_mode TEXT NOT NULL DEFAULT 'owner_upstream',
			billing_main_user_id TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'active',
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS agent_users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			agent_id TEXT NOT NULL,
			main_user_id TEXT NOT NULL,
			email TEXT NOT NULL DEFAULT '',
			display_name TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'active',
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			UNIQUE(agent_id, main_user_id),
			UNIQUE(agent_id, email)
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id_hash TEXT PRIMARY KEY,
			main_user_id TEXT NOT NULL,
			user_json TEXT NOT NULL,
			access_token TEXT NOT NULL,
			refresh_token TEXT NOT NULL DEFAULT '',
			expires_at INTEGER NOT NULL,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at)`,
		`CREATE TABLE IF NOT EXISTS agent_wallets (
			agent_id TEXT PRIMARY KEY,
			available_cents INTEGER NOT NULL DEFAULT 0,
			allocated_cents INTEGER NOT NULL DEFAULT 0,
			owner_main_user_id TEXT NOT NULL DEFAULT '',
			main_balance_cents INTEGER NOT NULL DEFAULT 0,
			main_balance_checked_at INTEGER NOT NULL DEFAULT 0,
			billing_status TEXT NOT NULL DEFAULT 'unknown',
			version INTEGER NOT NULL DEFAULT 0,
			updated_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS agent_user_wallets (
			agent_id TEXT NOT NULL,
			main_user_id TEXT NOT NULL,
			balance_cents INTEGER NOT NULL DEFAULT 0,
			version INTEGER NOT NULL DEFAULT 0,
			updated_at INTEGER NOT NULL,
			PRIMARY KEY(agent_id, main_user_id),
			FOREIGN KEY(agent_id, main_user_id) REFERENCES agent_users(agent_id, main_user_id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS wallet_ledger (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			agent_id TEXT NOT NULL,
			main_user_id TEXT NOT NULL DEFAULT '',
			kind TEXT NOT NULL,
			amount_cents INTEGER NOT NULL,
			balance_after_cents INTEGER NOT NULL,
			request_id TEXT NOT NULL DEFAULT '',
			order_id TEXT NOT NULL DEFAULT '',
			note TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'posted',
			usage_id TEXT NOT NULL DEFAULT '',
			billing_main_user_id TEXT NOT NULL DEFAULT '',
			balance_before_cents INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL,
			UNIQUE(agent_id, kind, request_id)
		)`,
		`CREATE TABLE IF NOT EXISTS agent_api_keys (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			agent_id TEXT NOT NULL,
			main_user_id TEXT NOT NULL,
			name TEXT NOT NULL DEFAULT '',
			prefix TEXT NOT NULL,
			key_hash TEXT NOT NULL UNIQUE,
			status TEXT NOT NULL DEFAULT 'active',
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			last_used_at INTEGER,
			FOREIGN KEY(agent_id, main_user_id) REFERENCES agent_users(agent_id, main_user_id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_agent_api_keys_user ON agent_api_keys(agent_id, main_user_id, status)`,
		`CREATE TABLE IF NOT EXISTS agent_balance_snapshots (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			agent_id TEXT NOT NULL,
			owner_main_user_id TEXT NOT NULL,
			balance_cents INTEGER NOT NULL,
			request_id TEXT NOT NULL DEFAULT '',
			checked_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_agent_balance_snapshots_agent ON agent_balance_snapshots(agent_id, checked_at DESC)`,
		`CREATE TABLE IF NOT EXISTS settlements (
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
		)`,
		`CREATE TABLE IF NOT EXISTS recharge_orders (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			agent_id TEXT NOT NULL,
			order_no TEXT NOT NULL,
			main_user_id TEXT NOT NULL,
			amount_cents INTEGER NOT NULL,
			currency TEXT NOT NULL,
			provider TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			provider_trade_no TEXT NOT NULL DEFAULT '',
			payment_url TEXT NOT NULL DEFAULT '',
			request_id TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL,
			expires_at INTEGER NOT NULL,
			paid_at INTEGER NOT NULL DEFAULT 0,
			allocated_at INTEGER NOT NULL DEFAULT 0,
			updated_at INTEGER NOT NULL,
			UNIQUE(agent_id, order_no)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_recharge_orders_user ON recharge_orders(agent_id, main_user_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_recharge_orders_status ON recharge_orders(agent_id, status, updated_at ASC)`,
		`CREATE TABLE IF NOT EXISTS payment_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			agent_id TEXT NOT NULL,
			provider TEXT NOT NULL,
			event_id TEXT NOT NULL,
			order_no TEXT NOT NULL,
			status TEXT NOT NULL,
			amount_cents INTEGER NOT NULL,
			currency TEXT NOT NULL,
			provider_trade_no TEXT NOT NULL DEFAULT '',
			payload_hash TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			UNIQUE(agent_id, provider, event_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_settlements_agent_user ON settlements(agent_id, proxy_main_user_id, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS consumed_sso_tickets (
			jti TEXT PRIMARY KEY,
			expires_at INTEGER NOT NULL,
			consumed_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_consumed_sso_tickets_expires ON consumed_sso_tickets(expires_at)`,
		`CREATE TABLE IF NOT EXISTS audit_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			actor_type TEXT NOT NULL,
			actor_id TEXT NOT NULL,
			agent_id TEXT NOT NULL,
			operation TEXT NOT NULL,
			target_type TEXT NOT NULL,
			target_id TEXT NOT NULL DEFAULT '',
			request_id TEXT NOT NULL,
			result TEXT NOT NULL,
			reason TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_events_agent ON audit_events(agent_id, id DESC)`,
		`CREATE TABLE IF NOT EXISTS video_tasks (
			agent_id TEXT NOT NULL,
			main_user_id TEXT NOT NULL,
			task_id TEXT NOT NULL,
			request_id TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			PRIMARY KEY(agent_id, task_id),
			UNIQUE(agent_id, request_id),
			FOREIGN KEY(agent_id, main_user_id) REFERENCES agent_users(agent_id, main_user_id) ON DELETE RESTRICT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_video_tasks_user ON video_tasks(agent_id, main_user_id, updated_at DESC)`,
	}
	for _, statement := range statements {
		if _, err := s.db.Exec(statement); err != nil {
			return fmt.Errorf("migrate agentapi store: %w", err)
		}
	}
	// These ALTER statements keep an already-created prototype database
	// upgradeable. SQLite has no IF NOT EXISTS form for ADD COLUMN.
	for _, column := range []struct {
		table string
		name  string
		def   string
	}{
		{"agent_config", "billing_mode", "TEXT NOT NULL DEFAULT 'owner_upstream'"},
		{"agent_config", "billing_main_user_id", "TEXT NOT NULL DEFAULT ''"},
		{"agent_wallets", "owner_main_user_id", "TEXT NOT NULL DEFAULT ''"},
		{"agent_wallets", "main_balance_cents", "INTEGER NOT NULL DEFAULT 0"},
		{"agent_wallets", "main_balance_checked_at", "INTEGER NOT NULL DEFAULT 0"},
		{"agent_wallets", "billing_status", "TEXT NOT NULL DEFAULT 'unknown'"},
		{"wallet_ledger", "status", "TEXT NOT NULL DEFAULT 'posted'"},
		{"wallet_ledger", "usage_id", "TEXT NOT NULL DEFAULT ''"},
		{"wallet_ledger", "billing_main_user_id", "TEXT NOT NULL DEFAULT ''"},
		{"wallet_ledger", "balance_before_cents", "INTEGER NOT NULL DEFAULT 0"},
	} {
		if err := s.addColumnIfMissing(column.table, column.name, column.def); err != nil {
			return fmt.Errorf("upgrade agentapi store: %w", err)
		}
	}
	return nil
}

func (s *Store) RecordAuditEvent(event AuditEvent) error {
	if strings.TrimSpace(event.ActorType) == "" || strings.TrimSpace(event.ActorID) == "" ||
		strings.TrimSpace(event.AgentID) == "" || strings.TrimSpace(event.Operation) == "" ||
		strings.TrimSpace(event.TargetType) == "" || strings.TrimSpace(event.RequestID) == "" ||
		strings.TrimSpace(event.Result) == "" {
		return fmt.Errorf("audit event is missing required fields")
	}
	_, err := s.db.Exec(`INSERT INTO audit_events(actor_type, actor_id, agent_id, operation, target_type, target_id, request_id, result, reason, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		strings.TrimSpace(event.ActorType), strings.TrimSpace(event.ActorID), strings.TrimSpace(event.AgentID),
		strings.TrimSpace(event.Operation), strings.TrimSpace(event.TargetType), strings.TrimSpace(event.TargetID),
		strings.TrimSpace(event.RequestID), strings.TrimSpace(event.Result), strings.TrimSpace(event.Reason), s.clock().UTC().Unix())
	return err
}

func (s *Store) AuditEvents(agentID string, limit int) ([]AuditEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(`SELECT id, actor_type, actor_id, agent_id, operation, target_type, target_id, request_id, result, reason, created_at FROM audit_events WHERE agent_id=? ORDER BY id DESC LIMIT ?`, strings.TrimSpace(agentID), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]AuditEvent, 0)
	for rows.Next() {
		var event AuditEvent
		var createdAt int64
		if err := rows.Scan(&event.ID, &event.ActorType, &event.ActorID, &event.AgentID, &event.Operation, &event.TargetType, &event.TargetID, &event.RequestID, &event.Result, &event.Reason, &createdAt); err != nil {
			return nil, err
		}
		event.CreatedAt = time.Unix(createdAt, 0).UTC().Format(time.RFC3339)
		items = append(items, event)
	}
	return items, rows.Err()
}

func (s *Store) RecordVideoTask(agentID, mainUserID, taskID, requestID, status string) (VideoTask, error) {
	agentID, mainUserID, taskID, requestID = strings.TrimSpace(agentID), strings.TrimSpace(mainUserID), strings.TrimSpace(taskID), strings.TrimSpace(requestID)
	status = strings.TrimSpace(status)
	if agentID == "" || mainUserID == "" || taskID == "" || requestID == "" {
		return VideoTask{}, fmt.Errorf("video task identity is incomplete")
	}
	if status == "" {
		status = "pending"
	}
	now := s.clock().UTC().Unix()
	_, err := s.db.Exec(`INSERT INTO video_tasks(agent_id, main_user_id, task_id, request_id, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT(agent_id, task_id) DO UPDATE SET status=excluded.status, updated_at=excluded.updated_at WHERE video_tasks.main_user_id=excluded.main_user_id AND video_tasks.request_id=excluded.request_id`, agentID, mainUserID, taskID, requestID, status, now, now)
	if err != nil {
		return VideoTask{}, err
	}
	task, err := s.VideoTask(agentID, taskID)
	if err == nil && (task.MainUserID != mainUserID || task.RequestID != requestID) {
		return VideoTask{}, errIdempotencyConflict
	}
	return task, err
}

func (s *Store) VideoTask(agentID, taskID string) (VideoTask, error) {
	var task VideoTask
	var createdAt, updatedAt int64
	err := s.db.QueryRow(`SELECT agent_id, main_user_id, task_id, request_id, status, created_at, updated_at FROM video_tasks WHERE agent_id=? AND task_id=?`, strings.TrimSpace(agentID), strings.TrimSpace(taskID)).Scan(&task.AgentID, &task.MainUserID, &task.TaskID, &task.RequestID, &task.Status, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return VideoTask{}, errNotFound
	}
	if err != nil {
		return VideoTask{}, err
	}
	task.CreatedAt = time.Unix(createdAt, 0).UTC().Format(time.RFC3339)
	task.UpdatedAt = time.Unix(updatedAt, 0).UTC().Format(time.RFC3339)
	return task, nil
}

func (s *Store) UpdateVideoTaskStatus(agentID, taskID, status string) error {
	result, err := s.db.Exec(`UPDATE video_tasks SET status=?, updated_at=? WHERE agent_id=? AND task_id=?`, strings.TrimSpace(status), s.clock().UTC().Unix(), strings.TrimSpace(agentID), strings.TrimSpace(taskID))
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return errNotFound
	}
	return nil
}

func (s *Store) PendingVideoTasks(agentID string, limit int) ([]VideoTask, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(`SELECT agent_id, main_user_id, task_id, request_id, status, created_at, updated_at FROM video_tasks WHERE agent_id=? AND lower(status) NOT IN ('completed','complete','succeeded','success','failed','error','cancelled','canceled','rejected','expired') ORDER BY updated_at ASC LIMIT ?`, strings.TrimSpace(agentID), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]VideoTask, 0)
	for rows.Next() {
		var task VideoTask
		var createdAt, updatedAt int64
		if err := rows.Scan(&task.AgentID, &task.MainUserID, &task.TaskID, &task.RequestID, &task.Status, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		task.CreatedAt = time.Unix(createdAt, 0).UTC().Format(time.RFC3339)
		task.UpdatedAt = time.Unix(updatedAt, 0).UTC().Format(time.RFC3339)
		items = append(items, task)
	}
	return items, rows.Err()
}

func (s *Store) addColumnIfMissing(table, name, definition string) error {
	rows, err := s.db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var column, typ string
		var notNull, pk int
		var defaultValue any
		if err := rows.Scan(&cid, &column, &typ, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		if column == name {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = s.db.Exec("ALTER TABLE " + table + " ADD COLUMN " + name + " " + definition)
	return err
}

func (s *Store) UpsertAgent(cfg Config) error {
	now := s.clock().UTC().Unix()
	billingMode := cfg.BillingMode
	if billingMode == "" {
		billingMode = "owner_upstream"
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var existingAgentID, existingOwner, existingStatus string
	var existingAvailable, existingAllocated, existingPending int64
	existingErr := tx.QueryRow(`SELECT agent_id, billing_main_user_id, status FROM agent_config WHERE id=1`).Scan(&existingAgentID, &existingOwner, &existingStatus)
	if existingErr != nil && !errors.Is(existingErr, sql.ErrNoRows) {
		return fmt.Errorf("read existing agent config: %w", existingErr)
	}
	if existingErr == nil && existingAgentID != "" && existingAgentID != cfg.AgentID {
		return fmt.Errorf("agent id is immutable once initialized (existing %q, requested %q)", existingAgentID, cfg.AgentID)
	}
	if existingErr == nil {
		// Never silently move spendable local credit to a different main-site
		// owner. The operator must reconcile/reset the old wallet explicitly.
		if existingOwner != strings.TrimSpace(cfg.OwnerMainUserID) {
			_ = tx.QueryRow(`SELECT available_cents, allocated_cents FROM agent_wallets WHERE agent_id=?`, cfg.AgentID).Scan(&existingAvailable, &existingAllocated)
			_ = tx.QueryRow(`SELECT COALESCE(SUM(reserved_cents), 0) FROM settlements WHERE agent_id=? AND status='pending'`, cfg.AgentID).Scan(&existingPending)
			if existingAvailable != 0 || existingAllocated != 0 || existingPending != 0 {
				return fmt.Errorf("billing owner change requires wallet reconciliation (available=%d allocated=%d pending=%d)", existingAvailable, existingAllocated, existingPending)
			}
		}
	}

	status := agentConfigStatus(cfg)
	if existingErr == nil && existingStatus == "revoked" {
		status = "revoked"
	}
	if existingErr == nil {
		if cfg.BrandSync {
			_, err = tx.Exec(`
				UPDATE agent_config SET domain=?, name=?, site_name=?, site_logo=?, billing_mode=?, billing_main_user_id=?, status=?, updated_at=?
				WHERE id=1
			`, cfg.AgentDomain, cfg.AgentName, cfg.SiteName, cfg.SiteLogo, billingMode, cfg.OwnerMainUserID, status, now)
		} else {
			// Branding can be edited from the Agent administrator console. Keep
			// that durable value across restarts unless an operator explicitly
			// opts into env-driven replacement with AGENT_BRAND_SYNC=true.
			_, err = tx.Exec(`
				UPDATE agent_config SET domain=?, billing_mode=?, billing_main_user_id=?, status=?, updated_at=?
				WHERE id=1
			`, cfg.AgentDomain, billingMode, cfg.OwnerMainUserID, status, now)
		}
	} else {
		_, err = tx.Exec(`
			INSERT INTO agent_config (id, agent_id, domain, name, site_name, site_logo, billing_mode, billing_main_user_id, status, created_at, updated_at)
			VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, cfg.AgentID, cfg.AgentDomain, cfg.AgentName, cfg.SiteName, cfg.SiteLogo, billingMode, cfg.OwnerMainUserID, status, now, now)
	}
	if err != nil {
		return fmt.Errorf("upsert agent config: %w", err)
	}

	if _, err = tx.Exec(`
		INSERT INTO agent_wallets (agent_id, available_cents, allocated_cents, owner_main_user_id, billing_status, version, updated_at)
		VALUES (?, ?, 0, ?, 'unknown', 0, ?)
		ON CONFLICT(agent_id) DO UPDATE SET
			owner_main_user_id=excluded.owner_main_user_id,
			billing_status=CASE WHEN agent_wallets.owner_main_user_id <> excluded.owner_main_user_id THEN 'owner_changed' ELSE agent_wallets.billing_status END,
			main_balance_cents=CASE WHEN agent_wallets.owner_main_user_id <> excluded.owner_main_user_id THEN 0 ELSE agent_wallets.main_balance_cents END,
			main_balance_checked_at=CASE WHEN agent_wallets.owner_main_user_id <> excluded.owner_main_user_id THEN 0 ELSE agent_wallets.main_balance_checked_at END,
			updated_at=excluded.updated_at
	`, cfg.AgentID, cfg.InitialBalanceCents, cfg.OwnerMainUserID, now); err != nil {
		return fmt.Errorf("upsert agent wallet: %w", err)
	}
	return tx.Commit()
}

func agentConfigStatus(cfg Config) string {
	if cfg.AgentDisabled || strings.TrimSpace(cfg.AdminKey) == "" || strings.TrimSpace(cfg.AppCredential) == "" || strings.TrimSpace(cfg.OwnerMainUserID) == "" || cfg.BillingMode != "" && cfg.BillingMode != "owner_upstream" {
		return "suspended"
	}
	return "active"
}

func (s *Store) Agent(agentID string) (AgentView, error) {
	var view AgentView
	var checkedAt, configUpdated int64
	err := s.db.QueryRow(`
		SELECT c.agent_id, c.domain, c.name, c.site_name, c.site_logo, c.status,
		       c.billing_mode, c.billing_main_user_id,
		       COALESCE(w.main_balance_cents, 0), COALESCE(w.main_balance_checked_at, 0),
		       COALESCE(w.billing_status, 'unknown'),
		       COALESCE(w.available_cents, 0), COALESCE(w.allocated_cents, 0), c.updated_at
		FROM agent_config c
		LEFT JOIN agent_wallets w ON w.agent_id = c.agent_id
		WHERE c.agent_id = ?
	`, agentID).Scan(&view.ID, &view.Domain, &view.Name, &view.SiteName, &view.SiteLogo, &view.Status, &view.BillingMode, &view.OwnerMainUserID, &view.MainBalance, &checkedAt, &view.BillingStatus, &view.WalletAvailable, &view.WalletAllocated, &configUpdated)
	if errors.Is(err, sql.ErrNoRows) {
		return AgentView{}, errNotFound
	}
	if err != nil {
		return AgentView{}, err
	}
	if checkedAt > 0 {
		view.BalanceChecked = time.Unix(checkedAt, 0).UTC().Format(time.RFC3339)
	}
	return view, nil
}

// UpdateBranding changes only the presentation fields for the current Agent.
// It deliberately cannot change the agent id, domain, owner or billing mode;
// those fields belong to provisioning and accounting configuration.
func (s *Store) UpdateBranding(agentID, name, siteName, siteLogo string) (AgentView, error) {
	now := s.clock().UTC().Unix()
	result, err := s.db.Exec(`
		UPDATE agent_config SET name=?, site_name=?, site_logo=?, updated_at=?
		WHERE agent_id=?
	`, strings.TrimSpace(name), strings.TrimSpace(siteName), strings.TrimSpace(siteLogo), now, strings.TrimSpace(agentID))
	if err != nil {
		return AgentView{}, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return AgentView{}, err
	}
	if rows == 0 {
		return AgentView{}, errNotFound
	}
	return s.Agent(agentID)
}

func (s *Store) UpsertUser(agentID, mainUserID, email, displayName string) (AgentUserView, error) {
	now := s.clock().UTC().Unix()
	email = strings.TrimSpace(email)
	// The prototype schema has a per-agent unique email constraint. Keep
	// anonymous upstream profiles distinct without inventing a login address.
	// The value is never used for authentication; it is only a stable mapping
	// placeholder until /auth/me returns a real email.
	if email == "" {
		email = "agent-user-" + strings.TrimSpace(mainUserID)
	}
	_, err := s.db.Exec(`
		INSERT INTO agent_users (agent_id, main_user_id, email, display_name, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'active', ?, ?)
		ON CONFLICT(agent_id, main_user_id) DO UPDATE SET
			email=CASE WHEN excluded.email <> '' THEN excluded.email ELSE agent_users.email END,
			display_name=CASE WHEN excluded.display_name <> '' THEN excluded.display_name ELSE agent_users.display_name END,
			updated_at=excluded.updated_at
	`, agentID, mainUserID, email, displayName, now, now)
	if err != nil {
		return AgentUserView{}, fmt.Errorf("upsert agent user: %w", err)
	}
	_, err = s.db.Exec(`
		INSERT INTO agent_user_wallets (agent_id, main_user_id, balance_cents, version, updated_at)
		VALUES (?, ?, 0, 0, ?)
		ON CONFLICT(agent_id, main_user_id) DO NOTHING
	`, agentID, mainUserID, now)
	if err != nil {
		return AgentUserView{}, fmt.Errorf("create agent user wallet: %w", err)
	}
	return s.User(agentID, mainUserID)
}

func (s *Store) User(agentID, mainUserID string) (AgentUserView, error) {
	var view AgentUserView
	var created, updated int64
	err := s.db.QueryRow(`
		SELECT u.agent_id, u.main_user_id, u.email, u.display_name, u.status,
		       COALESCE(w.balance_cents, 0), u.created_at, u.updated_at
		FROM agent_users u
		LEFT JOIN agent_user_wallets w ON w.agent_id=u.agent_id AND w.main_user_id=u.main_user_id
		WHERE u.agent_id=? AND u.main_user_id=?
	`, agentID, mainUserID).Scan(&view.AgentID, &view.MainUserID, &view.Email, &view.DisplayName, &view.Status, &view.BalanceCents, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return AgentUserView{}, errNotFound
	}
	if err != nil {
		return AgentUserView{}, err
	}
	view.CreatedAt = time.Unix(created, 0).UTC().Format(time.RFC3339)
	view.UpdatedAt = time.Unix(updated, 0).UTC().Format(time.RFC3339)
	return view, nil
}

func (s *Store) UserByEmail(agentID, email string) (AgentUserView, error) {
	var mainUserID string
	err := s.db.QueryRow(`SELECT main_user_id FROM agent_users WHERE agent_id=? AND lower(email)=lower(?)`, agentID, email).Scan(&mainUserID)
	if errors.Is(err, sql.ErrNoRows) {
		return AgentUserView{}, errNotFound
	}
	if err != nil {
		return AgentUserView{}, err
	}
	return s.User(agentID, mainUserID)
}

func (s *Store) Users(agentID string, limit int) ([]AgentUserView, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(`
		SELECT u.main_user_id
		FROM agent_users u
		WHERE u.agent_id=?
		ORDER BY u.id DESC LIMIT ?
	`, agentID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []AgentUserView
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		view, err := s.User(agentID, id)
		if err != nil {
			return nil, err
		}
		result = append(result, view)
	}
	return result, rows.Err()
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanRechargeOrder(row rowScanner) (RechargeOrder, error) {
	var order RechargeOrder
	var created, expires, paid, allocated, updated int64
	err := row.Scan(
		&order.ID, &order.AgentID, &order.OrderNo, &order.MainUserID,
		&order.AmountCents, &order.Currency, &order.Provider, &order.Status,
		&order.ProviderTradeNo, &order.PaymentURL, &order.RequestID,
		&created, &expires, &paid, &allocated, &updated,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return RechargeOrder{}, errNotFound
		}
		return RechargeOrder{}, err
	}
	order.CreatedAt = time.Unix(created, 0).UTC()
	order.ExpiresAt = time.Unix(expires, 0).UTC()
	order.PaidAt = unixTimeOrZero(paid)
	order.AllocatedAt = unixTimeOrZero(allocated)
	order.UpdatedAt = time.Unix(updated, 0).UTC()
	return order, nil
}

func unixTimeOrZero(value int64) time.Time {
	if value <= 0 {
		return time.Time{}
	}
	return time.Unix(value, 0).UTC()
}

const rechargeOrderSelect = `SELECT id, agent_id, order_no, main_user_id, amount_cents, currency, provider, status, provider_trade_no, payment_url, request_id, created_at, expires_at, paid_at, allocated_at, updated_at FROM recharge_orders`

// CreateRechargeOrder creates a provider-neutral pending order. It does not
// touch either wallet; only a verified payment webhook can move an order to
// paid_pending_allocation, and only synced owner credit can allocate it.
func (s *Store) CreateRechargeOrder(agentID, mainUserID string, amountCents int64, currency, provider, paymentURL, requestID string, expiresAt time.Time) (RechargeOrder, bool, error) {
	agentID = strings.TrimSpace(agentID)
	mainUserID = strings.TrimSpace(mainUserID)
	currency = strings.ToUpper(strings.TrimSpace(currency))
	provider = strings.TrimSpace(provider)
	if agentID == "" || mainUserID == "" || amountCents <= 0 || currency == "" || provider == "" {
		return RechargeOrder{}, false, fmt.Errorf("invalid recharge order")
	}
	if requestID == "" {
		requestID = randomID("order_req_")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return RechargeOrder{}, false, err
	}
	defer tx.Rollback()
	if existing, lookupErr := scanRechargeOrder(tx.QueryRow(rechargeOrderSelect+` WHERE agent_id=? AND request_id=? LIMIT 1`, agentID, requestID)); lookupErr == nil {
		if existing.MainUserID != mainUserID || existing.AmountCents != amountCents || existing.Currency != currency || existing.Provider != provider {
			return RechargeOrder{}, false, errIdempotencyConflict
		}
		if err := tx.Commit(); err != nil {
			return RechargeOrder{}, false, err
		}
		return existing, false, nil
	} else if !errors.Is(lookupErr, errNotFound) {
		return RechargeOrder{}, false, lookupErr
	}
	var exists int
	if err := tx.QueryRow(`SELECT 1 FROM agent_users WHERE agent_id=? AND main_user_id=? AND status='active'`, agentID, mainUserID).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
		return RechargeOrder{}, false, errNotFound
	} else if err != nil {
		return RechargeOrder{}, false, err
	}
	now := s.clock().UTC()
	orderNo := randomID("ord_")
	_, err = tx.Exec(`INSERT INTO recharge_orders(agent_id, order_no, main_user_id, amount_cents, currency, provider, status, payment_url, request_id, created_at, expires_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, 'pending', ?, ?, ?, ?, ?)`, agentID, orderNo, mainUserID, amountCents, currency, provider, paymentURL, requestID, now.Unix(), expiresAt.UTC().Unix(), now.Unix())
	if err != nil {
		return RechargeOrder{}, false, err
	}
	order, err := scanRechargeOrder(tx.QueryRow(rechargeOrderSelect+` WHERE agent_id=? AND order_no=?`, agentID, orderNo))
	if err != nil {
		return RechargeOrder{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return RechargeOrder{}, false, err
	}
	return order, true, nil
}

func (s *Store) RechargeOrder(agentID, orderNo string) (RechargeOrder, error) {
	return scanRechargeOrder(s.db.QueryRow(rechargeOrderSelect+` WHERE agent_id=? AND order_no=?`, agentID, strings.TrimSpace(orderNo)))
}

// SetRechargePaymentURL fills the provider checkout URL after the order number
// has been generated. The URL is derived from server configuration; clients
// never get to provide or replace it.
func (s *Store) SetRechargePaymentURL(agentID, orderNo, paymentURL string) (RechargeOrder, error) {
	orderNo = strings.TrimSpace(orderNo)
	if orderNo == "" {
		return RechargeOrder{}, errNotFound
	}
	result, err := s.db.Exec(`UPDATE recharge_orders SET payment_url=?, updated_at=? WHERE agent_id=? AND order_no=? AND status='pending'`, paymentURL, s.clock().UTC().Unix(), agentID, orderNo)
	if err != nil {
		return RechargeOrder{}, err
	}
	if count, err := result.RowsAffected(); err != nil {
		return RechargeOrder{}, err
	} else if count == 0 {
		// The order may have moved to a terminal state between creation and this
		// best-effort URL update. Return the durable row so callers can still
		// show the actual state.
		return s.RechargeOrder(agentID, orderNo)
	}
	return s.RechargeOrder(agentID, orderNo)
}

func (s *Store) RechargeOrders(agentID, mainUserID string, limit int) ([]RechargeOrder, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := rechargeOrderSelect + ` WHERE agent_id=?`
	args := []any{agentID}
	if strings.TrimSpace(mainUserID) != "" {
		query += ` AND main_user_id=?`
		args = append(args, mainUserID)
	}
	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]RechargeOrder, 0)
	for rows.Next() {
		order, err := scanRechargeOrder(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, order)
	}
	return result, rows.Err()
}

// ExpireRechargeOrders lazily closes pending orders whose payment window has
// elapsed. It is safe to call on every list/webhook request and keeps expiry a
// durable state transition rather than a UI-only calculation.
func (s *Store) ExpireRechargeOrders(agentID string) error {
	now := s.clock().UTC().Unix()
	_, err := s.db.Exec(`UPDATE recharge_orders SET status='expired', updated_at=? WHERE agent_id=? AND status='pending' AND expires_at > 0 AND expires_at <= ?`, now, strings.TrimSpace(agentID), now)
	return err
}

// RecordPaymentEvent is the only path that changes a pending order based on
// external payment input. The caller must verify the provider signature and
// amount before calling it. Allocation is deliberately a second local step so
// an upstream owner-balance outage leaves a durable paid_pending_allocation
// order that can be retried without accepting the payment twice.
func (s *Store) RecordPaymentEvent(agentID, provider, eventID, orderNo, status string, amountCents int64, currency, providerTradeNo, payloadHash string) (PaymentEventResult, error) {
	agentID = strings.TrimSpace(agentID)
	provider = strings.TrimSpace(provider)
	eventID = strings.TrimSpace(eventID)
	orderNo = strings.TrimSpace(orderNo)
	currency = strings.ToUpper(strings.TrimSpace(currency))
	providerTradeNo = strings.TrimSpace(providerTradeNo)
	status = strings.ToLower(strings.TrimSpace(status))
	if status != "paid" && status != "failed" && status != "closed" && status != "expired" {
		return PaymentEventResult{}, fmt.Errorf("unsupported payment event status")
	}
	if strings.TrimSpace(eventID) == "" || strings.TrimSpace(orderNo) == "" || amountCents <= 0 || strings.TrimSpace(payloadHash) == "" {
		return PaymentEventResult{}, fmt.Errorf("payment event fields are required")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return PaymentEventResult{}, err
	}
	defer tx.Rollback()
	var existingHash, existingOrderNo string
	err = tx.QueryRow(`SELECT payload_hash, order_no FROM payment_events WHERE agent_id=? AND provider=? AND event_id=?`, agentID, provider, eventID).Scan(&existingHash, &existingOrderNo)
	if err == nil {
		if existingHash != payloadHash || existingOrderNo != orderNo {
			return PaymentEventResult{}, errIdempotencyConflict
		}
		order, orderErr := scanRechargeOrder(tx.QueryRow(rechargeOrderSelect+` WHERE agent_id=? AND order_no=?`, agentID, orderNo))
		if orderErr != nil {
			return PaymentEventResult{}, orderErr
		}
		if err := tx.Commit(); err != nil {
			return PaymentEventResult{}, err
		}
		return PaymentEventResult{Order: order, Created: false}, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return PaymentEventResult{}, err
	}
	order, err := scanRechargeOrder(tx.QueryRow(rechargeOrderSelect+` WHERE agent_id=? AND order_no=?`, agentID, orderNo))
	if err != nil {
		return PaymentEventResult{}, err
	}
	if order.AmountCents != amountCents || !strings.EqualFold(order.Currency, currency) || !strings.EqualFold(order.Provider, provider) {
		return PaymentEventResult{}, errIdempotencyConflict
	}
	if order.Status != "pending" && order.Status != "paid_pending_allocation" {
		return PaymentEventResult{}, errRechargeStateConflict
	}
	if order.Status == "pending" && !order.ExpiresAt.IsZero() && !order.ExpiresAt.After(s.clock().UTC()) {
		return PaymentEventResult{}, errRechargeExpired
	}
	if order.Status == "paid_pending_allocation" && status != "paid" {
		// Once a paid event has been durably accepted, a later provider
		// cancellation/refusal event must not silently erase the pending
		// allocation. Refunds require a separate, explicit reverse-flow.
		return PaymentEventResult{}, errRechargeStateConflict
	}
	if providerTradeNo != "" {
		var existingOrderNo string
		tradeErr := tx.QueryRow(`SELECT order_no FROM recharge_orders WHERE agent_id=? AND provider=? AND provider_trade_no=? LIMIT 1`, agentID, provider, providerTradeNo).Scan(&existingOrderNo)
		if tradeErr == nil && existingOrderNo != order.OrderNo {
			return PaymentEventResult{}, errIdempotencyConflict
		}
		if tradeErr != nil && !errors.Is(tradeErr, sql.ErrNoRows) {
			return PaymentEventResult{}, tradeErr
		}
	}
	if status == "paid" {
		if !order.ExpiresAt.IsZero() && !order.ExpiresAt.After(s.clock().UTC()) {
			return PaymentEventResult{}, errRechargeExpired
		}
	}
	now := s.clock().UTC().Unix()
	orderStatus := status
	paidAt := int64(0)
	if status == "paid" {
		orderStatus = "paid_pending_allocation"
		paidAt = now
	}
	_, err = tx.Exec(`INSERT INTO payment_events(agent_id, provider, event_id, order_no, status, amount_cents, currency, provider_trade_no, payload_hash, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, agentID, provider, eventID, orderNo, status, amountCents, strings.ToUpper(currency), providerTradeNo, payloadHash, now)
	if err != nil {
		return PaymentEventResult{}, err
	}
	_, err = tx.Exec(`UPDATE recharge_orders SET status=?, provider_trade_no=?, paid_at=CASE WHEN ? > 0 THEN ? ELSE paid_at END, updated_at=? WHERE agent_id=? AND order_no=? AND status IN ('pending', 'paid_pending_allocation')`, orderStatus, providerTradeNo, paidAt, paidAt, now, agentID, orderNo)
	if err != nil {
		return PaymentEventResult{}, err
	}
	order, err = scanRechargeOrder(tx.QueryRow(rechargeOrderSelect+` WHERE agent_id=? AND order_no=?`, agentID, orderNo))
	if err != nil {
		return PaymentEventResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return PaymentEventResult{}, err
	}
	return PaymentEventResult{Order: order, Created: true}, nil
}

// AllocateRechargeOrder atomically consumes already-synced agent credit and
// marks the verified payment order allocated. It is safe to retry after a
// process crash because the order row and wallet ledger share one transaction.
func (s *Store) AllocateRechargeOrder(agentID, orderNo string) (RechargeOrder, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return RechargeOrder{}, err
	}
	defer tx.Rollback()
	order, err := scanRechargeOrder(tx.QueryRow(rechargeOrderSelect+` WHERE agent_id=? AND order_no=?`, agentID, orderNo))
	if err != nil {
		return RechargeOrder{}, err
	}
	if order.Status == "allocated" {
		if err := tx.Commit(); err != nil {
			return RechargeOrder{}, err
		}
		return order, nil
	}
	if order.Status != "paid_pending_allocation" {
		return order, errRechargeStateConflict
	}
	requestID := "recharge:" + order.OrderNo
	if userID, amount, exists, lookupErr := ledgerEntry(tx, agentID, "user_allocate", requestID); lookupErr != nil {
		return RechargeOrder{}, lookupErr
	} else if exists {
		if userID != order.MainUserID || amount != order.AmountCents {
			return RechargeOrder{}, errIdempotencyConflict
		}
	} else {
		var available, allocated, agentVersion int64
		if err := tx.QueryRow(`SELECT available_cents, allocated_cents, version FROM agent_wallets WHERE agent_id=?`, agentID).Scan(&available, &allocated, &agentVersion); err != nil {
			return RechargeOrder{}, err
		}
		if available < order.AmountCents {
			return RechargeOrder{}, errInsufficientBalance
		}
		var userBalance, userVersion int64
		if err := tx.QueryRow(`SELECT balance_cents, version FROM agent_user_wallets WHERE agent_id=? AND main_user_id=?`, agentID, order.MainUserID).Scan(&userBalance, &userVersion); err != nil {
			return RechargeOrder{}, err
		}
		now := s.clock().UTC().Unix()
		if _, err := tx.Exec(`UPDATE agent_wallets SET available_cents=?, allocated_cents=?, version=?, updated_at=? WHERE agent_id=? AND version=?`, available-order.AmountCents, allocated+order.AmountCents, agentVersion+1, now, agentID, agentVersion); err != nil {
			return RechargeOrder{}, err
		}
		if _, err := tx.Exec(`UPDATE agent_user_wallets SET balance_cents=?, version=?, updated_at=? WHERE agent_id=? AND main_user_id=? AND version=?`, userBalance+order.AmountCents, userVersion+1, now, agentID, order.MainUserID, userVersion); err != nil {
			return RechargeOrder{}, err
		}
		if _, err := tx.Exec(`INSERT INTO wallet_ledger(agent_id, main_user_id, kind, amount_cents, balance_after_cents, request_id, order_id, note, created_at) VALUES (?, ?, 'user_allocate', ?, ?, ?, ?, ?, ?)`, agentID, order.MainUserID, order.AmountCents, userBalance+order.AmountCents, requestID, order.OrderNo, "verified payment allocation", now); err != nil {
			return RechargeOrder{}, err
		}
		if _, err := tx.Exec(`INSERT INTO wallet_ledger(agent_id, main_user_id, kind, amount_cents, balance_after_cents, request_id, order_id, note, created_at) VALUES (?, '', 'agent_allocate', ?, ?, ?, ?, ?, ?)`, agentID, -order.AmountCents, available-order.AmountCents, requestID, order.OrderNo, "verified payment allocation", now); err != nil {
			return RechargeOrder{}, err
		}
	}
	now := s.clock().UTC().Unix()
	if _, err := tx.Exec(`UPDATE recharge_orders SET status='allocated', allocated_at=?, updated_at=? WHERE agent_id=? AND order_no=? AND status='paid_pending_allocation'`, now, now, agentID, orderNo); err != nil {
		return RechargeOrder{}, err
	}
	order, err = scanRechargeOrder(tx.QueryRow(rechargeOrderSelect+` WHERE agent_id=? AND order_no=?`, agentID, orderNo))
	if err != nil {
		return RechargeOrder{}, err
	}
	if err := tx.Commit(); err != nil {
		return RechargeOrder{}, err
	}
	return order, nil
}

func (s *Store) PaidPendingRechargeOrders(agentID string, limit int) ([]RechargeOrder, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(rechargeOrderSelect+` WHERE agent_id=? AND status='paid_pending_allocation' ORDER BY id ASC LIMIT ?`, agentID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]RechargeOrder, 0)
	for rows.Next() {
		order, err := scanRechargeOrder(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, order)
	}
	return result, rows.Err()
}

func (s *Store) CreateAPIKey(agentID, mainUserID, name string) (AgentAPIKeyView, string, error) {
	if _, err := s.User(agentID, mainUserID); err != nil {
		return AgentAPIKeyView{}, "", err
	}
	raw := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, raw); err != nil {
		return AgentAPIKeyView{}, "", err
	}
	key := "sk-agent-" + base64.RawURLEncoding.EncodeToString(raw)
	prefix := key
	if len(prefix) > 18 {
		prefix = prefix[:18]
	}
	now := s.clock().UTC().Unix()
	result, err := s.db.Exec(`INSERT INTO agent_api_keys(agent_id, main_user_id, name, prefix, key_hash, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, 'active', ?, ?)`, agentID, mainUserID, strings.TrimSpace(name), prefix, hashToken(key), now, now)
	if err != nil {
		return AgentAPIKeyView{}, "", err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return AgentAPIKeyView{}, "", err
	}
	view := AgentAPIKeyView{ID: id, Name: strings.TrimSpace(name), Prefix: prefix, Status: "active", CreatedAt: time.Unix(now, 0).UTC().Format(time.RFC3339)}
	return view, key, nil
}

func (s *Store) APIKeys(agentID, mainUserID string) ([]AgentAPIKeyView, error) {
	rows, err := s.db.Query(`SELECT id, name, prefix, status, created_at, COALESCE(last_used_at, 0) FROM agent_api_keys WHERE agent_id=? AND main_user_id=? ORDER BY id DESC`, agentID, mainUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]AgentAPIKeyView, 0)
	for rows.Next() {
		var view AgentAPIKeyView
		var created, lastUsed int64
		if err := rows.Scan(&view.ID, &view.Name, &view.Prefix, &view.Status, &created, &lastUsed); err != nil {
			return nil, err
		}
		view.CreatedAt = time.Unix(created, 0).UTC().Format(time.RFC3339)
		if lastUsed > 0 {
			view.LastUsedAt = time.Unix(lastUsed, 0).UTC().Format(time.RFC3339)
		}
		result = append(result, view)
	}
	return result, rows.Err()
}

func (s *Store) RevokeAPIKey(agentID, mainUserID string, id int64) error {
	result, err := s.db.Exec(`UPDATE agent_api_keys SET status='revoked', updated_at=? WHERE id=? AND agent_id=? AND main_user_id=?`, s.clock().UTC().Unix(), id, agentID, mainUserID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return errNotFound
	}
	return nil
}

func (s *Store) ResolveAPIKey(agentID, key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" || len(key) > 256 {
		return "", errNotFound
	}
	var mainUserID string
	err := s.db.QueryRow(`SELECT k.main_user_id FROM agent_api_keys k JOIN agent_users u ON u.agent_id=k.agent_id AND u.main_user_id=k.main_user_id WHERE k.agent_id=? AND k.key_hash=? AND k.status='active' AND u.status='active'`, agentID, hashToken(key)).Scan(&mainUserID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errNotFound
	}
	if err != nil {
		return "", err
	}
	_, _ = s.db.Exec(`UPDATE agent_api_keys SET last_used_at=?, updated_at=? WHERE agent_id=? AND key_hash=?`, s.clock().UTC().Unix(), s.clock().UTC().Unix(), agentID, hashToken(key))
	return mainUserID, nil
}

func (s *Store) CreateSession(mainUserID string, userJSON []byte, accessToken, refreshToken string, expiresAt time.Time) (string, error) {
	raw := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, raw); err != nil {
		return "", fmt.Errorf("generate session id: %w", err)
	}
	sessionID := base64.RawURLEncoding.EncodeToString(raw)
	encAccess, err := s.encrypt(accessToken)
	if err != nil {
		return "", err
	}
	encRefresh, err := s.encrypt(refreshToken)
	if err != nil {
		return "", err
	}
	now := s.clock().UTC().Unix()
	_, err = s.db.Exec(`
		INSERT INTO sessions (id_hash, main_user_id, user_json, access_token, refresh_token, expires_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, hashToken(sessionID), mainUserID, string(userJSON), encAccess, encRefresh, expiresAt.UTC().Unix(), now, now)
	if err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}
	return sessionID, nil
}

func (s *Store) LoadSession(rawID string) (Session, error) {
	var session Session
	var userJSON, access, refresh string
	var expiresAt, createdAt int64
	err := s.db.QueryRow(`
		SELECT main_user_id, user_json, access_token, refresh_token, expires_at, created_at
		FROM sessions WHERE id_hash=?
	`, hashToken(rawID)).Scan(&session.MainUserID, &userJSON, &access, &refresh, &expiresAt, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, errNotFound
	}
	if err != nil {
		return Session{}, err
	}
	if time.Unix(expiresAt, 0).Before(s.clock()) {
		_ = s.DeleteSession(rawID)
		return Session{}, errNotFound
	}
	session.ID = rawID
	session.UserJSON = []byte(userJSON)
	session.AccessToken, err = s.decrypt(access)
	if err != nil {
		return Session{}, err
	}
	session.RefreshToken, err = s.decrypt(refresh)
	if err != nil {
		return Session{}, err
	}
	session.ExpiresAt = time.Unix(expiresAt, 0).UTC()
	_ = createdAt
	return session, nil
}

func (s *Store) UpdateSession(rawID, mainUserID string, userJSON []byte, accessToken, refreshToken string, expiresAt time.Time) error {
	encAccess, err := s.encrypt(accessToken)
	if err != nil {
		return err
	}
	encRefresh, err := s.encrypt(refreshToken)
	if err != nil {
		return err
	}
	now := s.clock().UTC().Unix()
	result, err := s.db.Exec(`
		UPDATE sessions SET main_user_id=?, user_json=?, access_token=?, refresh_token=?, expires_at=?, updated_at=?
		WHERE id_hash=?
	`, mainUserID, string(userJSON), encAccess, encRefresh, expiresAt.UTC().Unix(), now, hashToken(rawID))
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil || count == 0 {
		return errNotFound
	}
	return nil
}

func (s *Store) DeleteSession(rawID string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE id_hash=?`, hashToken(rawID))
	return err
}

// ConsumeSSOTicket atomically records a one-time Sub2API HMAC ticket. The
// ticket id is persisted in SQLite instead of an in-memory map so a restart or
// a second worker cannot replay the same identity handoff.
func (s *Store) ConsumeSSOTicket(jti string, expiresAt time.Time) error {
	jti = strings.TrimSpace(jti)
	if jti == "" {
		return fmt.Errorf("sso ticket id is required")
	}
	now := s.clock().UTC().Unix()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM consumed_sso_tickets WHERE expires_at <= ?`, now); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO consumed_sso_tickets(jti, expires_at, consumed_at) VALUES (?, ?, ?)`, jti, expiresAt.UTC().Unix(), now); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return errIdempotencyConflict
		}
		return err
	}
	return tx.Commit()
}

func (s *Store) Wallet(agentID string) (AgentView, error) {
	return s.Agent(agentID)
}

// SyncOwnerBalance records the authoritative balance returned by Sub2API and
// recalculates only the unallocated portion of the local wallet. Allocated
// user credit is never silently increased by a sync; if the upstream balance
// is lower than already allocated credit, the wallet is marked overallocated
// and new allocations are blocked until the discrepancy is resolved.
func (s *Store) SyncOwnerBalance(agentID, ownerMainUserID string, balanceCents int64, requestID string) (AgentView, error) {
	if strings.TrimSpace(ownerMainUserID) == "" {
		return AgentView{}, fmt.Errorf("owner main user id is required")
	}
	if balanceCents < 0 {
		balanceCents = 0
	}
	tx, err := s.db.Begin()
	if err != nil {
		return AgentView{}, err
	}
	defer tx.Rollback()
	var allocated, version, pendingReserved int64
	if err := tx.QueryRow(`SELECT allocated_cents, version FROM agent_wallets WHERE agent_id=?`, agentID).Scan(&allocated, &version); err != nil {
		return AgentView{}, err
	}
	if err := tx.QueryRow(`SELECT COALESCE(SUM(reserved_cents), 0) FROM settlements WHERE agent_id=? AND status='pending'`, agentID).Scan(&pendingReserved); err != nil {
		return AgentView{}, err
	}
	available := balanceCents - allocated - pendingReserved
	billingStatus := "ok"
	if available < 0 {
		available = 0
		billingStatus = "overallocated"
	}
	now := s.clock().UTC().Unix()
	if _, err := tx.Exec(`UPDATE agent_wallets SET available_cents=?, owner_main_user_id=?, main_balance_cents=?, main_balance_checked_at=?, billing_status=?, version=?, updated_at=? WHERE agent_id=? AND version=?`, available, ownerMainUserID, balanceCents, now, billingStatus, version+1, now, agentID, version); err != nil {
		return AgentView{}, err
	}
	if _, err := tx.Exec(`INSERT INTO agent_balance_snapshots(agent_id, owner_main_user_id, balance_cents, request_id, checked_at) VALUES (?, ?, ?, ?, ?)`, agentID, ownerMainUserID, balanceCents, requestID, now); err != nil {
		return AgentView{}, err
	}
	if err := tx.Commit(); err != nil {
		return AgentView{}, err
	}
	return s.Agent(agentID)
}

func (s *Store) CreateSettlement(agentID, proxyMainUserID, billingMainUserID, requestID, usageID string, reservedCents int64) error {
	if requestID == "" || usageID == "" || reservedCents <= 0 {
		return fmt.Errorf("settlement request, usage id and reservation are required")
	}
	now := s.clock().UTC().Unix()
	_, err := s.db.Exec(`
		INSERT INTO settlements(agent_id, proxy_main_user_id, billing_main_user_id, request_id, usage_id, reserved_cents, actual_cents, status, error, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 0, 'pending', '', ?, ?)
		ON CONFLICT(agent_id, request_id) DO NOTHING
	`, agentID, proxyMainUserID, billingMainUserID, requestID, usageID, reservedCents, now, now)
	if err != nil {
		return err
	}
	record, err := s.Settlement(agentID, requestID)
	if err != nil {
		return err
	}
	if record.ProxyMainUserID != proxyMainUserID || record.BillingMainUserID != billingMainUserID || record.UsageID != usageID || record.ReservedCents != reservedCents {
		return errIdempotencyConflict
	}
	return nil
}

// PrepareSettlement atomically creates a pending settlement and consumes the
// user's local pre-authorization. The transaction closes the crash window that
// otherwise exists between a local Consume and settlement INSERT. If the
// request already exists, it returns the durable record with created=false and
// never changes wallet state.
func (s *Store) PrepareSettlement(agentID, proxyMainUserID, billingMainUserID, requestID, usageID string, reservedCents int64) (SettlementRecord, bool, error) {
	if requestID == "" || usageID == "" || reservedCents <= 0 {
		return SettlementRecord{}, false, fmt.Errorf("settlement request, usage id and reservation are required")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return SettlementRecord{}, false, err
	}
	defer tx.Rollback()

	if record, lookupErr := settlementTx(tx, agentID, requestID); lookupErr == nil {
		if record.ProxyMainUserID != proxyMainUserID || record.BillingMainUserID != billingMainUserID || record.UsageID != usageID || record.ReservedCents != reservedCents {
			return SettlementRecord{}, false, errIdempotencyConflict
		}
		if err := tx.Commit(); err != nil {
			return SettlementRecord{}, false, err
		}
		return record, false, nil
	} else if !errors.Is(lookupErr, errNotFound) {
		return SettlementRecord{}, false, lookupErr
	}

	// The ledger lookup makes this method compatible with data written by the
	// old two-step implementation. New writes cannot reach this branch because
	// settlement and consume are committed together.
	if userID, amount, exists, lookupErr := ledgerEntry(tx, agentID, "consume", "reserve:"+requestID); lookupErr != nil {
		return SettlementRecord{}, false, lookupErr
	} else if exists {
		if userID != proxyMainUserID || amount != -reservedCents {
			return SettlementRecord{}, false, errIdempotencyConflict
		}
	} else {
		var userBalance, userVersion int64
		if err := tx.QueryRow(`SELECT balance_cents, version FROM agent_user_wallets WHERE agent_id=? AND main_user_id=?`, agentID, proxyMainUserID).Scan(&userBalance, &userVersion); err != nil {
			return SettlementRecord{}, false, err
		}
		if userBalance < reservedCents {
			return SettlementRecord{}, false, errInsufficientBalance
		}
		var allocated, agentVersion int64
		if err := tx.QueryRow(`SELECT allocated_cents, version FROM agent_wallets WHERE agent_id=?`, agentID).Scan(&allocated, &agentVersion); err != nil {
			return SettlementRecord{}, false, err
		}
		if allocated < reservedCents {
			return SettlementRecord{}, false, errInsufficientBalance
		}
		now := s.clock().UTC().Unix()
		if _, err := tx.Exec(`UPDATE agent_user_wallets SET balance_cents=?, version=?, updated_at=? WHERE agent_id=? AND main_user_id=? AND version=?`, userBalance-reservedCents, userVersion+1, now, agentID, proxyMainUserID, userVersion); err != nil {
			return SettlementRecord{}, false, err
		}
		if _, err := tx.Exec(`UPDATE agent_wallets SET allocated_cents=?, version=?, updated_at=? WHERE agent_id=? AND version=?`, allocated-reservedCents, agentVersion+1, now, agentID, agentVersion); err != nil {
			return SettlementRecord{}, false, err
		}
		if _, err := tx.Exec(`INSERT INTO wallet_ledger(agent_id, main_user_id, kind, amount_cents, balance_after_cents, request_id, note, created_at) VALUES (?, ?, 'consume', ?, ?, ?, ?, ?)`, agentID, proxyMainUserID, -reservedCents, userBalance-reservedCents, "reserve:"+requestID, "model request reservation", now); err != nil {
			return SettlementRecord{}, false, err
		}
	}

	now := s.clock().UTC().Unix()
	if _, err := tx.Exec(`
		INSERT INTO settlements(agent_id, proxy_main_user_id, billing_main_user_id, request_id, usage_id, reserved_cents, actual_cents, status, error, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 0, 'pending', '', ?, ?)
	`, agentID, proxyMainUserID, billingMainUserID, requestID, usageID, reservedCents, now, now); err != nil {
		return SettlementRecord{}, false, err
	}
	record, err := settlementTx(tx, agentID, requestID)
	if err != nil {
		return SettlementRecord{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return SettlementRecord{}, false, err
	}
	return record, true, nil
}

func (s *Store) Settlement(agentID, requestID string) (SettlementRecord, error) {
	return settlementQuery(s.db, agentID, requestID)
}

func settlementQuery(db interface {
	QueryRow(query string, args ...any) *sql.Row
}, agentID, requestID string) (SettlementRecord, error) {
	var record SettlementRecord
	var created, updated int64
	err := db.QueryRow(`SELECT id, agent_id, proxy_main_user_id, billing_main_user_id, request_id, usage_id, reserved_cents, actual_cents, status, error, created_at, updated_at FROM settlements WHERE agent_id=? AND request_id=?`, agentID, requestID).Scan(&record.ID, &record.AgentID, &record.ProxyMainUserID, &record.BillingMainUserID, &record.RequestID, &record.UsageID, &record.ReservedCents, &record.ActualCents, &record.Status, &record.Error, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return SettlementRecord{}, errNotFound
	}
	if err != nil {
		return SettlementRecord{}, err
	}
	record.CreatedAt = time.Unix(created, 0).UTC()
	record.UpdatedAt = time.Unix(updated, 0).UTC()
	return record, nil
}

func settlementTx(tx *sql.Tx, agentID, requestID string) (SettlementRecord, error) {
	return settlementQuery(tx, agentID, requestID)
}

func (s *Store) SettleSettlement(agentID, requestID, status string, actualCents int64, settlementError string) error {
	return s.SettleSettlementWithUsage(agentID, requestID, "", status, actualCents, settlementError)
}

func (s *Store) SettleSettlementWithUsage(agentID, requestID, usageID, status string, actualCents int64, settlementError string) error {
	return s.FinalizeSettlement(agentID, requestID, usageID, status, actualCents, settlementError)
}

// FinalizeSettlement is the only operation that may move a pending
// settlement to a terminal state. For confirmed/released/reversed outcomes it
// also returns the unused reservation in the same SQLite transaction. This
// prevents a crash between updating settlements and writing the refund ledger
// from losing local credit or creating it twice.
func (s *Store) FinalizeSettlement(agentID, requestID, usageID, status string, actualCents int64, settlementError string) error {
	if status != "pending" && status != "confirmed" && status != "released" && status != "reversed" {
		return fmt.Errorf("invalid settlement status")
	}
	if actualCents < 0 {
		actualCents = 0
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	record, err := settlementTx(tx, agentID, requestID)
	if err != nil {
		return err
	}
	if record.Status != "pending" {
		if record.Status == status && (usageID == "" || usageID == record.UsageID) && record.ActualCents == actualCents && strings.TrimSpace(settlementError) == record.Error {
			return tx.Commit()
		}
		return errSettlementStateConflict
	}
	if usageID == "" {
		usageID = record.UsageID
	}
	if status != "pending" && actualCents > record.ReservedCents {
		return fmt.Errorf("actual settlement exceeds reservation")
	}
	now := s.clock().UTC().Unix()
	if _, err := tx.Exec(`UPDATE settlements SET usage_id=?, status=?, actual_cents=?, error=?, updated_at=? WHERE agent_id=? AND request_id=? AND status='pending'`, usageID, status, actualCents, strings.TrimSpace(settlementError), now, agentID, requestID); err != nil {
		return err
	}
	if status == "pending" {
		return tx.Commit()
	}
	refund := record.ReservedCents - actualCents
	if refund <= 0 {
		return tx.Commit()
	}
	refundRequestID := "refund:" + record.RequestID
	if userID, amount, exists, lookupErr := ledgerEntry(tx, agentID, "refund", refundRequestID); lookupErr != nil {
		return lookupErr
	} else if exists {
		if userID != record.ProxyMainUserID || amount != refund {
			return errIdempotencyConflict
		}
		return tx.Commit()
	}
	var userBalance, userVersion int64
	if err := tx.QueryRow(`SELECT balance_cents, version FROM agent_user_wallets WHERE agent_id=? AND main_user_id=?`, agentID, record.ProxyMainUserID).Scan(&userBalance, &userVersion); err != nil {
		return err
	}
	var available, allocated, agentVersion int64
	if err := tx.QueryRow(`SELECT available_cents, allocated_cents, version FROM agent_wallets WHERE agent_id=?`, agentID).Scan(&available, &allocated, &agentVersion); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE agent_user_wallets SET balance_cents=?, version=?, updated_at=? WHERE agent_id=? AND main_user_id=? AND version=?`, userBalance+refund, userVersion+1, now, agentID, record.ProxyMainUserID, userVersion); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE agent_wallets SET available_cents=?, allocated_cents=?, version=?, updated_at=? WHERE agent_id=? AND version=?`, available, allocated+refund, agentVersion+1, now, agentID, agentVersion); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO wallet_ledger(agent_id, main_user_id, kind, amount_cents, balance_after_cents, request_id, note, usage_id, billing_main_user_id, balance_before_cents, created_at) VALUES (?, ?, 'refund', ?, ?, ?, ?, ?, ?, ?, ?)`, agentID, record.ProxyMainUserID, refund, userBalance+refund, refundRequestID, "settlement unused reservation", usageID, record.BillingMainUserID, userBalance, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) PendingSettlements(agentID string, limit int) ([]SettlementRecord, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(`SELECT id, agent_id, proxy_main_user_id, billing_main_user_id, request_id, usage_id, reserved_cents, actual_cents, status, error, created_at, updated_at FROM settlements WHERE agent_id=? AND status='pending' ORDER BY id ASC LIMIT ?`, agentID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]SettlementRecord, 0)
	for rows.Next() {
		var record SettlementRecord
		var created, updated int64
		if err := rows.Scan(&record.ID, &record.AgentID, &record.ProxyMainUserID, &record.BillingMainUserID, &record.RequestID, &record.UsageID, &record.ReservedCents, &record.ActualCents, &record.Status, &record.Error, &created, &updated); err != nil {
			return nil, err
		}
		record.CreatedAt = time.Unix(created, 0).UTC()
		record.UpdatedAt = time.Unix(updated, 0).UTC()
		result = append(result, record)
	}
	return result, rows.Err()
}

func (s *Store) Usage(agentID, mainUserID string, limit int) ([]AgentUsageView, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := `
		SELECT st.usage_id, st.proxy_main_user_id, st.billing_main_user_id,
		       st.reserved_cents, st.actual_cents, st.status, st.error, st.created_at
		FROM settlements st
		WHERE st.agent_id=?`
	args := []any{agentID}
	if mainUserID != "" {
		query += ` AND st.proxy_main_user_id=?`
		args = append(args, mainUserID)
	}
	query += ` ORDER BY st.id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]AgentUsageView, 0)
	for rows.Next() {
		var item AgentUsageView
		var created int64
		if err := rows.Scan(&item.RequestID, &item.ProxyMainUserID, &item.BillingMainUserID, &item.ReservedCents, &item.ActualCents, &item.SettlementStatus, &item.Error, &created); err != nil {
			return nil, err
		}
		item.CreatedAt = time.Unix(created, 0).UTC().Format(time.RFC3339)
		result = append(result, item)
	}
	return result, rows.Err()
}

func ledgerEntry(tx *sql.Tx, agentID, kind, requestID string) (mainUserID string, amountCents int64, exists bool, err error) {
	err = tx.QueryRow(`SELECT main_user_id, amount_cents FROM wallet_ledger WHERE agent_id=? AND kind=? AND request_id=? LIMIT 1`, agentID, kind, requestID).Scan(&mainUserID, &amountCents)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, false, nil
	}
	if err != nil {
		return "", 0, false, err
	}
	return mainUserID, amountCents, true, nil
}

func (s *Store) Allocate(agentID, mainUserID string, cents int64, requestID, orderID, note string) error {
	if cents <= 0 {
		return fmt.Errorf("amount must be positive")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if requestID != "" {
		userID, amount, exists, lookupErr := ledgerEntry(tx, agentID, "user_allocate", requestID)
		if lookupErr != nil {
			return lookupErr
		}
		if exists {
			if userID != mainUserID || amount != cents {
				return errIdempotencyConflict
			}
			return tx.Commit()
		}
	}
	var available, allocated, version int64
	if err := tx.QueryRow(`SELECT available_cents, allocated_cents, version FROM agent_wallets WHERE agent_id=?`, agentID).Scan(&available, &allocated, &version); err != nil {
		return err
	}
	if available < cents {
		return errInsufficientBalance
	}
	if _, err := tx.Exec(`UPDATE agent_wallets SET available_cents=?, allocated_cents=?, version=?, updated_at=? WHERE agent_id=? AND version=?`, available-cents, allocated+cents, version+1, s.clock().UTC().Unix(), agentID, version); err != nil {
		return err
	}
	var userBalance, userVersion int64
	if err := tx.QueryRow(`SELECT balance_cents, version FROM agent_user_wallets WHERE agent_id=? AND main_user_id=?`, agentID, mainUserID).Scan(&userBalance, &userVersion); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE agent_user_wallets SET balance_cents=?, version=?, updated_at=? WHERE agent_id=? AND main_user_id=? AND version=?`, userBalance+cents, userVersion+1, s.clock().UTC().Unix(), agentID, mainUserID, userVersion); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO wallet_ledger(agent_id, main_user_id, kind, amount_cents, balance_after_cents, request_id, order_id, note, created_at) VALUES (?, '', 'agent_allocate', ?, ?, ?, ?, ?, ?)`, agentID, -cents, available-cents, requestID, orderID, note, s.clock().UTC().Unix()); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO wallet_ledger(agent_id, main_user_id, kind, amount_cents, balance_after_cents, request_id, order_id, note, created_at) VALUES (?, ?, 'user_allocate', ?, ?, ?, ?, ?, ?)`, agentID, mainUserID, cents, userBalance+cents, requestID, orderID, note, s.clock().UTC().Unix()); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Consume(agentID, mainUserID string, cents int64, requestID, note string) error {
	if cents <= 0 {
		return fmt.Errorf("amount must be positive")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if requestID != "" {
		userID, amount, exists, lookupErr := ledgerEntry(tx, agentID, "consume", requestID)
		if lookupErr != nil {
			return lookupErr
		}
		if exists {
			if userID != mainUserID || amount != -cents {
				return errIdempotencyConflict
			}
			return tx.Commit()
		}
	}
	var userBalance, userVersion int64
	if err := tx.QueryRow(`SELECT balance_cents, version FROM agent_user_wallets WHERE agent_id=? AND main_user_id=?`, agentID, mainUserID).Scan(&userBalance, &userVersion); err != nil {
		return err
	}
	if userBalance < cents {
		return errInsufficientBalance
	}
	var available, allocated, agentVersion int64
	if err := tx.QueryRow(`SELECT available_cents, allocated_cents, version FROM agent_wallets WHERE agent_id=?`, agentID).Scan(&available, &allocated, &agentVersion); err != nil {
		return err
	}
	if allocated < cents {
		return errInsufficientBalance
	}
	now := s.clock().UTC().Unix()
	if _, err := tx.Exec(`UPDATE agent_user_wallets SET balance_cents=?, version=?, updated_at=? WHERE agent_id=? AND main_user_id=? AND version=?`, userBalance-cents, userVersion+1, now, agentID, mainUserID, userVersion); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE agent_wallets SET allocated_cents=?, version=?, updated_at=? WHERE agent_id=? AND version=?`, allocated-cents, agentVersion+1, now, agentID, agentVersion); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO wallet_ledger(agent_id, main_user_id, kind, amount_cents, balance_after_cents, request_id, note, created_at) VALUES (?, ?, 'consume', ?, ?, ?, ?, ?)`, agentID, mainUserID, -cents, userBalance-cents, requestID, note, now); err != nil {
		return err
	}
	_ = available
	return tx.Commit()
}

// Credit reverses a prior local charge. It is deliberately represented as a
// new ledger row; historical entries are never edited. The operation is
// idempotent when requestID is reused (for example after a relay retry).
func (s *Store) Credit(agentID, mainUserID string, cents int64, requestID, note string) error {
	if cents <= 0 {
		return fmt.Errorf("amount must be positive")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if requestID != "" {
		userID, amount, exists, lookupErr := ledgerEntry(tx, agentID, "refund", requestID)
		if lookupErr != nil {
			return lookupErr
		}
		if exists {
			if userID != mainUserID || amount != cents {
				return errIdempotencyConflict
			}
			return tx.Commit()
		}
	}
	var userBalance, userVersion int64
	if err := tx.QueryRow(`SELECT balance_cents, version FROM agent_user_wallets WHERE agent_id=? AND main_user_id=?`, agentID, mainUserID).Scan(&userBalance, &userVersion); err != nil {
		return err
	}
	var available, allocated, agentVersion int64
	if err := tx.QueryRow(`SELECT available_cents, allocated_cents, version FROM agent_wallets WHERE agent_id=?`, agentID).Scan(&available, &allocated, &agentVersion); err != nil {
		return err
	}
	if allocated+cents < 0 {
		return fmt.Errorf("invalid allocated balance")
	}
	now := s.clock().UTC().Unix()
	if _, err := tx.Exec(`UPDATE agent_user_wallets SET balance_cents=?, version=?, updated_at=? WHERE agent_id=? AND main_user_id=? AND version=?`, userBalance+cents, userVersion+1, now, agentID, mainUserID, userVersion); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE agent_wallets SET available_cents=?, allocated_cents=?, version=?, updated_at=? WHERE agent_id=? AND version=?`, available, allocated+cents, agentVersion+1, now, agentID, agentVersion); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO wallet_ledger(agent_id, main_user_id, kind, amount_cents, balance_after_cents, request_id, note, created_at) VALUES (?, ?, 'refund', ?, ?, ?, ?, ?)`, agentID, mainUserID, cents, userBalance+cents, requestID, note, now); err != nil {
		return err
	}
	return tx.Commit()
}

// CreditAgent adds funds to the agent's unallocated wallet. It is intended for
// a verified control-plane callback, never for an end-user payment request.
func (s *Store) CreditAgent(agentID string, cents int64, requestID, orderID, note string) error {
	if cents <= 0 {
		return fmt.Errorf("amount must be positive")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if requestID != "" {
		_, amount, exists, lookupErr := ledgerEntry(tx, agentID, "agent_credit", requestID)
		if lookupErr != nil {
			return lookupErr
		}
		if exists {
			if amount != cents {
				return errIdempotencyConflict
			}
			return tx.Commit()
		}
	}
	var available, allocated, version int64
	if err := tx.QueryRow(`SELECT available_cents, allocated_cents, version FROM agent_wallets WHERE agent_id=?`, agentID).Scan(&available, &allocated, &version); err != nil {
		return err
	}
	now := s.clock().UTC().Unix()
	if _, err := tx.Exec(`UPDATE agent_wallets SET available_cents=?, version=?, updated_at=? WHERE agent_id=? AND version=?`, available+cents, version+1, now, agentID, version); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO wallet_ledger(agent_id, main_user_id, kind, amount_cents, balance_after_cents, request_id, order_id, note, created_at) VALUES (?, '', 'agent_credit', ?, ?, ?, ?, ?, ?)`, agentID, cents, available+cents, requestID, orderID, note, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) encrypt(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	data := s.aead.Seal(nonce, nonce, []byte(value), nil)
	return base64.RawStdEncoding.EncodeToString(data), nil
}

func (s *Store) decrypt(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	data, err := base64.RawStdEncoding.DecodeString(value)
	if err != nil {
		return "", err
	}
	if len(data) < s.aead.NonceSize() {
		return "", fmt.Errorf("invalid encrypted value")
	}
	nonce, ciphertext := data[:s.aead.NonceSize()], data[s.aead.NonceSize():]
	plaintext, err := s.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func userJSONFields(data []byte) (email, displayName, role string) {
	var value map[string]any
	if json.Unmarshal(data, &value) != nil {
		return "", "", ""
	}
	if raw, ok := value["email"].(string); ok {
		email = raw
	}
	if raw, ok := value["username"].(string); ok {
		displayName = raw
	}
	if displayName == "" {
		if raw, ok := value["display_name"].(string); ok {
			displayName = raw
		}
	}
	if raw, ok := value["role"].(string); ok {
		role = raw
	}
	return email, displayName, role
}
