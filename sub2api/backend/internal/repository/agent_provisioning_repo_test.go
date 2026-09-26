package repository

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestAgentProvisioningRepositoryCreateCommitsRecordAndEventTogether(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := NewAgentProvisioningRepository(db)
	createdAt := time.Date(2026, 9, 24, 8, 0, 0, 0, time.UTC)
	agent := service.AgentProvisioningAgent{
		AgentID: "agt_01234567890123456789012345678901", Slug: "agent01", Domain: "agent01.cc2.cx",
		DisplayName: "Agent 01", OwnerMainUserID: 42, PlanID: "standard", BrandName: "Brand 01",
		CreatedByUserID: 77, RequestID: "req-create",
	}
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO agent_provisioning_idempotency").
		WithArgs(int64(77), "key-hash", "request-hash", agent.AgentID, "req-create").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec("INSERT INTO agent_provisioning_agents").
		WithArgs(agent.AgentID, agent.Slug, agent.Domain, agent.DisplayName, int64(42), agent.PlanID, agent.BrandName, nil, int64(77), "req-create").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO agent_provisioning_events").
		WithArgs(agent.AgentID, int64(77), "req-create").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT agent_id, slug, domain, display_name").
		WithArgs(agent.AgentID).
		WillReturnRows(agentProvisioningTestRow(createdAt, "pending", "queued"))
	mock.ExpectCommit()

	created, replayed, err := repo.CreateAgent(context.Background(), 77, "key-hash", "request-hash", "req-create", agent)
	require.NoError(t, err)
	require.False(t, replayed)
	require.Equal(t, "pending", created.Status)
	require.Equal(t, "req-create", created.RequestID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAgentProvisioningRepositoryListsAgentRecordsWithFiltersAndPagination(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := NewAgentProvisioningRepository(db)
	createdAt := time.Date(2026, 9, 24, 8, 0, 0, 0, time.UTC)
	mock.ExpectQuery("SELECT COUNT\\(\\*\\)").
		WithArgs("pending", "%agent01%").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT agent_id, slug, domain, display_name").
		WithArgs("pending", "%agent01%", 10, 10).
		WillReturnRows(agentProvisioningTestRow(createdAt, "pending", "queued"))

	items, total, err := repo.ListAgents(context.Background(), service.AgentProvisioningListFilter{
		Page: 2, PageSize: 10, Status: "pending", Search: "agent01",
	})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, items, 1)
	require.Equal(t, "agent01.cc2.cx", items[0].Domain)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAgentProvisioningRepositoryClaimAcquiresExpiredLeaseAndReturnsAuthoritativeAgent(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := NewAgentProvisioningRepository(db)
	createdAt := time.Date(2026, 9, 24, 8, 0, 0, 0, time.UTC)
	expiresAt := createdAt.Add(90 * time.Second)
	agentID := "agt_01234567890123456789012345678901"

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT status FROM agent_provisioning_agents WHERE agent_id=$1 FOR UPDATE")).
		WithArgs(agentID).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("pending"))
	mock.ExpectQuery("INSERT INTO agent_provisioning_leases").
		WithArgs(agentID, "lease-hash", int64(90)).
		WillReturnRows(sqlmock.NewRows([]string{"expires_at"}).AddRow(expiresAt))
	mock.ExpectQuery("SELECT agent_id, slug, domain, display_name").
		WithArgs(agentID).
		WillReturnRows(agentProvisioningTestRow(createdAt, "pending", "queued"))
	mock.ExpectCommit()

	agent, expiry, err := repo.ClaimAgent(context.Background(), agentID, "lease-hash", 90*time.Second)
	require.NoError(t, err)
	require.Equal(t, agentID, agent.AgentID)
	require.Equal(t, expiresAt, expiry)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAgentProvisioningRepositoryClaimRejectsUnexpiredLease(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := NewAgentProvisioningRepository(db)
	agentID := "agt_01234567890123456789012345678901"

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT status FROM agent_provisioning_agents WHERE agent_id=$1 FOR UPDATE")).
		WithArgs(agentID).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("provisioning"))
	mock.ExpectQuery("INSERT INTO agent_provisioning_leases").
		WithArgs(agentID, "contender-hash", int64(90)).
		WillReturnRows(sqlmock.NewRows([]string{"expires_at"}))
	mock.ExpectRollback()

	_, _, err = repo.ClaimAgent(context.Background(), agentID, "contender-hash", 90*time.Second)
	require.ErrorIs(t, err, service.ErrAgentProvisioningLeaseHeld)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAgentProvisioningRepositoryRenewsOnlyMatchingLiveLease(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := NewAgentProvisioningRepository(db)
	createdAt := time.Date(2026, 9, 24, 8, 0, 0, 0, time.UTC)
	expiresAt := createdAt.Add(90 * time.Second)
	agentID := "agt_01234567890123456789012345678901"

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT status FROM agent_provisioning_agents WHERE agent_id=$1 FOR UPDATE")).
		WithArgs(agentID).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("provisioning"))
	mock.ExpectQuery("UPDATE agent_provisioning_leases").
		WithArgs(agentID, "lease-hash", int64(90)).
		WillReturnRows(sqlmock.NewRows([]string{"expires_at"}).AddRow(expiresAt))
	mock.ExpectQuery("SELECT agent_id, slug, domain, display_name").
		WithArgs(agentID).
		WillReturnRows(agentProvisioningTestRow(createdAt, "provisioning", "deploying"))
	mock.ExpectCommit()

	agent, expiry, err := repo.RenewAgentLease(context.Background(), agentID, "lease-hash", 90*time.Second)
	require.NoError(t, err)
	require.Equal(t, "deploying", agent.CurrentStep)
	require.Equal(t, expiresAt, expiry)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAgentProvisioningRepositoryRenewRejectsExpiredOrReplacedLease(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := NewAgentProvisioningRepository(db)
	agentID := "agt_01234567890123456789012345678901"

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT status FROM agent_provisioning_agents WHERE agent_id=$1 FOR UPDATE")).
		WithArgs(agentID).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("provisioning"))
	mock.ExpectQuery("UPDATE agent_provisioning_leases").
		WithArgs(agentID, "old-lease-hash", int64(90)).
		WillReturnRows(sqlmock.NewRows([]string{"expires_at"}))
	mock.ExpectRollback()

	_, _, err = repo.RenewAgentLease(context.Background(), agentID, "old-lease-hash", 90*time.Second)
	require.ErrorIs(t, err, service.ErrAgentProvisioningLeaseLost)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAgentProvisioningRepositoryProgressRejectsLostLeaseBeforeWritingIdempotency(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := NewAgentProvisioningRepository(db)
	agentID := "agt_01234567890123456789012345678901"

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT status, current_step, domain_status").
		WithArgs(agentID).
		WillReturnRows(sqlmock.NewRows([]string{"status", "current_step", "domain_status"}).AddRow("provisioning", "deploying", "pending"))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT token_hash FROM agent_provisioning_leases WHERE agent_id=$1 AND token_hash=$2 AND expires_at > NOW() FOR UPDATE")).
		WithArgs(agentID, "expired-lease-hash").
		WillReturnRows(sqlmock.NewRows([]string{"token_hash"}))
	mock.ExpectRollback()

	_, replayed, err := repo.ReportProgress(context.Background(), 77, "key-hash", "request-hash", "req-progress", service.AgentProvisioningProgress{
		AgentID: agentID, Step: "domain_check",
	}, "expired-lease-hash")
	require.ErrorIs(t, err, service.ErrAgentProvisioningLeaseLost)
	require.False(t, replayed)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAgentProvisioningRepositoryWorkerActivationConsumesValidLeaseAtomically(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := NewAgentProvisioningRepository(db)
	createdAt := time.Date(2026, 9, 24, 8, 0, 0, 0, time.UTC)
	agentID := "agt_01234567890123456789012345678901"

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO agent_provisioning_idempotency").
		WithArgs(int64(77), "key-hash", "request-hash", "activate", agentID, "req-activate").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT status, current_step, retryable FROM agent_provisioning_agents WHERE agent_id = $1 FOR UPDATE")).
		WithArgs(agentID).
		WillReturnRows(sqlmock.NewRows([]string{"status", "current_step", "retryable"}).AddRow("provisioning", "readiness_check", false))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT token_hash FROM agent_provisioning_leases WHERE agent_id=$1 AND token_hash=$2 AND expires_at > NOW() FOR UPDATE")).
		WithArgs(agentID, "lease-hash").
		WillReturnRows(sqlmock.NewRows([]string{"token_hash"}).AddRow("lease-hash"))
	mock.ExpectExec("UPDATE agent_provisioning_agents").
		WithArgs("active", "ready", agentID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM agent_provisioning_leases WHERE agent_id=$1")).
		WithArgs(agentID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO agent_provisioning_events").
		WithArgs(agentID, int64(77), "activate", "provisioning", "active", "req-activate").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT agent_id, slug, domain, display_name").
		WithArgs(agentID).
		WillReturnRows(agentProvisioningTestRow(createdAt, "active", "ready"))
	mock.ExpectCommit()

	agent, replayed, err := repo.TransitionAgent(context.Background(), 77, "key-hash", "request-hash", "activate", agentID, "req-activate", "lease-hash")
	require.NoError(t, err)
	require.False(t, replayed)
	require.Equal(t, "active", agent.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAgentProvisioningRepositoryWorkerActivationRejectsLostLease(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := NewAgentProvisioningRepository(db)
	agentID := "agt_01234567890123456789012345678901"

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO agent_provisioning_idempotency").
		WithArgs(int64(77), "key-hash", "request-hash", "activate", agentID, "req-activate").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT status, current_step, retryable FROM agent_provisioning_agents WHERE agent_id = $1 FOR UPDATE")).
		WithArgs(agentID).
		WillReturnRows(sqlmock.NewRows([]string{"status", "current_step", "retryable"}).AddRow("provisioning", "readiness_check", false))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT token_hash FROM agent_provisioning_leases WHERE agent_id=$1 AND token_hash=$2 AND expires_at > NOW() FOR UPDATE")).
		WithArgs(agentID, "replaced-lease-hash").
		WillReturnRows(sqlmock.NewRows([]string{"token_hash"}))
	mock.ExpectRollback()

	_, replayed, err := repo.TransitionAgent(context.Background(), 77, "key-hash", "request-hash", "activate", agentID, "req-activate", "replaced-lease-hash")
	require.ErrorIs(t, err, service.ErrAgentProvisioningLeaseLost)
	require.False(t, replayed)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAgentProvisioningRepositoryCreateIsIdempotent(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := NewAgentProvisioningRepository(db)
	createdAt := time.Date(2026, 9, 24, 8, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO agent_provisioning_idempotency").
		WithArgs(int64(77), "key-hash", "request-hash", "agt_01234567890123456789012345678901", "req-retry").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT request_hash, operation, agent_id").
		WithArgs(int64(77), "key-hash").
		WillReturnRows(sqlmock.NewRows([]string{"request_hash", "operation", "agent_id"}).AddRow("request-hash", "create", "agt_01234567890123456789012345678901"))
	mock.ExpectQuery("SELECT agent_id, slug, domain, display_name").
		WithArgs("agt_01234567890123456789012345678901").
		WillReturnRows(agentProvisioningTestRow(createdAt, "pending", "queued"))
	mock.ExpectCommit()

	agent, replayed, err := repo.CreateAgent(context.Background(), 77, "key-hash", "request-hash", "req-retry", service.AgentProvisioningAgent{
		AgentID: "agt_01234567890123456789012345678901",
	})
	require.NoError(t, err)
	require.True(t, replayed)
	require.Equal(t, "agent01.cc2.cx", agent.Domain)
	require.Equal(t, "req-create", agent.RequestID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAgentProvisioningRepositoryCreateRejectsReusedKey(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := NewAgentProvisioningRepository(db)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO agent_provisioning_idempotency").
		WithArgs(int64(77), "key-hash", "new-hash", "agt_01234567890123456789012345678901", "req-create").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT request_hash, operation, agent_id").
		WithArgs(int64(77), "key-hash").
		WillReturnRows(sqlmock.NewRows([]string{"request_hash", "operation", "agent_id"}).AddRow("original-hash", "create", "agt_11234567890123456789012345678901"))
	mock.ExpectRollback()

	_, replayed, err := repo.CreateAgent(context.Background(), 77, "key-hash", "new-hash", "req-create", service.AgentProvisioningAgent{
		AgentID: "agt_01234567890123456789012345678901",
	})
	require.ErrorIs(t, err, service.ErrAgentProvisioningIdempotency)
	require.False(t, replayed)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAgentProvisioningRepositoryTransitionUpdatesStateWithEvent(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := NewAgentProvisioningRepository(db)
	createdAt := time.Date(2026, 9, 24, 8, 0, 0, 0, time.UTC)
	agentID := "agt_01234567890123456789012345678901"
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO agent_provisioning_idempotency").
		WithArgs(int64(77), "key-hash", "request-hash", "suspend", agentID, "req-suspend").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT status, current_step, retryable FROM agent_provisioning_agents WHERE agent_id = $1 FOR UPDATE")).
		WithArgs(agentID).
		WillReturnRows(sqlmock.NewRows([]string{"status", "current_step", "retryable"}).AddRow("active", "ready", false))
	mock.ExpectExec("UPDATE agent_provisioning_agents").
		WithArgs("suspended", "active", "suspend_requested", agentID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM agent_provisioning_leases WHERE agent_id=$1")).
		WithArgs(agentID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO agent_provisioning_events").
		WithArgs(agentID, int64(77), "suspend", "active", "suspended", "req-suspend").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT agent_id, slug, domain, display_name").
		WithArgs(agentID).
		WillReturnRows(agentProvisioningTestRow(createdAt, "suspended", "suspend_requested"))
	mock.ExpectCommit()

	agent, replayed, err := repo.TransitionAgent(context.Background(), 77, "key-hash", "request-hash", "suspend", agentID, "req-suspend", "")
	require.NoError(t, err)
	require.False(t, replayed)
	require.Equal(t, "suspended", agent.Status)
	require.Equal(t, "suspend_requested", agent.CurrentStep)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAgentProvisioningRepositoryActivatesOnlyAfterExplicitReadinessConfirmation(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := NewAgentProvisioningRepository(db)
	createdAt := time.Date(2026, 9, 24, 8, 0, 0, 0, time.UTC)
	agentID := "agt_01234567890123456789012345678901"
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO agent_provisioning_idempotency").
		WithArgs(int64(77), "key-hash", "request-hash", "activate", agentID, "req-activate").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT status, current_step, retryable FROM agent_provisioning_agents WHERE agent_id = $1 FOR UPDATE")).
		WithArgs(agentID).
		WillReturnRows(sqlmock.NewRows([]string{"status", "current_step", "retryable"}).AddRow("pending", "queued", false))
	mock.ExpectExec("UPDATE agent_provisioning_agents").
		WithArgs("active", "ready", agentID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM agent_provisioning_leases WHERE agent_id=$1")).
		WithArgs(agentID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO agent_provisioning_events").
		WithArgs(agentID, int64(77), "activate", "pending", "active", "req-activate").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT agent_id, slug, domain, display_name").
		WithArgs(agentID).
		WillReturnRows(agentProvisioningTestRow(createdAt, "active", "ready"))
	mock.ExpectCommit()

	agent, replayed, err := repo.TransitionAgent(context.Background(), 77, "key-hash", "request-hash", "activate", agentID, "req-activate", "")
	require.NoError(t, err)
	require.False(t, replayed)
	require.Equal(t, "active", agent.Status)
	require.Equal(t, "ready", agent.CurrentStep)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAgentProvisioningRepositoryRetriesOnlyRetryableFailedAgents(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := NewAgentProvisioningRepository(db)
	createdAt := time.Date(2026, 9, 24, 8, 0, 0, 0, time.UTC)
	agentID := "agt_01234567890123456789012345678901"
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO agent_provisioning_idempotency").
		WithArgs(int64(77), "key-hash", "request-hash", "retry", agentID, "req-retry").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT status, current_step, retryable FROM agent_provisioning_agents WHERE agent_id = $1 FOR UPDATE")).
		WithArgs(agentID).
		WillReturnRows(sqlmock.NewRows([]string{"status", "current_step", "retryable"}).AddRow("failed", "tls_check", true))
	mock.ExpectExec("UPDATE agent_provisioning_agents").
		WithArgs("pending", "tls_check", "configuring", agentID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM agent_provisioning_leases WHERE agent_id=$1")).
		WithArgs(agentID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO agent_provisioning_events").
		WithArgs(agentID, int64(77), "retry", "failed", "pending", "req-retry").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT agent_id, slug, domain, display_name").
		WithArgs(agentID).
		WillReturnRows(agentProvisioningTestRowWithDomainStatus(createdAt, "pending", "tls_check", "configuring"))
	mock.ExpectCommit()

	agent, replayed, err := repo.TransitionAgent(context.Background(), 77, "key-hash", "request-hash", "retry", agentID, "req-retry", "")
	require.NoError(t, err)
	require.False(t, replayed)
	require.Equal(t, "pending", agent.Status)
	require.Equal(t, "tls_check", agent.CurrentStep)
	require.Equal(t, "configuring", agent.DomainStatus)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAgentProvisioningRetryRestoresDomainStatusForCheckpoint(t *testing.T) {
	for _, test := range []struct {
		step string
		want string
	}{
		{step: "queued", want: "pending"},
		{step: "deploying", want: "pending"},
		{step: "domain_check", want: "pending"},
		{step: "tls_check", want: "configuring"},
		{step: "readiness_check", want: "ready"},
	} {
		t.Run(test.step, func(t *testing.T) {
			require.Equal(t, test.want, agentProvisioningDomainStatusForRetry(test.step))
		})
	}
}

func TestAgentProvisioningRepositoryRecordsOrderedWorkerCheckpointAtomically(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := NewAgentProvisioningRepository(db)
	createdAt := time.Date(2026, 9, 24, 8, 0, 0, 0, time.UTC)
	agentID := "agt_01234567890123456789012345678901"
	progress := service.AgentProvisioningProgress{AgentID: agentID, Step: "validating"}

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT status, current_step, domain_status").
		WithArgs(agentID).
		WillReturnRows(sqlmock.NewRows([]string{"status", "current_step", "domain_status"}).AddRow("pending", "queued", "pending"))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT token_hash FROM agent_provisioning_leases WHERE agent_id=$1 AND token_hash=$2 AND expires_at > NOW() FOR UPDATE")).
		WithArgs(agentID, "lease-hash").
		WillReturnRows(sqlmock.NewRows([]string{"token_hash"}).AddRow("lease-hash"))
	mock.ExpectExec("INSERT INTO agent_provisioning_idempotency").
		WithArgs(int64(77), "key-hash", "request-hash", "progress", agentID, "req-progress").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE agent_provisioning_agents").
		WithArgs("provisioning", "validating", "pending", "", false, agentID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO agent_provisioning_events").
		WithArgs(agentID, int64(77), "progress", "pending", "provisioning", "req-progress").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE agent_provisioning_leases SET expires_at=NOW() + (90 * INTERVAL '1 second'), updated_at=NOW() WHERE agent_id=$1 AND token_hash=$2 AND expires_at > NOW()")).
		WithArgs(agentID, "lease-hash").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT agent_id, slug, domain, display_name").
		WithArgs(agentID).
		WillReturnRows(agentProvisioningTestRow(createdAt, "provisioning", "validating"))
	mock.ExpectCommit()

	agent, replayed, err := repo.ReportProgress(context.Background(), 77, "key-hash", "request-hash", "req-progress", progress, "lease-hash")
	require.NoError(t, err)
	require.False(t, replayed)
	require.Equal(t, "provisioning", agent.Status)
	require.Equal(t, "validating", agent.CurrentStep)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAgentProvisioningRepositoryRejectsOutOfOrderCheckpoint(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := NewAgentProvisioningRepository(db)
	agentID := "agt_01234567890123456789012345678901"
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT status, current_step, domain_status").
		WithArgs(agentID).
		WillReturnRows(sqlmock.NewRows([]string{"status", "current_step", "domain_status"}).AddRow("pending", "queued", "pending"))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT token_hash FROM agent_provisioning_leases WHERE agent_id=$1 AND token_hash=$2 AND expires_at > NOW() FOR UPDATE")).
		WithArgs(agentID, "lease-hash").
		WillReturnRows(sqlmock.NewRows([]string{"token_hash"}).AddRow("lease-hash"))
	mock.ExpectExec("INSERT INTO agent_provisioning_idempotency").
		WithArgs(int64(77), "key-hash", "request-hash", "progress", agentID, "req-progress").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()

	_, replayed, err := repo.ReportProgress(context.Background(), 77, "key-hash", "request-hash", "req-progress", service.AgentProvisioningProgress{
		AgentID: agentID, Step: "readiness_check",
	}, "lease-hash")
	require.ErrorIs(t, err, service.ErrAgentProvisioningStateConflict)
	require.False(t, replayed)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAgentProvisioningProgressTargetEnforcesSuccessfulOrderedStages(t *testing.T) {
	progressSteps := service.AgentProvisioningSteps()
	status, step, domainStatus := "pending", "queued", "pending"
	for _, next := range progressSteps {
		nextStatus, nextStep, nextDomainStatus, recentError, retryable, allowed := agentProvisioningProgressTarget(
			service.AgentProvisioningProgress{Step: next}, status, step, domainStatus,
		)
		require.Truef(t, allowed, "stage %s should follow %s", next, step)
		require.Equal(t, "provisioning", nextStatus)
		require.Empty(t, recentError)
		require.False(t, retryable)
		status, step, domainStatus = nextStatus, nextStep, nextDomainStatus
	}
	require.Equal(t, "ready", domainStatus)
	_, _, _, _, _, allowed := agentProvisioningProgressTarget(service.AgentProvisioningProgress{Step: "failed", FailureCode: "readiness_failed"}, "active", "ready", "ready")
	require.False(t, allowed, "worker progress must not override an activated agent")
}

func TestAgentProvisioningProgressTargetStoresOnlyMappedFailureTextAndSupportsRetry(t *testing.T) {
	status, step, domainStatus, recentError, retryable, allowed := agentProvisioningProgressTarget(
		service.AgentProvisioningProgress{Step: "failed", FailureCode: "readiness_failed", FailureStep: "readiness_check", Retryable: true},
		"provisioning", "readiness_check", "ready",
	)
	require.True(t, allowed)
	require.Equal(t, "failed", status)
	require.Equal(t, "readiness_check", step)
	require.Equal(t, "agent readiness check did not pass", recentError)
	require.True(t, retryable)
	require.Equal(t, "ready", domainStatus)

	retryStatus, retryStep, allowed := agentProvisioningTargetState("retry", status, step, retryable)
	require.True(t, allowed)
	require.Equal(t, "pending", retryStatus)
	require.Equal(t, "readiness_check", retryStep)

	legacyStatus, legacyStep, legacyAllowed := agentProvisioningTargetState("retry", "failed", "failed", true)
	require.True(t, legacyAllowed)
	require.Equal(t, "pending", legacyStatus)
	require.Equal(t, "queued", legacyStep)
}

func TestAgentProvisioningRepositoryCreateMapsUniqueConflict(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := NewAgentProvisioningRepository(db)
	agent := service.AgentProvisioningAgent{
		AgentID: "agt_01234567890123456789012345678901", Slug: "agent01", Domain: "agent01.cc2.cx",
		DisplayName: "Agent 01", OwnerMainUserID: 42, PlanID: "standard", BrandName: "Agent 01",
	}
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO agent_provisioning_idempotency").
		WithArgs(int64(77), "key-hash", "request-hash", agent.AgentID, "req-create").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec("INSERT INTO agent_provisioning_agents").
		WithArgs(agent.AgentID, agent.Slug, agent.Domain, agent.DisplayName, int64(42), agent.PlanID, agent.BrandName, nil, int64(77), "req-create").
		WillReturnError(&pq.Error{Code: "23505", Constraint: "agent_provisioning_agents_slug_key"})
	mock.ExpectRollback()

	_, _, err = repo.CreateAgent(context.Background(), 77, "key-hash", "request-hash", "req-create", agent)
	require.ErrorIs(t, err, service.ErrAgentProvisioningSlugTaken)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAgentProvisioningRepositoryStoresRuntimeCredentialHashesUnderLease(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := NewAgentProvisioningRepository(db)
	agentID := "agt_01234567890123456789012345678901"
	controlHash, modelHash, leaseHash := strings.Repeat("1", 64), strings.Repeat("2", 64), strings.Repeat("3", 64)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT status FROM agent_provisioning_agents WHERE agent_id=$1 FOR UPDATE")).
		WithArgs(agentID).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("provisioning"))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT token_hash FROM agent_provisioning_leases WHERE agent_id=$1 AND token_hash=$2 AND expires_at > NOW() FOR UPDATE")).
		WithArgs(agentID, leaseHash).
		WillReturnRows(sqlmock.NewRows([]string{"token_hash"}).AddRow(leaseHash))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT agent_id, purpose, token_hash, status")).
		WithArgs(controlHash, modelHash).
		WillReturnRows(sqlmock.NewRows([]string{"agent_id", "purpose", "token_hash", "status"}))
	mock.ExpectExec("UPDATE agent_runtime_credentials").
		WithArgs(agentID).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO agent_runtime_credentials").
		WithArgs(agentID, "control", controlHash).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO agent_runtime_credentials").
		WithArgs(agentID, "model", modelHash).
		WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectExec("UPDATE agent_provisioning_leases SET expires_at").
		WithArgs(agentID, leaseHash).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err = repo.StoreRuntimeCredentialHashes(context.Background(), agentID, controlHash, modelHash, leaseHash)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAgentProvisioningRepositoryRuntimeCredentialRegistrationReplayIsIdempotent(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := NewAgentProvisioningRepository(db)
	agentID := "agt_01234567890123456789012345678901"
	controlHash, modelHash, leaseHash := strings.Repeat("1", 64), strings.Repeat("2", 64), strings.Repeat("3", 64)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT status FROM agent_provisioning_agents WHERE agent_id=$1 FOR UPDATE")).
		WithArgs(agentID).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("provisioning"))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT token_hash FROM agent_provisioning_leases WHERE agent_id=$1 AND token_hash=$2 AND expires_at > NOW() FOR UPDATE")).
		WithArgs(agentID, leaseHash).
		WillReturnRows(sqlmock.NewRows([]string{"token_hash"}).AddRow(leaseHash))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT agent_id, purpose, token_hash, status")).
		WithArgs(controlHash, modelHash).
		WillReturnRows(sqlmock.NewRows([]string{"agent_id", "purpose", "token_hash", "status"}).
			AddRow(agentID, "control", controlHash, "active").
			AddRow(agentID, "model", modelHash, "active"))
	mock.ExpectExec("UPDATE agent_provisioning_leases SET expires_at").
		WithArgs(agentID, leaseHash).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err = repo.StoreRuntimeCredentialHashes(context.Background(), agentID, controlHash, modelHash, leaseHash)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAgentProvisioningRepositoryRuntimeCredentialReplayCannotReactivateRevokedHash(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := NewAgentProvisioningRepository(db)
	agentID := "agt_01234567890123456789012345678901"
	controlHash, modelHash, leaseHash := strings.Repeat("1", 64), strings.Repeat("2", 64), strings.Repeat("3", 64)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT status FROM agent_provisioning_agents WHERE agent_id=$1 FOR UPDATE")).
		WithArgs(agentID).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("provisioning"))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT token_hash FROM agent_provisioning_leases WHERE agent_id=$1 AND token_hash=$2 AND expires_at > NOW() FOR UPDATE")).
		WithArgs(agentID, leaseHash).
		WillReturnRows(sqlmock.NewRows([]string{"token_hash"}).AddRow(leaseHash))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT agent_id, purpose, token_hash, status")).
		WithArgs(controlHash, modelHash).
		WillReturnRows(sqlmock.NewRows([]string{"agent_id", "purpose", "token_hash", "status"}).
			AddRow(agentID, "control", controlHash, "revoked"))
	mock.ExpectRollback()

	err = repo.StoreRuntimeCredentialHashes(context.Background(), agentID, controlHash, modelHash, leaseHash)
	require.ErrorIs(t, err, service.ErrAgentRuntimeCredentialInvalid)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAgentProvisioningRepositoryPersistsAndResolvesRuntimeModelAllowlist(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := NewAgentProvisioningRepository(db)
	agentID := "agt_01234567890123456789012345678901"
	models := []string{"gpt-5.5", "gpt-image-2"}
	mock.ExpectExec("UPDATE agent_runtime_credentials").
		WithArgs(agentID, `["gpt-5.5","gpt-image-2"]`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, repo.SetRuntimeModelAllowlist(context.Background(), agentID, models))

	tokenHash := strings.Repeat("a", 64)
	mock.ExpectQuery("SELECT c.agent_id, a.owner_main_user_id, c.purpose, a.status").
		WithArgs(tokenHash).
		WillReturnRows(sqlmock.NewRows([]string{"agent_id", "owner_main_user_id", "purpose", "status", "model_allowlist"}).
			AddRow(agentID, int64(42), "model", "active", `["gpt-5.5","gpt-image-2"]`))
	mock.ExpectExec("UPDATE agent_runtime_credentials SET last_used_at").
		WithArgs(tokenHash).
		WillReturnResult(sqlmock.NewResult(0, 1))
	credential, err := repo.ResolveRuntimeCredential(context.Background(), tokenHash)
	require.NoError(t, err)
	require.NotNil(t, credential)
	require.True(t, credential.ModelAllowlistWasSet)
	require.Equal(t, models, credential.ModelAllowlist)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAgentProvisioningRepositoryKeepsDefaultRuntimeModelScopeDistinctFromEmptyScope(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := NewAgentProvisioningRepository(db)
	tokenHash := strings.Repeat("b", 64)
	mock.ExpectQuery("SELECT c.agent_id, a.owner_main_user_id, c.purpose, a.status").
		WithArgs(tokenHash).
		WillReturnRows(sqlmock.NewRows([]string{"agent_id", "owner_main_user_id", "purpose", "status", "model_allowlist"}).
			AddRow("agt_01234567890123456789012345678901", int64(42), "model", "active", ""))
	mock.ExpectExec("UPDATE agent_runtime_credentials SET last_used_at").
		WithArgs(tokenHash).
		WillReturnResult(sqlmock.NewResult(0, 1))
	credential, err := repo.ResolveRuntimeCredential(context.Background(), tokenHash)
	require.NoError(t, err)
	require.NotNil(t, credential)
	require.False(t, credential.ModelAllowlistWasSet)
	require.Empty(t, credential.ModelAllowlist)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAgentProvisioningRepositoryRequiresMappedUserToRemainActive(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := NewAgentProvisioningRepository(db)
	agentID := "agt_01234567890123456789012345678901"

	query := `SELECT EXISTS (
    SELECT 1
    FROM agent_runtime_user_mappings AS mappings
    JOIN users ON users.id = mappings.main_user_id
    WHERE mappings.agent_id = $1
      AND mappings.main_user_id = $2
      AND users.status = 'active'
      AND users.deleted_at IS NULL
)`
	for _, test := range []struct {
		name   string
		active bool
	}{
		{name: "active mapped user", active: true},
		{name: "disabled or unmapped user", active: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			mock.ExpectQuery(regexp.QuoteMeta(query)).WithArgs(agentID, int64(43)).
				WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(test.active))
			got, err := repo.IsActiveRuntimeUser(context.Background(), agentID, 43)
			require.NoError(t, err)
			require.Equal(t, test.active, got)
		})
	}
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAgentProvisioningRepositoryKeepsRuntimeUserMappingsAgentScoped(t *testing.T) {
	const agentID = "agt_01234567890123456789012345678901"
	const otherAgentID = "agt_11234567890123456789012345678901"
	const userID int64 = 43

	for _, test := range []struct {
		name      string
		inserted  int64
		insertErr error
		mappedTo  string
		wantErr   error
	}{
		{name: "new mapping", inserted: 1},
		{name: "same agent mapping is idempotent", mappedTo: agentID},
		{name: "user already belongs to another agent", insertErr: &pq.Error{Code: "23505"}, wantErr: service.ErrAgentRuntimeUserScopeConflict},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			repo := NewAgentProvisioningRepository(db)

			mock.ExpectBegin()
			mock.ExpectQuery(regexp.QuoteMeta("SELECT status FROM agent_provisioning_agents WHERE agent_id=$1 FOR UPDATE")).
				WithArgs(agentID).
				WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("active"))
			insert := mock.ExpectExec(regexp.QuoteMeta("INSERT INTO agent_runtime_user_mappings (agent_id, main_user_id)\nVALUES ($1, $2)\nON CONFLICT (agent_id, main_user_id) DO NOTHING")).WithArgs(agentID, userID)
			if test.insertErr != nil {
				insert.WillReturnError(test.insertErr)
				mock.ExpectRollback()
			} else {
				insert.WillReturnResult(sqlmock.NewResult(0, test.inserted))
				if test.inserted == 0 {
					mock.ExpectQuery(regexp.QuoteMeta("SELECT agent_id FROM agent_runtime_user_mappings WHERE main_user_id=$1")).
						WithArgs(userID).
						WillReturnRows(sqlmock.NewRows([]string{"agent_id"}).AddRow(test.mappedTo))
				}
				mock.ExpectCommit()
			}

			err = repo.MapRuntimeUser(context.Background(), agentID, userID)
			if test.wantErr != nil {
				require.ErrorIs(t, err, test.wantErr)
			} else {
				require.NoError(t, err)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func agentProvisioningTestRow(createdAt time.Time, status, step string) *sqlmock.Rows {
	return agentProvisioningTestRowWithDomainStatus(createdAt, status, step, "pending")
}

func agentProvisioningTestRowWithDomainStatus(createdAt time.Time, status, step, domainStatus string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"agent_id", "slug", "domain", "display_name", "owner_main_user_id", "plan_id",
		"brand_name", "logo_url", "status", "current_step", "recent_error", "retryable",
		"domain_status", "created_by_user_id", "request_id", "created_at", "updated_at",
	}).AddRow(
		"agt_01234567890123456789012345678901", "agent01", "agent01.cc2.cx", "Agent 01", int64(42), "standard",
		"Agent 01", nil, status, step, nil, false, domainStatus, int64(77), "req-create", createdAt, createdAt,
	)
}
