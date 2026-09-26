package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

func main() {
	var agentID string
	flag.StringVar(&agentID, "agent-id", "", "optional single agent id to provision and exit")
	flag.Parse()

	cfg, err := loadConfig()
	if err != nil {
		slog.Error("provisioning worker configuration is invalid", "error", err)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	api := newControlAPI(cfg)
	backend := newHostBackend(cfg)
	worker := newWorker(cfg, api, backend)
	if agentID != "" {
		if err := worker.process(ctx, strings.TrimSpace(agentID)); err != nil {
			slog.Error("agent provisioning failed", "agent_id", agentID, "error", err)
			os.Exit(1)
		}
		return
	}
	if err := worker.run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("provisioning worker stopped", "error", err)
		os.Exit(1)
	}
}

func loadConfig() (config, error) {
	mainURL := strings.TrimRight(strings.TrimSpace(os.Getenv("MAIN_API_URL")), "/")
	if mainURL == "" {
		mainURL = strings.TrimRight(strings.TrimSpace(os.Getenv("LINK")), "/")
	}
	if mainURL == "" {
		return config{}, fmt.Errorf("MAIN_API_URL or LINK is required")
	}
	if !strings.Contains(mainURL, "://") {
		// The workspace's documented local LINK omits the scheme; accept that
		// shorthand only for loopback development endpoints.
		localCandidate, parseErr := url.Parse("http://" + mainURL)
		if parseErr == nil && localDevelopmentHost(localCandidate.Hostname()) {
			mainURL = "http://" + mainURL
		}
	}
	mainURL = strings.TrimSuffix(mainURL, "/api/v1")
	parsedMainURL, err := url.Parse(mainURL)
	if err != nil || parsedMainURL.Host == "" || parsedMainURL.User != nil || parsedMainURL.Path != "" || parsedMainURL.RawQuery != "" || parsedMainURL.Fragment != "" {
		return config{}, fmt.Errorf("MAIN_API_URL must be an origin")
	}
	localMain := parsedMainURL.Scheme == "http" && localDevelopmentHost(parsedMainURL.Hostname())
	if parsedMainURL.Scheme != "https" && !localMain {
		return config{}, fmt.Errorf("main API must use HTTPS (HTTP is allowed only for local development)")
	}

	workerCredentialFile, err := requiredSecretFile("AGENT_PROVISIONING_WORKER_CREDENTIAL")
	if err != nil {
		return config{}, err
	}
	workerCredential, err := os.ReadFile(workerCredentialFile)
	if err != nil {
		return config{}, fmt.Errorf("AGENT_PROVISIONING_WORKER_CREDENTIAL_FILE could not be read")
	}
	workerCredentialValue := strings.TrimSpace(string(workerCredential))
	if len(workerCredentialValue) < 32 {
		return config{}, fmt.Errorf("AGENT_PROVISIONING_WORKER_CREDENTIAL_FILE must contain at least 32 characters")
	}
	ssoSecretFile, err := requiredSecretFile("SUB2API_SSO_SECRET")
	if err != nil {
		return config{}, err
	}
	nginxDir := strings.TrimSpace(os.Getenv("AGENTAPI_NGINX_CONFIG_DIR"))
	nginxContainer := strings.TrimSpace(os.Getenv("AGENTAPI_NGINX_CONTAINER"))
	if nginxDir == "" || nginxContainer == "" {
		return config{}, fmt.Errorf("AGENTAPI_NGINX_CONFIG_DIR and AGENTAPI_NGINX_CONTAINER are required")
	}
	if _, err := os.Stat(nginxDir); err != nil {
		return config{}, fmt.Errorf("NGINX config directory is unavailable")
	}
	if strings.ContainsAny(nginxContainer, "\r\n\x00") {
		return config{}, fmt.Errorf("invalid nginx container name")
	}
	if !containerNamePattern.MatchString(nginxContainer) {
		return config{}, fmt.Errorf("invalid nginx container name")
	}
	nginxDir, err = filepath.Abs(nginxDir)
	if err != nil {
		return config{}, fmt.Errorf("invalid NGINX config directory")
	}
	edgeIPs := splitNonEmpty(os.Getenv("AGENTAPI_EDGE_IPS"))
	if len(edgeIPs) == 0 {
		return config{}, fmt.Errorf("AGENTAPI_EDGE_IPS must contain the expected public edge address")
	}
	for _, edgeIP := range edgeIPs {
		if net.ParseIP(edgeIP) == nil {
			return config{}, fmt.Errorf("AGENTAPI_EDGE_IPS contains an invalid address")
		}
	}
	edgeNetwork := firstNonEmpty(os.Getenv("AGENTAPI_EDGE_NETWORK"), "agentapi-edge")
	if !containerNamePattern.MatchString(edgeNetwork) {
		return config{}, fmt.Errorf("invalid Docker edge network name")
	}
	stateDir, err := filepath.Abs(firstNonEmpty(os.Getenv("AGENTAPI_PROVISIONING_STATE_DIR"), "/var/lib/agentapi-provisioning"))
	if err != nil {
		return config{}, fmt.Errorf("invalid provisioning state directory")
	}
	return config{
		MainURL:                mainURL,
		ProvisioningCredential: workerCredentialValue,
		SSOSecretFile:          ssoSecretFile,
		StateDir:               stateDir,
		ProvisionerPath:        firstNonEmpty(os.Getenv("AGENTAPI_PROVISION_BIN"), "/usr/local/bin/agentapi-provision"),
		Image:                  firstNonEmpty(os.Getenv("AGENTAPI_IMAGE"), "cc2cx/agentapi:latest"),
		DockerBin:              firstNonEmpty(os.Getenv("AGENTAPI_DOCKER_BIN"), "docker"),
		EdgeNetwork:            edgeNetwork,
		NginxContainer:         nginxContainer,
		NginxConfigDir:         nginxDir,
		ExpectedEdgeIPs:        edgeIPs,
		RequestTimeout:         envDuration("AGENTAPI_PROVISIONING_REQUEST_TIMEOUT", 15*time.Second),
		ReadinessTimeout:       envDuration("AGENTAPI_PROVISIONING_READINESS_TIMEOUT", 2*time.Minute),
		PollInterval:           envDuration("AGENTAPI_PROVISIONING_POLL_INTERVAL", 30*time.Second),
		TLSCheckTimeout:        envDuration("AGENTAPI_PROVISIONING_TLS_TIMEOUT", 15*time.Second),
		ProvisionCommandLimit:  envDuration("AGENTAPI_PROVISIONING_COMMAND_TIMEOUT", 5*time.Minute),
	}, nil
}

func localDevelopmentHost(host string) bool {
	return strings.EqualFold(host, "localhost") || host == "127.0.0.1" || host == "::1"
}

var containerNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

func requiredSecretFile(name string) (string, error) {
	path := strings.TrimSpace(os.Getenv(name + "_FILE"))
	if path == "" {
		return "", fmt.Errorf("%s_FILE is required; the worker does not copy secret contents", name)
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("%s_FILE must use an absolute path", name)
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		return "", fmt.Errorf("%s_FILE must name an available regular file", name)
	}
	return path, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func splitNonEmpty(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\n' })
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			result = append(result, part)
		}
	}
	return result
}

func envDuration(name string, fallback time.Duration) time.Duration {
	if raw := strings.TrimSpace(os.Getenv(name)); raw != "" {
		if value, err := time.ParseDuration(raw); err == nil && value > 0 {
			return value
		}
	}
	return fallback
}
