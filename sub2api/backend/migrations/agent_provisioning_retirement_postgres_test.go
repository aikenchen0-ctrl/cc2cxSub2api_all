//go:build integration

package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

func TestAgentProvisioningRetirementMigrationsOnPostgres(t *testing.T) {
	ctx := context.Background()
	image := strings.TrimSpace(os.Getenv("SUB2API_TEST_POSTGRES_IMAGE"))
	if image == "" {
		image = "postgres:16-alpine"
	}

	container, err := tcpostgres.Run(
		ctx,
		image,
		tcpostgres.WithDatabase("agent_migration_test"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		tcpostgres.BasicWaitStrategies(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, container.Terminate(context.Background())) })

	dsn, err := container.ConnectionString(ctx, "sslmode=disable", "TimeZone=UTC")
	require.NoError(t, err)
	db := openAgentMigrationDB(t, ctx, dsn)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	t.Run("preserves one logical tenant per live owner and removes legacy control tables", func(t *testing.T) {
		testAgentRetirementSuccess(t, ctx, db)
	})
	t.Run("rolls back before dropping legacy data when preservation fails", func(t *testing.T) {
		testAgentRetirementRollback(t, ctx, db)
	})
}

func testAgentRetirementSuccess(t *testing.T, ctx context.Context, db *sql.DB) {
	schema := createAgentMigrationSchema(t, ctx, db)
	conn := openAgentMigrationConnection(t, ctx, db, schema)

	execAgentMigrationFile(t, ctx, conn, "242_agent_provisioning.sql")
	execAgentMigrationFile(t, ctx, conn, "243_agent_provisioning_leases.sql")
	execAgentMigrationFile(t, ctx, conn, "244_agent_runtime_credentials.sql")
	execAgentMigrationFile(t, ctx, conn, "245_agent_runtime_model_scope.sql")

	_, err := conn.ExecContext(ctx, `
INSERT INTO users (id, email, deleted_at) VALUES
    (1, 'owner-one@example.test', NULL),
    (2, 'owner-two@example.test', NULL),
    (3, 'deleted-owner@example.test', NOW());

INSERT INTO agent_provisioning_agents (
    agent_id, slug, domain, display_name, owner_main_user_id, plan_id,
    brand_name, status, current_step, domain_status, created_by_user_id,
    request_id, created_at, updated_at
) VALUES
    ('owner-1-suspended-newer', 'owner-1-suspended', 'owner-1-suspended.example.test',
     'Owner 1 suspended', 1, 'legacy', 'Owner 1 suspended', 'suspended',
     'ready', 'ready', 1, 'req-owner-1-suspended', NOW() - INTERVAL '2 days', NOW()),
    ('owner-1-active-older', 'owner-1-active', 'owner-1-active.example.test',
     'Owner 1 active', 1, 'legacy', 'Owner 1 active', 'active',
     'ready', 'ready', 1, 'req-owner-1-active', NOW() - INTERVAL '5 days', NOW() - INTERVAL '1 day'),
    ('owner-2-provisioning', 'owner-2-provisioning', 'owner-2-provisioning.example.test',
     'Owner 2 provisioning', 2, 'legacy', 'Owner 2 provisioning', 'provisioning',
     'queued', 'pending', 2, 'req-owner-2-provisioning', NOW() - INTERVAL '4 days', NOW() - INTERVAL '2 days'),
    ('owner-2-revoked', 'owner-2-revoked', 'owner-2-revoked.example.test',
     'Owner 2 revoked', 2, 'legacy', 'Owner 2 revoked', 'revoked',
     'revoked', 'revoked', 2, 'req-owner-2-revoked', NOW() - INTERVAL '3 days', NOW() - INTERVAL '1 hour'),
    ('deleted-owner-agent', 'deleted-owner-agent', 'deleted-owner-agent.example.test',
     'Deleted owner', 3, 'legacy', 'Deleted owner', 'active',
     'ready', 'ready', 3, 'req-deleted-owner', NOW() - INTERVAL '1 day', NOW());

INSERT INTO agent_provisioning_idempotency (
    actor_user_id, key_hash, request_hash, operation, agent_id, request_id
) VALUES (1, repeat('a', 64), repeat('b', 64), 'create', 'owner-1-active-older', 'req-idempotency');
INSERT INTO agent_provisioning_events (
    agent_id, actor_user_id, operation, to_status, request_id
) VALUES ('owner-1-active-older', 1, 'create', 'active', 'req-event');
INSERT INTO agent_provisioning_leases (agent_id, token_hash, expires_at)
VALUES ('owner-1-active-older', repeat('c', 64), NOW() + INTERVAL '1 hour');
INSERT INTO agent_runtime_credentials (agent_id, purpose, token_hash)
VALUES ('owner-1-active-older', 'control', repeat('d', 64));
INSERT INTO agent_runtime_user_mappings (agent_id, main_user_id)
VALUES ('owner-1-active-older', 1);
`)
	require.NoError(t, err)

	execAgentMigrationFile(t, ctx, conn, "246_agent_shared_tenants.sql")
	execAgentMigrationFile(t, ctx, conn, "247_retire_agent_provisioning.sql")

	rows, err := conn.QueryContext(ctx, `
SELECT owner_main_user_id, agent_id, status
FROM agent_shared_tenants
ORDER BY owner_main_user_id
`)
	require.NoError(t, err)

	type tenantRow struct {
		ownerID int64
		agentID string
		status  string
	}
	var tenants []tenantRow
	for rows.Next() {
		var row tenantRow
		require.NoError(t, rows.Scan(&row.ownerID, &row.agentID, &row.status))
		tenants = append(tenants, row)
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	require.Equal(t, []tenantRow{
		{ownerID: 1, agentID: "owner-1-active-older", status: "active"},
		{ownerID: 2, agentID: "owner-2-revoked", status: "revoked"},
	}, tenants)

	memberRows, err := conn.QueryContext(ctx, `
SELECT main_user_id, agent_id, role, status
FROM agent_shared_tenant_members
ORDER BY main_user_id
`)
	require.NoError(t, err)

	type memberRow struct {
		userID  int64
		agentID string
		role    string
		status  string
	}
	var members []memberRow
	for memberRows.Next() {
		var row memberRow
		require.NoError(t, memberRows.Scan(&row.userID, &row.agentID, &row.role, &row.status))
		members = append(members, row)
	}
	require.NoError(t, memberRows.Err())
	require.NoError(t, memberRows.Close())
	require.Equal(t, []memberRow{
		{userID: 1, agentID: "owner-1-active-older", role: "owner", status: "active"},
		{userID: 2, agentID: "owner-2-revoked", role: "owner", status: "disabled"},
	}, members)

	for _, retired := range []string{
		"agent_runtime_user_mappings",
		"agent_runtime_credentials",
		"agent_provisioning_leases",
		"agent_provisioning_events",
		"agent_provisioning_idempotency",
		"agent_provisioning_agents",
	} {
		var relation sql.NullString
		require.NoError(t, conn.QueryRowContext(ctx, "SELECT to_regclass($1)", schema+"."+retired).Scan(&relation))
		require.False(t, relation.Valid, "%s should be removed", retired)
	}

	var triggerFunction sql.NullString
	require.NoError(t, conn.QueryRowContext(ctx,
		"SELECT to_regprocedure($1)", schema+".notify_agent_provisioning_agent_change()").Scan(&triggerFunction))
	require.False(t, triggerFunction.Valid, "legacy notification function should be removed")
}

func testAgentRetirementRollback(t *testing.T, ctx context.Context, db *sql.DB) {
	schema := createAgentMigrationSchema(t, ctx, db)
	conn := openAgentMigrationConnection(t, ctx, db, schema)

	execAgentMigrationFile(t, ctx, conn, "242_agent_provisioning.sql")
	execAgentMigrationFile(t, ctx, conn, "246_agent_shared_tenants.sql")

	_, err := conn.ExecContext(ctx, `
INSERT INTO users (id, email) VALUES
    (1, 'legacy-owner@example.test'),
    (2, 'existing-owner@example.test');
INSERT INTO agent_shared_tenants (
    agent_id, owner_main_user_id, display_name, brand_name, status
) VALUES ('conflicting-agent-id', 2, 'Existing tenant', 'Existing tenant', 'active');
INSERT INTO agent_provisioning_agents (
    agent_id, slug, domain, display_name, owner_main_user_id, plan_id,
    brand_name, status, current_step, domain_status, created_by_user_id, request_id
) VALUES (
    'conflicting-agent-id', 'legacy-owner', 'legacy-owner.example.test',
    'Legacy owner', 1, 'legacy', 'Legacy owner', 'active', 'ready', 'ready', 1, 'req-legacy-owner'
);
`)
	require.NoError(t, err)

	migrationSQL, err := FS.ReadFile("247_retire_agent_provisioning.sql")
	require.NoError(t, err)

	tx, err := conn.BeginTx(ctx, nil)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(migrationSQL))
	require.Error(t, err, "conflicting agent_id must abort the retirement migration")
	require.NoError(t, tx.Rollback())

	var legacyCount int
	require.NoError(t, conn.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM agent_provisioning_agents").Scan(&legacyCount))
	require.Equal(t, 1, legacyCount, "failed migration must not drop legacy control data")

	var ownerID int64
	require.NoError(t, conn.QueryRowContext(ctx, `
SELECT owner_main_user_id
FROM agent_shared_tenants
WHERE agent_id = 'conflicting-agent-id'
`).Scan(&ownerID))
	require.Equal(t, int64(2), ownerID, "failed migration must not overwrite the existing tenant")
}

func openAgentMigrationDB(t *testing.T, ctx context.Context, dsn string) *sql.DB {
	t.Helper()

	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	deadline := time.Now().Add(30 * time.Second)
	for {
		err = db.PingContext(ctx)
		if err == nil {
			return db
		}
		if time.Now().After(deadline) {
			require.NoError(t, err, "postgres did not become ready")
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func createAgentMigrationSchema(t *testing.T, ctx context.Context, db *sql.DB) string {
	t.Helper()

	schema := fmt.Sprintf("agent_migration_%d", time.Now().UnixNano())
	_, err := db.ExecContext(ctx, "CREATE SCHEMA "+schema)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, dropErr := db.ExecContext(context.Background(), "DROP SCHEMA IF EXISTS "+schema+" CASCADE")
		require.NoError(t, dropErr)
	})
	return schema
}

func openAgentMigrationConnection(t *testing.T, ctx context.Context, db *sql.DB, schema string) *sql.Conn {
	t.Helper()

	conn, err := db.Conn(ctx)
	require.NoError(t, err)
	_, err = conn.ExecContext(ctx, "SET search_path TO "+schema)
	require.NoError(t, err)
	_, err = conn.ExecContext(ctx, `
CREATE TABLE users (
    id BIGINT PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    deleted_at TIMESTAMPTZ NULL
)
`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = conn.ExecContext(context.Background(), "SET search_path TO public")
		_ = conn.Close()
	})
	return conn
}

func execAgentMigrationFile(t *testing.T, ctx context.Context, conn interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, name string) {
	t.Helper()

	migrationSQL, err := FS.ReadFile(name)
	require.NoError(t, err)
	_, err = conn.ExecContext(ctx, string(migrationSQL))
	require.NoError(t, err, "apply %s", name)
}
