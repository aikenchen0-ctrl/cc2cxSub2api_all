CREATE TABLE IF NOT EXISTS agent_runtime_credentials (
    id BIGSERIAL PRIMARY KEY,
    agent_id TEXT NOT NULL REFERENCES agent_provisioning_agents(agent_id) ON DELETE CASCADE,
    purpose VARCHAR(16) NOT NULL CHECK (purpose IN ('control', 'model')),
    token_hash CHAR(64) NOT NULL UNIQUE,
    status VARCHAR(16) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'revoked')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at TIMESTAMPTZ NULL,
    last_used_at TIMESTAMPTZ NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_runtime_credentials_active_purpose
    ON agent_runtime_credentials (agent_id, purpose)
    WHERE status = 'active';

CREATE TABLE IF NOT EXISTS agent_runtime_user_mappings (
    agent_id TEXT NOT NULL REFERENCES agent_provisioning_agents(agent_id) ON DELETE CASCADE,
    main_user_id BIGINT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (agent_id, main_user_id)
);

CREATE INDEX IF NOT EXISTS idx_agent_runtime_user_mappings_agent
    ON agent_runtime_user_mappings (agent_id, main_user_id);
