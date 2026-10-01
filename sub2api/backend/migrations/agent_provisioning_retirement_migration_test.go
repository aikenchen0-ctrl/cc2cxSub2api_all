package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAgentProvisioningRetirementMigrationPreservesTenantBeforeDroppingLegacyTables(t *testing.T) {
	content, err := FS.ReadFile("247_retire_agent_provisioning.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	preserveAt := strings.Index(sql, "INSERT INTO agent_shared_tenants")
	dropAt := strings.Index(sql, "DROP TABLE IF EXISTS agent_provisioning_agents")
	require.Greater(t, preserveAt, -1)
	require.Greater(t, dropAt, preserveAt)
	require.Contains(t, sql, "DISTINCT ON (a.owner_main_user_id)")
	require.Contains(t, sql, "ON CONFLICT (owner_main_user_id) DO NOTHING")
	require.Contains(t, sql, "INSERT INTO agent_shared_tenant_members")
	require.Contains(t, sql, "to_regclass('agent_provisioning_agents') IS NOT NULL")

	for _, retired := range []string{
		"agent_runtime_user_mappings",
		"agent_runtime_credentials",
		"agent_provisioning_leases",
		"agent_provisioning_events",
		"agent_provisioning_idempotency",
		"agent_provisioning_agents",
	} {
		require.Contains(t, sql, "DROP TABLE IF EXISTS "+retired)
	}
	require.Contains(t, sql, "DROP FUNCTION IF EXISTS notify_agent_provisioning_agent_change()")
}
