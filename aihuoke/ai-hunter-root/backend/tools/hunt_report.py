"""Operator hunt report: insight, keywords, raw search counts, keyword effectiveness.

The CSV (`lead_contract.leads_to_csv`) is the Instantly/Smartlead integration.
This file is the operator-facing counterpart — it must not include per-lead
`reason`, emails, or fit scores.
"""

from __future__ import annotations

from datetime import datetime, timezone
from typing import Any


EFFECTIVENESS_ZH = {"high": "高", "medium": "中", "low": "低"}


def _utc_now() -> str:
    return datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")


def _str_list(value: Any) -> list[str]:
    if not isinstance(value, list):
        return []
    return [str(item).strip() for item in value if str(item).strip()]


def _as_dict(value: Any) -> dict[str, Any]:
    return value if isinstance(value, dict) else {}


def _as_int(value: Any, default: int = 0) -> int:
    try:
        return int(value)
    except (TypeError, ValueError):
        return default


def _effectiveness_label(level: Any) -> str:
    key = str(level or "low").strip().lower()
    return EFFECTIVENESS_ZH.get(key, str(level or "low"))


def _join_counts(mapping: Any) -> str:
    data = _as_dict(mapping)
    parts = [f"{name} {count}" for name, count in data.items()]
    return ", ".join(parts)


def render_hunt_report(
    hunt_id: str,
    result: dict[str, Any] | None,
    *,
    exported_at: str | None = None,
) -> str:
    """Render the operator .txt report from a persisted hunt result.

    Empty / partial results still produce a header. Search counts are raw;
    keyword effectiveness comes from `round_feedback.keyword_performance`.
    """
    result = result or {}
    lines = [
        "=" * 60,
        "AI HUNTER — INSIGHT / KEYWORD / EFFECTIVENESS REPORT",
        f"Hunt ID : {hunt_id}",
        f"Exported: {exported_at or _utc_now()}",
        "=" * 60,
    ]

    insight = _as_dict(result.get("insight"))
    if insight:
        lines.extend(["", "── COMPANY INSIGHT ──────────────────────────────────────"])
        if insight.get("company_name"):
            lines.append(f"Company  : {insight['company_name']}")
        if insight.get("summary"):
            lines.append(f"Summary  : {insight['summary']}")
        products = _str_list(insight.get("products"))
        if products:
            lines.append(f"Products : {', '.join(products)}")
        industries = _str_list(insight.get("industries"))
        if industries:
            lines.append(f"Industries: {', '.join(industries)}")
        profile = str(insight.get("target_customer_profile") or "").strip()
        if profile:
            lines.append(f"Target Customer:\n  {profile}")
        value_props = _str_list(insight.get("value_propositions"))
        if value_props:
            lines.append("Value Props:\n  " + "\n  ".join(f"• {item}" for item in value_props))
        negatives = _str_list(insight.get("negative_targeting_criteria"))
        if negatives:
            lines.append("Negative Criteria:\n  " + "\n  ".join(f"• {item}" for item in negatives))
        seeds = _str_list(insight.get("recommended_keywords_seed"))
        if seeds:
            lines.append(f"Seed Keywords: {', '.join(seeds)}")
        regions = _str_list(insight.get("recommended_regions"))
        if regions:
            lines.append(f"Target Regions: {', '.join(regions)}")

    used_keywords = _str_list(result.get("used_keywords"))
    hunt_round = _as_int(result.get("hunt_round"), 0)
    if used_keywords:
        unique = list(dict.fromkeys(used_keywords))
        lines.extend(["", "── GENERATED KEYWORDS ───────────────────────────────────"])
        if hunt_round:
            lines.append(f"Round {hunt_round}: {', '.join(unique)}")
        lines.append("")
        lines.append(f"Total unique keywords: {len(unique)}")
        lines.extend(unique)

    stats = _as_dict(result.get("keyword_search_stats"))
    search_result_count = _as_int(result.get("search_result_count"), 0)
    if not search_result_count and isinstance(result.get("search_results"), list):
        search_result_count = len(result.get("search_results") or [])
    if stats or search_result_count:
        lines.extend([
            "",
            "── SEARCH COUNTS (raw) ──────────────────────────────────",
            "Counts only. Effectiveness is in the evaluate section below.",
            "",
            f"Round {hunt_round or 1}: {search_result_count} results",
        ])
        if stats:
            lines.append("  keyword | results | leads")
            for kw, raw in stats.items():
                row = _as_dict(raw)
                err = str(row.get("error") or "").strip()
                line = f"  {kw} | {row.get('result_count', 0)} | {row.get('leads_found', 0)}"
                if err:
                    line += f" | error: {err}"
                lines.append(line)

    feedback = result.get("round_feedback")
    if isinstance(feedback, dict) and feedback:
        lines.extend(["", "── KEYWORD EFFECTIVENESS ────────────────────────────────"])
        new_leads = _as_int(feedback.get("new_leads_this_round"), 0)
        total_leads = _as_int(feedback.get("total_leads"), 0)
        target = _as_int(feedback.get("target"), 200)
        round_no = _as_int(feedback.get("round"), hunt_round or 1)
        lines.append("")
        lines.append(f"Round {round_no}: +{new_leads} leads, {total_leads}/{target}")
        best = _str_list(feedback.get("best_keywords"))
        weak = _str_list(feedback.get("worst_keywords"))
        if best:
            lines.append(f"  Best : {', '.join(best)}")
        if weak:
            lines.append(f"  Weak : {', '.join(weak)}")
        perf_rows = feedback.get("keyword_performance")
        if isinstance(perf_rows, list) and perf_rows:
            lines.append("  keyword | results | leads | precision | effectiveness")
            for row in perf_rows:
                item = _as_dict(row)
                try:
                    precision = float(item.get("precision") or 0)
                except (TypeError, ValueError):
                    precision = 0.0
                lines.append(
                    "  {kw} | {results} | {leads} | {prec:.1f}% | {eff}".format(
                        kw=item.get("keyword") or "—",
                        results=item.get("search_results", 0),
                        leads=item.get("leads_found", 0),
                        prec=precision * 100,
                        eff=_effectiveness_label(item.get("effectiveness")),
                    )
                )
        industry_line = _join_counts(feedback.get("industry_distribution"))
        region_line = _join_counts(feedback.get("region_distribution"))
        if industry_line:
            lines.append(f"  Industry: {industry_line}")
        if region_line:
            lines.append(f"  Region  : {region_line}")
        blocked = str(feedback.get("search_backend_error") or "").strip()
        if blocked:
            lines.append(f"  Search blocked: {blocked}")

    lines.extend(["", "=" * 60])
    return "\n".join(lines)
