package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func validRequest() request {
	return request{IdempotencyKey: "request-12345", AgentID: "agt_0123456789abcdef0123456789abcdef", Slug: "agent01", Domain: "agent01.cc2.cx", DisplayName: "Agent 01", OwnerMainUser: "42", MainURL: "https://api.cc2.cx", Image: "registry.example/agentapi:v1"}
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
	if first.Fingerprint != second.Fingerprint || first.BundlePath != second.BundlePath || first.RuntimeSecretDir != second.RuntimeSecretDir || first.ControlTokenHash != second.ControlTokenHash || first.ModelTokenHash != second.ModelTokenHash {
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
	controlCredential, err := os.ReadFile(filepath.Join(first.RuntimeSecretDir, "agent_control_credential"))
	if err != nil {
		t.Fatal(err)
	}
	modelCredential, err := os.ReadFile(filepath.Join(first.RuntimeSecretDir, "agent_model_credential"))
	if err != nil {
		t.Fatal(err)
	}
	sessionSecret, err := os.ReadFile(filepath.Join(first.RuntimeSecretDir, "session_secret"))
	if err != nil {
		t.Fatal(err)
	}
	controlSum := sha256.Sum256([]byte(strings.TrimSpace(string(controlCredential))))
	modelSum := sha256.Sum256([]byte(strings.TrimSpace(string(modelCredential))))
	if first.ControlTokenHash != hex.EncodeToString(controlSum[:]) || first.ModelTokenHash != hex.EncodeToString(modelSum[:]) ||
		!strings.HasPrefix(string(controlCredential), "agt_ctl_") || !strings.HasPrefix(string(modelCredential), "agt_model_") {
		t.Fatal("generated runtime credential metadata does not match the file-backed secrets")
	}
	for _, secret := range []string{"admin-secret", "app-secret", "sso-secret", strings.TrimSpace(string(controlCredential)), strings.TrimSpace(string(modelCredential))} {
		if strings.Contains(string(compose), secret) || strings.Contains(string(env), secret) {
			t.Fatalf("secret leaked into bundle")
		}
	}
	secretDir := filepath.ToSlash(first.RuntimeSecretDir)
	if !strings.Contains(string(compose), "${SUB2API_SSO_SECRET_FILE:?") || strings.Contains(string(compose), "SUB2API_ADMIN_KEY") || !strings.Contains(string(compose), secretDir+"/agent_control_credential") || !strings.Contains(string(compose), secretDir+"/agent_model_credential") || !strings.Contains(string(env), `AGENT_ID="agt_0123456789abcdef0123456789abcdef"`) || !strings.Contains(string(env), `AGENT_DOMAIN="agent01.cc2.cx"`) || !strings.Contains(string(env), "AGENT_PROVISIONING_CONTROL_ENABLED=true") {
		t.Fatalf("unexpected bundle: %s\n%s", compose, env)
	}
	if _, err := os.Stat(filepath.Join(first.BundlePath, "secrets")); !os.IsNotExist(err) {
		t.Fatalf("generated bundle must not contain a secrets directory: %v", err)
	}
	info, err := os.Stat(filepath.Join(first.RuntimeSecretDir, "session_secret"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("session secret permissions are too broad: %v", info.Mode())
	}
	secretDirInfo, err := os.Stat(first.RuntimeSecretDir)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && secretDirInfo.Mode().Perm()&0o077 != 0 {
		t.Fatalf("runtime secret directory permissions are too broad: %v", secretDirInfo.Mode())
	}
	if err := filepath.WalkDir(first.BundlePath, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, secret := range [][]byte{controlCredential, modelCredential, sessionSecret} {
			if strings.Contains(string(data), strings.TrimSpace(string(secret))) {
				return os.ErrPermission
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("generated bundle contains a runtime secret: %v", err)
	}
}

func TestProvisionMigratesLegacyBundleSecretsOutOfPackage(t *testing.T) {
	dir := t.TempDir()
	req := validRequest()
	generated, err := provision(dir, req)
	if err != nil {
		t.Fatal(err)
	}
	legacyDir := filepath.Join(generated.BundlePath, "secrets")
	if err := os.Mkdir(legacyDir, 0o750); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"session_secret", "agent_control_credential", "agent_model_credential"} {
		if err := os.Rename(filepath.Join(generated.RuntimeSecretDir, name), filepath.Join(legacyDir, name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Remove(generated.RuntimeSecretDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(legacyDir, 0o750); err != nil {
		t.Fatal(err)
	}
	legacyState := persisted{Request: normalize(req), Result: generated}
	legacyState.Result.RuntimeSecretDir = ""
	legacyJSON, err := json.Marshal(legacyState)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(generated.BundlePath, "provision.json"), legacyJSON, 0o640); err != nil {
		t.Fatal(err)
	}
	requestPath := filepath.Join(dir, "requests", hash(req.IdempotencyKey)+".json")
	if err := os.WriteFile(requestPath, legacyJSON, 0o640); err != nil {
		t.Fatal(err)
	}

	migrated, err := provision(dir, req)
	if err != nil {
		t.Fatal(err)
	}
	if migrated.RuntimeSecretDir != filepath.Join(dir, "runtime-secrets", req.Slug) {
		t.Fatalf("legacy runtime secrets were not moved to the external secret root: %+v", migrated)
	}
	if _, err := os.Stat(filepath.Join(migrated.BundlePath, "secrets")); !os.IsNotExist(err) {
		t.Fatalf("legacy package still contains secret files: %v", err)
	}
	if _, err := os.Stat(filepath.Join(migrated.RuntimeSecretDir, "agent_control_credential")); err != nil {
		t.Fatalf("migrated control credential is missing: %v", err)
	}
	compose, err := os.ReadFile(filepath.Join(migrated.BundlePath, "compose.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(compose), filepath.ToSlash(migrated.RuntimeSecretDir)+"/agent_control_credential") {
		t.Fatalf("compose manifest was not migrated to the external secret path: %s", compose)
	}
}

func TestProvisionRequiresMainSiteAgentID(t *testing.T) {
	for _, agentID := range []string{"", "agent-agent01", "agt_not-a-uuid", "agt_0123456789ABCDEF0123456789abcdef"} {
		req := validRequest()
		req.AgentID = agentID
		if _, err := provision(t.TempDir(), req); err == nil || !strings.Contains(err.Error(), "agent id") {
			t.Fatalf("invalid agent id %q was not rejected: %v", agentID, err)
		}
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

func TestRenderNginxSupportsModelBodyLimitAndStreaming(t *testing.T) {
	got := renderNginx(validRequest())
	for _, directive := range []string{
		"client_max_body_size 32m;",
		"proxy_http_version 1.1;",
		"proxy_request_buffering off;",
		"proxy_buffering off;",
		"proxy_read_timeout 3600s;",
		"proxy_send_timeout 3600s;",
		"proxy_pass http://agentapi-agent01:8080;",
	} {
		if !strings.Contains(got, directive) {
			t.Errorf("generated Nginx config is missing %q:\n%s", directive, got)
		}
	}
}

func TestWriteJSONAtomicDoesNotReusePredictableTemporaryPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "record.json")
	predictableTemp := path + ".tmp"
	marker := []byte("leave this file untouched")
	if err := os.WriteFile(predictableTemp, marker, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := writeJSONAtomic(path, map[string]string{"status": "ready"}, 0o600); err != nil {
		t.Fatalf("writeJSONAtomic() error = %v", err)
	}
	if got, err := os.ReadFile(predictableTemp); err != nil || string(got) != string(marker) {
		t.Fatalf("predictable temporary path changed: got %q, err %v", got, err)
	}
	if got, err := os.ReadFile(path); err != nil || !strings.Contains(string(got), `"status": "ready"`) {
		t.Fatalf("atomic destination = %q, err %v", got, err)
	}
}
