CREATE TABLE IF NOT EXISTS agent_provisioning_leases (
    agent_id TEXT PRIMARY KEY REFERENCES agent_provisioning_agents(agent_id) ON DELETE CASCADE,
    token_hash CHAR(64) NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_agent_provisioning_leases_expires_at
    ON agent_provisioning_leases (expires_at);
