package main

import (
	"path/filepath"
	"testing"
)

func TestMainAdminCredentialAliasAndFailClosed(t *testing.T) {
	for _, name := range []string{"SUB2API_ADMIN_API_KEY", "SUB2API_ADMIN_API_KEY_FILE", "SUB2API_ADMIN_KEY", "SUB2API_ADMIN_KEY_FILE"} {
		t.Setenv(name, "")
	}
	t.Setenv("SUB2API_ADMIN_KEY", "legacy")
	if mainAdminCredential() != "legacy" {
		t.Fatal("legacy deployment incompatible")
	}
	t.Setenv("SUB2API_ADMIN_API_KEY", "current")
	if mainAdminCredential() != "current" {
		t.Fatal("new name must win")
	}
	t.Setenv("SUB2API_ADMIN_API_KEY_FILE", filepath.Join(t.TempDir(), "missing"))
	if mainAdminCredential() != "" {
		t.Fatal("unreadable configured secret must disable registration, not fall back")
	}
	for _, name := range []string{"SUB2API_ADMIN_API_KEY", "SUB2API_ADMIN_API_KEY_FILE", "SUB2API_ADMIN_KEY", "SUB2API_ADMIN_KEY_FILE"} {
		t.Setenv(name, "")
	}
	if mainAdminCredential() != builtinMainAdminAPIKey || builtinMainAdminAPIKey == "" {
		t.Fatal("missing deployment key must use the server-side builtin key")
	}
	t.Setenv("SUB2API_ADMIN_KEY", "local-agentapi-admin-placeholder")
	if mainAdminCredential() != builtinMainAdminAPIKey {
		t.Fatal("placeholder key must not replace the builtin key")
	}
}
