-- Repair legacy deployments where the 240 migration created only partial
-- constraints or where duplicate rows existed before the unique indexes.
DELETE FROM security_secrets a
USING security_secrets b
WHERE a.key = b.key
  AND a.id < b.id;

CREATE UNIQUE INDEX IF NOT EXISTS security_secrets_key_key
    ON security_secrets (key);

DELETE FROM orphan_allowed_groups_audit a
USING orphan_allowed_groups_audit b
WHERE a.user_id = b.user_id
  AND a.group_id = b.group_id
  AND a.id < b.id;

CREATE UNIQUE INDEX IF NOT EXISTS orphan_allowed_groups_audit_user_id_group_id_key
    ON orphan_allowed_groups_audit (user_id, group_id);
