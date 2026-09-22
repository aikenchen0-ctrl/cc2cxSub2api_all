package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func validRequest() request {
	return request{IdempotencyKey: "request-12345", Slug: "agent01", Domain: "agent01.cc2.cx", DisplayName: "Agent 01", OwnerMainUser: "42", MainURL: "https://api.cc2.cx", Image: "registry.example/agentapi:v1"}
}

func TestProvisionCreatesSecretSafeIdempotentBundle(t *testing.T) {
	dir := t.TempDir()
	first, err := provision(dir, validRequest())
	if err != nil {
		t.Fatal(err)
	}
	second, err := provision(dir, validRequest())
	if err != nil {
		t.Fatal(err)
	}
	if first.Fingerprint != second.Fingerprint || first.BundlePath != second.BundlePath {
		t.Fatalf("replay changed result: %+v %+v", first, second)
	}
	compose, err := os.ReadFile(filepath.Join(first.BundlePath, "compose.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	env, err := os.ReadFile(filepath.Join(first.BundlePath, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"admin-secret", "app-secret", "sso-secret"} {
		if strings.Contains(string(compose), secret) || strings.Contains(string(env), secret) {
			t.Fatalf("secret leaked into bundle")
		}
	}
	if !strings.Contains(string(compose), "${SUB2API_ADMIN_KEY_FILE:?") || !strings.Contains(string(env), `AGENT_DOMAIN="agent01.cc2.cx"`) {
		t.Fatalf("unexpected bundle: %s\n%s", compose, env)
	}
	info, err := os.Stat(filepath.Join(first.BundlePath, "secrets", "session_secret"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("session secret permissions are too broad: %v", info.Mode())
	}
}

func TestProvisionRejectsIdempotencySlugAndDomainConflicts(t *testing.T) {
	dir := t.TempDir()
	req := validRequest()
	if _, err := provision(dir, req); err != nil {
		t.Fatal(err)
	}
	changed := req
	changed.Domain = "other.cc2.cx"
	if _, err := provision(dir, changed); err == nil || !strings.Contains(err.Error(), "idempotency") {
		t.Fatalf("idempotency conflict not rejected: %v", err)
	}
	slugConflict := req
	slugConflict.IdempotencyKey = "request-other-1"
	slugConflict.Domain = "other.cc2.cx"
	if _, err := provision(dir, slugConflict); err == nil || !strings.Contains(err.Error(), "slug") {
		t.Fatalf("slug conflict not rejected: %v", err)
	}
	domainConflict := req
	domainConflict.IdempotencyKey = "request-other-2"
	domainConflict.Slug = "agent02"
	if _, err := provision(dir, domainConflict); err == nil || !strings.Contains(err.Error(), "domain") {
		t.Fatalf("domain conflict not rejected: %v", err)
	}
}

func TestProvisionRejectsUnsafeInput(t *testing.T) {
	for name, mutate := range map[string]func(*request){
		"slug":    func(r *request) { r.Slug = "../escape" },
		"domain":  func(r *request) { r.Domain = "bad domain" },
		"display": func(r *request) { r.DisplayName = "bad\nname" },
		"url":     func(r *request) { r.MainURL = "http://remote.example" },
		"image":   func(r *request) { r.Image = "image; touch bad" },
	} {
		t.Run(name, func(t *testing.T) {
			req := validRequest()
			mutate(&req)
			if _, err := provision(t.TempDir(), req); err == nil {
				t.Fatal("unsafe input was accepted")
			}
		})
	}
}
