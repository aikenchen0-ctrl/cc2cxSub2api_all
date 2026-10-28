#!/usr/bin/env python3

import argparse
import fcntl
import json
import os
import shutil
import sqlite3
import subprocess
import uuid
from dataclasses import dataclass
from datetime import datetime, timedelta, timezone
from pathlib import Path


ROOT = Path(os.environ.get("AUDIT_STATE_ROOT", "/opt/cc2cx-audit-validation"))
CONTAINER = os.environ.get("AUDIT_CONTAINER", "cc2cx-helicone-audit")
PERMANENT_USER = os.environ.get("AUDIT_PERMANENT_USER_ID", "11")
USER_FIELD = "helicone-property-sub2api-user-id"
RETENTION_DAYS = max(1, int(os.environ.get("AUDIT_RETENTION_DAYS", "2")))
RAW_RETENTION_DAYS = max(1, int(os.environ.get("AUDIT_RAW_RETENTION_DAYS", str(RETENTION_DAYS))))
LEDGER_BACKUP_DAYS = max(1, int(os.environ.get("AUDIT_LEDGER_BACKUP_DAYS", "14")))
MINIO_ROOT = Path("/var/lib/docker/volumes/cc2cx-audit-minio/_data")
BUCKET = "request-response-storage"
CH_MUTATION_RESERVE = 2 * 1024**3

os.umask(0o077)


def now_utc():
    return datetime.now(timezone.utc)


def chunks(values, size):
    values = list(values)
    for offset in range(0, len(values), size):
        yield values[offset:offset + size]


def run(args, data=None, timeout=180):
    result = subprocess.run(
        args,
        input=data,
        capture_output=True,
        text=True,
        timeout=timeout,
    )
    if result.returncode:
        detail = (result.stderr or result.stdout).strip()[-4000:]
        raise RuntimeError("外部命令失败：" + " ".join(args[:6]) + "\n" + detail)
    return result.stdout.strip()


def ch(sql, timeout=300):
    return run(
        ["docker", "exec", CONTAINER, "clickhouse-client", "--query", sql],
        timeout=timeout,
    )


def pg(sql, timeout=180):
    return run(
        [
            "docker",
            "exec",
            "-i",
            "-u",
            "postgres",
            CONTAINER,
            "psql",
            "-X",
            "-qAt",
            "-v",
            "ON_ERROR_STOP=1",
            "-d",
            "helicone_test",
        ],
        data=sql,
        timeout=timeout,
    )


def ch_rows(sql, timeout=300):
    text = ch(sql + " FORMAT JSONEachRow", timeout=timeout)
    return [json.loads(line) for line in text.splitlines() if line.strip()]


def ch_ids(sql, timeout=300):
    text = ch(sql + " FORMAT TSV", timeout=timeout)
    return {line.strip() for line in text.splitlines() if line.strip()}


def pg_ids(sql, timeout=180):
    text = pg(sql, timeout=timeout)
    return {line.strip() for line in text.splitlines() if line.strip()}


def ids_sql(ids):
    return ",".join("'" + str(uuid.UUID(value)) + "'" for value in sorted(ids))


def valid_uuid(value):
    try:
        return str(uuid.UUID(value)) == value
    except (ValueError, TypeError, AttributeError):
        return False


def atomic_json(path, value):
    temporary = path.with_suffix(path.suffix + ".new")
    temporary.write_text(json.dumps(value, ensure_ascii=False, separators=(",", ":")))
    temporary.replace(path)


def read_organization():
    data = json.loads((ROOT / "dashboard-credentials.json").read_text())
    organization = str(uuid.UUID(data["organization_id"]))
    return organization


def request_root(organization):
    root = (MINIO_ROOT / BUCKET / "organizations" / organization / "requests").resolve()
    expected = MINIO_ROOT.resolve()
    if expected not in root.parents:
        raise RuntimeError("对象目录越界，停止清理")
    return root


def verify_schema(organization):
    allowed = {
        "cache_hits",
        "properties",
        "feedback",
        "request_job_task",
        "job_node_request",
        "finetune_dataset_data",
        "asset",
        "prompt_input_record",
        "experiment_output",
        "score_value",
        "experiment_v2_hypothesis_run",
        "response",
    }
    text = pg(
        "SELECT c.relname AS name "
        "FROM pg_constraint f "
        "JOIN pg_class c ON c.oid=f.conrelid "
        "WHERE f.contype='f' "
        "AND f.confrelid IN ('public.request'::regclass,'public.response'::regclass)",
        timeout=60,
    )
    rows = [{"name": line.strip()} for line in text.splitlines() if line.strip()]
    if any(row["name"] not in allowed for row in rows):
        raise RuntimeError("发现未知关联表，停止清理并等待表结构复核")

    lifecycle = subprocess.run(
        [
            "docker",
            "exec",
            CONTAINER,
            "mc",
            "ilm",
            "rule",
            "ls",
            "--json",
            f"localminio/{BUCKET}",
        ],
        capture_output=True,
        text=True,
        timeout=30,
    )
    if lifecycle.returncode == 0 and lifecycle.stdout.strip():
        raise RuntimeError("发现 MinIO 生命周期规则，停止清理并等待规则复核")

    root = request_root(organization)
    if not root.is_dir():
        raise RuntimeError("审计对象根目录不存在，停止清理")


def strip_body_ttl():
    for table in ("request_response_rmt", "request_response_versioned"):
        for column in ch_rows(f"DESCRIBE TABLE {table}"):
            if column.get("name") in {"request_body", "response_body"} and column.get("ttl_expression"):
                ch(f"ALTER TABLE {table} MODIFY COLUMN {column['name']} REMOVE TTL")


def query_permanent_ids(organization):
    protected = set()
    for table in ("request_response_rmt", "request_response_versioned"):
        protected.update(
            ch_ids(
                "SELECT DISTINCT toString(request_id) AS request_id "
                f"FROM {table} "
                f"WHERE organization_id='{organization}' "
                f"AND properties['{USER_FIELD}']='{PERMANENT_USER}'"
            )
        )
    protected.update(
        pg_ids(
            "SELECT id::text "
            "FROM request "
            f"WHERE helicone_org_id='{organization}' "
            f"AND properties->>'{USER_FIELD}'='{PERMANENT_USER}'"
        )
    )
    return {value for value in protected if valid_uuid(value)}


def query_metadata_ids(organization):
    metadata = set()
    for table in ("request_response_rmt", "request_response_versioned"):
        metadata.update(
            ch_ids(
                "SELECT DISTINCT toString(request_id) AS request_id "
                f"FROM {table} "
                f"WHERE organization_id='{organization}'"
            )
        )
    metadata.update(
        pg_ids(
            "SELECT id::text "
            "FROM request "
            f"WHERE helicone_org_id='{organization}'"
        )
    )
    return {value for value in metadata if valid_uuid(value)}


def query_expired_metadata(organization, cutoff, limit):
    selected = set()
    stamp = cutoff.strftime("%Y-%m-%d %H:%M:%S")
    for table in ("request_response_rmt", "request_response_versioned"):
        rows = ch_rows(
            "SELECT toString(request_id) AS request_id "
            f"FROM {table} "
            f"WHERE organization_id='{organization}' "
            "GROUP BY request_id "
            f"HAVING max(request_created_at)<toDateTime('{stamp}','UTC') "
            f"AND max(properties['{USER_FIELD}']='{PERMANENT_USER}')=0 "
            "ORDER BY max(request_created_at) "
            f"LIMIT {max(1, limit)}"
        )
        selected.update(row["request_id"] for row in rows)

    rows_text = pg(
        "SELECT id::text "
        "FROM request "
        f"WHERE helicone_org_id='{organization}' "
        f"AND created_at<'{cutoff.isoformat()}' "
        f"AND COALESCE(properties->>'{USER_FIELD}','')<>'{PERMANENT_USER}' "
        "ORDER BY created_at "
        f"LIMIT {max(1, limit)}"
    )
    selected.update(line.strip() for line in rows_text.splitlines() if line.strip())
    return {value for value in selected if valid_uuid(value)}


def current_object_ids(root):
    objects = set()
    invalid = 0
    symlinks = 0
    for entry in os.scandir(root):
        if entry.is_symlink():
            symlinks += 1
            continue
        if not entry.is_dir(follow_symlinks=False):
            continue
        if valid_uuid(entry.name):
            objects.add(entry.name)
        else:
            invalid += 1
    return objects, {"invalid_directories": invalid, "symlink_directories": symlinks}


@dataclass
class ObjectStats:
    files: int = 0
    bytes: int = 0
    canonical_files: int = 0
    canonical_bytes: int = 0
    raw_files: int = 0
    raw_bytes: int = 0
    canonical_parts: int = 0
    canonical_meta: bool = False
    raw_parts: int = 0
    raw_meta: bool = False

    @property
    def canonical_complete(self):
        return self.canonical_parts > 0 and self.canonical_meta

    @property
    def raw_exists(self):
        return self.raw_parts > 0 or self.raw_meta


def object_stats(path):
    stats = ObjectStats()
    if not path.is_dir() or path.is_symlink():
        return stats
    for root, directories, files in os.walk(path, followlinks=False):
        directories[:] = [
            name
            for name in directories
            if not (Path(root) / name).is_symlink()
        ]
        relative = Path(root).relative_to(path)
        for name in files:
            file_path = Path(root) / name
            if file_path.is_symlink():
                continue
            size = file_path.stat().st_size
            stats.files += 1
            stats.bytes += size
            first = relative.parts[0] if relative.parts else ""
            if first == "request_response_body":
                stats.canonical_files += 1
                stats.canonical_bytes += size
                if name.startswith("part."):
                    stats.canonical_parts += 1
                if name == "xl.meta":
                    stats.canonical_meta = True
            elif first == "raw_request_response_body":
                stats.raw_files += 1
                stats.raw_bytes += size
                if name.startswith("part."):
                    stats.raw_parts += 1
                if name == "xl.meta":
                    stats.raw_meta = True
    return stats


def open_ledger(apply):
    path = ROOT / "retention-state.sqlite3"
    if apply:
        ledger = sqlite3.connect(path, timeout=30)
        ledger.execute("PRAGMA journal_mode=WAL")
        ledger.execute("PRAGMA synchronous=FULL")
        ledger.execute("PRAGMA busy_timeout=30000")
        ledger.executescript(
            """
        CREATE TABLE IF NOT EXISTS permanent_requests (
            organization TEXT NOT NULL,
            request_id TEXT NOT NULL,
            user_id TEXT NOT NULL,
            first_seen TEXT NOT NULL,
            last_seen TEXT NOT NULL,
            canonical_seen INTEGER NOT NULL DEFAULT 0,
            canonical_bytes INTEGER NOT NULL DEFAULT 0,
            raw_state TEXT NOT NULL DEFAULT 'unknown',
            raw_checked_at TEXT,
            raw_removed_at TEXT,
            raw_removed_bytes INTEGER NOT NULL DEFAULT 0,
            PRIMARY KEY (organization, request_id)
        );
        CREATE TABLE IF NOT EXISTS expired_pending (
            organization TEXT NOT NULL,
            request_id TEXT NOT NULL,
            first_seen TEXT NOT NULL,
            last_seen TEXT NOT NULL,
            PRIMARY KEY (organization, request_id)
        );
        CREATE TABLE IF NOT EXISTS raw_cleanup_pending (
            organization TEXT NOT NULL,
            request_id TEXT NOT NULL,
            first_seen TEXT NOT NULL,
            last_seen TEXT NOT NULL,
            raw_bytes INTEGER NOT NULL,
            PRIMARY KEY (organization, request_id)
        );
        CREATE TABLE IF NOT EXISTS cleanup_runs (
            run_id TEXT PRIMARY KEY,
            organization TEXT NOT NULL,
            started_at TEXT NOT NULL,
            finished_at TEXT,
            mode TEXT NOT NULL,
            status TEXT NOT NULL,
            summary TEXT NOT NULL
        );
            """
        )
        ledger.commit()
        ensure_column(ledger, "permanent_requests", "raw_state", "TEXT NOT NULL DEFAULT 'unknown'")
        ensure_column(ledger, "permanent_requests", "raw_checked_at", "TEXT")
        return ledger

    if path.exists():
        ledger = sqlite3.connect(f"file:{path}?mode=ro", uri=True, timeout=30)
    else:
        ledger = sqlite3.connect(":memory:")
    ledger.execute("PRAGMA busy_timeout=30000")
    return ledger


def backup_ledger(ledger, started):
    backup_root = ROOT / "state-backups"
    backup_root.mkdir(mode=0o700, parents=True, exist_ok=True)
    backup_path = backup_root / f"retention-state-{started:%Y%m%d}.sqlite3"
    if not backup_path.exists():
        destination = sqlite3.connect(backup_path)
        try:
            ledger.backup(destination)
        finally:
            destination.close()
        backup_path.chmod(0o600)

    cutoff = started.timestamp() - timedelta(days=LEDGER_BACKUP_DAYS).total_seconds()
    for path in backup_root.glob("retention-state-*.sqlite3"):
        if path.is_file() and path.stat().st_mtime < cutoff:
            path.unlink()
    return str(backup_path)


def ledger_has_table(ledger, name):
    row = ledger.execute(
        "SELECT 1 FROM sqlite_master WHERE type='table' AND name=?",
        (name,),
    ).fetchone()
    return row is not None


def ensure_column(ledger, table, name, definition):
    columns = {
        row[1]
        for row in ledger.execute(f"PRAGMA table_info({table})")
    }
    if name not in columns:
        ledger.execute(f"ALTER TABLE {table} ADD COLUMN {name} {definition}")


def ledger_permanent_ids(ledger, organization):
    if not ledger_has_table(ledger, "permanent_requests"):
        return set()
    rows = ledger.execute(
        "SELECT request_id FROM permanent_requests WHERE organization=?",
        (organization,),
    )
    return {row[0] for row in rows}


def ledger_raw_states(ledger, organization):
    if not ledger_has_table(ledger, "permanent_requests"):
        return {}
    return {
        row[0]: row[1]
        for row in ledger.execute(
            "SELECT request_id, raw_state "
            "FROM permanent_requests "
            "WHERE organization=?",
            (organization,),
        )
    }


def raw_state_counts(ledger, organization):
    if not ledger_has_table(ledger, "permanent_requests"):
        return {}
    return {
        row[0]: row[1]
        for row in ledger.execute(
            "SELECT raw_state, count(*) "
            "FROM permanent_requests "
            "WHERE organization=? "
            "GROUP BY raw_state",
            (organization,),
        )
    }


def persist_permanent_ids(ledger, organization, current_ids):
    timestamp = now_utc().isoformat()
    for batch in chunks(sorted(current_ids), 500):
        ledger.executemany(
            """
            INSERT INTO permanent_requests
                (organization, request_id, user_id, first_seen, last_seen)
            VALUES (?, ?, ?, ?, ?)
            ON CONFLICT(organization, request_id) DO UPDATE SET
                user_id=excluded.user_id,
                last_seen=excluded.last_seen
            """,
            [
                (organization, request_id, PERMANENT_USER, timestamp, timestamp)
                for request_id in batch
            ],
        )
        ledger.commit()


def save_pending(ledger, table, organization, values):
    timestamp = now_utc().isoformat()
    if table == "expired_pending":
        ledger.executemany(
            """
            INSERT INTO expired_pending
                (organization, request_id, first_seen, last_seen)
            VALUES (?, ?, ?, ?)
            ON CONFLICT(organization, request_id) DO UPDATE SET
                last_seen=excluded.last_seen
            """,
            [(organization, request_id, timestamp, timestamp) for request_id in values],
        )
    elif table == "raw_cleanup_pending":
        ledger.executemany(
            """
            INSERT INTO raw_cleanup_pending
                (organization, request_id, first_seen, last_seen, raw_bytes)
            VALUES (?, ?, ?, ?, ?)
            ON CONFLICT(organization, request_id) DO UPDATE SET
                last_seen=excluded.last_seen,
                raw_bytes=excluded.raw_bytes
            """,
            [
                (organization, request_id, timestamp, timestamp, raw_bytes)
                for request_id, raw_bytes in values
            ],
        )
    else:
        raise ValueError("未知待处理表")
    ledger.commit()


def pending_ids(ledger, table, organization):
    if not ledger_has_table(ledger, table):
        return set()
    return {
        row[0]
        for row in ledger.execute(
            f"SELECT request_id FROM {table} WHERE organization=?",
            (organization,),
        )
    }


def remove_pending(ledger, table, organization, request_ids):
    ledger.executemany(
        f"DELETE FROM {table} WHERE organization=? AND request_id=?",
        [(organization, request_id) for request_id in request_ids],
    )
    ledger.commit()


def mark_canonical_seen(ledger, organization, manifest):
    timestamp = now_utc().isoformat()
    ledger.executemany(
        """
        UPDATE permanent_requests
        SET canonical_seen=?,
            canonical_bytes=?,
            last_seen=?
        WHERE organization=? AND request_id=?
        """,
        [
            (1, stats.canonical_bytes, timestamp, organization, request_id)
            for request_id, stats in manifest
            if stats.canonical_complete
        ],
    )
    ledger.commit()


def reset_canonical_seen(ledger, organization, request_ids):
    for batch in chunks(sorted(request_ids), 500):
        placeholders = ",".join("?" for _ in batch)
        ledger.execute(
            "UPDATE permanent_requests "
            "SET canonical_seen=0, canonical_bytes=0 "
            f"WHERE organization=? AND request_id IN ({placeholders})",
            (organization, *batch),
        )
        ledger.commit()


def mark_raw_removed(ledger, organization, rows):
    timestamp = now_utc().isoformat()
    ledger.executemany(
        """
        UPDATE permanent_requests
        SET raw_removed_at=?,
            raw_removed_bytes=raw_removed_bytes+?,
            raw_state='removed',
            raw_checked_at=?,
            canonical_seen=1
        WHERE organization=? AND request_id=?
        """,
        [
            (timestamp, raw_bytes, timestamp, organization, request_id)
            for request_id, raw_bytes in rows
        ],
    )
    ledger.commit()


def mark_raw_preserved(ledger, organization, rows):
    timestamp = now_utc().isoformat()
    ledger.executemany(
        """
        UPDATE permanent_requests
        SET raw_state='preserve_raw_only',
            raw_checked_at=?
        WHERE organization=? AND request_id=?
        """,
        [(timestamp, organization, request_id) for request_id in rows],
    )
    ledger.commit()


def mark_raw_pending(ledger, organization, rows):
    timestamp = now_utc().isoformat()
    ledger.executemany(
        """
        UPDATE permanent_requests
        SET raw_state='pending',
            raw_checked_at=?
        WHERE organization=? AND request_id=?
        """,
        [(timestamp, organization, request_id) for request_id in rows],
    )
    ledger.commit()


def delete_metadata(organization, request_ids):
    values = ids_sql(request_ids)
    scoped = (
        "SELECT id FROM request "
        f"WHERE helicone_org_id='{organization}' "
        f"AND id IN ({values})"
    )
    statements = [
        "BEGIN;",
        "SET LOCAL lock_timeout='5s';",
        "SET LOCAL statement_timeout='30s';",
        f"DELETE FROM feedback WHERE response_id IN "
        f"(SELECT id FROM response WHERE request IN ({scoped}));",
    ]
    for table, column in (
        ("cache_hits", "request_id"),
        ("properties", "request_id"),
        ("request_job_task", "request_id"),
        ("job_node_request", "request_id"),
        ("prompt_input_record", "source_request"),
        ("experiment_output", "request_id"),
        ("score_value", "request_id"),
    ):
        statements.append(
            f"DELETE FROM {table} WHERE {column} IN ({scoped});"
        )
    statements.extend(
        [
            f"DELETE FROM response WHERE request IN ({scoped});",
            f"DELETE FROM request "
            f"WHERE helicone_org_id='{organization}' "
            f"AND id IN ({values});",
            "COMMIT;",
        ]
    )
    pg("\n".join(statements))

    columns = ch_rows(
        "SELECT table,groupArray(name) AS names "
        "FROM system.columns "
        "WHERE database='default' "
        "GROUP BY table "
        "HAVING has(names,'request_id')"
    )
    for item in columns:
        table = item["table"]
        if not table.replace("_", "").isalnum() or table.startswith("cc2_"):
            continue
        condition = f"request_id IN ({values})"
        if "organization_id" in item["names"]:
            condition += f" AND organization_id='{organization}'"
        if int(ch(f"SELECT count() FROM {table} WHERE {condition}")):
            ch(
                f"ALTER TABLE {table} "
                f"DELETE WHERE {condition} "
                "SETTINGS mutations_sync=1",
                timeout=900,
            )


def mc_delete(targets, recursive=False, timeout=600):
    if not targets:
        return
    args = [
        "docker",
        "exec",
        CONTAINER,
        "mc",
        "rm",
        "--force",
        "--quiet",
    ]
    if recursive:
        args.append("--recursive")
    args.extend(targets)
    run(
        args,
        timeout=timeout,
    )


def minio_prefix(organization, request_id, object_name=""):
    if not valid_uuid(request_id):
        raise RuntimeError("请求编号格式异常，停止对象删除")
    suffix = f"/{object_name}" if object_name else ""
    return (
        f"localminio/{BUCKET}/organizations/{organization}"
        f"/requests/{request_id}{suffix}/"
    )


def minio_raw_key(organization, request_id):
    if not valid_uuid(request_id):
        raise RuntimeError("请求编号格式异常，停止对象删除")
    return (
        f"localminio/{BUCKET}/organizations/{organization}"
        f"/requests/{request_id}/raw_request_response_body"
    )


def remove_expired_objects(ledger, organization, request_ids, root):
    ordered = sorted(request_ids)
    object_ids, _ = current_object_ids(root)
    for offset in range(0, len(ordered), 100):
        batch = ordered[offset:offset + 100]
        existing = [request_id for request_id in batch if request_id in object_ids]
        mc_delete(
            [minio_prefix(organization, request_id) for request_id in existing],
            recursive=True,
        )
        for request_id in batch:
            path = root / request_id
            if path.exists() or path.is_symlink():
                raise RuntimeError("对象目录删除后仍存在，停止并保留待处理状态")
        remove_pending(ledger, "expired_pending", organization, batch)


def prune_permanent_raw(ledger, organization, root, cutoff, limit, protected, apply):
    permanent = ledger_permanent_ids(ledger, organization) | protected
    actual, _ = current_object_ids(root)
    states = ledger_raw_states(ledger, organization)
    candidates = []
    manifest = []
    preserve_raw_only = []
    pending_raw = []
    total_raw_bytes = 0
    protected_without_canonical = 0
    protected_without_canonical_bytes = 0
    scan_ids = {
        request_id
        for request_id in permanent & actual
        if states.get(request_id, "unknown") not in {"preserve_raw_only", "removed"}
    }
    for request_id in sorted(scan_ids):
        path = root / request_id
        try:
            last_modified = path.stat().st_mtime
        except FileNotFoundError:
            continue
        stats = object_stats(path)
        manifest.append((request_id, stats))
        if not stats.canonical_complete:
            protected_without_canonical += 1
            protected_without_canonical_bytes += stats.raw_bytes
            if stats.raw_exists:
                preserve_raw_only.append(request_id)
            continue
        if not stats.raw_exists:
            continue
        if last_modified >= cutoff.timestamp():
            pending_raw.append(request_id)
            continue
        candidates.append((request_id, stats))
        total_raw_bytes += stats.raw_bytes

    ordered = sorted(candidates, key=lambda item: item[0])
    if limit > 0:
        ordered = ordered[:limit]
    result = {
        "permanent_actual": len(permanent & actual),
        "raw_scan_skipped_preserved": len((permanent & actual) - scan_ids),
        "protected_without_canonical": protected_without_canonical,
        "protected_without_canonical_bytes": protected_without_canonical_bytes,
        "raw_candidates": len(candidates),
        "raw_selected": len(ordered),
        "raw_selected_bytes": sum(stats.raw_bytes for _, stats in ordered),
        "raw_candidate_bytes": total_raw_bytes,
        "raw_sample_ids": [request_id for request_id, _ in ordered[:20]],
        "raw_deleted": 0,
        "raw_deleted_bytes": 0,
    }
    if not apply:
        return result

    reset_canonical_seen(ledger, organization, scan_ids)
    mark_canonical_seen(ledger, organization, manifest)
    mark_raw_preserved(ledger, organization, preserve_raw_only)
    mark_raw_pending(ledger, organization, pending_raw)
    save_pending(
        ledger,
        "raw_cleanup_pending",
        organization,
        [(request_id, stats.raw_bytes) for request_id, stats in ordered],
    )
    for offset in range(0, len(ordered), 200):
        batch = ordered[offset:offset + 200]
        live_ids = [
            request_id
            for request_id, _ in batch
            if request_id in permanent
            and (root / request_id / "request_response_body").is_dir()
        ]
        mc_delete(
            [minio_raw_key(organization, request_id) for request_id in live_ids],
            recursive=False,
            timeout=900,
        )
        removed = []
        for request_id, stats in batch:
            raw_path = root / request_id / "raw_request_response_body"
            if raw_path.exists() or raw_path.is_symlink():
                raise RuntimeError("raw 对象删除后仍存在，停止并保留待处理状态")
            removed.append((request_id, stats.raw_bytes))
        mark_raw_removed(ledger, organization, removed)
        remove_pending(
            ledger,
            "raw_cleanup_pending",
            organization,
            [request_id for request_id, _ in removed],
        )
        result["raw_deleted"] += len(removed)
        result["raw_deleted_bytes"] += sum(size for _, size in removed)
    return result


def retention(ledger, organization, root, cutoff, limit, protected, apply):
    metadata = query_metadata_ids(organization)
    actual, object_errors = current_object_ids(root)
    ledger_protected = ledger_permanent_ids(ledger, organization)
    protected = protected | ledger_protected

    expired_metadata = query_expired_metadata(organization, cutoff, limit * 2)
    orphan_objects = set()
    for request_id in actual - metadata - protected:
        path = root / request_id
        try:
            if path.stat().st_mtime < cutoff.timestamp():
                orphan_objects.add(request_id)
        except FileNotFoundError:
            pass

    pending = pending_ids(ledger, "expired_pending", organization)
    selected = (expired_metadata | orphan_objects | pending) - protected
    selected = set(sorted(selected)[:limit])

    result = {
        "metadata_objects": len(metadata),
        "minio_objects": len(actual),
        **object_errors,
        "orphan_objects": len(orphan_objects),
        "expired_metadata": len(expired_metadata),
        "retention_candidates": len(selected),
        "retention_sample_ids": sorted(selected)[:20],
        "protected_from_retention": len((expired_metadata | orphan_objects | pending) & protected),
        "metadata_deleted": 0,
        "objects_deleted": 0,
    }
    if not apply or not selected:
        return result

    save_pending(ledger, "expired_pending", organization, sorted(selected))
    for offset in range(0, len(selected), 200):
        batch = sorted(selected)[offset:offset + 200]
        still_protected = protected & set(batch)
        if still_protected:
            selected -= still_protected
            remove_pending(ledger, "expired_pending", organization, still_protected)
            continue
        delete_metadata(organization, batch)
        result["metadata_deleted"] += len(batch)

    if selected:
        remove_expired_objects(ledger, organization, selected, root)
        result["objects_deleted"] = len(selected)
    return result


def clean_known_backups(cutoff):
    removed = []
    for name in ("audit-db-before-volume-fix.dump", "postgres-volume-copy"):
        path = ROOT / name
        if not path.exists() or path.is_symlink():
            continue
        if path.stat().st_mtime >= cutoff.timestamp():
            continue
        resolved = path.resolve()
        if resolved.parent != ROOT.resolve():
            raise RuntimeError("临时备份路径越界，停止清理")
        if resolved.is_dir():
            shutil.rmtree(resolved)
        else:
            resolved.unlink()
        removed.append(name)
    return removed


def main():
    parser = argparse.ArgumentParser(description="CC2 审计持久化与保留清理")
    parser.add_argument("--apply", action="store_true", help="执行实际变更，缺省仅预检")
    parser.add_argument(
        "--mode",
        choices=("all", "ledger", "retention", "raw"),
        default="all",
        help="选择持久台账、过期清理或 raw 清理阶段",
    )
    parser.add_argument("--max-expired", type=int, default=200)
    parser.add_argument("--max-raw", type=int, default=2000, help="0 表示本轮不限制")
    parser.add_argument("--remove-default-ttl", action="store_true")
    args = parser.parse_args()

    ROOT.mkdir(mode=0o700, parents=True, exist_ok=True)
    lock = (ROOT / "retention.lock").open("w")
    fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)

    organization = read_organization()
    root = request_root(organization)
    verify_schema(organization)
    if args.remove_default_ttl:
        if not args.apply:
            raise RuntimeError("移除默认过期规则需要同时指定 --apply")
        strip_body_ttl()

    started = now_utc()
    retention_cutoff = started - timedelta(days=RETENTION_DAYS)
    raw_cutoff = started - timedelta(days=RAW_RETENTION_DAYS)
    ledger = open_ledger(args.apply)
    run_id = str(uuid.uuid4())
    if args.apply:
        ledger.execute(
            """
            INSERT INTO cleanup_runs
                (run_id, organization, started_at, mode, status, summary)
            VALUES (?, ?, ?, ?, 'running', '{}')
            """,
            (run_id, organization, started.isoformat(), args.mode),
        )
        ledger.commit()

    stats = {
        "run_id": run_id,
        "time": started.isoformat(),
        "apply": args.apply,
        "mode": args.mode,
        "retention_days": RETENTION_DAYS,
        "raw_retention_days": RAW_RETENTION_DAYS,
        "permanent_user_id": PERMANENT_USER,
        "free_bytes_before": shutil.disk_usage(ROOT).free,
    }
    try:
        current_permanent = query_permanent_ids(organization)
        ledger_permanent = ledger_permanent_ids(ledger, organization)
        if not current_permanent and not ledger_permanent:
            raise RuntimeError("未发现永久审计用户，停止自动清理以防误删")
        protected = current_permanent | ledger_permanent
        if args.apply:
            persist_permanent_ids(ledger, organization, current_permanent)
        stats["permanent_current"] = len(current_permanent)
        stats["permanent_ledger"] = len(ledger_permanent)
        stats["permanent_protected"] = len(protected)
        stats["raw_state_counts"] = raw_state_counts(ledger, organization)
        if args.apply:
            stats["ledger_backup"] = backup_ledger(ledger, started)

        if args.mode in {"all", "raw"}:
            stats["raw"] = prune_permanent_raw(
                ledger,
                organization,
                root,
                raw_cutoff,
                max(0, args.max_raw),
                protected,
                args.apply,
            )

        if args.apply and shutil.disk_usage(ROOT).free < CH_MUTATION_RESERVE:
            raise RuntimeError("可用空间不足，跳过期元数据清理")

        if args.mode in {"all", "retention"}:
            stats["retention"] = retention(
                ledger,
                organization,
                root,
                retention_cutoff,
                max(1, args.max_expired),
                protected,
                args.apply,
            )

        stats["pending_expired"] = len(
            pending_ids(ledger, "expired_pending", organization)
        )
        stats["pending_raw"] = len(
            pending_ids(ledger, "raw_cleanup_pending", organization)
        )
        stats["free_bytes_after"] = shutil.disk_usage(ROOT).free
        stats["temporary_backups_removed"] = (
            clean_known_backups(retention_cutoff) if args.apply else []
        )
        if args.apply:
            ledger.execute(
                """
                UPDATE cleanup_runs
                SET finished_at=?, status='success', summary=?
                WHERE run_id=?
                """,
                (now_utc().isoformat(), json.dumps(stats, ensure_ascii=False), run_id),
            )
            ledger.commit()
            atomic_json(ROOT / "retention-status.json", stats)
        print(json.dumps(stats, ensure_ascii=False, separators=(",", ":")))
    except Exception as error:
        failure = {
            **stats,
            "error_type": type(error).__name__,
            "error": str(error)[:8000],
            "free_bytes_after": shutil.disk_usage(ROOT).free,
        }
        if args.apply:
            ledger.execute(
                """
                UPDATE cleanup_runs
                SET finished_at=?, status='failed', summary=?
                WHERE run_id=?
                """,
                (now_utc().isoformat(), json.dumps(failure, ensure_ascii=False), run_id),
            )
            ledger.commit()
            atomic_json(ROOT / "retention-status.json", failure)
        raise
    finally:
        ledger.close()


if __name__ == "__main__":
    main()
