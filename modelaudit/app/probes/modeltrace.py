from __future__ import annotations

import hashlib
import importlib.util
import math
from pathlib import Path
from typing import Any

from .common import ProbeContext, ProbeResult, correlated_chat, result
from .trace_calibration import load_candidate_calibration


def _fingerprint_path() -> Path:
    candidates = [
        Path(__file__).resolve().parents[3] / "ModelTrace" / "fingerprint.py",
        Path(__file__).resolve().parents[2] / "vendor" / "modeltrace" / "fingerprint.py",
    ]
    source = next((path for path in candidates if path.is_file()), None)
    if source is None:
        raise RuntimeError("ModelTrace fingerprint.py is missing")
    return source


def _load_fingerprint_module() -> Any:
    source = _fingerprint_path()
    spec = importlib.util.spec_from_file_location("modelaudit_modeltrace_fingerprint", source)
    if spec is None or spec.loader is None:
        raise RuntimeError("ModelTrace fingerprint module could not be loaded")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def _bank_path() -> Path:
    candidates = [
        Path(__file__).resolve().parents[3] / "ModelTrace" / "data" / "unified_bank.json",
        Path(__file__).resolve().parents[2] / "vendor" / "modeltrace" / "data" / "unified_bank.json",
    ]
    bank = next((path for path in candidates if path.is_file()), None)
    if bank is None:
        raise RuntimeError("ModelTrace unified fingerprint bank is missing")
    return bank


def _message_text(payload: dict[str, Any]) -> tuple[str, str]:
    # Native Anthropic Messages response. OAuth/setup-token probes use this path
    # so the gateway does not add its Claude Code system prompt to the challenge.
    content = payload.get("content")
    if isinstance(content, list):
        stop_reason = str(payload.get("stop_reason") or "")
        if stop_reason == "refusal":
            return "", "refusal"
        if stop_reason == "max_tokens":
            finish_reason = "length"
        else:
            finish_reason = stop_reason
        parts = [
            str(part.get("text") or "")
            for part in content
            if isinstance(part, dict) and part.get("type") == "text"
        ]
        if parts:
            return "".join(parts), finish_reason

    choices = payload.get("choices")
    if not isinstance(choices, list) or not choices or not isinstance(choices[0], dict):
        return "", "invalid_response"
    choice = choices[0]
    finish_reason = str(choice.get("finish_reason") or "")
    message = choice.get("message")
    if not isinstance(message, dict):
        return "", "invalid_response"
    if message.get("refusal"):
        return "", "refusal"
    content = message.get("content")
    if isinstance(content, str):
        return content, finish_reason
    if isinstance(content, list):
        return "".join(str(part.get("text") or "") for part in content if isinstance(part, dict)), finish_reason
    return "", "invalid_response"


def _local_token_count(model: str, text: str, *, chat_message: bool = False) -> tuple[int, str]:
    """Estimate text tokens locally; this deliberately never reads response usage."""
    try:
        import tiktoken

        try:
            encoding = tiktoken.encoding_for_model(model)
            encoding_name = encoding.name
        except KeyError:
            encoding = tiktoken.get_encoding("cl100k_base")
            encoding_name = "cl100k_base_fallback"
        count = len(encoding.encode(text))
    except Exception:
        count = max(0, (len(text.encode("utf-8")) + 3) // 4)
        encoding_name = "utf8_bytes_div_4_fallback"
    if chat_message:
        count += 4
    return count, encoding_name


def _valid_price(value: Any) -> float | None:
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        return None
    parsed = float(value)
    return parsed if math.isfinite(parsed) and parsed >= 0 else None


def _modeltrace_cost_metrics(
    context: ProbeContext,
    challenge_calls: list[tuple[str, str | None, bool]],
) -> dict[str, Any]:
    """Record a price-table estimate without consulting ModelTrace response usage."""
    input_tokens = 0
    output_tokens = 0
    tokenizers: set[str] = set()
    attributed_calls = 0
    incomplete_output = False
    for prompt, response_text, pinned in challenge_calls:
        if not pinned:
            continue
        attributed_calls += 1
        prompt_tokens, prompt_tokenizer = _local_token_count(
            context.profile.model, prompt, chat_message=True
        )
        input_tokens += prompt_tokens
        tokenizers.add(prompt_tokenizer)
        if response_text is None:
            incomplete_output = True
            continue
        response_token_count, response_tokenizer = _local_token_count(
            context.profile.model, response_text
        )
        output_tokens += response_token_count
        tokenizers.add(response_tokenizer)

    metrics: dict[str, Any] = {
        "estimated_modeltrace_challenges": len(challenge_calls),
        "estimated_modeltrace_attributed_challenges": attributed_calls,
        "estimated_modeltrace_input_tokens": input_tokens,
        "estimated_modeltrace_output_tokens": output_tokens,
        "estimated_modeltrace_tokenizers": sorted(tokenizers),
        "estimated_modeltrace_cost_basis": "local_tokenization_and_configured_price_snapshot; response_usage_not_read",
        "estimated_modeltrace_pricing_source": context.profile.pricing_source,
        "estimated_modeltrace_price_table_revision": context.profile.price_table_revision,
        "estimated_modeltrace_price_table_status": context.profile.price_table_status,
    }
    if attributed_calls == 0:
        metrics.update(
            {
                "estimated_modeltrace_cost_usd": None,
                "estimated_modeltrace_cost_status": "not_attributed",
            }
        )
        return metrics

    pricing = context.profile.pricing
    input_rate = _valid_price(pricing.get("input_per_million"))
    output_rate = _valid_price(pricing.get("output_per_million"))
    if output_rate is None and input_rate is not None:
        completion_ratio = _valid_price(context.profile.declared_completion_ratio)
        if completion_ratio is not None:
            output_rate = input_rate * completion_ratio
    multiplier = _valid_price(context.profile.gateway_cost_multiplier)
    if multiplier is None:
        multiplier = 1.0 if context.profile.gateway_cost_multiplier is None else None
    if input_rate is None or output_rate is None or multiplier is None:
        metrics.update(
            {
                "estimated_modeltrace_cost_usd": None,
                "estimated_modeltrace_cost_status": "price_or_multiplier_unavailable",
            }
        )
        return metrics

    estimate = multiplier * (
        input_tokens * input_rate + output_tokens * output_rate
    ) / 1_000_000
    metrics.update(
        {
            "estimated_modeltrace_cost_usd": estimate,
            "estimated_modeltrace_cost_status": "partial_estimate" if incomplete_output else "estimated",
        }
    )
    return metrics


def _ood_decision(
    analysis: dict[str, Any],
    context: ProbeContext,
    calibration: dict[str, Any] | None,
    calibration_status: str,
    bank_hash: str,
    fingerprint_hash: str,
) -> tuple[bool, str, dict[str, Any]]:
    ranked = analysis.get("results") or []
    if not ranked:
        return True, "empty_classifier_result", {}
    top = ranked[0]
    candidate = str(top.get("model") or "")
    second_probability = float(ranked[1].get("probability", 0.0)) if len(ranked) > 1 else 0.0
    probability = float(top.get("probability", 0.0))
    similarity = float(top.get("profile_similarity", 0.0))
    margin = probability - second_probability

    if calibration is None:
        return True, f"candidate_calibration_{calibration_status}", {
            "candidate_count": len(ranked),
            "candidate_probability": probability,
            "candidate_profile_similarity": similarity,
            "candidate_margin": margin,
            "calibration_status": calibration_status,
            "calibration_bank_sha256": bank_hash,
            "fingerprint_sha256": fingerprint_hash,
        }

    candidate_thresholds = calibration["candidate_minimum_profile_similarity"]
    calibrated_similarity = candidate_thresholds.get(candidate)
    if calibrated_similarity is None:
        return True, "candidate_similarity_threshold_missing", {
            "candidate_count": len(ranked),
            "candidate_probability": probability,
            "candidate_profile_similarity": similarity,
            "candidate_margin": margin,
            "calibration_status": calibration_status,
            "calibration_bank_sha256": bank_hash,
            "fingerprint_sha256": fingerprint_hash,
        }

    calibrated_probability = float(calibration["minimum_probability"])
    calibrated_margin = float(calibration["minimum_margin"])
    if (
        context.settings.trace_min_probability < calibrated_probability
        or context.settings.trace_min_margin < calibrated_margin
    ):
        return True, "runtime_confidence_threshold_below_calibrated_floor", {
            "candidate_count": len(ranked),
            "candidate_probability": probability,
            "candidate_profile_similarity": similarity,
            "candidate_margin": margin,
            "calibrated_minimum_probability": calibrated_probability,
            "configured_minimum_probability": context.settings.trace_min_probability,
            "calibrated_minimum_margin": calibrated_margin,
            "configured_minimum_margin": context.settings.trace_min_margin,
            "calibration_status": calibration_status,
            "calibration_bank_sha256": bank_hash,
            "fingerprint_sha256": fingerprint_hash,
        }

    minimum_similarity = max(
        float(calibrated_similarity),
        context.settings.trace_min_profile_similarity,
    )
    thresholds = {
        "minimum_probability": max(context.settings.trace_min_probability, calibrated_probability),
        "minimum_profile_similarity": minimum_similarity,
        "minimum_margin": context.settings.trace_min_margin,
    }
    failed = []
    if probability < thresholds["minimum_probability"]:
        failed.append("low_probability")
    if similarity < thresholds["minimum_profile_similarity"]:
        failed.append("low_profile_similarity")
    if margin < thresholds["minimum_margin"]:
        failed.append("small_candidate_margin")
    metrics = {
        "candidate": candidate,
        "candidate_probability": probability,
        "candidate_profile_similarity": similarity,
        "candidate_margin": margin,
        "thresholds": thresholds,
        "threshold_source": "candidate_omitted_model_calibration",
        "calibration_bank_sha256": bank_hash,
        "calibration_fingerprint_sha256": calibration.get("fingerprint_sha256"),
        "calibration_method": calibration.get("method"),
        "calibration_similarity_margin": calibration.get("similarity_margin"),
        "other_candidates": [
            {"model": item.get("model"), "probability": item.get("probability")}
            for item in ranked[1:4]
        ],
        "candidate_gates_passed": not failed,
    }
    return bool(failed), ",".join(failed), metrics


async def run_modeltrace_async(context: ProbeContext) -> ProbeResult:
    if not context.profile.model:
        return result("modeltrace", "not_configured", "该分组未配置探针模型", model=context.profile.model)

    try:
        fingerprint = _load_fingerprint_module()
        bank_path = _bank_path()
        bank = fingerprint.load_bank(bank_path)
        bank_hash = hashlib.sha256(bank_path.read_bytes()).hexdigest()
        fingerprint_hash = hashlib.sha256(_fingerprint_path().read_bytes()).hexdigest()
        calibration, calibration_status = load_candidate_calibration(bank_hash, fingerprint_hash)
    except Exception as exc:
        return result("modeltrace", "error", "ModelTrace 指纹库不可用", evidence={"error_code": type(exc).__name__})

    bank_models = bank.get("models")
    bank_model_ids = {
        str(item.get("id") or "").strip()
        for item in bank_models
        if isinstance(item, dict) and str(item.get("id") or "").strip()
    } if isinstance(bank_models, list) else set()
    expected = context.profile.expected_model.strip()
    if expected and bank_model_ids and expected not in bank_model_ids:
        return result(
            "modeltrace",
            "unknown",
            "期望型号不在 ModelTrace 指纹库中，无法安全判断型号是否变化",
            model=context.profile.model,
            expected_model=expected,
            severity="yellow",
            metrics={
                "candidate_count": len(bank_model_ids),
                "reference_model_count": len(bank_model_ids),
                "expected_model_in_reference_bank": False,
                "bank_sha256": bank_hash,
                "fingerprint_sha256": fingerprint_hash,
                "bank_built_at": bank.get("built_at"),
            },
            evidence={"reason": "expected_model_not_in_reference_bank"},
        )
    if calibration is None:
        return result(
            "modeltrace",
            "unknown",
            "ModelTrace 置信度校准文件缺失或与指纹代码不匹配，未发送挑战",
            model=context.profile.model,
            expected_model=expected,
            severity="yellow",
            metrics={
                "candidate_count": len(bank_model_ids),
                "calibration_status": calibration_status,
                "bank_sha256": bank_hash,
                "fingerprint_sha256": fingerprint_hash,
                "bank_built_at": bank.get("built_at"),
            },
            evidence={"reason": "candidate_calibration_unavailable"},
        )

    challenges = fingerprint.generate_challenges(3)

    outputs: list[dict[str, Any]] = []
    request_ids: list[str] = []
    challenge_calls: list[tuple[str, str | None, bool]] = []
    model = context.profile.model
    for challenge in challenges:
        prompt = str(challenge["prompt"])
        call = await correlated_chat(
            context,
            probe="modeltrace",
            messages=[{"role": "user", "content": prompt}],
            max_tokens=4096,
        )
        request_ids.append(call.request_id)
        challenge_calls.append((prompt, None, call.pinned))
        if not call.pinned:
            return result(
                "modeltrace",
                "unpinned",
                "该挑战未能确认命中目标账号，归因作废",
                model=model,
                expected_model=context.profile.expected_model,
                actual_account_id=call.actual_account_id,
                request_ids=tuple(request_ids),
                metrics=_modeltrace_cost_metrics(context, challenge_calls),
                evidence={"reason": call.pin_reason, "bank_sha256": bank_hash},
            )
        if call.http_status < 200 or call.http_status >= 300 or not isinstance(call.payload, dict):
            return result(
                "modeltrace",
                "error",
                "ModelTrace 挑战请求未成功完成",
                model=model,
                expected_model=context.profile.expected_model,
                request_ids=tuple(request_ids),
                metrics=_modeltrace_cost_metrics(context, challenge_calls),
                evidence={"http_status": call.http_status, "error_code": call.error_code or "invalid_response", "bank_sha256": bank_hash},
            )
        text, finish_reason = _message_text(call.payload)
        challenge_calls[-1] = (prompt, text if text else None, call.pinned)
        numbers = fingerprint.parse_numbers(text)
        expected_count = int(challenge.get("expected_count") or 0)
        minimum = max(80, math.ceil(expected_count * 0.55))
        if finish_reason in {"length", "content_filter"} or len(numbers) < minimum:
            return result(
                "modeltrace",
                "unknown",
                "挑战回答不足以支持模型归因",
                model=model,
                expected_model=context.profile.expected_model,
                request_ids=tuple(request_ids),
                metrics={
                    **_modeltrace_cost_metrics(context, challenge_calls),
                    "challenge_count": len(challenges),
                    "valid_challenges": len(outputs),
                    "received_numbers": len(numbers),
                    "minimum_numbers": minimum,
                },
                evidence={"reason": "truncated_or_insufficient_numbers", "finish_reason": finish_reason, "bank_sha256": bank_hash},
            )
        outputs.append({"text": text, "expected_count": expected_count})

    try:
        analysis = fingerprint.analyze_global_outputs(outputs, bank)
        is_unknown, unknown_reason, gate_metrics = _ood_decision(
            analysis,
            context,
            calibration,
            calibration_status,
            bank_hash,
            fingerprint_hash,
        )
    except Exception as exc:
        return result(
            "modeltrace",
            "unknown",
            "ModelTrace 无法完成闭集归因",
            model=model,
            expected_model=context.profile.expected_model,
            request_ids=tuple(request_ids),
            metrics=_modeltrace_cost_metrics(context, challenge_calls),
            evidence={"error_code": type(exc).__name__, "bank_sha256": bank_hash},
        )

    prediction = str(analysis.get("prediction") or "")
    metrics = {
        **gate_metrics,
        **_modeltrace_cost_metrics(context, challenge_calls),
        "observed_candidate": prediction,
        "observed_family": analysis.get("family_prediction"),
        "family_probability": analysis.get("family_probability"),
        "probabilities": analysis.get("results", []),
        "valid_challenges": 3,
        "bank_sha256": bank_hash,
        "bank_built_at": bank.get("built_at"),
    }
    if is_unknown:
        candidate_differs_from_expected = bool(expected and prediction != expected)
        if candidate_differs_from_expected:
            summary = "闭集首选与期望型号不一致，但证据未达识别阈值；结果标为 unknown，候选型号名已隐藏"
            severity = "yellow"
        elif expected:
            summary = "闭集首选与期望型号一致，但证据未达确认阈值；结果标为 unknown"
            severity = "none"
        else:
            summary = "指纹与候选库的匹配证据不足，按 unknown 处理"
            severity = "none"
        unknown_metrics = {
            **_modeltrace_cost_metrics(context, challenge_calls),
            "candidate_count": len(analysis.get("results") or []),
            "best_probability": gate_metrics.get("candidate_probability"),
            "best_profile_similarity": gate_metrics.get("candidate_profile_similarity"),
            "best_candidate_margin": gate_metrics.get("candidate_margin"),
            "thresholds": gate_metrics.get("thresholds"),
            "threshold_source": gate_metrics.get("threshold_source"),
            "calibration_status": gate_metrics.get("calibration_status", "verified"),
            "calibration_bank_sha256": gate_metrics.get("calibration_bank_sha256", bank_hash),
            "fingerprint_sha256": gate_metrics.get("fingerprint_sha256", fingerprint_hash),
            "candidate_gates_passed": False,
            "valid_challenges": 3,
            "bank_sha256": bank_hash,
            "bank_built_at": bank.get("built_at"),
        }
        if expected:
            unknown_metrics["closed_set_candidate_differs_from_expected"] = candidate_differs_from_expected
            expected_result = next(
                (item for item in analysis.get("results", []) if item.get("model") == expected),
                None,
            )
            if isinstance(expected_result, dict):
                unknown_metrics["expected_model_probability"] = expected_result.get("probability")
                unknown_metrics["expected_model_profile_similarity"] = expected_result.get("profile_similarity")
        return result(
            "modeltrace",
            "unknown",
            summary,
            model=model,
            expected_model=expected,
            severity=severity,
            request_ids=tuple(request_ids),
            metrics=unknown_metrics,
            evidence={"reason": unknown_reason},
        )
    if not expected:
        return result(
            "modeltrace",
            "ok",
            "已获得高置信模型指纹；未配置期望型号，不能判定是否暗改",
            model=model,
            request_ids=tuple(request_ids),
            metrics=metrics,
            evidence={"method": analysis.get("method"), "calibration": analysis.get("calibration")},
        )
    matches = prediction == expected
    return result(
        "modeltrace",
        "ok" if matches else "alert",
        "指纹与期望型号一致" if matches else "高置信指纹与期望型号不一致",
        severity="normal" if matches else "red",
        model=model,
        expected_model=expected,
        request_ids=tuple(request_ids),
        metrics=metrics,
        evidence={"reason": "candidate_matches_expectation" if matches else "candidate_differs_from_expectation", "method": analysis.get("method"), "calibration": analysis.get("calibration")},
    )


async def run_modeltrace_with_confirmation(context: ProbeContext) -> ProbeResult:
    """Repeat only a high-confidence mismatch; permit auto-stop only if it reproduces."""
    first = await run_modeltrace_async(context)
    if first.status != "alert" or first.severity != "red":
        return first

    second = await run_modeltrace_async(context)
    first_candidate = first.metrics.get("observed_candidate")
    second_candidate = second.metrics.get("observed_candidate")
    confirmed = (
        second.status == "alert"
        and second.severity == "red"
        and isinstance(first_candidate, str)
        and bool(first_candidate)
        and first_candidate == second_candidate
        and first.expected_model == second.expected_model
    )
    confirmation = {
        "status": second.status,
        "severity": second.severity,
        "observed_candidate": second_candidate,
        "confirmed_same_mismatch": confirmed,
        "request_ids": list(second.request_ids),
        "estimated_modeltrace_cost_usd": second.metrics.get("estimated_modeltrace_cost_usd"),
        "estimated_modeltrace_cost_status": second.metrics.get("estimated_modeltrace_cost_status"),
        "estimated_modeltrace_challenges": second.metrics.get("estimated_modeltrace_challenges"),
    }
    summary = first.summary
    if confirmed:
        summary += "；独立第二轮挑战确认"
    else:
        summary += "；独立第二轮挑战未确认，禁止自动停用"
    cost_metrics = _combine_modeltrace_cost_metrics(first.metrics, second.metrics)
    return result(
        "modeltrace",
        "alert",
        summary,
        severity="red",
        model=first.model,
        expected_model=first.expected_model,
        actual_account_id=first.actual_account_id,
        request_ids=tuple(dict.fromkeys((*first.request_ids, *second.request_ids))),
        metrics={
            **first.metrics,
            **cost_metrics,
            "confirmation": confirmation,
            "auto_stop_eligible": confirmed,
        },
        evidence={
            "first_challenge": first.evidence,
            "confirmation": second.evidence,
            "reason": "same_high_confidence_mismatch_confirmed" if confirmed else "high_confidence_mismatch_not_confirmed",
        },
    )


def _combine_modeltrace_cost_metrics(first: dict[str, Any], second: dict[str, Any]) -> dict[str, Any]:
    numeric_keys = (
        "estimated_modeltrace_challenges",
        "estimated_modeltrace_attributed_challenges",
        "estimated_modeltrace_input_tokens",
        "estimated_modeltrace_output_tokens",
    )
    combined: dict[str, Any] = {}
    for key in numeric_keys:
        values = [first.get(key), second.get(key)]
        present = [value for value in values if isinstance(value, int) and not isinstance(value, bool)]
        if present:
            combined[key] = sum(present)
    tokenizers = sorted(
        {
            tokenizer
            for metrics in (first, second)
            for tokenizer in metrics.get("estimated_modeltrace_tokenizers", [])
            if isinstance(tokenizer, str)
        }
    )
    if tokenizers:
        combined["estimated_modeltrace_tokenizers"] = tokenizers

    costs = [first.get("estimated_modeltrace_cost_usd"), second.get("estimated_modeltrace_cost_usd")]
    valid_costs = [value for value in costs if isinstance(value, (int, float)) and not isinstance(value, bool) and math.isfinite(float(value))]
    if valid_costs:
        combined["estimated_modeltrace_cost_usd"] = sum(float(value) for value in valid_costs)
    statuses = [
        status
        for status in (first.get("estimated_modeltrace_cost_status"), second.get("estimated_modeltrace_cost_status"))
        if isinstance(status, str)
    ]
    if any(status == "partial_estimate" for status in statuses) or (valid_costs and len(valid_costs) != len(costs)):
        combined["estimated_modeltrace_cost_status"] = "partial_estimate"
    elif len(valid_costs) == 2 and all(status == "estimated" for status in statuses):
        combined["estimated_modeltrace_cost_status"] = "estimated"
    elif statuses and all(status == statuses[0] for status in statuses):
        combined["estimated_modeltrace_cost_status"] = statuses[0]
    elif statuses:
        combined["estimated_modeltrace_cost_status"] = "unavailable"
    if first.get("estimated_modeltrace_cost_basis") or second.get("estimated_modeltrace_cost_basis"):
        combined["estimated_modeltrace_cost_basis"] = "local_tokenization_and_configured_price_snapshot; response_usage_not_read"
    for key in (
        "estimated_modeltrace_pricing_source",
        "estimated_modeltrace_price_table_revision",
        "estimated_modeltrace_price_table_status",
    ):
        value = first.get(key) or second.get(key)
        if value is not None:
            combined[key] = value
    return combined
