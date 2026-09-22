package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
var domainPattern = regexp.MustCompile(`^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
var imagePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/:@-]{0,255}$`)

type request struct {
	IdempotencyKey string `json:"idempotency_key"`
	Slug           string `json:"slug"`
	Domain         string `json:"domain"`
	DisplayName    string `json:"display_name"`
	OwnerMainUser  string `json:"owner_main_user_id"`
	MainURL        string `json:"main_url"`
	Image          string `json:"image"`
}

type result struct {
	AgentID     string `json:"agent_id"`
	Slug        string `json:"slug"`
	Domain      string `json:"domain"`
	Status      string `json:"status"`
	BundlePath  string `json:"bundle_path"`
	Fingerprint string `json:"fingerprint"`
}

type persisted struct {
	Request request `json:"request"`
	Result  result  `json:"result"`
}

func main() {
	var req request
	var stateDir string
	flag.StringVar(&stateDir, "state-dir", "./provisioning", "directory holding idempotency state and generated bundles")
	flag.StringVar(&req.IdempotencyKey, "idempotency-key", "", "required one-click provisioning request key")
	flag.StringVar(&req.Slug, "slug", "", "agent slug")
	flag.StringVar(&req.Domain, "domain", "", "public platform subdomain")
	flag.StringVar(&req.DisplayName, "display-name", "", "agent display name")
	flag.StringVar(&req.OwnerMainUser, "owner-main-user-id", "", "Sub2API owner user id")
	flag.StringVar(&req.MainURL, "main-url", "", "Sub2API public origin")
	flag.StringVar(&req.Image, "image", "agentapi:latest", "AgentAPI image reference")
	flag.Parse()
	res, err := provision(stateDir, req)
	if err != nil {
		fmt.Fprintln(os.Stderr, "provision failed:", err)
		os.Exit(1)
	}
	_ = json.NewEncoder(os.Stdout).Encode(res)
}

func provision(stateDir string, req request) (result, error) {
	req = normalize(req)
	if err := validate(req); err != nil {
		return result{}, err
	}
	absState, err := filepath.Abs(stateDir)
	if err != nil {
		return result{}, err
	}
	for _, dir := range []string{absState, filepath.Join(absState, "agents"), filepath.Join(absState, "requests"), filepath.Join(absState, "domains")} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return result{}, err
		}
	}
	release, err := acquireLock(absState, 5*time.Second)
	if err != nil {
		return result{}, err
	}
	defer release()
	fingerprint, err := fingerprint(req)
	if err != nil {
		return result{}, err
	}
	requestPath := filepath.Join(absState, "requests", hash(req.IdempotencyKey)+".json")
	if existing, readErr := readPersisted(requestPath); readErr == nil {
		if existing.Result.Fingerprint != fingerprint {
			return result{}, fmt.Errorf("idempotency key belongs to a different provisioning request")
		}
		return existing.Result, nil
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return result{}, readErr
	}
	agentDir := filepath.Join(absState, "agents", req.Slug)
	if existing, readErr := readPersisted(filepath.Join(agentDir, "provision.json")); readErr == nil {
		if existing.Result.Fingerprint != fingerprint {
			return result{}, fmt.Errorf("slug is already assigned")
		}
		if err := writeJSONAtomic(requestPath, existing, 0o640); err != nil {
			return result{}, err
		}
		return existing.Result, nil
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return result{}, readErr
	}
	domainClaim := filepath.Join(absState, "domains", hash(req.Domain)+".json")
	domainClaimExists := false
	if claim, readErr := readPersisted(domainClaim); readErr == nil {
		domainClaimExists = true
		if claim.Request.Slug != req.Slug {
			return result{}, fmt.Errorf("domain is already assigned")
		}
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return result{}, readErr
	}
	tmp, err := os.MkdirTemp(filepath.Join(absState, "agents"), ".provision-"+req.Slug+"-")
	if err != nil {
		return result{}, err
	}
	defer os.RemoveAll(tmp)
	secret, err := randomSecret()
	if err != nil {
		return result{}, err
	}
	res := result{AgentID: "agent-" + req.Slug, Slug: req.Slug, Domain: req.Domain, Status: "generated", BundlePath: agentDir, Fingerprint: fingerprint}
	state := persisted{Request: req, Result: res}
	files := map[string]struct {
		body string
		mode os.FileMode
	}{
		".env":         {renderEnv(req, res), 0o640},
		"compose.yaml": {renderCompose(req), 0o640},
		"nginx.conf":   {renderNginx(req), 0o640},
		filepath.Join("secrets", "session_secret"): {secret + "\n", 0o600},
	}
	for name, file := range files {
		path := filepath.Join(tmp, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return result{}, err
		}
		if err := os.WriteFile(path, []byte(file.body), file.mode); err != nil {
			return result{}, err
		}
	}
	if err := writeJSONAtomic(filepath.Join(tmp, "provision.json"), state, 0o640); err != nil {
		return result{}, err
	}
	if !domainClaimExists {
		if err := writeJSONAtomic(domainClaim, state, 0o640); err != nil {
			return result{}, err
		}
	}
	if err := os.Rename(tmp, agentDir); err != nil {
		if !domainClaimExists {
			_ = os.Remove(domainClaim)
		}
		if os.IsExist(err) {
			return result{}, fmt.Errorf("slug is already assigned")
		}
		return result{}, err
	}
	if err := writeJSONAtomic(requestPath, state, 0o640); err != nil {
		return result{}, err
	}
	return res, nil
}

func normalize(req request) request {
	req.IdempotencyKey = strings.TrimSpace(req.IdempotencyKey)
	req.Slug = strings.ToLower(strings.TrimSpace(req.Slug))
	req.Domain = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(req.Domain)), ".")
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	req.OwnerMainUser = strings.TrimSpace(req.OwnerMainUser)
	req.MainURL = strings.TrimRight(strings.TrimSpace(req.MainURL), "/")
	req.Image = strings.TrimSpace(req.Image)
	return req
}

func validate(req request) error {
	if len(req.IdempotencyKey) < 8 || len(req.IdempotencyKey) > 200 {
		return fmt.Errorf("idempotency key must contain 8..200 characters")
	}
	if !slugPattern.MatchString(req.Slug) {
		return fmt.Errorf("invalid slug")
	}
	if !domainPattern.MatchString(req.Domain) {
		return fmt.Errorf("invalid domain")
	}
	if req.DisplayName == "" || len(req.DisplayName) > 100 || strings.ContainsAny(req.DisplayName, "\r\n") {
		return fmt.Errorf("invalid display name")
	}
	if req.OwnerMainUser == "" || len(req.OwnerMainUser) > 200 || strings.ContainsAny(req.OwnerMainUser, "\r\n") {
		return fmt.Errorf("invalid owner main user id")
	}
	mainURL, err := url.Parse(req.MainURL)
	if err != nil || mainURL.Host == "" || mainURL.User != nil || mainURL.RawQuery != "" || mainURL.Fragment != "" ||
		(mainURL.Scheme != "https" && !(mainURL.Scheme == "http" && (mainURL.Hostname() == "localhost" || mainURL.Hostname() == "127.0.0.1" || mainURL.Hostname() == "::1"))) {
		return fmt.Errorf("main URL must use HTTPS")
	}
	if !imagePattern.MatchString(req.Image) {
		return fmt.Errorf("invalid image reference")
	}
	return nil
}

func fingerprint(req request) (string, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
func hash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
func randomSecret() (string, error) {
	raw := make([]byte, 48)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func acquireLock(stateDir string, timeout time.Duration) (func(), error) {
	lock := filepath.Join(stateDir, ".provision.lock")
	deadline := time.Now().Add(timeout)
	for {
		if err := os.Mkdir(lock, 0o700); err == nil {
			return func() { _ = os.Remove(lock) }, nil
		} else if !os.IsExist(err) {
			return nil, err
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("provisioning state is locked")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func readPersisted(path string) (persisted, error) {
	var value persisted
	data, err := os.ReadFile(path)
	if err != nil {
		return value, err
	}
	err = json.Unmarshal(data, &value)
	return value, err
}
func writeJSONAtomic(path string, value any, mode os.FileMode) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func renderEnv(req request, res result) string {
	q := strconv.Quote
	return fmt.Sprintf("AGENT_ID=%s\nAGENT_DOMAIN=%s\nAGENT_NAME=%s\nAGENT_SITE_NAME=%s\nAGENT_OWNER_MAIN_USER_ID=%s\nMAIN_API_URL=%s\nMAIN_MODEL_URL=%s\nAGENTAPI_DATABASE_PATH=/app/data/agentapi.db\nAGENT_ENABLED=true\nAGENT_BILLING_MODE=owner_upstream\nCOOKIE_SECURE=true\nSUB2API_SATELLITE=agentapi\nAGENT_SETTLEMENT_RECONCILE_INTERVAL=5m\nAGENT_VIDEO_TASK_RECONCILE_AGE=30m\n", q(res.AgentID), q(req.Domain), q(req.DisplayName), q(req.DisplayName), q(req.OwnerMainUser), q(req.MainURL), q(req.MainURL))
}

func renderCompose(req request) string {
	return fmt.Sprintf("services:\n  agentapi:\n    image: %s\n    container_name: agentapi-%s\n    restart: unless-stopped\n    env_file: .env\n    read_only: true\n    tmpfs:\n      - /tmp:rw,noexec,nosuid,size=16m\n    volumes:\n      - agentapi_data:/app/data\n    secrets:\n      - session_secret\n      - sub2api_admin_key\n      - sub2api_app_credential\n      - sub2api_sso_secret\n    environment:\n      SESSION_SECRET_FILE: /run/secrets/session_secret\n      SUB2API_ADMIN_KEY_FILE: /run/secrets/sub2api_admin_key\n      SUB2API_APP_CREDENTIAL_FILE: /run/secrets/sub2api_app_credential\n      SUB2API_SSO_SECRET_FILE: /run/secrets/sub2api_sso_secret\n    networks: [agentapi-edge]\nvolumes:\n  agentapi_data:\nsecrets:\n  session_secret:\n    file: ./secrets/session_secret\n  sub2api_admin_key:\n    file: ${SUB2API_ADMIN_KEY_FILE:?set SUB2API_ADMIN_KEY_FILE}\n  sub2api_app_credential:\n    file: ${SUB2API_APP_CREDENTIAL_FILE:?set SUB2API_APP_CREDENTIAL_FILE}\n  sub2api_sso_secret:\n    file: ${SUB2API_SSO_SECRET_FILE:?set SUB2API_SSO_SECRET_FILE}\nnetworks:\n  agentapi-edge:\n    external: true\n", req.Image, req.Slug)
}

func renderNginx(req request) string {
	return fmt.Sprintf("server {\n    listen 443 ssl http2;\n    server_name %s;\n    include /etc/nginx/snippets/agentapi-wildcard-tls.conf;\n\n    location / {\n        proxy_set_header Host $host;\n        proxy_set_header X-Forwarded-Proto $scheme;\n        proxy_set_header X-Request-ID $request_id;\n        proxy_pass http://agentapi-%s:8080;\n    }\n}\n", req.Domain, req.Slug)
}
