package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAgentTenantRepositoryEnsuresOneSharedTenantPerOwner(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := NewAgentTenantRepository(db)
	createdAt := time.Date(2026, 9, 30, 8, 0, 0, 0, time.UTC)
	tenant := service.AgentSharedTenant{
		AgentID: "agt_01234567890123456789012345678901", OwnerMainUserID: 42,
		DisplayName: "Agent Owner", BrandName: "Agent Owner", Status: "active", Role: "owner",
	}

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT EXISTS").WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec("INSERT INTO agent_shared_tenants").
		WithArgs(tenant.AgentID, int64(42), tenant.DisplayName, tenant.BrandName).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO agent_shared_tenant_members").
		WithArgs(int64(42)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT t.agent_id, t.owner_main_user_id").WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{
			"agent_id", "owner_main_user_id", "display_name", "brand_name",
			"status", "role", "created_at", "updated_at",
		}).AddRow(tenant.AgentID, int64(42), tenant.DisplayName, tenant.BrandName, "active", "owner", createdAt, createdAt))
	mock.ExpectCommit()

	created, err := repo.EnsureSharedTenant(context.Background(), tenant)
	require.NoError(t, err)
	require.Equal(t, tenant.AgentID, created.AgentID)
	require.Equal(t, "owner", created.Role)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAgentTenantRepositoryBindsMemberWithoutPromotingRole(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := NewAgentTenantRepository(db)
	createdAt := time.Date(2026, 9, 30, 8, 0, 0, 0, time.UTC)
	const agentID = "agt_01234567890123456789012345678901"

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT EXISTS").WithArgs(agentID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(int64(43)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec("INSERT INTO agent_shared_tenant_members").
		WithArgs(agentID, int64(43)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT t.agent_id, t.owner_main_user_id").WithArgs(agentID, int64(43)).
		WillReturnRows(sqlmock.NewRows([]string{
			"agent_id", "owner_main_user_id", "display_name", "brand_name",
			"status", "role", "created_at", "updated_at",
		}).AddRow(agentID, int64(42), "Station", "Station", "active", "member", createdAt, createdAt))
	mock.ExpectCommit()

	member, err := repo.EnsureSharedTenantMember(context.Background(), agentID, 43)
	require.NoError(t, err)
	require.Equal(t, agentID, member.AgentID)
	require.Equal(t, int64(42), member.OwnerMainUserID)
	require.Equal(t, "member", member.Role)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAgentTenantRepositoryFindsExistingMembership(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := NewAgentTenantRepository(db)
	createdAt := time.Date(2026, 9, 30, 8, 0, 0, 0, time.UTC)
	const agentID = "agt_01234567890123456789012345678901"

	mock.ExpectQuery("SELECT t.agent_id, t.owner_main_user_id").WithArgs(int64(43)).
		WillReturnRows(sqlmock.NewRows([]string{
			"agent_id", "owner_main_user_id", "display_name", "brand_name",
			"status", "role", "created_at", "updated_at",
		}).AddRow(agentID, int64(42), "Station", "Station", "active", "member", createdAt, createdAt))

	member, err := repo.FindActiveSharedTenantMembership(context.Background(), 43)
	require.NoError(t, err)
	require.Equal(t, agentID, member.AgentID)
	require.Equal(t, "member", member.Role)
	require.NoError(t, mock.ExpectationsWereMet())
}
