package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBackupSQLiteDatabaseIncludesCommittedWALData(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "agentapi.db")
	backup := filepath.Join(dir, "backups", "agentapi.db")
	store, err := OpenStore(source, strings.Repeat("s", 32))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	cfg := Config{
		AgentID: "agent-backup", AgentDomain: "backup.example.test", AgentName: "Backup Agent",
		SiteName: "Backup Agent", BillingMode: "user_upstream", AppCredential: "app-secret",
	}
	if err := store.UpsertAgent(cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertUser(cfg.AgentID, "42", "owner@example.test", "Owner"); err != nil {
		t.Fatal(err)
	}
	if err := backupSQLiteDatabase(source, backup); err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open("sqlite", backup)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var agentName, email string
	if err := db.QueryRow(`SELECT name FROM agent_config WHERE agent_id=?`, cfg.AgentID).Scan(&agentName); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT email FROM agent_users WHERE agent_id=? AND main_user_id=?`, cfg.AgentID, "42").Scan(&email); err != nil {
		t.Fatal(err)
	}
	if agentName != cfg.AgentName || email != "owner@example.test" {
		t.Fatalf("backup data mismatch: agent=%q email=%q", agentName, email)
	}
}

func TestBackupSQLiteDatabaseRejectsUnsafeDestinations(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "agentapi.db")
	if err := os.WriteFile(source, []byte("not-used"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := backupSQLiteDatabase(source, source); err == nil || !strings.Contains(err.Error(), "must differ") {
		t.Fatalf("same-path backup error=%v", err)
	}
	existing := filepath.Join(dir, "existing.db")
	if err := os.WriteFile(existing, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := backupSQLiteDatabase(source, existing); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("existing destination error=%v", err)
	}
}
