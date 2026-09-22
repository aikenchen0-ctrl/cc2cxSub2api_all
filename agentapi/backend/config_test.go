package main

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSecretEnvPrefersFileBackedSecret(t *testing.T) {
	t.Setenv("AGENTAPI_TEST_SECRET", "inline-value")
	path := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(path, []byte("file-value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENTAPI_TEST_SECRET_FILE", path)
	if got := secretEnv("AGENTAPI_TEST_SECRET"); got != "file-value" {
		t.Fatalf("secretEnv()=%q, want file-backed value", got)
	}
}

func TestSecretEnvFallsBackWhenFileIsUnavailable(t *testing.T) {
	t.Setenv("AGENTAPI_TEST_SECRET", "inline-value")
	t.Setenv("AGENTAPI_TEST_SECRET_FILE", filepath.Join(t.TempDir(), "missing"))
	if got := secretEnv("AGENTAPI_TEST_SECRET"); got != "inline-value" {
		t.Fatalf("secretEnv()=%q, want inline fallback", got)
	}
}

func TestDecodeOptionalJSONAcceptsEmptyBody(t *testing.T) {
	req := httptest.NewRequest("POST", "/", strings.NewReader(""))
	var payload struct {
		RequestID string `json:"request_id"`
	}
	hasBody, err := decodeOptionalJSON(req, &payload, 1024)
	if err != nil || hasBody || payload.RequestID != "" {
		t.Fatalf("decodeOptionalJSON()=(%v, %v, %+v), want empty body", hasBody, err, payload)
	}
}

func TestDecodeOptionalJSONHandlesChunkedJSON(t *testing.T) {
	req := httptest.NewRequest("POST", "/", strings.NewReader(`{"request_id":"req-1"}`))
	var payload struct {
		RequestID string `json:"request_id"`
	}
	hasBody, err := decodeOptionalJSON(req, &payload, 1024)
	if err != nil || !hasBody || payload.RequestID != "req-1" {
		t.Fatalf("decodeOptionalJSON()=(%v, %v, %+v), want request id", hasBody, err, payload)
	}
}
