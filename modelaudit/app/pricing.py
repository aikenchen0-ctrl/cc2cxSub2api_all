from __future__ import annotations

import hashlib
import json
import math
from functools import lru_cache
from pathlib import Path
from typing import Any


PRICE_TABLE_COMMIT = "081f73f021620bce438fb86a71435ec34a707d54"
PRICE_TABLE_SHA256 = "e1ed31bbf608a61c1929f19f0f41395959785bc4fde07fbc2f8284df4396d669"
PRICE_TABLE_PATH = (
    Path(__file__).resolve().parents[1]
    / "vendor"
    / "litellm"
    / "model_prices_and_context_window.json"
)
_PER_MILLION = 1_000_000


def _rate(value: Any) -> float | None:
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        return None
    parsed = float(value)
    if not math.isfinite(parsed) or parsed < 0:
        return None
    return parsed


@lru_cache(maxsize=1)
def _load_price_table() -> tuple[dict[str, Any], str]:
    try:
        raw = PRICE_TABLE_PATH.read_bytes()
    except OSError:
        return {}, "missing"
    if hashlib.sha256(raw).hexdigest() != PRICE_TABLE_SHA256:
        return {}, "sha256_mismatch"
    try:
        table = json.loads(raw)
    except (UnicodeDecodeError, json.JSONDecodeError):
        return {}, "invalid_json"
    if not isinstance(table, dict):
        return {}, "invalid_json"
    return table, "verified"


def resolve_group_pricing(
    model: str,
    pricing_model: str,
    overrides: dict[str, Any],
) -> tuple[dict[str, float], str, str, str]:
    """Resolve LiteLLM USD/token prices, then apply configured per-million overrides."""
    lookup_model = (pricing_model or model).strip()
    table, table_status = _load_price_table()
    entry = table.get(lookup_model)
    if not isinstance(entry, dict):
        entry = None
        if table_status == "verified":
            table_status = "model_not_found"

    rates: dict[str, float] = {}
    if entry is not None:
        input_rate = _rate(entry.get("input_cost_per_token"))
        output_rate = _rate(entry.get("output_cost_per_token"))
        if input_rate is not None:
            rates["input_per_million"] = input_rate * _PER_MILLION
            cache_read_rate = _rate(entry.get("cache_read_input_token_cost"))
            cache_write_rate = _rate(entry.get("cache_creation_input_token_cost"))
            # LiteLLM falls back to the ordinary input rate when a separate
            # cache price is absent from its model record.
            rates["cache_read_per_million"] = (cache_read_rate if cache_read_rate is not None else input_rate) * _PER_MILLION
            rates["cache_write_per_million"] = (cache_write_rate if cache_write_rate is not None else input_rate) * _PER_MILLION
        if output_rate is not None:
            rates["output_per_million"] = output_rate * _PER_MILLION

    used_overrides = False
    for name in (
        "input_per_million",
        "output_per_million",
        "cache_read_per_million",
        "cache_write_per_million",
    ):
        value = _rate(overrides.get(name))
        if value is not None:
            rates[name] = value
            used_overrides = True

    if entry is not None and used_overrides:
        source = "litellm_snapshot_with_manual_overrides"
    elif entry is not None:
        source = "litellm_snapshot"
    elif rates:
        source = "manual_override"
    else:
        source = "unavailable"
    return rates, source, lookup_model, table_status
