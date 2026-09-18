-- Keep the server-managed screen2code credential idempotent per user.
-- Other user-created API keys may keep their existing duplicate-name behavior.
CREATE UNIQUE INDEX IF NOT EXISTS idx_api_keys_screen2code_application_name
    ON api_keys (user_id, name)
    WHERE deleted_at IS NULL AND name = 'Screen2Code Application Key';
