from __future__ import annotations

import json
from pathlib import Path
from typing import Any

import aiosqlite

from . import db as database


def _parsed(raw: str | None) -> Any:
    if not raw:
        return {}
    try:
        return json.loads(raw)
    except (TypeError, json.JSONDecodeError):
        return {}


async def build_report(db: aiosqlite.Connection, run_id: str) -> dict[str, Any] | None:
    run = await database.fetch_one(db, "SELECT * FROM probe_runs WHERE id=?", (run_id,))
    if run is None:
        return None
    results = await database.fetch_all(
        db,
        "SELECT * FROM probe_results WHERE run_id=? ORDER BY account_id, probe",
        (run_id,),
    )
    accounts: dict[int, dict[str, Any]] = {}
    for item in results:
        account_id = int(item["account_id"])
        if account_id not in accounts:
            account = await database.fetch_one(db, "SELECT * FROM accounts WHERE account_id=?", (account_id,))
            accounts[account_id] = {
                "account": {
                    "id": account_id,
                    "name": account["name"] if account else f"account-{account_id}",
                    "platform": account["platform"] if account else "unknown",
                    "account_type": account["account_type"] if account else "unknown",
                    "status": account["status"] if account else "unknown",
                    "schedulable": bool(account["schedulable"]) if account else False,
                    "rate_multiplier": account["rate_multiplier"] if account else None,
                    "group_ids": _parsed(account["group_ids_json"] if account else "[]"),
                },
                "checks": {},
                "usage_snapshots": {},
                "gateway_calls": [],
                "alerts": [],
            }
        accounts[account_id]["checks"][item["probe"]] = {
            "status": item["status"],
            "severity": item["severity"],
            "summary": item["summary"],
            "model": item["model"],
            "expected_model": item["expected_model"],
            "actual_account_id": item["actual_account_id"],
            "request_ids": _parsed(item["request_ids_json"]),
            "metrics": _parsed(item["metrics_json"]),
            "evidence": _parsed(item["evidence_json"]),
        }

    snapshots = await database.fetch_all(
        db,
        "SELECT account_id, position, status, normalized_json FROM usage_snapshots WHERE run_id=? ORDER BY position",
        (run_id,),
    )
    for item in snapshots:
        account_id = int(item["account_id"])
        if account_id in accounts:
            accounts[account_id]["usage_snapshots"][item["position"]] = {
                "status": item["status"],
                "data": _parsed(item["normalized_json"]),
            }

    calls = await database.fetch_all(
        db,
        """SELECT request_id, account_id, probe, group_id, http_status, error_code, actual_account_id, pinned, pin_reason,
                  input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens
           FROM probe_calls WHERE run_id=? ORDER BY created_at, request_id""",
        (run_id,),
    )
    for item in calls:
        target = accounts.get(int(item["account_id"]))
        if target is None:
            continue
        target["gateway_calls"].append(
            {
                "probe": item["probe"],
                "request_id": item["request_id"],
                "group_id": item["group_id"],
                "http_status": item["http_status"],
                "error_code": item["error_code"],
                "actual_account_id": item["actual_account_id"],
                "pinned": bool(item["pinned"]),
                "pin_reason": item["pin_reason"],
                "input_tokens": item["input_tokens"],
                "output_tokens": item["output_tokens"],
                "cache_creation_tokens": item["cache_creation_tokens"],
                "cache_read_tokens": item["cache_read_tokens"],
            }
        )

    alerts = await database.fetch_all(db, "SELECT * FROM alerts WHERE run_id=? ORDER BY created_at", (run_id,))
    for item in alerts:
        account_id = int(item["account_id"])
        if account_id in accounts:
            accounts[account_id]["alerts"].append(
                {
                    "rule_id": item["rule_id"],
                    "severity": item["severity"],
                    "status": item["status"],
                    "message": item["message"],
                    "details": _parsed(item["details_json"]),
                    "action_taken": item["action_taken"],
                }
            )
    return {
        "report_version": 2,
        "generated_at": database.utc_now(),
        "run": run,
        "checks_are_separate": True,
        "accounts": list(accounts.values()),
        "credential_policy": "No upstream credentials or raw prompts are stored in this report.",
    }


async def write_report(db: aiosqlite.Connection, db_path: Path, run_id: str) -> Path | None:
    report = await build_report(db, run_id)
    if report is None:
        return None
    destination = db_path.parent / "reports" / f"{run_id}.json"
    destination.parent.mkdir(parents=True, exist_ok=True)
    destination.write_text(database.json_dump(report), encoding="utf-8")
    return destination
