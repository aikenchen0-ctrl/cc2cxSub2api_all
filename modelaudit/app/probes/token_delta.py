from __future__ import annotations

from typing import Any

from .common import ProbeContext, ProbeResult, correlated_chat, result


def _input_usage(payload: dict[str, Any], model: str) -> tuple[int | None, dict[str, Any]]:
    usage = payload.get("usage")
    if not isinstance(usage, dict):
        return None, {"usage_fields": []}
    anthropic_fields = (
        "input_tokens",
        "cache_creation_input_tokens",
        "cache_read_input_tokens",
    )
    if any(field in usage for field in anthropic_fields):
        values = {field: usage.get(field) for field in anthropic_fields}
        if not isinstance(values["input_tokens"], int):
            return None, {"usage_fields": list(usage.keys())}
        total = 0
        for field, value in values.items():
            if value is None:
                value = 0
            if isinstance(value, bool) or not isinstance(value, int) or value < 0:
                return None, {"usage_fields": list(usage.keys())}
            total += value
        return total, {"usage_semantics": "anthropic_disjoint_input_cache_tokens", "usage_fields": list(usage.keys())}

    total = usage.get("prompt_tokens", usage.get("input_tokens"))
    if isinstance(total, bool) or not isinstance(total, int) or total < 0:
        return None, {"usage_fields": list(usage.keys())}
    details = usage.get("prompt_tokens_details") or usage.get("input_tokens_details")
    cached = details.get("cached_tokens") if isinstance(details, dict) else None
    # OpenAI prompt_tokens already includes cached tokens; retain the total once.
    return total, {
        "usage_semantics": "openai_total_includes_cached_tokens" if cached is not None else "input_total",
        "cached_tokens_included_in_total": cached,
        "usage_fields": list(usage.keys()),
    }


def _estimate_tokens(model: str, text: str) -> tuple[int, str]:
    try:
        import tiktoken

        try:
            encoding = tiktoken.encoding_for_model(model)
            encoding_name = encoding.name
        except KeyError:
            encoding = tiktoken.get_encoding("cl100k_base")
            encoding_name = "cl100k_base_fallback"
        # Local count of content plus a bounded chat-envelope estimate for one user message.
        return len(encoding.encode(text)) + 4, encoding_name
    except Exception:
        # This is a clearly-labeled approximation for minimal dependencies and unknown encodings.
        return max(1, (len(text.encode("utf-8")) + 3) // 4) + 4, "utf8_bytes_div_4_fallback"


async def run_token_delta(context: ProbeContext) -> ProbeResult:
    if not context.profile.model:
        return result("token_delta", "not_configured", "该分组未配置探针模型")
    expected, tokenizer = _estimate_tokens(context.profile.model, "Say hi")
    call = await correlated_chat(
        context,
        probe="token_delta",
        messages=[{"role": "user", "content": "Say hi"}],
        max_tokens=8,
    )
    if not call.pinned:
        return result(
            "token_delta",
            "unpinned",
            "短请求未能确认命中目标账号，delta 作废",
            model=context.profile.model,
            expected_model=context.profile.expected_model,
            actual_account_id=call.actual_account_id,
            request_ids=(call.request_id,),
            evidence={"reason": call.pin_reason},
        )
    if call.http_status < 200 or call.http_status >= 300 or not isinstance(call.payload, dict):
        return result(
            "token_delta",
            "error",
            "短请求失败，无法比较 input token",
            model=context.profile.model,
            actual_account_id=call.actual_account_id,
            request_ids=(call.request_id,),
            evidence={"http_status": call.http_status, "error_code": call.error_code},
        )
    actual, usage_meta = _input_usage(call.payload, context.profile.model)
    if actual is None:
        return result(
            "token_delta",
            "inconclusive",
            "回包没有可核对的 input token 数",
            model=context.profile.model,
            actual_account_id=call.actual_account_id,
            request_ids=(call.request_id,),
            metrics={"expected_input_tokens": expected, "tokenizer": tokenizer},
            evidence=usage_meta,
        )

    # The prompt specifies expected - reported. Keep that signed value and classify by
    # magnitude so hidden additions (negative delta) and under-reported inputs are visible.
    delta = expected - actual
    magnitude = abs(delta)
    if magnitude > 100:
        severity = "red"
        status = "alert"
    elif magnitude > 20:
        severity = "yellow"
        status = "alert"
    else:
        severity = "normal"
        status = "ok"
    direction = "reported_input_exceeds_local_estimate" if delta < 0 else "reported_input_below_local_estimate" if delta > 0 else "equal"
    return result(
        "token_delta",
        status,
        f"|expected - actual| = {magnitude} tokens，{severity}",
        severity=severity,
        model=context.profile.model,
        expected_model=context.profile.expected_model,
        actual_account_id=call.actual_account_id,
        request_ids=(call.request_id,),
        metrics={
            "expected_input_tokens": expected,
            "actual_input_tokens": actual,
            "delta_expected_minus_actual": delta,
            "absolute_delta": magnitude,
            "direction": direction,
            "tokenizer": tokenizer,
            "thresholds": {"yellow_if_abs_gt": 20, "red_if_abs_gt": 100},
        },
        evidence=usage_meta,
    )
