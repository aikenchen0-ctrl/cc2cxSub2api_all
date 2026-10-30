package service

import (
	"context"
	"errors"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/google/uuid"
)

var (
	ErrAgentTenantInvalid       = infraerrors.BadRequest("AGENT_TENANT_INVALID", "invalid AgentAPI tenant request")
	ErrAgentTenantOwnerNotFound = infraerrors.NotFound("AGENT_OWNER_NOT_FOUND", "owner user not found")
	ErrAgentTenantNotFound      = infraerrors.NotFound("AGENT_TENANT_NOT_FOUND", "AgentAPI tenant not found")
)

// AgentSharedTenant is the main-site source of truth for one logical tenant in
// the shared AgentAPI deployment. It contains no deployment or runtime secret.
type AgentSharedTenant struct {
	AgentID         string    `json:"agent_id"`
	OwnerMainUserID int64     `json:"owner_main_user_id"`
	DisplayName     string    `json:"display_name"`
	BrandName       string    `json:"brand_name"`
	Status          string    `json:"status"`
	Role            string    `json:"role"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// AgentTenantRepository is the main-site registry for logical tenants in the
// single shared AgentAPI deployment.
type AgentTenantRepository interface {
	EnsureSharedTenant(ctx context.Context, tenant AgentSharedTenant) (*AgentSharedTenant, error)
	EnsureSharedTenantMember(ctx context.Context, agentID string, mainUserID int64) (*AgentSharedTenant, error)
	FindActiveSharedTenantMembership(ctx context.Context, mainUserID int64) (*AgentSharedTenant, error)
}

// AgentTenantService owns shared-tenant creation and lookup. It does not
// provision containers or issue runtime credentials.
type AgentTenantService struct {
	repo AgentTenantRepository
}

func NewAgentTenantService(repo AgentTenantRepository) *AgentTenantService {
	return &AgentTenantService{repo: repo}
}

// EnsureSharedTenant returns the authenticated user's existing AgentAPI tenant
// or creates it once. Repository uniqueness makes repeated/concurrent clicks
// converge on the same logical tenant without deploying another container.
func (s *AgentTenantService) EnsureSharedTenant(ctx context.Context, ownerMainUserID int64, displayName string) (*AgentSharedTenant, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("AgentAPI tenant repository is unavailable")
	}
	if ownerMainUserID <= 0 {
		return nil, ErrAgentTenantInvalid
	}
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		displayName = "AgentAPI"
	}
	if len([]rune(displayName)) > 100 || strings.ContainsAny(displayName, "\r\n") {
		return nil, ErrAgentTenantInvalid
	}

	agentID := "agt_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	return s.repo.EnsureSharedTenant(ctx, AgentSharedTenant{
		AgentID:         agentID,
		OwnerMainUserID: ownerMainUserID,
		DisplayName:     displayName,
		BrandName:       displayName,
		Status:          "active",
		Role:            "owner",
	})
}

// ResolveSharedTenant enters the caller's existing tenant before creating a new
// one. Users registered through a station domain are members, not owners, and
// must keep landing in that station when they use main-site SSO.
func (s *AgentTenantService) ResolveSharedTenant(ctx context.Context, mainUserID int64, displayName string) (*AgentSharedTenant, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("AgentAPI tenant repository is unavailable")
	}
	if mainUserID <= 0 {
		return nil, ErrAgentTenantInvalid
	}
	existing, err := s.repo.FindActiveSharedTenantMembership(ctx, mainUserID)
	if err == nil && existing != nil {
		return existing, nil
	}
	if err != nil && !errors.Is(err, ErrAgentTenantNotFound) {
		return nil, err
	}
	return s.EnsureSharedTenant(ctx, mainUserID, displayName)
}

// EnsureSharedTenantMember binds an existing active main-site user to a tenant
// as a member. The repository keeps an existing owner role intact and never
// accepts a caller-supplied role.
func (s *AgentTenantService) EnsureSharedTenantMember(ctx context.Context, agentID string, mainUserID int64) (*AgentSharedTenant, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("AgentAPI tenant repository is unavailable")
	}
	agentID = strings.TrimSpace(agentID)
	if len(agentID) != 36 || !strings.HasPrefix(agentID, "agt_") || mainUserID <= 0 {
		return nil, ErrAgentTenantInvalid
	}
	for _, value := range agentID[4:] {
		if value < '0' || value > '9' && (value < 'a' || value > 'f') {
			return nil, ErrAgentTenantInvalid
		}
	}
	return s.repo.EnsureSharedTenantMember(ctx, agentID, mainUserID)
}
