package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPreflightReadOnlyAndRedacted(t *testing.T) {
	for _, status := range []int{200, 401} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != http.MethodGet {
					t.Errorf("not read-only: %s", r.Method)
				}
				switch r.URL.Path {
				case "/api/v1/admin/users/42":
					if r.Header.Get("x-api-key") != "admin-secret" || r.Header.Get("Authorization") != "" {
						t.Error("incorrect admin credentials")
					}
				case "/v1/sub2api/balance":
					if r.Header.Get("Authorization") != "Bearer app-secret" || r.Header.Get("X-Sub2API-On-Behalf-Of") != "42" || r.Header.Get("X-Sub2API-Satellite") != "agentapi" || r.Header.Get("x-api-key") != "" {
						t.Error("incorrect satellite credentials")
					}
				default:
					t.Errorf("unexpected endpoint: %s", r.URL.Path)
				}
				w.WriteHeader(status)
				if status == 401 {
					_, _ = w.Write([]byte("admin-secret app-secret private-user"))
					return
				}
				if strings.Contains(r.URL.Path, "/admin/") {
					_, _ = w.Write([]byte(`{"code":0,"data":{"id":42,"email":"private-user"}}`))
					return
				}
				_, _ = w.Write([]byte(`{"balance":0}`))
			}))
			defer upstream.Close()
			cfg := preflightTestConfig(upstream.URL)
			report := runPreflight(context.Background(), cfg)
			if report.OK != (status == 200) || calls != 2 {
				t.Fatalf("report=%+v calls=%d", report, calls)
			}
			raw, _ := json.Marshal(report)
			for _, secret := range []string{"admin-secret", "app-secret", "private-user", upstream.URL} {
				if strings.Contains(string(raw), secret) {
					t.Error("sensitive report")
				}
			}
		})
	}
}

func preflightTestConfig(base string) Config {
	return Config{MainAPIBaseURL: base + "/api/v1", MainModelBaseURL: base + "/v1", PublicMainURL: base,
		MainAdminAPIKey: "admin-secret", AppCredential: "app-secret", SatelliteSlug: "agentapi", PreflightMainUserID: "42",
		SSOSecret: strings.Repeat("s", 32), SessionSecret: strings.Repeat("t", 32), BillingMode: "user_upstream"}
}

func TestPreflightRejectsRedirectsAndMalformedResponses(t *testing.T) {
	for _, body := range []string{`{}`, `{"balance":null}`, `{"code":1,"data":{"id":42,"balance":1}}`, `{"code":"error","data":{"id":42,"balance":1}}`, `not json`, "redirect"} {
		t.Run(body, func(t *testing.T) {
			redirectCalls := 0
			destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirectCalls++ }))
			defer destination.Close()
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if body == "redirect" {
					http.Redirect(w, r, destination.URL, http.StatusFound)
					return
				}
				_, _ = w.Write([]byte(body))
			}))
			defer upstream.Close()
			report := runPreflight(context.Background(), preflightTestConfig(upstream.URL))
			if report.OK || redirectCalls != 0 {
				t.Fatalf("report=%+v redirected=%d", report, redirectCalls)
			}
		})
	}
}

func TestPreflightInvalidIdentityNeverSendsCredentials(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
	defer upstream.Close()
	cfg := preflightTestConfig(upstream.URL)
	cfg.PreflightMainUserID = "../users"
	if runPreflight(context.Background(), cfg).OK || calls != 0 {
		t.Fatal("invalid identity reached main")
	}
}

func TestPreflightAdminFailureDoesNotHideWorkingSatellite(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/admin/") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"balance":0.0001}`))
	}))
	defer upstream.Close()
	report := runPreflight(context.Background(), preflightTestConfig(upstream.URL))
	if report.OK {
		t.Fatal("admin failure must fail full preflight")
	}
	checks := map[string]preflightCheck{}
	for _, check := range report.Checks {
		checks[check.Name] = check
	}
	if checks["admin_user_read"].HTTPStatus != 401 || !checks["satellite_balance_read"].OK {
		t.Fatalf("independent results missing: %+v", report)
	}
}

func TestPreflightCancelledContextReturnsTransportFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	report := runPreflight(ctx, preflightTestConfig("http://127.0.0.1:1"))
	if report.OK {
		t.Fatal("cancelled probes passed")
	}
	for _, check := range report.Checks {
		if strings.HasSuffix(check.Name, "_read") && check.Code != "transport_error" {
			t.Fatalf("unexpected cancelled result: %+v", check)
		}
	}
}

func TestPreflightCLIExitsBeforeCreatingDatabase(t *testing.T) {
	if os.Getenv("AGENTAPI_PREFLIGHT_TEST_CHILD") == "1" {
		os.Args = []string{"agentapi", "--preflight"}
		main()
		os.Exit(0)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(t.TempDir(), "must-not-exist.db")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, "-test.run=^TestPreflightCLIExitsBeforeCreatingDatabase$")
	// Do not inherit deployment credentials into the diagnostic child.
	cmd.Env = []string{"AGENTAPI_PREFLIGHT_TEST_CHILD=1", "AGENTAPI_DATABASE_PATH=" + dbPath,
		"SYSTEMROOT=" + os.Getenv("SYSTEMROOT"), "TEMP=" + os.TempDir(), "TMP=" + os.TempDir(),
		"SESSION_SECRET=" + strings.Repeat("t", 32), "SUB2API_SSO_SECRET=" + strings.Repeat("s", 32),
		"SUB2API_APP_CREDENTIAL=app-secret", "AGENTAPI_SHARED_HOSTS=agent.example.test"}
	output, err := cmd.Output()
	if ctx.Err() != nil {
		t.Fatal("preflight started a server or hung")
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok || exitErr.ExitCode() != 1 {
		t.Fatalf("expected configuration failure exit 1: %v", err)
	}
	var report preflightReport
	if json.Unmarshal(output, &report) != nil || report.OK || len(report.Checks) != 7 {
		t.Fatalf("invalid CLI report: %s", output)
	}
	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Fatalf("preflight touched database: %v", err)
	}
}
