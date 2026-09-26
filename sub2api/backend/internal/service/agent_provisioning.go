package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/google/uuid"
)

var (
	ErrAgentProvisioningInvalid         = infraerrors.BadRequest("AGENT_PROVISIONING_INVALID", "invalid agent provisioning request")
	ErrAgentProvisioningNotFound        = infraerrors.NotFound("AGENT_NOT_FOUND", "agent not found")
	ErrAgentProvisioningOwnerNotFound   = infraerrors.NotFound("AGENT_OWNER_NOT_FOUND", "owner user not found")
	ErrAgentProvisioningIdempotency     = infraerrors.Conflict("AGENT_DUPLICATE_REQUEST", "idempotency key was already used for a different request")
	ErrAgentProvisioningSlugTaken       = infraerrors.Conflict("AGENT_SLUG_TAKEN", "requested slug is already in use")
	ErrAgentProvisioningStateConflict   = infraerrors.Conflict("AGENT_STATE_CONFLICT", "agent state does not allow this operation")
	ErrAgentProvisioningConfirmRequired = infraerrors.BadRequest("AGENT_CONFIRMATION_REQUIRED", "confirmation must match the agent id")
	ErrAgentProvisioningLeaseHeld       = infraerrors.Conflict("AGENT_PROVISIONING_LEASE_HELD", "agent is currently claimed by another provisioning worker")
	ErrAgentProvisioningLeaseLost       = infraerrors.Conflict("AGENT_PROVISIONING_LEASE_LOST", "provisioning worker lease is missing, expired, or no longer valid")
	ErrAgentRuntimeCredentialInvalid    = infraerrors.Unauthorized("AGENT_RUNTIME_CREDENTIAL_INVALID", "agent runtime credential is invalid or revoked")
	ErrAgentRuntimeScopeMismatch        = infraerrors.Forbidden("AGENT_RUNTIME_SCOPE_MISMATCH", "agent runtime credential is not allowed for this resource")
	ErrAgentRuntimeAgentInactive        = infraerrors.Forbidden("AGENT_RUNTIME_AGENT_INACTIVE", "agent runtime is not active")
	ErrAgentRuntimeUserScopeConflict    = infraerrors.Conflict("AGENT_RUNTIME_USER_SCOPE_CONFLICT", "main user is already mapped to another agent")
)

const agentProvisioningLeaseTTL = 90 * time.Second

const agentPlatformDomain = "cc2.cx"

var (
	agentSlugPattern   = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
	agentPlanPattern   = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)
	agentReservedSlugs = map[string]struct{}{
		"admin": {}, "api": {}, "auth": {}, "mail": {}, "support": {}, "www": {},
	}
)

type AgentProvisioningAgent struct {
	AgentID         string    `json:"agent_id"`
	Slug            string    `json:"slug"`
	Domain          string    `json:"domain"`
	DisplayName     string    `json:"display_name"`
	OwnerMainUserID int64     `json:"owner_main_user_id"`
	PlanID          string    `json:"plan_id"`
	BrandName       string    `json:"brand_name"`
	LogoURL         *string   `json:"logo_url,omitempty"`
	Status          string    `json:"status"`
	CurrentStep     string    `json:"current_step"`
	RecentError     string    `json:"recent_error,omitempty"`
	CanRetry        bool      `json:"can_retry"`
	DomainStatus    string    `json:"domain_status"`
	RequestID       string    `json:"request_id"`
	CreatedByUserID int64     `json:"-"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type AgentProvisioningCreateInput struct {
	RequestedSlug   string
	DisplayName     string
	OwnerMainUserID int64
	PlanID          string
	BrandName       string
	LogoURL         *string
	DomainMode      string
}

type AgentProvisioningListFilter struct {
	Page     int
	PageSize int
	Status   string
	Search   string
}

// AgentProvisioningProgress is a worker-reported, non-secret checkpoint. The
// server derives user-visible failure text from FailureCode instead of
// accepting arbitrary worker logs that could contain credentials.
type AgentProvisioningProgress struct {
	AgentID     string
	Step        string
	FailureCode string
	FailureStep string
	Retryable   bool
}

type AgentProvisioningLease struct {
	Agent     *AgentProvisioningAgent
	Token     string
	ExpiresAt time.Time
}

// AgentRuntimeCredential is the non-secret identity resolved from a
// per-instance runtime credential. Only the hash is persisted by Sub2API.
type AgentRuntimeCredential struct {
	AgentID              string
	OwnerMainUserID      int64
	Purpose              string
	AgentStatus          string
	ModelAllowlist       []string
	ModelAllowlistWasSet bool
}

// AgentProvisioningRepository is the persistence boundary for the main-site
// provisioning control plane. It intentionally does not execute deployment
// operations; infrastructure orchestration remains an external worker.
type AgentProvisioningRepository interface {
	CreateAgent(ctx context.Context, actorUserID int64, keyHash, requestHash, requestID string, agent AgentProvisioningAgent) (*AgentProvisioningAgent, bool, error)
	GetAgent(ctx context.Context, agentID string) (*AgentProvisioningAgent, error)
	ListAgents(ctx context.Context, filter AgentProvisioningListFilter) ([]AgentProvisioningAgent, int, error)
	ListenAgentUpdates(ctx context.Context) (<-chan string, error)
	ClaimAgent(ctx context.Context, agentID, tokenHash string, ttl time.Duration) (*AgentProvisioningAgent, time.Time, error)
	RenewAgentLease(ctx context.Context, agentID, tokenHash string, ttl time.Duration) (*AgentProvisioningAgent, time.Time, error)
	TransitionAgent(ctx context.Context, actorUserID int64, keyHash, requestHash, operation, agentID, requestID, leaseTokenHash string) (*AgentProvisioningAgent, bool, error)
	ReportProgress(ctx context.Context, actorUserID int64, keyHash, requestHash, requestID string, progress AgentProvisioningProgress, leaseTokenHash string) (*AgentProvisioningAgent, bool, error)
	StoreRuntimeCredentialHashes(ctx context.Context, agentID, controlHash, modelHash, leaseTokenHash string) error
	ResolveRuntimeCredential(ctx context.Context, tokenHash string) (*AgentRuntimeCredential, error)
	SetRuntimeModelAllowlist(ctx context.Context, agentID string, models []string) error
	RevokeRuntimeCredentials(ctx context.Context, agentID string) error
	MapRuntimeUser(ctx context.Context, agentID string, userID int64) error
	IsRuntimeUser(ctx context.Context, agentID string, userID int64) (bool, error)
	IsActiveRuntimeUser(ctx context.Context, agentID string, userID int64) (bool, error)
}

var agentProvisioningFailureMessages = map[string]string{
	"bundle_generation_failed": "deployment bundle generation failed",
	"network_unavailable":      "required deployment network is unavailable",
	"secrets_unavailable":      "required per-agent secret files are unavailable",
	"deployment_failed":        "agent container deployment failed",
	"dns_failed":               "agent domain did not resolve to the configured edge",
	"tls_failed":               "agent domain did not present a valid certificate",
	"readiness_failed":         "agent readiness check did not pass",
}

var agentProvisioningSteps = []string{
	"validating",
	"bundle_generated",
	"deploying",
	"domain_check",
	"tls_check",
	"readiness_check",
}

func AgentProvisioningFailureMessage(code string) (string, bool) {
	message, ok := agentProvisioningFailureMessages[code]
	return message, ok
}

func AgentProvisioningSteps() []string {
	return append([]string(nil), agentProvisioningSteps...)
}

type AgentProvisioningService struct {
	repo AgentProvisioningRepository
}

func NewAgentProvisioningService(repo AgentProvisioningRepository) *AgentProvisioningService {
	return &AgentProvisioningService{repo: repo}
}

// RegisterRuntimeCredentialHashes installs the per-Agent control and model
// credentials during a currently leased provisioning attempt. Sub2API only
// receives hashes; the worker keeps raw values in the instance Secret files.
func (s *AgentProvisioningService) RegisterRuntimeCredentialHashes(ctx context.Context, agentID, controlHash, modelHash, leaseToken string) error {
	if s == nil || s.repo == nil {
		return errors.New("agent provisioning repository is unavailable")
	}
	agentID = strings.TrimSpace(agentID)
	if !validProvisioningAgentID(agentID) {
		return ErrAgentProvisioningNotFound
	}
	controlHash, err := normalizeRuntimeCredentialHash(controlHash)
	if err != nil {
		return ErrAgentProvisioningInvalid
	}
	modelHash, err = normalizeRuntimeCredentialHash(modelHash)
	if err != nil || controlHash == modelHash {
		return ErrAgentProvisioningInvalid
	}
	leaseHash, err := hashProvisioningLeaseToken(leaseToken)
	if err != nil {
		return err
	}
	return s.repo.StoreRuntimeCredentialHashes(ctx, agentID, controlHash, modelHash, leaseHash)
}

// LookupRuntimeCredential resolves a presented per-Agent secret without ever
// persisting or returning its raw value. Purpose is bound to the token prefix
// as well as the database row so a control credential cannot be used as a
// model bearer (or vice versa).
func (s *AgentProvisioningService) LookupRuntimeCredential(ctx context.Context, rawCredential string) (*AgentRuntimeCredential, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("agent provisioning repository is unavailable")
	}
	rawCredential = strings.TrimSpace(rawCredential)
	purpose := ""
	switch {
	case strings.HasPrefix(rawCredential, "agt_ctl_"):
		purpose = "control"
	case strings.HasPrefix(rawCredential, "agt_model_"):
		purpose = "model"
	default:
		return nil, ErrAgentRuntimeCredentialInvalid
	}
	if len(rawCredential) < 40 || len(rawCredential) > 256 || strings.ContainsAny(rawCredential, "\r\n") {
		return nil, ErrAgentRuntimeCredentialInvalid
	}
	sum := sha256.Sum256([]byte(rawCredential))
	credential, err := s.repo.ResolveRuntimeCredential(ctx, hex.EncodeToString(sum[:]))
	if err != nil {
		return nil, err
	}
	if credential == nil || credential.Purpose != purpose {
		return nil, ErrAgentRuntimeCredentialInvalid
	}
	if credential.AgentStatus != "active" {
		return nil, ErrAgentRuntimeAgentInactive
	}
	return credential, nil
}

// UpdateRuntimeModelAllowlist changes only the authenticated Agent's model
// credential scope. The gateway applies this server-side even if a request
// bypasses the AgentAPI application's own model-policy check.
func (s *AgentProvisioningService) UpdateRuntimeModelAllowlist(ctx context.Context, agentID string, requested []string) ([]string, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("agent provisioning repository is unavailable")
	}
	agentID = strings.TrimSpace(agentID)
	if !validProvisioningAgentID(agentID) {
		return nil, ErrAgentProvisioningNotFound
	}
	models, err := normalizeRuntimeModelAllowlist(requested)
	if err != nil {
		return nil, ErrAgentProvisioningInvalid
	}
	if err := s.repo.SetRuntimeModelAllowlist(ctx, agentID, models); err != nil {
		return nil, err
	}
	return models, nil
}

func normalizeRuntimeModelAllowlist(requested []string) ([]string, error) {
	public := make([]string, 0, len(domain.SatelliteTextModels)+len(domain.SatelliteImageModels)+len(domain.SatelliteVideoModels))
	public = append(public, domain.SatelliteTextModels...)
	public = append(public, domain.SatelliteImageModels...)
	public = append(public, domain.SatelliteVideoModels...)
	if len(requested) > len(public) {
		return nil, ErrAgentProvisioningInvalid
	}
	known := make(map[string]struct{}, len(public))
	for _, name := range public {
		known[name] = struct{}{}
	}
	selected := make(map[string]struct{}, len(requested))
	for _, raw := range requested {
		name := strings.TrimSpace(raw)
		if _, ok := known[name]; !ok {
			return nil, ErrAgentProvisioningInvalid
		}
		if _, duplicate := selected[name]; duplicate {
			return nil, ErrAgentProvisioningInvalid
		}
		selected[name] = struct{}{}
	}
	ordered := make([]string, 0, len(selected))
	for _, name := range public {
		if _, ok := selected[name]; ok {
			ordered = append(ordered, name)
		}
	}
	return ordered, nil
}

func (s *AgentProvisioningService) RevokeRuntimeCredentials(ctx context.Context, agentID string) error {
	if s == nil || s.repo == nil {
		return errors.New("agent provisioning repository is unavailable")
	}
	agentID = strings.TrimSpace(agentID)
	if !validProvisioningAgentID(agentID) {
		return ErrAgentProvisioningNotFound
	}
	return s.repo.RevokeRuntimeCredentials(ctx, agentID)
}

func (s *AgentProvisioningService) MapRuntimeUser(ctx context.Context, agentID string, userID int64) error {
	if s == nil || s.repo == nil {
		return errors.New("agent provisioning repository is unavailable")
	}
	agentID = strings.TrimSpace(agentID)
	if !validProvisioningAgentID(agentID) || userID <= 0 {
		return ErrAgentProvisioningInvalid
	}
	return s.repo.MapRuntimeUser(ctx, agentID, userID)
}

func (s *AgentProvisioningService) IsRuntimeUser(ctx context.Context, agentID string, userID int64) (bool, error) {
	if s == nil || s.repo == nil {
		return false, errors.New("agent provisioning repository is unavailable")
	}
	agentID = strings.TrimSpace(agentID)
	if !validProvisioningAgentID(agentID) || userID <= 0 {
		return false, ErrAgentProvisioningInvalid
	}
	return s.repo.IsRuntimeUser(ctx, agentID, userID)
}

func (s *AgentProvisioningService) IsActiveRuntimeUser(ctx context.Context, agentID string, userID int64) (bool, error) {
	if s == nil || s.repo == nil {
		return false, errors.New("agent provisioning repository is unavailable")
	}
	agentID = strings.TrimSpace(agentID)
	if !validProvisioningAgentID(agentID) || userID <= 0 {
		return false, ErrAgentProvisioningInvalid
	}
	return s.repo.IsActiveRuntimeUser(ctx, agentID, userID)
}

func normalizeRuntimeCredentialHash(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if len(value) != sha256.Size*2 {
		return "", errors.New("invalid runtime credential hash")
	}
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != sha256.Size {
		return "", errors.New("invalid runtime credential hash")
	}
	return value, nil
}

func (s *AgentProvisioningService) CreateAgent(ctx context.Context, actorUserID int64, idempotencyKey, requestID string, input AgentProvisioningCreateInput) (*AgentProvisioningAgent, bool, error) {
	if s == nil || s.repo == nil {
		return nil, false, errors.New("agent provisioning repository is unavailable")
	}
	if actorUserID <= 0 {
		return nil, false, infraerrors.Unauthorized("UNAUTHORIZED", "authenticated administrator required")
	}
	keyHash, err := hashAgentProvisioningKey(idempotencyKey)
	if err != nil {
		return nil, false, err
	}
	input, domain, err := normalizeAgentProvisioningInput(input)
	if err != nil {
		return nil, false, err
	}
	fingerprint, err := json.Marshal(input)
	if err != nil {
		return nil, false, err
	}
	requestHash := sha256.Sum256(fingerprint)
	requestID = normalizeAgentProvisioningRequestID(requestID)
	agentID := "agt_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	agent := AgentProvisioningAgent{
		AgentID: agentID, Slug: input.RequestedSlug, Domain: domain,
		DisplayName: input.DisplayName, OwnerMainUserID: input.OwnerMainUserID,
		PlanID: input.PlanID, BrandName: input.BrandName, LogoURL: input.LogoURL,
		Status: "pending", CurrentStep: "queued", CanRetry: false,
		DomainStatus: "pending", CreatedByUserID: actorUserID,
		RequestID: requestID,
	}
	return s.repo.CreateAgent(ctx, actorUserID, keyHash, hex.EncodeToString(requestHash[:]), requestID, agent)
}

func (s *AgentProvisioningService) GetAgent(ctx context.Context, agentID string) (*AgentProvisioningAgent, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("agent provisioning repository is unavailable")
	}
	agentID = strings.TrimSpace(agentID)
	if !strings.HasPrefix(agentID, "agt_") || len(agentID) != 36 {
		return nil, ErrAgentProvisioningNotFound
	}
	return s.repo.GetAgent(ctx, agentID)
}

func (s *AgentProvisioningService) ListAgents(ctx context.Context, filter AgentProvisioningListFilter) ([]AgentProvisioningAgent, int, error) {
	if s == nil || s.repo == nil {
		return nil, 0, errors.New("agent provisioning repository is unavailable")
	}
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 {
		filter.PageSize = 20
	} else if filter.PageSize > 100 {
		filter.PageSize = 100
	}
	filter.Status = strings.TrimSpace(filter.Status)
	if filter.Status != "" {
		switch filter.Status {
		case "pending", "provisioning", "active", "suspended", "failed", "revoked":
		default:
			return nil, 0, ErrAgentProvisioningInvalid
		}
	}
	filter.Search = strings.TrimSpace(filter.Search)
	if len([]rune(filter.Search)) > 100 {
		return nil, 0, ErrAgentProvisioningInvalid
	}
	return s.repo.ListAgents(ctx, filter)
}

func (s *AgentProvisioningService) ListenAgentUpdates(ctx context.Context) (<-chan string, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("agent provisioning repository is unavailable")
	}
	return s.repo.ListenAgentUpdates(ctx)
}

// ClaimAgent gives one external worker a short, renewable lease. The raw
// token is returned only to the worker; only its SHA-256 hash is persisted.
func (s *AgentProvisioningService) ClaimAgent(ctx context.Context, agentID string) (*AgentProvisioningLease, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("agent provisioning repository is unavailable")
	}
	agentID = strings.TrimSpace(agentID)
	if !validProvisioningAgentID(agentID) {
		return nil, ErrAgentProvisioningNotFound
	}
	rawToken := make([]byte, 32)
	if _, err := rand.Read(rawToken); err != nil {
		return nil, errors.New("generate provisioning lease token")
	}
	token := base64.RawURLEncoding.EncodeToString(rawToken)
	tokenHash := sha256.Sum256([]byte(token))
	agent, expiresAt, err := s.repo.ClaimAgent(ctx, agentID, hex.EncodeToString(tokenHash[:]), agentProvisioningLeaseTTL)
	if err != nil {
		return nil, err
	}
	return &AgentProvisioningLease{Agent: agent, Token: token, ExpiresAt: expiresAt}, nil
}

// RenewAgentLease extends a live worker lease without changing the Agent row,
// so heartbeats do not create noisy provisioning SSE notifications.
func (s *AgentProvisioningService) RenewAgentLease(ctx context.Context, agentID, token string) (*AgentProvisioningLease, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("agent provisioning repository is unavailable")
	}
	token = strings.TrimSpace(token)
	if !validProvisioningAgentID(strings.TrimSpace(agentID)) || len(token) < 32 || len(token) > 256 {
		return nil, ErrAgentProvisioningLeaseLost
	}
	tokenHash := sha256.Sum256([]byte(token))
	agent, expiresAt, err := s.repo.RenewAgentLease(ctx, strings.TrimSpace(agentID), hex.EncodeToString(tokenHash[:]), agentProvisioningLeaseTTL)
	if err != nil {
		return nil, err
	}
	return &AgentProvisioningLease{Agent: agent, Token: token, ExpiresAt: expiresAt}, nil
}

// ReportProgress records one ordered worker checkpoint. SSE remains only a
// notification: consumers still GET the authoritative record after a change.
func (s *AgentProvisioningService) ReportProgress(ctx context.Context, actorUserID int64, idempotencyKey, requestID string, progress AgentProvisioningProgress, leaseToken string) (*AgentProvisioningAgent, bool, error) {
	if s == nil || s.repo == nil {
		return nil, false, errors.New("agent provisioning repository is unavailable")
	}
	if actorUserID <= 0 {
		return nil, false, infraerrors.Unauthorized("UNAUTHORIZED", "authenticated administrator required")
	}
	progress.AgentID = strings.TrimSpace(progress.AgentID)
	if !strings.HasPrefix(progress.AgentID, "agt_") || len(progress.AgentID) != 36 {
		return nil, false, ErrAgentProvisioningNotFound
	}
	if !validAgentProvisioningProgress(progress) {
		return nil, false, ErrAgentProvisioningInvalid
	}
	leaseTokenHash, err := hashProvisioningLeaseToken(leaseToken)
	if err != nil {
		return nil, false, err
	}
	keyHash, err := hashAgentProvisioningKey(idempotencyKey)
	if err != nil {
		return nil, false, err
	}
	fingerprintBytes, err := json.Marshal(progress)
	if err != nil {
		return nil, false, err
	}
	fingerprint := sha256.Sum256(fingerprintBytes)
	return s.repo.ReportProgress(ctx, actorUserID, keyHash, hex.EncodeToString(fingerprint[:]), normalizeAgentProvisioningRequestID(requestID), progress, leaseTokenHash)
}

func validAgentProvisioningProgress(progress AgentProvisioningProgress) bool {
	if progress.Step == "failed" {
		_, known := agentProvisioningFailureMessages[progress.FailureCode]
		if !known || progress.FailureStep == "" {
			return known
		}
		for _, step := range agentProvisioningSteps {
			if step == progress.FailureStep {
				return true
			}
		}
		return false
	}
	if progress.FailureCode != "" || progress.FailureStep != "" || progress.Retryable {
		return false
	}
	for _, step := range agentProvisioningSteps {
		if step == progress.Step {
			return true
		}
	}
	return false
}

func (s *AgentProvisioningService) TransitionAgent(ctx context.Context, actorUserID int64, idempotencyKey, requestID, operation, agentID, confirmation, leaseToken string) (*AgentProvisioningAgent, bool, error) {
	if s == nil || s.repo == nil {
		return nil, false, errors.New("agent provisioning repository is unavailable")
	}
	if actorUserID <= 0 {
		return nil, false, infraerrors.Unauthorized("UNAUTHORIZED", "authenticated administrator required")
	}
	agentID = strings.TrimSpace(agentID)
	if !strings.HasPrefix(agentID, "agt_") || len(agentID) != 36 {
		return nil, false, ErrAgentProvisioningNotFound
	}
	if operation != "activate" && operation != "suspend" && operation != "resume" && operation != "revoke" && operation != "retry" {
		return nil, false, ErrAgentProvisioningInvalid
	}
	if (operation == "activate" || operation == "revoke") && strings.TrimSpace(confirmation) != agentID {
		return nil, false, ErrAgentProvisioningConfirmRequired
	}
	leaseTokenHash := ""
	if strings.TrimSpace(leaseToken) != "" {
		hashed, err := hashProvisioningLeaseToken(leaseToken)
		if err != nil {
			return nil, false, err
		}
		leaseTokenHash = hashed
	}
	keyHash, err := hashAgentProvisioningKey(idempotencyKey)
	if err != nil {
		return nil, false, err
	}
	fingerprintBytes, err := json.Marshal(struct {
		Operation string `json:"operation"`
		AgentID   string `json:"agent_id"`
	}{operation, agentID})
	if err != nil {
		return nil, false, err
	}
	fingerprint := sha256.Sum256(fingerprintBytes)
	return s.repo.TransitionAgent(ctx, actorUserID, keyHash, hex.EncodeToString(fingerprint[:]), operation, agentID, normalizeAgentProvisioningRequestID(requestID), leaseTokenHash)
}

func validProvisioningAgentID(agentID string) bool {
	if len(agentID) != 36 || !strings.HasPrefix(agentID, "agt_") {
		return false
	}
	for _, char := range agentID[4:] {
		if !(char >= '0' && char <= '9') && !(char >= 'a' && char <= 'f') {
			return false
		}
	}
	return true
}

func hashProvisioningLeaseToken(token string) (string, error) {
	token = strings.TrimSpace(token)
	if len(token) < 32 || len(token) > 256 || strings.ContainsAny(token, "\r\n") {
		return "", ErrAgentProvisioningLeaseLost
	}
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:]), nil
}

func normalizeAgentProvisioningInput(input AgentProvisioningCreateInput) (AgentProvisioningCreateInput, string, error) {
	input.RequestedSlug = strings.ToLower(strings.TrimSpace(input.RequestedSlug))
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.PlanID = strings.TrimSpace(input.PlanID)
	input.DomainMode = strings.TrimSpace(input.DomainMode)
	input.BrandName = strings.TrimSpace(input.BrandName)
	if input.DomainMode == "" {
		input.DomainMode = "platform_subdomain"
	}
	if input.BrandName == "" {
		input.BrandName = input.DisplayName
	}
	if input.LogoURL != nil {
		logo := strings.TrimSpace(*input.LogoURL)
		if logo == "" {
			input.LogoURL = nil
		} else {
			parsed, err := url.Parse(logo)
			if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" || len(logo) > 2048 {
				return input, "", ErrAgentProvisioningInvalid
			}
			input.LogoURL = &logo
		}
	}
	if !agentSlugPattern.MatchString(input.RequestedSlug) || input.RequestedSlug == "" {
		return input, "", ErrAgentProvisioningInvalid
	}
	if _, reserved := agentReservedSlugs[input.RequestedSlug]; reserved {
		return input, "", ErrAgentProvisioningInvalid
	}
	if input.DisplayName == "" || len([]rune(input.DisplayName)) > 100 || strings.ContainsAny(input.DisplayName, "\r\n") ||
		len([]rune(input.BrandName)) > 100 || strings.ContainsAny(input.BrandName, "\r\n") ||
		input.OwnerMainUserID <= 0 || !agentPlanPattern.MatchString(input.PlanID) || input.DomainMode != "platform_subdomain" {
		return input, "", ErrAgentProvisioningInvalid
	}
	return input, input.RequestedSlug + "." + agentPlatformDomain, nil
}

func hashAgentProvisioningKey(key string) (string, error) {
	key = strings.TrimSpace(key)
	if len(key) < 8 || len(key) > 200 || strings.ContainsAny(key, "\r\n") {
		return "", ErrAgentProvisioningInvalid
	}
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:]), nil
}

func normalizeAgentProvisioningRequestID(requestID string) string {
	requestID = strings.TrimSpace(requestID)
	if len(requestID) > 64 {
		requestID = requestID[:64]
	}
	if requestID == "" {
		return "req_" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return requestID
}
