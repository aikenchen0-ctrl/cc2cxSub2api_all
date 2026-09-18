CREATE TABLE IF NOT EXISTS artisan_user_identities (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES artisan_users(id) ON DELETE CASCADE,
    provider VARCHAR(40) NOT NULL,
    subject VARCHAR(160) NOT NULL,
    provider_username VARCHAR(160),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (provider, subject)
);

CREATE INDEX IF NOT EXISTS idx_artisan_user_identities_user
    ON artisan_user_identities(user_id);
