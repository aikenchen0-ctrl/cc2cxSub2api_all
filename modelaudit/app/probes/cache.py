from __future__ import annotations

import json
import math
from typing import Any

from .common import ProbeContext, ProbeResult, account_metadata_user_id, correlated_chat, probe_protocol, result


_PRICE_FIELDS = (
    "input_per_million",
    "output_per_million",
    "cache_read_per_million",
    "cache_write_per_million",
)


def _encoding(model: str) -> tuple[Any, str]:
    import tiktoken

    try:
        encoding = tiktoken.encoding_for_model(model)
        return encoding, encoding.name
    except KeyError:
        encoding = tiktoken.get_encoding("cl100k_base")
        return encoding, "cl100k_base_fallback"


def _cache_prompt(context: ProbeContext, protocol: str | None = None) -> tuple[list[dict[str, Any]], int, str] | None:
    threshold = context.profile.cache_min_prefix_tokens
    if threshold is None or threshold <= 0 or threshold >= 32_000:
        return None
    protocol = protocol or probe_protocol(context)
    try:
        encoding, encoding_name = _encoding(context.profile.model)
    except Exception:
        return None

    margin = max(128, math.ceil(threshold * (0.50 if encoding_name.endswith("_fallback") or protocol == "anthropic" else 0.15)))
    target_tokens = threshold + margin
    if target_tokens > 32_000:
        return None
    unit = "The quick brown fox jumps over the lazy dog. "
    chunks: list[str] = []
    current_count = 0
    while current_count < target_tokens:
        chunks.append(unit)
        current_count = len(encoding.encode("".join(chunks)))
        if len(chunks) > 100_000:
            return None
    prefix = "".join(chunks).rstrip()
    content = f"{prefix}\n\nSay hi."
    if protocol == "anthropic":
        message_content: Any = [
            {
                "type": "text",
                "text": content,
                "cache_control": {"type": "ephemeral"},
            }
        ]
    else:
        message_content = content
    return [{"role": "user", "content": message_content}], current_count, encoding_name


def _usage_tokens(payload: dict[str, Any], protocol: str) -> dict[str, int] | None:
    usage = payload.get("usage")
    if not isinstance(usage, dict):
        return None

    def token(name: str, default: int = 0) -> int | None:
        value = usage.get(name, default)
        if isinstance(value, bool) or not isinstance(value, int) or value < 0:
            return None
        return value

    output = token("completion_tokens")
    if output is None and "completion_tokens" not in usage:
        output = token("output_tokens")
    if output is None:
        return None

    anthropic_fields = (
        "cache_creation_input_tokens",
        "cache_creation_tokens",
        "cache_read_input_tokens",
        "cache_read_tokens",
    )
    if protocol == "anthropic" or any(field in usage for field in anthropic_fields):
        input_tokens = token("input_tokens")
        write_tokens = token("cache_creation_input_tokens")
        if write_tokens is None and "cache_creation_input_tokens" not in usage:
            write_tokens = token("cache_creation_tokens")
        if write_tokens is None and not any(key in usage for key in ("cache_creation_input_tokens", "cache_creation_tokens")):
            write_tokens = 0
        read_tokens = token("cache_read_input_tokens")
        if read_tokens is None and "cache_read_input_tokens" not in usage:
            read_tokens = token("cache_read_tokens")
        if read_tokens is None and not any(key in usage for key in ("cache_read_input_tokens", "cache_read_tokens")):
            read_tokens = 0
        if input_tokens is None or write_tokens is None or read_tokens is None:
            return None
        return {
            "input_tokens": input_tokens,
            "cache_write_tokens": write_tokens,
            "cache_read_tokens": read_tokens,
            "output_tokens": output,
        }

    prompt_tokens = token("prompt_tokens")
    if prompt_tokens is None and "prompt_tokens" not in usage:
        prompt_tokens = token("input_tokens")
    details = usage.get("prompt_tokens_details") or usage.get("input_tokens_details")
    cached = 0
    if isinstance(details, dict):
        cached_value = details.get("cached_tokens", 0)
        if isinstance(cached_value, bool) or not isinstance(cached_value, int) or cached_value < 0:
            return None
        cached = cached_value
    if prompt_tokens is None or cached > prompt_tokens:
        return None
    return {
        "input_tokens": prompt_tokens - cached,
        "cache_write_tokens": 0,
        "cache_read_tokens": cached,
        "output_tokens": output,
    }


def _prices(
    profile: Any,
    *,
    cache_write_tokens: int = 0,
    protocol: str | None = None,
) -> dict[str, float] | None:
    protocol = protocol or profile.cache_protocol
    raw = profile.pricing
    input_rate = raw.get("input_per_million")
    if isinstance(input_rate, bool) or not isinstance(input_rate, (int, float)) or not math.isfinite(float(input_rate)) or input_rate < 0:
        return None
    values = {
        "input_per_million": float(input_rate),
        "output_per_million": raw.get("output_per_million"),
        "cache_read_per_million": raw.get("cache_read_per_million"),
        "cache_write_per_million": raw.get("cache_write_per_million"),
    }
    if values["output_per_million"] is None and profile.declared_completion_ratio is not None:
        values["output_per_million"] = input_rate * profile.declared_completion_ratio
    if values["cache_read_per_million"] is None and profile.declared_cache_ratio is not None:
        values["cache_read_per_million"] = input_rate * profile.declared_cache_ratio
    if values["cache_write_per_million"] is None:
        if protocol == "openai":
            values["cache_write_per_million"] = input_rate
        elif cache_write_tokens == 0:
            values["cache_write_per_million"] = 0.0
    prices: dict[str, float] = {}
    for field in _PRICE_FIELDS:
        value = values[field]
        if isinstance(value, bool) or not isinstance(value, (int, float)) or not math.isfinite(float(value)) or value < 0:
            return None
        prices[field] = float(value)
    return prices


def _estimated_cost(tokens: dict[str, int], prices: dict[str, float], multiplier: float) -> float:
    return multiplier * (
        tokens["input_tokens"] * prices["input_per_million"]
        + tokens["cache_write_tokens"] * prices["cache_write_per_million"]
        + tokens["cache_read_tokens"] * prices["cache_read_per_million"]
        + tokens["output_tokens"] * prices["output_per_million"]
    ) / 1_000_000


async def run_cache(context: ProbeContext) -> ProbeResult:
    protocol = probe_protocol(context)
    prompt = _cache_prompt(context, protocol)
    if prompt is None:
        return result(
            "cache",
            "not_configured",
            "Cache probing needs cache_min_prefix_tokens between 1 and 32000 and an available tokenizer.",
        )
    messages, estimated_prefix_tokens, tokenizer = prompt
    wire_body = {
        "model": context.profile.model,
        "messages": messages,
        "max_tokens": 8,
        "stream": False,
        "metadata": {
            "user_id": account_metadata_user_id(
                context.settings.session_secret,
                context.account.account_id,
                context.session_id,
            )
        },
    }
    body_bytes = json.dumps(wire_body, ensure_ascii=False, separators=(",", ":")).encode("utf-8")
    body_sha256 = __import__("hashlib").sha256(body_bytes).hexdigest()

    calls = []
    parsed_usages: list[dict[str, int] | None] = []
    for _ in range(2):
        call = await correlated_chat(
            context,
            probe="cache",
            messages=messages,
            max_tokens=8,
            serialized_body=body_bytes,
        )
        calls.append(call)
        if not call.pinned:
            return result(
                "cache",
                "unpinned",
                "Cache replay was discarded because the target account was not verified.",
                model=context.profile.model,
                expected_model=context.profile.expected_model,
                actual_account_id=call.actual_account_id,
                request_ids=tuple(item.request_id for item in calls),
                evidence={"reason": call.pin_reason, "request_body_sha256": body_sha256},
            )
        if call.http_status < 200 or call.http_status >= 300 or not isinstance(call.payload, dict):
            return result(
                "cache",
                "error",
                "A cache replay request failed.",
                model=context.profile.model,
                expected_model=context.profile.expected_model,
                actual_account_id=call.actual_account_id,
                request_ids=tuple(item.request_id for item in calls),
                evidence={"http_status": call.http_status, "error_code": call.error_code or "invalid_response"},
            )
        parsed_usages.append(_usage_tokens(call.payload, protocol))

    first_usage, second_usage = parsed_usages
    cache_status = "inconclusive"
    cache_severity = "none"
    if first_usage is not None and second_usage is not None:
        if second_usage["cache_read_tokens"] > 0:
            cache_status = "ok"
            cache_severity = "normal"
        else:
            cache_status = "alert"
            cache_severity = "red"

    total_cache_write_tokens = sum(
        usage["cache_write_tokens"] for usage in parsed_usages if usage is not None
    )
    prices = _prices(context.profile, cache_write_tokens=total_cache_write_tokens, protocol=protocol)
    cost_status = "inconclusive"
    cost_severity = "none"
    pricing_reference = {
        "pricing_model": context.profile.pricing_model,
        "pricing_source": context.profile.pricing_source,
        "price_table_revision": context.profile.price_table_revision,
        "price_table_status": context.profile.price_table_status,
    }
    cost_metrics: dict[str, Any] = {
        "status": cost_status,
        "reason": "pricing_or_usage_unavailable",
        **pricing_reference,
    }
    if prices is not None and first_usage is not None and second_usage is not None:
        multiplier = context.profile.gateway_cost_multiplier
        if multiplier is None:
            multiplier = 1.0
        expected_costs = [
            _estimated_cost(tokens, prices, multiplier)
            for tokens in (first_usage, second_usage)
        ]
        expected_total = sum(expected_costs)
        if expected_total <= 0:
            cost_metrics = {
                "status": "inconclusive",
                "reason": "configured_expected_cost_is_zero",
                "expected_per_call": expected_costs,
                **pricing_reference,
            }
        else:
            # The allowed Sub2API account endpoints expose provider usage snapshots,
            # not per-request gateway cost. Do not add the admin usage-log endpoint
            # or infer an invoice delta from potentially concurrent account traffic.
            cost_metrics = {
                "status": "inconclusive",
                "reason": "per_request_gateway_cost_unavailable_from_allowed_endpoints",
                "expected_gateway_cost": expected_total,
                "expected_per_call": expected_costs,
                "pricing_basis": "configured model prices and response usage",
                **pricing_reference,
            }

    severities = {cache_severity, cost_severity}
    severity = "red" if "red" in severities else "yellow" if "yellow" in severities else "normal" if "normal" in severities else "none"
    if "alert" in {cache_status, cost_status}:
        overall_status = "alert"
    elif cache_status == "ok" and cost_status == "ok":
        overall_status = "ok"
    else:
        overall_status = "inconclusive"

    cache_metrics = {
        "status": cache_status,
        "protocol": protocol,
        "configured_cache_protocol": context.profile.cache_protocol,
        "configured_protocol_matches_route": context.profile.cache_protocol == protocol,
        "cache_min_prefix_tokens": context.profile.cache_min_prefix_tokens,
        "estimated_prefix_tokens": estimated_prefix_tokens,
        "tokenizer": tokenizer,
        "first_cache_write_tokens": first_usage["cache_write_tokens"] if first_usage else None,
        "second_cache_read_tokens": second_usage["cache_read_tokens"] if second_usage else None,
        "second_cache_write_tokens": second_usage["cache_write_tokens"] if second_usage else None,
        "request_bodies_identical": True,
        "request_body_sha256": body_sha256,
        "declared_cache_ratio": context.profile.declared_cache_ratio,
        "declared_completion_ratio": context.profile.declared_completion_ratio,
        **pricing_reference,
    }
    summary = f"Cache: {cache_status}; cost: {cost_status}."
    return result(
        "cache",
        overall_status,
        summary,
        severity=severity,
        model=context.profile.model,
        expected_model=context.profile.expected_model,
        actual_account_id=context.account.account_id,
        request_ids=tuple(call.request_id for call in calls),
        metrics={"cache_check": cache_metrics, "cost_check": cost_metrics},
    )
