-- Shared AgentAPI tenants are logical records. Creating one must never imply
-- a container, port, database, runtime credential, DNS record, or TLS job.
CREATE TABLE IF NOT EXISTS agent_shared_tenants (
    agent_id TEXT PRIMARY KEY,
    owner_main_user_id BIGINT NOT NULL UNIQUE REFERENCES users(id),
    display_name VARCHAR(100) NOT NULL,
    brand_name VARCHAR(100) NOT NULL,
    status VARCHAR(24) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'suspended', 'revoked')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_agent_shared_tenants_status_updated
    ON agent_shared_tenants (status, updated_at DESC);

CREATE TABLE IF NOT EXISTS agent_shared_tenant_members (
    agent_id TEXT NOT NULL REFERENCES agent_shared_tenants(agent_id) ON DELETE CASCADE,
    main_user_id BIGINT NOT NULL REFERENCES users(id),
    role VARCHAR(16) NOT NULL CHECK (role IN ('owner', 'member')),
    status VARCHAR(16) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'disabled')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (agent_id, main_user_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_shared_tenant_owner_member
    ON agent_shared_tenant_members (main_user_id)
    WHERE role = 'owner' AND status = 'active';

CREATE INDEX IF NOT EXISTS idx_agent_shared_tenant_members_agent
    ON agent_shared_tenant_members (agent_id, status, main_user_id);
