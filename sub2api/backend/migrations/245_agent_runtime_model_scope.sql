ALTER TABLE agent_runtime_credentials
    ADD COLUMN IF NOT EXISTS model_allowlist JSONB NULL;

COMMENT ON COLUMN agent_runtime_credentials.model_allowlist IS
    'Optional per-Agent public model allowlist. NULL means the default satellite catalog; an empty array denies all model requests.';
