from __future__ import annotations

import hashlib
import uuid
from typing import Any, Iterable

import aiosqlite

from . import db as database
from .probes.common import AccountSnapshot, GroupProfile, ProbeCall, ProbeResult


def session_hash(session_id: str) -> str:
    return hashlib.sha256(("modelaudit:" + session_id).encode("utf-8")).hexdigest()


async def save_account(db: aiosqlite.Connection, account: AccountSnapshot) -> None:
    await db.execute(
        """INSERT INTO accounts(
           account_id, name, platform, account_type, status, schedulable,
           concurrency, current_concurrency, rate_multiplier, group_ids_json, last_seen_at
        ) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT(account_id) DO UPDATE SET
           name=excluded.name, platform=excluded.platform, account_type=excluded.account_type,
           status=excluded.status, schedulable=excluded.schedulable, concurrency=excluded.concurrency,
           current_concurrency=excluded.current_concurrency, rate_multiplier=excluded.rate_multiplier,
           group_ids_json=excluded.group_ids_json, last_seen_at=excluded.last_seen_at""",
        (
            account.account_id,
            account.name,
            account.platform,
            account.account_type,
            account.status,
            int(account.schedulable),
            account.concurrency,
            account.current_concurrency,
            account.rate_multiplier,
            database.json_dump(list(account.group_ids)),
            database.utc_now(),
        ),
    )
    await db.commit()


async def save_group_profile(db: aiosqlite.Connection, profile: GroupProfile) -> None:
    await db.execute(
        """INSERT INTO group_profiles(
           group_id, model, expected_model, cache_min_prefix_tokens,
           expected_provider_cost_multiplier, declared_cache_ratio,
           declared_completion_ratio, cache_protocol, pricing_json,
           gateway_cost_multiplier, updated_at
        ) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT(group_id) DO UPDATE SET
           model=excluded.model, expected_model=excluded.expected_model,
           cache_min_prefix_tokens=excluded.cache_min_prefix_tokens,
           expected_provider_cost_multiplier=excluded.expected_provider_cost_multiplier,
           declared_cache_ratio=excluded.declared_cache_ratio,
           declared_completion_ratio=excluded.declared_completion_ratio,
           cache_protocol=excluded.cache_protocol, pricing_json=excluded.pricing_json,
           gateway_cost_multiplier=excluded.gateway_cost_multiplier,
           updated_at=excluded.updated_at""",
        (
            profile.group_id,
            profile.model,
            profile.expected_model,
            profile.cache_min_prefix_tokens,
            profile.expected_provider_cost_multiplier,
            profile.declared_cache_ratio,
            profile.declared_completion_ratio,
            profile.cache_protocol,
            database.json_dump(profile.pricing),
            profile.gateway_cost_multiplier,
            database.utc_now(),
        ),
    )
    await db.commit()


async def save_usage_snapshot(
    db: aiosqlite.Connection,
    *,
    run_id: str,
    account_id: int,
    position: str,
    normalized: dict[str, Any] | None,
    error_code: str = "",
) -> None:
    status = "ok" if normalized is not None else "error"
    payload = normalized or {"error_code": error_code[:64]}
    await db.execute(
        "INSERT INTO usage_snapshots(id, run_id, account_id, position, status, normalized_json, created_at) "
        "VALUES(?, ?, ?, ?, ?, ?, ?)",
        (str(uuid.uuid4()), run_id, account_id, position, status, database.json_dump(payload), database.utc_now()),
    )
    await db.commit()


async def save_probe_result(
    db: aiosqlite.Connection,
    *,
    run_id: str,
    account_id: int,
    profile: GroupProfile | None,
    item: ProbeResult,
    session_id: str = "",
) -> None:
    await db.execute(
        """INSERT INTO probe_results(
          id, run_id, account_id, probe, status, severity, summary, model, expected_model,
          actual_account_id, session_id_hash, request_ids_json, metrics_json, evidence_json, created_at
        ) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)""",
        (
            str(uuid.uuid4()),
            run_id,
            account_id,
            item.probe,
            item.status,
            item.severity,
            item.summary[:400],
            item.model or (profile.model if profile else ""),
            item.expected_model or (profile.expected_model if profile else ""),
            item.actual_account_id,
            session_hash(session_id) if session_id else "",
            database.json_dump(list(item.request_ids)),
            database.json_dump(item.metrics),
            database.json_dump(item.evidence),
            database.utc_now(),
        ),
    )
    await db.commit()


async def save_probe_calls(
    db: aiosqlite.Connection,
    *,
    run_id: str,
    account_id: int,
    call_rows: Iterable[tuple[ProbeCall, int, str]],
) -> int:
    unique: dict[str, tuple[ProbeCall, int, str]] = {}
    for call, group_id, session_id in call_rows:
        unique[call.request_id] = (call, group_id, session_id)
    for call, group_id, session_id in unique.values():
        await db.execute(
            """INSERT OR IGNORE INTO probe_calls(
              request_id, run_id, account_id, group_id, probe, http_status, error_code,
              actual_account_id, pinned, pin_reason, session_id_hash, input_tokens, output_tokens,
              cache_creation_tokens, cache_read_tokens, created_at
            ) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)""",
            (
                call.request_id,
                run_id,
                account_id,
                group_id,
                call.probe,
                call.http_status,
                call.error_code[:64],
                call.actual_account_id,
                int(call.pinned),
                call.pin_reason[:64],
                session_hash(session_id),
                call.input_tokens,
                call.output_tokens,
                call.cache_creation_tokens,
                call.cache_read_tokens,
                database.utc_now(),
            ),
        )
    await db.commit()
    return len(unique)


async def save_alert(
    db: aiosqlite.Connection,
    *,
    run_id: str,
    account_id: int,
    rule_id: str,
    severity: str,
    status: str,
    message: str,
    details: dict[str, Any],
    action_taken: str = "",
) -> str:
    alert_id = str(uuid.uuid4())
    await db.execute(
        "INSERT INTO alerts(id, run_id, account_id, rule_id, severity, status, message, details_json, created_at, action_taken) "
        "VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
        (
            alert_id,
            run_id,
            account_id,
            rule_id,
            severity,
            status,
            message[:400],
            database.json_dump(details),
            database.utc_now(),
            action_taken[:120],
        ),
    )
    await db.commit()
    return alert_id


async def audit_action(
    db: aiosqlite.Connection,
    *,
    account_id: int,
    actor_sub: str,
    action: str,
    result: str,
    details: dict[str, Any],
) -> None:
    await db.execute(
        "INSERT INTO audit_actions(id, account_id, actor_sub, action, result, details_json, created_at) "
        "VALUES(?, ?, ?, ?, ?, ?, ?)",
        (
            str(uuid.uuid4()),
            account_id,
            actor_sub[:80],
            action[:80],
            result[:80],
            database.json_dump(details),
            database.utc_now(),
        ),
    )
    await db.commit()
