from __future__ import annotations

import math
from typing import Any


def _finite_number(value: Any) -> float | None:
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        return None
    parsed = float(value)
    return parsed if math.isfinite(parsed) else None


def _finite_integer(value: Any) -> int | None:
    parsed = _finite_number(value)
    if parsed is None or not parsed.is_integer():
        return None
    return int(parsed)


def normalize_usage(raw: dict[str, Any]) -> dict[str, Any]:
    """Keep safe provider quota counters and summaries; never persist raw payloads."""
    has_error = bool(raw.get("error") or raw.get("error_code"))
    normalized: dict[str, Any] = {
        "source": str(raw.get("source") or "")[:40],
        # An upstream error_code is untrusted text and can contain credentials.
        # Keep a stable marker instead of persisting its contents.
        "error_code": "upstream_usage_error" if has_error else "",
        "has_error": has_error,
        "subscription_tier": str(raw.get("subscription_tier") or "")[:40],
    }
    for name in (
        "five_hour",
        "seven_day",
        "seven_day_sonnet",
        "seven_day_fable",
        "gemini_shared_daily",
        "gemini_pro_daily",
        "gemini_flash_daily",
        "thirty_day",
    ):
        value = raw.get(name)
        if isinstance(value, dict):
            utilization = _finite_number(value.get("utilization"))
            if utilization is not None:
                normalized[name] = {"utilization_percent": utilization}

    quota_state = raw.get("grok_quota_snapshot_state")
    if isinstance(quota_state, str):
        normalized["grok_quota_snapshot_state"] = quota_state[:40]
    headers_seen = raw.get("grok_last_headers_seen_at")
    if isinstance(headers_seen, str):
        normalized["grok_last_headers_seen_at"] = headers_seen[:80]

    token_quota = raw.get("grok_token_quota")
    if isinstance(token_quota, dict):
        safe_quota: dict[str, Any] = {}
        for field in ("limit", "remaining", "reset_unix"):
            value = _finite_integer(token_quota.get(field))
            if value is not None and value >= 0:
                safe_quota[field] = value
        reset_at = token_quota.get("reset_at")
        if isinstance(reset_at, str):
            safe_quota["reset_at"] = reset_at[:80]
        if safe_quota:
            normalized["grok_token_quota"] = safe_quota

    billing = raw.get("grok_billing")
    if isinstance(billing, dict):
        safe_billing: dict[str, Any] = {}
        for source, target in (
            ("period_type", "period_type"),
            ("billing_period_start", "period_start"),
            ("billing_period_end", "period_end"),
            ("source", "source"),
        ):
            value = billing.get(source)
            if isinstance(value, str):
                safe_billing[target] = value[:80]
        for source, target in (
            ("status_code", "status_code"),
            ("monthly_status_code", "monthly_status_code"),
        ):
            value = billing.get(source)
            if isinstance(value, int) and not isinstance(value, bool):
                safe_billing[target] = value
        for source, target in (
            ("monthly_used", "monthly_used_usd"),
            ("monthly_limit", "monthly_limit_usd"),
            ("on_demand_used", "on_demand_used_usd"),
            ("on_demand_cap", "on_demand_cap_usd"),
        ):
            value = _finite_number(billing.get(source))
            if value is not None:
                safe_billing[target] = value
        if "partial" in billing:
            safe_billing["partial"] = bool(billing.get("partial"))
        if safe_billing:
            normalized["grok_billing"] = safe_billing
    return normalized


def supplier_money_counter(snapshot: dict[str, Any]) -> tuple[str, float, str] | None:
    """Return a comparable supplier invoice counter only for a complete Grok USD snapshot."""
    billing = snapshot.get("grok_billing")
    if not isinstance(billing, dict):
        return None
    if billing.get("status_code") != 200 or billing.get("partial") is True:
        return None
    amount = _finite_number(billing.get("monthly_used_usd"))
    period = billing.get("period_start")
    if amount is None or not isinstance(period, str) or not period:
        return None
    return "grok_monthly_usd", amount, period


def supplier_token_counter(snapshot: dict[str, Any]) -> dict[str, int | str | None] | None:
    """Return Grok token quota only when Sub2API observed real upstream headers."""
    if snapshot.get("grok_quota_snapshot_state") not in {"observed", "billing_observed"}:
        return None
    headers_seen = snapshot.get("grok_last_headers_seen_at")
    quota = snapshot.get("grok_token_quota")
    if not isinstance(headers_seen, str) or not headers_seen or not isinstance(quota, dict):
        return None
    limit = _finite_integer(quota.get("limit"))
    remaining = _finite_integer(quota.get("remaining"))
    if limit is None or remaining is None or limit <= 0 or remaining < 0 or remaining > limit:
        return None
    reset_unix = _finite_integer(quota.get("reset_unix"))
    reset_at = quota.get("reset_at") if isinstance(quota.get("reset_at"), str) else None
    return {
        "limit": limit,
        "remaining": remaining,
        "reset_unix": reset_unix,
        "reset_at": reset_at,
        "headers_seen_at": headers_seen,
    }


def supplier_token_delta(
    before: dict[str, Any], after: dict[str, Any]
) -> tuple[int | None, str]:
    """Return provider token quota consumption between fresh, matching windows."""
    first = supplier_token_counter(before)
    second = supplier_token_counter(after)
    if first is None or second is None:
        return None, "upstream_token_quota_unavailable"
    if first["limit"] != second["limit"]:
        return None, "token_quota_window_changed"
    first_reset = first["reset_unix"] or first["reset_at"]
    second_reset = second["reset_unix"] or second["reset_at"]
    if first_reset != second_reset:
        return None, "token_quota_window_changed"
    if first["headers_seen_at"] == second["headers_seen_at"]:
        return None, "upstream_token_headers_not_refreshed"
    consumed = int(first["remaining"]) - int(second["remaining"])
    if consumed <= 0:
        return None, "token_quota_delta_not_positive"
    return consumed, ""
