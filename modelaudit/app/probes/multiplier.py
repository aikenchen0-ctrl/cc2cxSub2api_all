from __future__ import annotations

import math
from typing import Any

from .common import ProbeContext, ProbeResult, probe_protocol, result
from .usage import supplier_money_counter, supplier_token_delta


def _profile_prices(context: ProbeContext) -> dict[str, float] | None:
    raw = context.profile.pricing
    input_rate = raw.get("input_per_million")
    if isinstance(input_rate, bool) or not isinstance(input_rate, (int, float)) or not math.isfinite(float(input_rate)) or input_rate < 0:
        return None
    values = {
        "input_per_million": float(input_rate),
        "output_per_million": raw.get("output_per_million"),
        "cache_read_per_million": raw.get("cache_read_per_million"),
        "cache_write_per_million": raw.get("cache_write_per_million"),
    }
    protocol = probe_protocol(context)
    if values["output_per_million"] is None and context.profile.declared_completion_ratio is not None:
        values["output_per_million"] = input_rate * context.profile.declared_completion_ratio
    if values["cache_read_per_million"] is None and context.profile.declared_cache_ratio is not None:
        values["cache_read_per_million"] = input_rate * context.profile.declared_cache_ratio
    if values["cache_write_per_million"] is None:
        if protocol == "openai":
            values["cache_write_per_million"] = input_rate
        elif not any(call.cache_creation_tokens for call in context.calls if call.cache_creation_tokens is not None):
            values["cache_write_per_million"] = 0.0
    prices: dict[str, float] = {}
    for field in (
        "input_per_million",
        "output_per_million",
        "cache_read_per_million",
        "cache_write_per_million",
    ):
        value = values[field]
        if isinstance(value, bool) or not isinstance(value, (int, float)) or not math.isfinite(float(value)) or value < 0:
            return None
        prices[field] = float(value)
    return prices


def _expected_cost(
    context: ProbeContext,
    probe_names: frozenset[str] = frozenset({"token_delta", "cache"}),
) -> tuple[float | None, int]:
    prices = _profile_prices(context)
    if prices is None:
        return None, 0
    multiplier = context.profile.gateway_cost_multiplier
    if multiplier is None:
        multiplier = 1.0
    total = 0.0
    measured_calls = 0
    for call in context.calls:
        if call.probe not in probe_names:
            continue
        if not call.pinned or call.actual_account_id != context.account.account_id:
            continue
        values = (
            call.input_tokens,
            call.output_tokens,
            call.cache_creation_tokens,
            call.cache_read_tokens,
        )
        if any(value is None or value < 0 for value in values):
            continue
        input_tokens, output_tokens, cache_write, cache_read = values
        total += multiplier * (
            input_tokens * prices["input_per_million"]
            + output_tokens * prices["output_per_million"]
            + cache_write * prices["cache_write_per_million"]
            + cache_read * prices["cache_read_per_million"]
        ) / 1_000_000
        measured_calls += 1
    return (total, measured_calls) if measured_calls else (None, 0)


def _reported_probe_tokens(context: ProbeContext) -> tuple[int | None, int, str]:
    calls = [call for call in context.calls if call.probe in {"token_delta", "cache"}]
    if not calls:
        return None, 0, "probe_usage_missing"
    total = 0
    for call in calls:
        if not call.pinned or call.actual_account_id != context.account.account_id:
            return None, 0, "probe_not_pinned"
        if call.http_status < 200 or call.http_status >= 300:
            return None, 0, "probe_request_failed"
        values = (
            call.input_tokens,
            call.output_tokens,
            call.cache_creation_tokens,
            call.cache_read_tokens,
        )
        if any(value is None or value < 0 for value in values):
            return None, 0, "probe_usage_missing"
        total += sum(int(value) for value in values if value is not None)
    if total <= 0:
        return None, 0, "probe_usage_not_positive"
    return total, len(calls), ""


def _invoice_diagnostic(
    context: ProbeContext,
    before: dict[str, Any],
    after: dict[str, Any],
) -> dict[str, Any]:
    pricing_reference = {
        "pricing_model": context.profile.pricing_model,
        "pricing_source": context.profile.pricing_source,
        "price_table_revision": context.profile.price_table_revision,
        "price_table_status": context.profile.price_table_status,
    }
    before_counter = supplier_money_counter(before)
    after_counter = supplier_money_counter(after)
    expected_cost, measured_calls = _expected_cost(context)
    if before_counter is None or after_counter is None:
        return {"status": "inconclusive", "reason": "supplier_invoice_unavailable", **pricing_reference}
    before_kind, before_amount, before_period = before_counter
    after_kind, after_amount, after_period = after_counter
    if before_kind != after_kind or before_period != after_period:
        return {"status": "inconclusive", "reason": "supplier_billing_period_changed", **pricing_reference}
    if expected_cost is None or expected_cost <= 0:
        return {"status": "inconclusive", "reason": "configured_probe_cost_unavailable", **pricing_reference}
    delta = after_amount - before_amount
    if delta <= 0:
        return {
            "status": "inconclusive",
            "reason": "supplier_invoice_delta_not_positive",
            "supplier_charge_delta": delta,
            "supplier_counter": after_kind,
            "supplier_period": after_period,
            **pricing_reference,
        }
    observed_multiplier = delta / expected_cost
    threshold = context.settings.multiplier_order_of_magnitude
    baselines = {
        "configured_account_rate_multiplier": context.account.rate_multiplier,
        "expected_provider_cost_multiplier": context.profile.expected_provider_cost_multiplier,
    }
    comparisons: dict[str, dict[str, Any]] = {}
    mismatches: list[str] = []
    for name, expected in baselines.items():
        if expected is None or not math.isfinite(float(expected)) or expected <= 0:
            comparisons[name] = {"status": "not_comparable", "reason": "positive_baseline_not_configured"}
            continue
        ratio = observed_multiplier / float(expected)
        mismatch = ratio >= threshold or ratio <= 1 / threshold
        comparisons[name] = {
            "status": "mismatch" if mismatch else "within_threshold",
            "expected_multiplier": float(expected),
            "observed_to_expected_ratio": ratio,
            "order_of_magnitude_threshold": threshold,
        }
        if mismatch:
            mismatches.append(name)

    comparable_baselines = [item for item in comparisons.values() if item.get("status") != "not_comparable"]
    if mismatches:
        comparison_status = "mismatch"
    elif comparisons and len(comparable_baselines) == len(comparisons) and comparable_baselines:
        comparison_status = "within_threshold"
    elif comparable_baselines:
        comparison_status = "partially_comparable"
    else:
        comparison_status = "not_comparable"
    return {
        "status": comparison_status,
        "supplier_counter": after_kind,
        "supplier_period": after_period,
        "supplier_charge_delta": delta,
        "expected_probe_cost": expected_cost,
        "observed_cost_multiplier": observed_multiplier,
        "estimated_cost_multiplier": observed_multiplier,
        "comparisons": comparisons,
        "baseline_mismatches": mismatches,
        "order_of_magnitude_threshold": threshold,
        "measured_probe_calls": measured_calls,
        "used_for_alert": bool(mismatches),
        "account_aggregate_may_include_concurrent_activity": True,
        **pricing_reference,
    }


def run_multiplier(
    context: ProbeContext,
    before: dict[str, Any] | None,
    after: dict[str, Any] | None,
) -> ProbeResult:
    if before is None or after is None:
        return result(
            "multiplier",
            "inconclusive",
            "Supplier usage snapshots were unavailable before or after the probes.",
            model=context.profile.model,
            expected_model=context.profile.expected_model,
            actual_account_id=context.account.account_id,
            evidence={"reason": "usage_snapshot_missing"},
        )

    invoice_diagnostic = _invoice_diagnostic(context, before, after)
    reported_tokens, measured_calls, usage_error = _reported_probe_tokens(context)
    estimated_probe_cost, estimated_cost_calls = _expected_cost(context)
    estimated_pin_cost, estimated_pin_calls = _expected_cost(context, frozenset({"pin_calibration"}))
    estimated_total_cost = (
        estimated_probe_cost + estimated_pin_cost
        if estimated_probe_cost is not None and estimated_pin_cost is not None
        else None
    )
    token_delta, counter_error = supplier_token_delta(before, after)
    base_metrics = {
        "configured_account_rate_multiplier": context.account.rate_multiplier,
        "expected_provider_cost_multiplier": context.profile.expected_provider_cost_multiplier,
        "declared_cache_ratio": context.profile.declared_cache_ratio,
        "declared_completion_ratio": context.profile.declared_completion_ratio,
        "reported_probe_tokens": reported_tokens,
        "measured_probe_calls": measured_calls,
        "estimated_probe_cost_usd": estimated_probe_cost,
        "estimated_probe_cost_measured_calls": estimated_cost_calls,
        "estimated_pin_calibration_cost_usd": estimated_pin_cost,
        "estimated_pin_calibration_measured_calls": estimated_pin_calls,
        "estimated_total_usage_aware_probe_cost_usd": estimated_total_cost,
        "estimated_probe_cost_source": "pinned_response_usage_and_configured_litellm_or_manual_prices; excludes_modeltrace_usage",
        "invoice_diagnostic": invoice_diagnostic,
        "pricing_model": context.profile.pricing_model,
        "pricing_source": context.profile.pricing_source,
        "price_table_revision": context.profile.price_table_revision,
        "price_table_status": context.profile.price_table_status,
    }
    if token_delta is None or reported_tokens is None:
        observed = None
        token_mismatch = False
    else:
        observed = token_delta / reported_tokens

    # Token quota units and money multipliers are distinct measurements. Keep
    # the quota ratio on its nominal token baseline; compare money only in the
    # separate supplier-invoice diagnostic above.
    token_usage_baseline = 1.0
    threshold = context.settings.multiplier_order_of_magnitude
    token_baselines = {
        "nominal_provider_token_quota_per_reported_token": token_usage_baseline,
        "configured_account_rate_multiplier": context.account.rate_multiplier,
        "expected_provider_cost_multiplier": context.profile.expected_provider_cost_multiplier,
    }
    token_comparisons: dict[str, dict[str, Any]] = {}
    token_mismatches: list[str] = []
    for name, expected in token_baselines.items():
        if (
            observed is None
            or expected is None
            or not math.isfinite(float(expected))
            or expected <= 0
        ):
            token_comparisons[name] = {
                "status": "not_comparable",
                "reason": "positive_baseline_or_verified_token_delta_unavailable",
            }
            continue
        ratio = observed / float(expected)
        mismatch = ratio >= threshold or ratio <= 1 / threshold
        token_comparisons[name] = {
            "status": "mismatch" if mismatch else "within_threshold",
            "expected_multiplier": float(expected),
            "observed_to_expected_ratio": ratio,
            "order_of_magnitude_threshold": threshold,
        }
        if mismatch:
            token_mismatches.append(name)
    token_mismatch = bool(token_mismatches)
    invoice_mismatches = bool(invoice_diagnostic.get("baseline_mismatches"))
    if token_mismatch or invoice_mismatches:
        status = "alert"
        severity = "red" if token_mismatch else "yellow"
        reasons = []
        if token_mismatch:
            reasons.append("provider token-quota ratio differs by an order of magnitude from " + ", ".join(token_mismatches))
        if invoice_mismatches:
            reasons.append("供应商账单倍率与配置基线相差一个数量级")
        summary = "账号级筛查信号：" + "；".join(reasons)
    elif invoice_diagnostic.get("status") == "within_threshold":
        status = "ok"
        severity = "normal"
        summary = "供应商账单倍率在配置的数量级阈值范围内。"
    elif observed is not None:
        status = "inconclusive"
        severity = "none"
        summary = "上游 token 配额比值在名义 1.0 基线范围内，但供应商账单倍率缺少完整对比基线。"
    else:
        status = "inconclusive"
        severity = "none"
        summary = "缺少可比较的上游 token 配额增量或供应商账单倍率。"

    return result(
        "multiplier",
        status,
        summary,
        severity=severity,
        model=context.profile.model,
        expected_model=context.profile.expected_model,
        actual_account_id=context.account.account_id,
        request_ids=tuple(
            call.request_id
            for call in context.calls
            if call.probe in {"token_delta", "cache"} and call.pinned
        ),
        metrics={
            **base_metrics,
            "supplier_counter": "grok_token_quota" if token_delta is not None else None,
            "provider_token_delta": token_delta,
            "observed_usage_multiplier": observed,
            "nominal_token_usage_baseline": token_usage_baseline,
            "token_quota_multiplier_comparisons": token_comparisons,
            "configured_account_rate_multiplier": context.account.rate_multiplier,
            "expected_provider_cost_multiplier": context.profile.expected_provider_cost_multiplier,
            "billing_multiplier_comparison": "token_quota_ratio_compared_separately_from_invoice_usd_ratio",
            "order_of_magnitude_threshold": threshold,
            "token_quota_baseline_mismatches": token_mismatches,
            "invoice_multiplier_baseline_mismatches": invoice_diagnostic.get("baseline_mismatches", []),
            "invoice_multiplier_used_for_alert": bool(invoice_diagnostic.get("used_for_alert")),
        },
        evidence={
            "comparison": "upstream token quota delta divided by pinned response usage tokens",
            "invoice_comparison": "supplier invoice cost delta divided by locally estimated probe cost",
            "quota_delta_can_include_concurrent_account_activity": True,
            "supplier_invoice_delta_can_include_concurrent_account_activity": True,
            "supplier_invoice_alert_is_an_account_level_screening_signal": True,
            "token_quota_error": counter_error or usage_error if observed is None else "",
        },
    )
