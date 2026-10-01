package main

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// backupSQLiteDatabase creates a transactionally consistent copy of the live
// SQLite database. VACUUM INTO includes committed WAL contents, unlike copying
// agentapi.db directly while the service is running.
func backupSQLiteDatabase(sourcePath, destinationPath string) error {
	sourcePath = strings.TrimSpace(sourcePath)
	destinationPath = strings.TrimSpace(destinationPath)
	if sourcePath == "" || sourcePath == ":memory:" {
		return errors.New("backup source must be a persistent SQLite database")
	}
	if destinationPath == "" {
		return errors.New("backup destination is required")
	}
	sourceAbs, err := filepath.Abs(sourcePath)
	if err != nil {
		return fmt.Errorf("resolve backup source: %w", err)
	}
	destinationAbs, err := filepath.Abs(destinationPath)
	if err != nil {
		return fmt.Errorf("resolve backup destination: %w", err)
	}
	if filepath.Clean(sourceAbs) == filepath.Clean(destinationAbs) {
		return errors.New("backup destination must differ from the source database")
	}
	info, err := os.Stat(sourceAbs)
	if err != nil {
		return fmt.Errorf("stat backup source: %w", err)
	}
	if !info.Mode().IsRegular() {
		return errors.New("backup source is not a regular file")
	}
	if _, err := os.Stat(destinationAbs); err == nil {
		return errors.New("backup destination already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat backup destination: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(destinationAbs), 0o750); err != nil {
		return fmt.Errorf("create backup directory: %w", err)
	}

	db, err := sql.Open("sqlite", sourceAbs)
	if err != nil {
		return fmt.Errorf("open backup source: %w", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err := db.Exec(`PRAGMA busy_timeout = 5000`); err != nil {
		return fmt.Errorf("configure backup source: %w", err)
	}
	if _, err := db.Exec(`VACUUM INTO ?`, destinationAbs); err != nil {
		_ = os.Remove(destinationAbs)
		return fmt.Errorf("create SQLite backup: %w", err)
	}
	if err := verifySQLiteBackup(destinationAbs); err != nil {
		_ = os.Remove(destinationAbs)
		return err
	}
	return nil
}

func verifySQLiteBackup(path string) error {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return fmt.Errorf("open generated backup: %w", err)
	}
	defer db.Close()
	var result string
	if err := db.QueryRow(`PRAGMA integrity_check`).Scan(&result); err != nil {
		return fmt.Errorf("verify generated backup: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("generated backup failed integrity check: %s", result)
	}
	return nil
}
