package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type agentTenantRepository struct {
	db *sql.DB
}

func NewAgentTenantRepository(db *sql.DB) service.AgentTenantRepository {
	return &agentTenantRepository{db: db}
}

func ProvideAgentTenantRepository(db *sql.DB) service.AgentTenantRepository {
	return NewAgentTenantRepository(db)
}

type agentTenantScanner interface {
	Scan(dest ...any) error
}

const agentSharedTenantSelect = `
SELECT t.agent_id, t.owner_main_user_id, t.display_name, t.brand_name,
       t.status, m.role, t.created_at, t.updated_at
FROM agent_shared_tenants t
JOIN agent_shared_tenant_members m
  ON m.agent_id=t.agent_id AND m.main_user_id=t.owner_main_user_id
WHERE t.owner_main_user_id=$1 AND m.status='active'`

func scanAgentSharedTenant(row agentTenantScanner) (*service.AgentSharedTenant, error) {
	var tenant service.AgentSharedTenant
	if err := row.Scan(
		&tenant.AgentID, &tenant.OwnerMainUserID, &tenant.DisplayName,
		&tenant.BrandName, &tenant.Status, &tenant.Role,
		&tenant.CreatedAt, &tenant.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &tenant, nil
}

// EnsureSharedTenant is the only create path behind “一键开分站”. The owner
// uniqueness constraint makes repeated and concurrent clicks converge on the
// same tenant in the shared AgentAPI deployment.
func (r *agentTenantRepository) EnsureSharedTenant(ctx context.Context, tenant service.AgentSharedTenant) (*service.AgentSharedTenant, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("AgentAPI tenant database is unavailable")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin shared AgentAPI tenant create: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var activeUser bool
	if err := tx.QueryRowContext(ctx, `
SELECT EXISTS (
    SELECT 1 FROM users WHERE id=$1 AND status='active' AND deleted_at IS NULL
)`, tenant.OwnerMainUserID).Scan(&activeUser); err != nil {
		return nil, fmt.Errorf("validate shared AgentAPI owner: %w", err)
	}
	if !activeUser {
		return nil, service.ErrAgentTenantOwnerNotFound
	}

	if _, err := tx.ExecContext(ctx, `
INSERT INTO agent_shared_tenants (
    agent_id, owner_main_user_id, display_name, brand_name, status,
    created_at, updated_at
) VALUES ($1,$2,$3,$4,'active',NOW(),NOW())
ON CONFLICT (owner_main_user_id) DO NOTHING`,
		tenant.AgentID, tenant.OwnerMainUserID, tenant.DisplayName, tenant.BrandName,
	); err != nil {
		return nil, fmt.Errorf("insert shared AgentAPI tenant: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO agent_shared_tenant_members (
    agent_id, main_user_id, role, status, created_at, updated_at
)
SELECT agent_id, owner_main_user_id, 'owner', 'active', NOW(), NOW()
FROM agent_shared_tenants WHERE owner_main_user_id=$1
ON CONFLICT (agent_id, main_user_id) DO UPDATE
SET role='owner', status='active', updated_at=NOW()`, tenant.OwnerMainUserID); err != nil {
		return nil, fmt.Errorf("bind shared AgentAPI owner: %w", err)
	}
	created, err := scanAgentSharedTenant(tx.QueryRowContext(ctx, agentSharedTenantSelect, tenant.OwnerMainUserID))
	if err != nil {
		return nil, fmt.Errorf("read shared AgentAPI tenant: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit shared AgentAPI tenant: %w", err)
	}
	return created, nil
}
