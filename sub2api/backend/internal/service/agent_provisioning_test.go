package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"
)

type agentProvisioningRepoStub struct {
	creates           []AgentProvisioningAgent
	actorUserID       int64
	keyHash           string
	requestHash       string
	requestID         string
	transitionAction  string
	transitionAgent   string
	progress          AgentProvisioningProgress
	progressKeyHash   string
	progressHash      string
	leaseTokenHash    string
	controlHash       string
	modelHash         string
	runtimeCredential *AgentRuntimeCredential
	mappedUsers       map[string]int64
}

func (r *agentProvisioningRepoStub) CreateAgent(_ context.Context, actorUserID int64, keyHash, requestHash, requestID string, agent AgentProvisioningAgent) (*AgentProvisioningAgent, bool, error) {
	r.actorUserID, r.keyHash, r.requestHash, r.requestID = actorUserID, keyHash, requestHash, requestID
	r.creates = append(r.creates, agent)
	return &agent, false, nil
}

func (r *agentProvisioningRepoStub) GetAgent(_ context.Context, agentID string) (*AgentProvisioningAgent, error) {
	return &AgentProvisioningAgent{AgentID: agentID}, nil
}

func (r *agentProvisioningRepoStub) ListAgents(_ context.Context, filter AgentProvisioningListFilter) ([]AgentProvisioningAgent, int, error) {
	return []AgentProvisioningAgent{{AgentID: "agt_01234567890123456789012345678901"}}, 1, nil
}

func (r *agentProvisioningRepoStub) ListenAgentUpdates(_ context.Context) (<-chan string, error) {
	return make(chan string), nil
}

func (r *agentProvisioningRepoStub) ClaimAgent(_ context.Context, agentID, tokenHash string, _ time.Duration) (*AgentProvisioningAgent, time.Time, error) {
	r.leaseTokenHash = tokenHash
	return &AgentProvisioningAgent{AgentID: agentID, Status: "pending", CurrentStep: "queued"}, time.Now().Add(agentProvisioningLeaseTTL), nil
}

func (r *agentProvisioningRepoStub) RenewAgentLease(_ context.Context, agentID, tokenHash string, _ time.Duration) (*AgentProvisioningAgent, time.Time, error) {
	r.leaseTokenHash = tokenHash
	return &AgentProvisioningAgent{AgentID: agentID, Status: "provisioning"}, time.Now().Add(agentProvisioningLeaseTTL), nil
}

func (r *agentProvisioningRepoStub) TransitionAgent(_ context.Context, _ int64, _, _, operation, agentID, _, leaseTokenHash string) (*AgentProvisioningAgent, bool, error) {
	r.transitionAction, r.transitionAgent = operation, agentID
	r.leaseTokenHash = leaseTokenHash
	return &AgentProvisioningAgent{AgentID: agentID, Status: "revoked"}, false, nil
}

func (r *agentProvisioningRepoStub) ReportProgress(_ context.Context, _ int64, keyHash, requestHash, requestID string, progress AgentProvisioningProgress, leaseTokenHash string) (*AgentProvisioningAgent, bool, error) {
	r.progress, r.progressKeyHash, r.progressHash = progress, keyHash, requestHash
	r.leaseTokenHash = leaseTokenHash
	return &AgentProvisioningAgent{AgentID: progress.AgentID, Status: "provisioning", CurrentStep: progress.Step, RequestID: requestID}, false, nil
}

func (r *agentProvisioningRepoStub) StoreRuntimeCredentialHashes(_ context.Context, agentID, controlHash, modelHash, leaseTokenHash string) error {
	r.controlHash, r.modelHash, r.leaseTokenHash = controlHash, modelHash, leaseTokenHash
	return nil
}

func (r *agentProvisioningRepoStub) ResolveRuntimeCredential(_ context.Context, _ string) (*AgentRuntimeCredential, error) {
	return r.runtimeCredential, nil
}

func (r *agentProvisioningRepoStub) SetRuntimeModelAllowlist(_ context.Context, _ string, models []string) error {
	if r.runtimeCredential == nil {
		r.runtimeCredential = &AgentRuntimeCredential{}
	}
	r.runtimeCredential.ModelAllowlist = append([]string(nil), models...)
	r.runtimeCredential.ModelAllowlistWasSet = true
	return nil
}

func TestUpdateRuntimeModelAllowlistAcceptsOnlyPublicCatalogAndReturnsStableOrder(t *testing.T) {
	repo := &agentProvisioningRepoStub{}
	svc := NewAgentProvisioningService(repo)
	models, err := svc.UpdateRuntimeModelAllowlist(context.Background(), "agt_0123456789abcdef0123456789abcdef", []string{"gpt-image-2", "gpt-5.5"})
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 || models[0] != "gpt-5.5" || models[1] != "gpt-image-2" {
		t.Fatalf("model scope order=%v", models)
	}
	if repo.runtimeCredential == nil || !repo.runtimeCredential.ModelAllowlistWasSet {
		t.Fatal("model scope was not persisted")
	}
	if _, err := svc.UpdateRuntimeModelAllowlist(context.Background(), "agt_0123456789abcdef0123456789abcdef", []string{"private-provider-model"}); err == nil {
		t.Fatal("non-public model was accepted in runtime scope")
	}
	if _, err := svc.UpdateRuntimeModelAllowlist(context.Background(), "agt_0123456789abcdef0123456789abcdef", []string{"gpt-5.5", "gpt-5.5"}); err == nil {
		t.Fatal("duplicate model was accepted in runtime scope")
	}
}

func (r *agentProvisioningRepoStub) RevokeRuntimeCredentials(_ context.Context, _ string) error {
	return nil
}

func (r *agentProvisioningRepoStub) MapRuntimeUser(_ context.Context, agentID string, userID int64) error {
	if r.mappedUsers == nil {
		r.mappedUsers = make(map[string]int64)
	}
	r.mappedUsers[agentID] = userID
	return nil
}

func (r *agentProvisioningRepoStub) IsRuntimeUser(_ context.Context, agentID string, userID int64) (bool, error) {
	return r.mappedUsers[agentID] == userID, nil
}

func (r *agentProvisioningRepoStub) IsActiveRuntimeUser(ctx context.Context, agentID string, userID int64) (bool, error) {
	return r.IsRuntimeUser(ctx, agentID, userID)
}

func TestAgentProvisioningServiceCreateNormalizesAndHashesRequest(t *testing.T) {
	repo := &agentProvisioningRepoStub{}
	svc := NewAgentProvisioningService(repo)
	logo := "https://assets.example.com/logo.svg"
	agent, replayed, err := svc.CreateAgent(context.Background(), 77, " request-123 ", "http-request-1", AgentProvisioningCreateInput{
		RequestedSlug: " Agent-01 ", DisplayName: " Example Agent ", OwnerMainUserID: 42,
		PlanID: "standard", LogoURL: &logo,
	})
	if err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}
	if replayed {
		t.Fatal("CreateAgent() replayed = true, want false")
	}
	if agent.Slug != "agent-01" || agent.Domain != "agent-01.cc2.cx" || agent.BrandName != "Example Agent" {
		t.Fatalf("normalized agent = %+v", agent)
	}
	if agent.Status != "pending" || agent.CurrentStep != "queued" || agent.RequestID != "http-request-1" {
		t.Fatalf("initial state = %+v", agent)
	}
	if repo.actorUserID != 77 || repo.requestID != "http-request-1" || repo.keyHash == "" || repo.requestHash == "" {
		t.Fatalf("repository arguments were not populated: %+v", repo)
	}
}

func TestAgentProvisioningServiceRejectsUnsafeInputsAndMissingRevokeConfirmation(t *testing.T) {
	repo := &agentProvisioningRepoStub{}
	svc := NewAgentProvisioningService(repo)
	base := AgentProvisioningCreateInput{
		RequestedSlug: "agent-01", DisplayName: "Agent 01", OwnerMainUserID: 42,
		PlanID: "standard", DomainMode: "platform_subdomain",
	}
	for name, mutate := range map[string]func(*AgentProvisioningCreateInput){
		"reserved slug":      func(input *AgentProvisioningCreateInput) { input.RequestedSlug = "admin" },
		"unsupported domain": func(input *AgentProvisioningCreateInput) { input.DomainMode = "custom_domain" },
		"non-https logo": func(input *AgentProvisioningCreateInput) {
			logo := "http://assets.example.com/logo.svg"
			input.LogoURL = &logo
		},
	} {
		t.Run(name, func(t *testing.T) {
			input := base
			mutate(&input)
			if _, _, err := svc.CreateAgent(context.Background(), 77, "request-123", "req-1", input); err == nil {
				t.Fatal("CreateAgent() unexpectedly accepted unsafe input")
			}
		})
	}
	if _, _, err := svc.TransitionAgent(context.Background(), 77, "request-456", "req-2", "revoke", "agt_01234567890123456789012345678901", "", ""); err != ErrAgentProvisioningConfirmRequired {
		t.Fatalf("TransitionAgent() missing confirmation error = %v", err)
	}
	if repo.transitionAction != "" {
		t.Fatal("repository was called without an explicit revoke confirmation")
	}
}

func TestAgentProvisioningServiceRequiresExplicitActivationConfirmation(t *testing.T) {
	const agentID = "agt_01234567890123456789012345678901"
	repo := &agentProvisioningRepoStub{}
	svc := NewAgentProvisioningService(repo)
	if _, _, err := svc.TransitionAgent(context.Background(), 77, "activate-01", "req-activate", "activate", agentID, "", ""); err != ErrAgentProvisioningConfirmRequired {
		t.Fatalf("TransitionAgent() missing activation confirmation error = %v", err)
	}
	if repo.transitionAction != "" {
		t.Fatal("repository was called without an explicit activation confirmation")
	}
	if _, _, err := svc.TransitionAgent(context.Background(), 77, "activate-02", "req-activate", "activate", agentID, agentID, ""); err != nil {
		t.Fatalf("TransitionAgent() confirmed activation error = %v", err)
	}
	if repo.transitionAction != "activate" || repo.transitionAgent != agentID {
		t.Fatalf("activation was not passed to repository: action=%q agent=%q", repo.transitionAction, repo.transitionAgent)
	}
}

func TestAgentProvisioningServiceAllowsExplicitRetryOperation(t *testing.T) {
	const agentID = "agt_01234567890123456789012345678901"
	repo := &agentProvisioningRepoStub{}
	svc := NewAgentProvisioningService(repo)
	if _, _, err := svc.TransitionAgent(context.Background(), 77, "retry-001", "req-retry", "retry", agentID, "", ""); err != nil {
		t.Fatalf("TransitionAgent(retry) error = %v", err)
	}
	if repo.transitionAction != "retry" || repo.transitionAgent != agentID {
		t.Fatalf("retry was not passed to repository: action=%q agent=%q", repo.transitionAction, repo.transitionAgent)
	}
}

func TestAgentProvisioningServiceValidatesAndHashesWorkerProgress(t *testing.T) {
	const agentID = "agt_01234567890123456789012345678901"
	repo := &agentProvisioningRepoStub{}
	svc := NewAgentProvisioningService(repo)
	for _, progress := range []AgentProvisioningProgress{
		{AgentID: agentID, Step: "unknown"},
		{AgentID: agentID, Step: "failed", FailureCode: "raw error containing secrets"},
		{AgentID: agentID, Step: "failed", FailureCode: "readiness_failed", FailureStep: "unknown", Retryable: true},
		{AgentID: agentID, Step: "deploying", Retryable: true},
	} {
		if _, _, err := svc.ReportProgress(context.Background(), 77, "progress-invalid", "req-invalid", progress, ""); err != ErrAgentProvisioningInvalid {
			t.Fatalf("ReportProgress(%+v) error = %v, want invalid request", progress, err)
		}
	}
	if repo.progress.AgentID != "" {
		t.Fatal("invalid progress reached repository")
	}
	result, replayed, err := svc.ReportProgress(context.Background(), 77, "progress-valid-01", "req-progress-01", AgentProvisioningProgress{
		AgentID: agentID, Step: "validating",
	}, "provisioning-worker-lease-token-0123456789")
	if err != nil || replayed || result.CurrentStep != "validating" {
		t.Fatalf("ReportProgress() = %+v, replayed=%v, err=%v", result, replayed, err)
	}
	if repo.progressKeyHash == "" || repo.progressHash == "" || repo.progress.AgentID != agentID {
		t.Fatalf("progress idempotency data missing: %+v", repo)
	}
}

func TestAgentProvisioningServiceClaimReturnsRawOneTimeLeaseAndPersistsHash(t *testing.T) {
	repo := &agentProvisioningRepoStub{}
	svc := NewAgentProvisioningService(repo)
	claim, err := svc.ClaimAgent(context.Background(), "agt_01234567890123456789012345678901")
	if err != nil || claim == nil || claim.Agent == nil || claim.Token == "" || claim.ExpiresAt.IsZero() {
		t.Fatalf("ClaimAgent() = %+v, err=%v", claim, err)
	}
	if len(repo.leaseTokenHash) != 64 || repo.leaseTokenHash == claim.Token {
		t.Fatalf("lease token was not hashed before persistence: hash=%q token=%q", repo.leaseTokenHash, claim.Token)
	}
	if _, err := svc.RenewAgentLease(context.Background(), claim.Agent.AgentID, claim.Token); err != nil {
		t.Fatalf("RenewAgentLease() error = %v", err)
	}
}

func TestAgentProvisioningProgressMessagesAreBounded(t *testing.T) {
	message, ok := AgentProvisioningFailureMessage("deployment_failed")
	if !ok || message == "" {
		t.Fatal("known worker failure code has no safe message")
	}
	if _, ok := AgentProvisioningFailureMessage("token=secret-value"); ok {
		t.Fatal("arbitrary failure text must not be accepted")
	}
	steps := AgentProvisioningSteps()
	if len(steps) != 6 || steps[0] != "validating" || steps[len(steps)-1] != "readiness_check" {
		t.Fatalf("unexpected ordered stages: %v", steps)
	}
	steps[0] = "overwritten"
	if AgentProvisioningSteps()[0] != "validating" {
		t.Fatal("caller mutated canonical step order")
	}
}

func TestAgentProvisioningServiceRegistersOnlyCredentialHashesAndResolvesPurpose(t *testing.T) {
	const agentID = "agt_01234567890123456789012345678901"
	repo := &agentProvisioningRepoStub{runtimeCredential: &AgentRuntimeCredential{
		AgentID: agentID, OwnerMainUserID: 42, Purpose: "model", AgentStatus: "active",
	}}
	svc := NewAgentProvisioningService(repo)
	control := "agt_ctl_0123456789012345678901234567890123456789"
	model := "agt_model_0123456789012345678901234567890123456789"
	lease := "provisioning-worker-lease-token-0123456789"
	controlSum := sha256.Sum256([]byte(control))
	modelSum := sha256.Sum256([]byte(model))
	if err := svc.RegisterRuntimeCredentialHashes(context.Background(), agentID,
		hex.EncodeToString(controlSum[:]), hex.EncodeToString(modelSum[:]), lease); err != nil {
		t.Fatalf("RegisterRuntimeCredentialHashes() error = %v", err)
	}
	if repo.controlHash != hex.EncodeToString(controlSum[:]) || repo.modelHash != hex.EncodeToString(modelSum[:]) {
		t.Fatalf("credential hashes were not passed to the repository: %+v", repo)
	}
	if repo.controlHash == control || repo.modelHash == model || len(repo.leaseTokenHash) != 64 {
		t.Fatalf("raw credentials or lease token reached persistence: %+v", repo)
	}
	credential, err := svc.LookupRuntimeCredential(context.Background(), model)
	if err != nil || credential == nil || credential.AgentID != agentID || credential.OwnerMainUserID != 42 {
		t.Fatalf("LookupRuntimeCredential() = %+v, err=%v", credential, err)
	}
	if _, err := svc.LookupRuntimeCredential(context.Background(), control); err != ErrAgentRuntimeCredentialInvalid {
		t.Fatalf("control credential resolved as model credential: %v", err)
	}
	repo.runtimeCredential.AgentStatus = "suspended"
	if _, err := svc.LookupRuntimeCredential(context.Background(), model); err != ErrAgentRuntimeAgentInactive {
		t.Fatalf("inactive Agent credential error = %v", err)
	}
}
