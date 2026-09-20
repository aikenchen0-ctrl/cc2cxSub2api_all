-- Repair partially-created tables from older deployments before the server
-- persists JWT/TOTP secrets with ON CONFLICT (key).
DELETE FROM security_secrets a
USING security_secrets b
WHERE a.key = b.key
  AND a.id < b.id;

CREATE UNIQUE INDEX IF NOT EXISTS idx_security_secrets_key_unique
    ON security_secrets (key);

CREATE UNIQUE INDEX IF NOT EXISTS idx_orphan_allowed_groups_audit_user_group
    ON orphan_allowed_groups_audit (user_id, group_id);
