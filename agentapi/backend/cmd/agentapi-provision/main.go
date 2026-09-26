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
	"runtime"
	"strconv"
	"strings"
	"time"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
var domainPattern = regexp.MustCompile(`^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
var imagePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/:@-]{0,255}$`)
var agentIDPattern = regexp.MustCompile(`^agt_[0-9a-f]{32}$`)

type request struct {
	IdempotencyKey string `json:"idempotency_key"`
	AgentID        string `json:"agent_id"`
	Slug           string `json:"slug"`
	Domain         string `json:"domain"`
	DisplayName    string `json:"display_name"`
	OwnerMainUser  string `json:"owner_main_user_id"`
	MainURL        string `json:"main_url"`
	Image          string `json:"image"`
}

type result struct {
	AgentID          string `json:"agent_id"`
	Slug             string `json:"slug"`
	Domain           string `json:"domain"`
	Status           string `json:"status"`
	BundlePath       string `json:"bundle_path"`
	RuntimeSecretDir string `json:"runtime_secret_dir"`
	Fingerprint      string `json:"fingerprint"`
	ControlTokenHash string `json:"control_token_hash"`
	ModelTokenHash   string `json:"model_token_hash"`
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
	flag.StringVar(&req.AgentID, "agent-id", "", "required agent_id returned by the Sub2API provisioning API")
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
		existing, err = ensureRuntimeSecretsOutsideBundle(absState, existing)
		if err != nil {
			return result{}, err
		}
		if err := writeJSONAtomic(requestPath, existing, 0o640); err != nil {
			return result{}, err
		}
		return existing.Result, nil
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return result{}, readErr
	}
	agentDir := filepath.Join(absState, "agents", req.Slug)
	runtimeSecretRoot := filepath.Join(absState, "runtime-secrets")
	runtimeSecretDir := filepath.Join(runtimeSecretRoot, req.Slug)
	if existing, readErr := readPersisted(filepath.Join(agentDir, "provision.json")); readErr == nil {
		if existing.Result.Fingerprint != fingerprint {
			return result{}, fmt.Errorf("slug is already assigned")
		}
		existing, err = ensureRuntimeSecretsOutsideBundle(absState, existing)
		if err != nil {
			return result{}, err
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
	if err := os.MkdirAll(runtimeSecretRoot, 0o700); err != nil {
		return result{}, err
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(runtimeSecretRoot)
		if err != nil || !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
			return result{}, fmt.Errorf("runtime secret directory must be private to the worker account")
		}
	}
	if _, err := os.Lstat(runtimeSecretDir); err == nil {
		return result{}, fmt.Errorf("runtime secret directory is already assigned")
	} else if !errors.Is(err, os.ErrNotExist) {
		return result{}, err
	}
	secretTmp, err := os.MkdirTemp(runtimeSecretRoot, ".provision-"+req.Slug+"-")
	if err != nil {
		return result{}, err
	}
	defer os.RemoveAll(secretTmp)
	secret, err := randomSecret()
	if err != nil {
		return result{}, err
	}
	controlCredential, controlHash, err := randomRuntimeCredential("agt_ctl_")
	if err != nil {
		return result{}, err
	}
	modelCredential, modelHash, err := randomRuntimeCredential("agt_model_")
	if err != nil {
		return result{}, err
	}
	res := result{
		AgentID: req.AgentID, Slug: req.Slug, Domain: req.Domain, Status: "generated",
		BundlePath: agentDir, RuntimeSecretDir: runtimeSecretDir, Fingerprint: fingerprint,
		ControlTokenHash: controlHash, ModelTokenHash: modelHash,
	}
	state := persisted{Request: req, Result: res}
	files := map[string]struct {
		body string
		mode os.FileMode
	}{
		".env":         {renderEnv(req, res), 0o640},
		"compose.yaml": {renderCompose(req, runtimeSecretDir), 0o640},
		"nginx.conf":   {renderNginx(req), 0o640},
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
	secretFiles := map[string]string{
		"session_secret":           secret + "\n",
		"agent_control_credential": controlCredential + "\n",
		"agent_model_credential":   modelCredential + "\n",
	}
	for name, body := range secretFiles {
		if err := os.WriteFile(filepath.Join(secretTmp, name), []byte(body), 0o600); err != nil {
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
	if err := os.Rename(secretTmp, runtimeSecretDir); err != nil {
		if !domainClaimExists {
			_ = os.Remove(domainClaim)
		}
		return result{}, err
	}
	if err := os.Rename(tmp, agentDir); err != nil {
		_ = os.RemoveAll(runtimeSecretDir)
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
	req.AgentID = strings.TrimSpace(req.AgentID)
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
	if !agentIDPattern.MatchString(req.AgentID) {
		return fmt.Errorf("agent id must be the agt_<32 lowercase hex> id returned by Sub2API")
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

func randomRuntimeCredential(prefix string) (string, string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	credential := prefix + base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(credential))
	return credential, hex.EncodeToString(sum[:]), nil
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
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func ensureRuntimeSecretsOutsideBundle(stateDir string, state persisted) (persisted, error) {
	if state.Result.BundlePath != filepath.Join(stateDir, "agents", state.Request.Slug) {
		return persisted{}, fmt.Errorf("existing provisioning bundle path is invalid")
	}
	expectedSecretDir := filepath.Join(stateDir, "runtime-secrets", state.Request.Slug)
	if err := os.MkdirAll(filepath.Dir(expectedSecretDir), 0o700); err != nil {
		return persisted{}, err
	}
	if runtime.GOOS != "windows" {
		rootInfo, err := os.Stat(filepath.Dir(expectedSecretDir))
		if err != nil || !rootInfo.IsDir() || rootInfo.Mode().Perm()&0o077 != 0 {
			return persisted{}, fmt.Errorf("runtime secret root permissions are too broad")
		}
	}
	if state.Result.RuntimeSecretDir != "" && state.Result.RuntimeSecretDir != expectedSecretDir {
		return persisted{}, fmt.Errorf("existing runtime secret path is invalid")
	}
	legacySecretDir := filepath.Join(state.Result.BundlePath, "secrets")
	if state.Result.RuntimeSecretDir == "" {
		if err := os.MkdirAll(filepath.Dir(expectedSecretDir), 0o700); err != nil {
			return persisted{}, err
		}
		if _, err := os.Lstat(expectedSecretDir); err == nil {
			return persisted{}, fmt.Errorf("runtime secrets exist without matching provisioning metadata")
		} else if !errors.Is(err, os.ErrNotExist) {
			return persisted{}, err
		}
		if err := os.Chmod(legacySecretDir, 0o700); err != nil {
			return persisted{}, err
		}
		if err := validateRuntimeSecretFiles(legacySecretDir, state.Result); err != nil {
			return persisted{}, fmt.Errorf("legacy runtime secret files are unavailable: %w", err)
		}
		if err := os.Rename(legacySecretDir, expectedSecretDir); err != nil {
			return persisted{}, err
		}
		state.Result.RuntimeSecretDir = expectedSecretDir
		composePath := filepath.Join(state.Result.BundlePath, "compose.yaml")
		if err := writeFileAtomic(composePath, []byte(renderCompose(state.Request, expectedSecretDir)), 0o640); err != nil {
			return persisted{}, err
		}
		if err := writeJSONAtomic(filepath.Join(state.Result.BundlePath, "provision.json"), state, 0o640); err != nil {
			return persisted{}, err
		}
	}
	if err := validateRuntimeSecretFiles(state.Result.RuntimeSecretDir, state.Result); err != nil {
		return persisted{}, fmt.Errorf("runtime secret files are unavailable or unsafe: %w", err)
	}
	return state, nil
}

func validateRuntimeSecretFiles(dir string, result result) error {
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("secret directory is not a real directory")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("secret directory permissions are too broad")
	}
	for _, name := range []string{"session_secret", "agent_control_credential", "agent_model_credential"} {
		fileInfo, statErr := os.Lstat(filepath.Join(dir, name))
		if statErr != nil || !fileInfo.Mode().IsRegular() || fileInfo.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s is not a regular secret file", name)
		}
		if runtime.GOOS != "windows" && fileInfo.Mode().Perm()&0o077 != 0 {
			return fmt.Errorf("%s permissions are too broad", name)
		}
		data, readErr := os.ReadFile(filepath.Join(dir, name))
		if readErr != nil || strings.TrimSpace(string(data)) == "" {
			return fmt.Errorf("%s is empty", name)
		}
		if name == "agent_control_credential" {
			sum := sha256.Sum256([]byte(strings.TrimSpace(string(data))))
			if hex.EncodeToString(sum[:]) != result.ControlTokenHash {
				return fmt.Errorf("control credential hash does not match")
			}
		}
		if name == "agent_model_credential" {
			sum := sha256.Sum256([]byte(strings.TrimSpace(string(data))))
			if hex.EncodeToString(sum[:]) != result.ModelTokenHash {
				return fmt.Errorf("model credential hash does not match")
			}
		}
	}
	return nil
}

func renderEnv(req request, res result) string {
	q := strconv.Quote
	return fmt.Sprintf("AGENT_ID=%s\nAGENT_DOMAIN=%s\nAGENT_NAME=%s\nAGENT_SITE_NAME=%s\nAGENT_OWNER_MAIN_USER_ID=%s\nMAIN_API_URL=%s\nMAIN_MODEL_URL=%s\nAGENTAPI_DATABASE_PATH=/app/data/agentapi.db\nAGENT_ENABLED=true\nAGENT_PROVISIONING_CONTROL_ENABLED=true\nAGENT_PROVISIONING_CONTROL_STALE_AFTER=90s\nAGENT_BILLING_MODE=owner_upstream\nCOOKIE_SECURE=true\nSUB2API_SATELLITE=agentapi\nAGENT_SETTLEMENT_RECONCILE_INTERVAL=5m\nAGENT_VIDEO_TASK_RECONCILE_AGE=30m\n", q(res.AgentID), q(req.Domain), q(req.DisplayName), q(req.DisplayName), q(req.OwnerMainUser), q(req.MainURL), q(req.MainURL))
}

func renderCompose(req request, secretDir string) string {
	secretDir = filepath.ToSlash(secretDir)
	sessionSecretPath := strconv.Quote(strings.TrimRight(secretDir, "/") + "/session_secret")
	controlCredentialPath := strconv.Quote(strings.TrimRight(secretDir, "/") + "/agent_control_credential")
	modelCredentialPath := strconv.Quote(strings.TrimRight(secretDir, "/") + "/agent_model_credential")
	return fmt.Sprintf(`services:
  agentapi:
    image: %s
    container_name: agentapi-%s
    restart: unless-stopped
    env_file: .env
    read_only: true
    tmpfs:
      - /tmp:rw,noexec,nosuid,size=16m
    volumes:
      - agentapi_data:/app/data
    secrets:
      - session_secret
      - agent_control_credential
      - agent_model_credential
      - sub2api_sso_secret
    environment:
      SESSION_SECRET_FILE: /run/secrets/session_secret
      AGENT_RUNTIME_CONTROL_CREDENTIAL_FILE: /run/secrets/agent_control_credential
      SUB2API_APP_CREDENTIAL_FILE: /run/secrets/agent_model_credential
      SUB2API_SSO_SECRET_FILE: /run/secrets/sub2api_sso_secret
    networks: [agentapi-edge]
volumes:
  agentapi_data:
secrets:
  session_secret:
    file: %s
  agent_control_credential:
    file: %s
  agent_model_credential:
    file: %s
  sub2api_sso_secret:
    file: ${SUB2API_SSO_SECRET_FILE:?set SUB2API_SSO_SECRET_FILE}
networks:
  agentapi-edge:
    external: true
`, req.Image, req.Slug, sessionSecretPath, controlCredentialPath, modelCredentialPath)
}

func renderNginx(req request) string {
	return fmt.Sprintf(`# managed-by-agentapi-provision-worker agent_id=%s
server {
    listen 443 ssl http2;
    server_name %s;
    include /etc/nginx/snippets/agentapi-wildcard-tls.conf;
    client_max_body_size 32m;

    location / {
        proxy_http_version 1.1;
        proxy_set_header Connection "";
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Request-ID $request_id;
        proxy_request_buffering off;
        proxy_buffering off;
        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
        proxy_pass http://agentapi-%s:8080;
    }
}
`, req.AgentID, req.Domain, req.Slug)
}
