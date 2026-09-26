from __future__ import annotations

import json
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

import aiosqlite


SCHEMA = """
PRAGMA journal_mode=WAL;
PRAGMA foreign_keys=ON;
CREATE TABLE IF NOT EXISTS sso_tickets (
  jti TEXT PRIMARY KEY,
  expires_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS sessions (
  token_hash TEXT PRIMARY KEY,
  sub TEXT NOT NULL,
  email TEXT NOT NULL DEFAULT '',
  display_name TEXT NOT NULL DEFAULT '',
  expires_at INTEGER NOT NULL,
  created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS users_seen (
  sub TEXT PRIMARY KEY,
  email TEXT NOT NULL DEFAULT '',
  display_name TEXT NOT NULL DEFAULT '',
  role TEXT NOT NULL,
  last_seen_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS probe_runs (
  id TEXT PRIMARY KEY,
  trigger TEXT NOT NULL,
  status TEXT NOT NULL,
  started_at TEXT NOT NULL,
  finished_at TEXT,
  account_count INTEGER NOT NULL DEFAULT 0,
  result_count INTEGER NOT NULL DEFAULT 0,
  error_count INTEGER NOT NULL DEFAULT 0,
  note TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS accounts (
  account_id INTEGER PRIMARY KEY,
  name TEXT NOT NULL DEFAULT '',
  platform TEXT NOT NULL DEFAULT '',
  account_type TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT '',
  schedulable INTEGER NOT NULL DEFAULT 0,
  concurrency INTEGER NOT NULL DEFAULT 1,
  current_concurrency INTEGER NOT NULL DEFAULT 0,
  rate_multiplier REAL,
  group_ids_json TEXT NOT NULL DEFAULT '[]',
  last_seen_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS group_profiles (
  group_id INTEGER PRIMARY KEY,
  model TEXT NOT NULL DEFAULT '',
  expected_model TEXT NOT NULL DEFAULT '',
  cache_min_prefix_tokens INTEGER,
  expected_provider_cost_multiplier REAL,
  declared_cache_ratio REAL,
  declared_completion_ratio REAL,
  cache_protocol TEXT NOT NULL DEFAULT 'openai',
  pricing_json TEXT NOT NULL DEFAULT '{}',
  gateway_cost_multiplier REAL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS probe_results (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES probe_runs(id) ON DELETE CASCADE,
  account_id INTEGER NOT NULL,
  probe TEXT NOT NULL,
  status TEXT NOT NULL,
  severity TEXT NOT NULL,
  summary TEXT NOT NULL DEFAULT '',
  model TEXT NOT NULL DEFAULT '',
  expected_model TEXT NOT NULL DEFAULT '',
  actual_account_id INTEGER,
  session_id_hash TEXT NOT NULL DEFAULT '',
  request_ids_json TEXT NOT NULL DEFAULT '[]',
  metrics_json TEXT NOT NULL DEFAULT '{}',
  evidence_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS probe_results_account_created ON probe_results(account_id, created_at DESC);
CREATE INDEX IF NOT EXISTS probe_results_run ON probe_results(run_id, account_id, probe);
CREATE TABLE IF NOT EXISTS probe_calls (
  request_id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES probe_runs(id) ON DELETE CASCADE,
  account_id INTEGER NOT NULL,
  group_id INTEGER NOT NULL,
  probe TEXT NOT NULL,
  http_status INTEGER NOT NULL,
  error_code TEXT NOT NULL DEFAULT '',
  actual_account_id INTEGER,
  pinned INTEGER NOT NULL DEFAULT 0,
  pin_reason TEXT NOT NULL DEFAULT '',
  session_id_hash TEXT NOT NULL DEFAULT '',
  input_tokens INTEGER,
  output_tokens INTEGER,
  cache_creation_tokens INTEGER,
  cache_read_tokens INTEGER,
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS probe_calls_account_created ON probe_calls(account_id, created_at DESC);
CREATE INDEX IF NOT EXISTS probe_calls_run ON probe_calls(run_id, account_id, probe);
CREATE TABLE IF NOT EXISTS usage_snapshots (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES probe_runs(id) ON DELETE CASCADE,
  account_id INTEGER NOT NULL,
  position TEXT NOT NULL,
  status TEXT NOT NULL,
  normalized_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS alerts (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES probe_runs(id) ON DELETE CASCADE,
  account_id INTEGER NOT NULL,
  rule_id TEXT NOT NULL,
  severity TEXT NOT NULL,
  status TEXT NOT NULL,
  message TEXT NOT NULL,
  details_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  action_taken TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS alerts_account_created ON alerts(account_id, created_at DESC);
CREATE TABLE IF NOT EXISTS settings (
  key TEXT PRIMARY KEY,
  value_json TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS audit_actions (
  id TEXT PRIMARY KEY,
  account_id INTEGER,
  actor_sub TEXT NOT NULL DEFAULT '',
  action TEXT NOT NULL,
  result TEXT NOT NULL,
  details_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);
"""


def utc_now() -> str:
    return datetime.now(timezone.utc).isoformat(timespec="seconds")


def json_dump(value: Any) -> str:
    return json.dumps(value, ensure_ascii=False, separators=(",", ":"), allow_nan=False)


async def connect(path: Path) -> aiosqlite.Connection:
    path.parent.mkdir(parents=True, exist_ok=True)
    db = await aiosqlite.connect(path)
    db.row_factory = aiosqlite.Row
    await db.execute("PRAGMA busy_timeout=5000")
    return db


async def initialize(path: Path) -> None:
    db = await connect(path)
    try:
        await db.executescript(SCHEMA)
        for table, column, definition in (
            ("group_profiles", "cache_protocol", "TEXT NOT NULL DEFAULT 'openai'"),
            ("group_profiles", "pricing_json", "TEXT NOT NULL DEFAULT '{}'"),
            ("group_profiles", "gateway_cost_multiplier", "REAL"),
            ("probe_results", "summary", "TEXT NOT NULL DEFAULT ''"),
        ):
            cursor = await db.execute(f"PRAGMA table_info({table})")
            existing = {row[1] for row in await cursor.fetchall()}
            await cursor.close()
            if column not in existing:
                await db.execute(f"ALTER TABLE {table} ADD COLUMN {column} {definition}")
        await db.commit()
    finally:
        await db.close()


async def fetch_all(db: aiosqlite.Connection, sql: str, params: tuple[Any, ...] = ()) -> list[dict[str, Any]]:
    cursor = await db.execute(sql, params)
    rows = await cursor.fetchall()
    await cursor.close()
    return [dict(row) for row in rows]


async def fetch_one(db: aiosqlite.Connection, sql: str, params: tuple[Any, ...] = ()) -> dict[str, Any] | None:
    cursor = await db.execute(sql, params)
    row = await cursor.fetchone()
    await cursor.close()
    return dict(row) if row else None
