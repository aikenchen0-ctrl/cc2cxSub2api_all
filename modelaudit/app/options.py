from __future__ import annotations

import json
from typing import Any

import aiosqlite
from fastapi import HTTPException

from . import db as database
from .config import Settings


STOP_RULES = {
    "modeltrace_red",
}
OPTIONS_VERSION = 4


def defaults(settings: Settings) -> dict[str, Any]:
    return {
        "interval_minutes": settings.interval_minutes,
        "scheduler_enabled": settings.schedule_enabled,
        "auto_stop_enabled": settings.auto_stop_enabled,
        "options_version": OPTIONS_VERSION,
        # Auto-stop requires an explicit rule selection on every deployment.
        "stop_rules": [],
        "target_model": "",
    }


async def get_options(db: aiosqlite.Connection, settings: Settings) -> dict[str, Any]:
    row = await database.fetch_one(db, "SELECT value_json FROM settings WHERE key='runtime_options'")
    if not row:
        value = defaults(settings)
        await set_options(db, value)
        return value
    try:
        stored = json.loads(row["value_json"])
    except (TypeError, json.JSONDecodeError):
        stored = {}
    value = defaults(settings)
    if isinstance(stored, dict):
        stored_version = stored.get("options_version", 1)
        value.update(stored)
        if not isinstance(stored_version, int) or stored_version < OPTIONS_VERSION:
            # Older builds persisted all stop rules as defaults, so they do not
            # prove the operator explicitly selected those destructive actions.
            value["stop_rules"] = []
            value["options_version"] = OPTIONS_VERSION
            await set_options(db, value)
    return value


async def set_options(db: aiosqlite.Connection, value: dict[str, Any]) -> None:
    await db.execute(
        "INSERT INTO settings(key, value_json, updated_at) VALUES('runtime_options', ?, ?) "
        "ON CONFLICT(key) DO UPDATE SET value_json=excluded.value_json, updated_at=excluded.updated_at",
        (database.json_dump(value), database.utc_now()),
    )
    await db.commit()


def merge_options(current: dict[str, Any], update: dict[str, Any]) -> dict[str, Any]:
    value = dict(current)
    if "interval_minutes" in update:
        interval = update["interval_minutes"]
        if isinstance(interval, bool) or interval not in (30, 60):
            raise HTTPException(status_code=422, detail="interval_minutes_must_be_30_or_60")
        value["interval_minutes"] = interval
    for key in ("scheduler_enabled", "auto_stop_enabled"):
        if key in update:
            if not isinstance(update[key], bool):
                raise HTTPException(status_code=422, detail=f"{key}_must_be_boolean")
            value[key] = update[key]
    if "stop_rules" in update:
        rules = update["stop_rules"]
        if not isinstance(rules, list) or any(not isinstance(rule, str) or rule not in STOP_RULES for rule in rules):
            raise HTTPException(status_code=422, detail="stop_rules_invalid")
        value["stop_rules"] = sorted(set(rules))
    if "target_model" in update:
        target = update["target_model"]
        if not isinstance(target, str) or len(target) > 160:
            raise HTTPException(status_code=422, detail="target_model_invalid")
        value["target_model"] = target.strip()
    return value
