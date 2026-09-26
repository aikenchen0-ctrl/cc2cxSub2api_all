package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigAcceptsDocumentedSchemeLessLocalLink(t *testing.T) {
	setupWorkerConfigEnv(t)
	t.Setenv("MAIN_API_URL", "")
	t.Setenv("LINK", "localhost:18080")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() error = %v", err)
	}
	if cfg.MainURL != "http://localhost:18080" {
		t.Fatalf("MainURL = %q, want http://localhost:18080", cfg.MainURL)
	}
}

func TestLoadConfigUsesProvisioningCredentialInsteadOfAgentRuntimeAdminKey(t *testing.T) {
	setupWorkerConfigEnv(t)
	workerCredentialPath := os.Getenv("AGENT_PROVISIONING_WORKER_CREDENTIAL_FILE")
	workerCredential, err := os.ReadFile(workerCredentialPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("SUB2API_ADMIN_KEY_FILE", "")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() error = %v", err)
	}
	if cfg.ProvisioningCredential != string(workerCredential) {
		t.Fatalf("worker loaded the wrong control-plane credential")
	}
}

func TestLoadConfigRejectsInsecureNonLoopbackMainURL(t *testing.T) {
	setupWorkerConfigEnv(t)
	t.Setenv("MAIN_API_URL", "http://api.example.com")

	if _, err := loadConfig(); err == nil {
		t.Fatal("loadConfig() accepted a non-loopback HTTP control plane")
	}
}

func TestLoadConfigRejectsSchemeLessRemoteLink(t *testing.T) {
	setupWorkerConfigEnv(t)
	t.Setenv("MAIN_API_URL", "")
	t.Setenv("LINK", "api.example.com")

	if _, err := loadConfig(); err == nil {
		t.Fatal("loadConfig() accepted a remote LINK without HTTPS")
	}
}

func setupWorkerConfigEnv(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	for _, secret := range []struct {
		name  string
		env   string
		value string
	}{
		{name: "worker", env: "AGENT_PROVISIONING_WORKER_CREDENTIAL_FILE", value: "worker-provisioning-test-secret-32-bytes-minimum"},
		{name: "sso", env: "SUB2API_SSO_SECRET_FILE", value: "sso-test-secret"},
	} {
		path := filepath.Join(root, secret.name+".secret")
		if err := os.WriteFile(path, []byte(secret.value), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv(secret.env, path)
	}
	nginxDir := filepath.Join(root, "nginx")
	if err := os.Mkdir(nginxDir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MAIN_API_URL", "https://api.example.com")
	t.Setenv("LINK", "")
	t.Setenv("AGENTAPI_NGINX_CONFIG_DIR", nginxDir)
	t.Setenv("AGENTAPI_NGINX_CONTAINER", "nginx-edge")
	t.Setenv("AGENTAPI_EDGE_IPS", "203.0.113.7")
}
