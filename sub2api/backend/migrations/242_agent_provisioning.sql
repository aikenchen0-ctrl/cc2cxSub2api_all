CREATE TABLE IF NOT EXISTS agent_provisioning_agents (
    agent_id TEXT PRIMARY KEY,
    slug VARCHAR(63) NOT NULL UNIQUE,
    domain VARCHAR(253) NOT NULL UNIQUE,
    display_name VARCHAR(100) NOT NULL,
    owner_main_user_id BIGINT NOT NULL,
    plan_id VARCHAR(64) NOT NULL,
    brand_name VARCHAR(100) NOT NULL,
    logo_url TEXT NULL,
    status VARCHAR(24) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'provisioning', 'active', 'suspended', 'failed', 'revoked')),
    status_before_suspend VARCHAR(24) NULL,
    current_step VARCHAR(64) NOT NULL DEFAULT 'queued',
    recent_error TEXT NULL,
    retryable BOOLEAN NOT NULL DEFAULT FALSE,
    domain_status VARCHAR(24) NOT NULL DEFAULT 'pending',
    created_by_user_id BIGINT NOT NULL,
    request_id VARCHAR(64) NOT NULL,
    revoked_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_agent_provisioning_agents_status_updated
    ON agent_provisioning_agents (status, updated_at DESC);

CREATE TABLE IF NOT EXISTS agent_provisioning_idempotency (
    actor_user_id BIGINT NOT NULL,
    key_hash CHAR(64) NOT NULL,
    request_hash CHAR(64) NOT NULL,
    operation VARCHAR(24) NOT NULL,
    agent_id TEXT NOT NULL,
    request_id VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (actor_user_id, key_hash)
);

CREATE INDEX IF NOT EXISTS idx_agent_provisioning_idempotency_agent
    ON agent_provisioning_idempotency (agent_id, created_at DESC);

CREATE TABLE IF NOT EXISTS agent_provisioning_events (
    id BIGSERIAL PRIMARY KEY,
    agent_id TEXT NOT NULL REFERENCES agent_provisioning_agents(agent_id),
    actor_user_id BIGINT NOT NULL,
    operation VARCHAR(24) NOT NULL,
    from_status VARCHAR(24) NULL,
    to_status VARCHAR(24) NOT NULL,
    request_id VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_agent_provisioning_events_agent_created
    ON agent_provisioning_events (agent_id, created_at DESC, id DESC);

CREATE OR REPLACE FUNCTION notify_agent_provisioning_agent_change()
RETURNS trigger AS $$
BEGIN
    PERFORM pg_notify('agent_provisioning_agents', NEW.agent_id);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_agent_provisioning_agent_change ON agent_provisioning_agents;
CREATE TRIGGER trg_agent_provisioning_agent_change
AFTER INSERT OR UPDATE ON agent_provisioning_agents
FOR EACH ROW
EXECUTE FUNCTION notify_agent_provisioning_agent_change();
