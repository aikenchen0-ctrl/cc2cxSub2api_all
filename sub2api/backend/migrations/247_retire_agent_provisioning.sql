-- AgentAPI is one shared multi-tenant deployment. Preserve one logical tenant
-- per historical owner, then remove the retired per-container control plane.
DO $$
BEGIN
    IF to_regclass('agent_provisioning_agents') IS NOT NULL THEN
        WITH latest_owner_agent AS (
            SELECT DISTINCT ON (a.owner_main_user_id)
                a.agent_id,
                a.owner_main_user_id,
                a.display_name,
                a.brand_name,
                CASE
                    WHEN a.status = 'suspended' THEN 'suspended'
                    WHEN a.status = 'revoked' THEN 'revoked'
                    ELSE 'active'
                END AS status,
                a.created_at,
                a.updated_at
            FROM agent_provisioning_agents a
            JOIN users u
              ON u.id = a.owner_main_user_id
             AND u.deleted_at IS NULL
            ORDER BY a.owner_main_user_id,
                     (a.status = 'active') DESC,
                     a.updated_at DESC,
                     a.agent_id
        )
        INSERT INTO agent_shared_tenants (
            agent_id, owner_main_user_id, display_name, brand_name, status,
            created_at, updated_at
        )
        SELECT agent_id, owner_main_user_id, display_name, brand_name, status,
               created_at, updated_at
        FROM latest_owner_agent
        ON CONFLICT (owner_main_user_id) DO NOTHING;

        INSERT INTO agent_shared_tenant_members (
            agent_id, main_user_id, role, status, created_at, updated_at
        )
        SELECT agent_id, owner_main_user_id, 'owner',
               CASE WHEN status = 'active' THEN 'active' ELSE 'disabled' END,
               created_at, updated_at
        FROM agent_shared_tenants
        ON CONFLICT (agent_id, main_user_id) DO UPDATE
        SET role = 'owner',
            status = EXCLUDED.status,
            updated_at = EXCLUDED.updated_at;
    END IF;
END $$;

DROP TABLE IF EXISTS agent_runtime_user_mappings;
DROP TABLE IF EXISTS agent_runtime_credentials;
DROP TABLE IF EXISTS agent_provisioning_leases;
DROP TABLE IF EXISTS agent_provisioning_events;
DROP TABLE IF EXISTS agent_provisioning_idempotency;
DROP TABLE IF EXISTS agent_provisioning_agents;
DROP FUNCTION IF EXISTS notify_agent_provisioning_agent_change();
