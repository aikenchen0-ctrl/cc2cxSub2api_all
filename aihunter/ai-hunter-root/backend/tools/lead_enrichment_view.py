"""Shape P2/P3 enrichment fields for operators and EmailCraft.

OSINT writes `tech_stack` / `legal_name` / `github_org` / `evidence_urls`.
Social presence writes `social_profiles[]`. This module does not fetch
anything — it only presents what LeadExtract already stored. Failures
and empty fields stay silent. `reason` is operator-only and never
included here (do not paste it into outreach).
"""

from __future__ import annotations

from typing import Any


def as_string_list(value: Any, *, limit: int = 8) -> list[str]:
    if not isinstance(value, list):
        return []
    out: list[str] = []
    seen: set[str] = set()
    for item in value:
        text = str(item or "").strip()
        if not text or text in seen:
            continue
        seen.add(text)
        out.append(text)
        if len(out) >= limit:
            break
    return out


def social_profile_rows(lead: dict[str, Any], *, limit: int = 8) -> list[dict[str, str]]:
    rows: list[dict[str, str]] = []
    seen: set[str] = set()
    for item in lead.get("social_profiles") or []:
        if not isinstance(item, dict):
            continue
        url = str(item.get("url") or "").strip()
        network = str(item.get("network") or "").strip()
        username = str(item.get("username") or "").strip()
        key = (url or f"{network}:{username}").lower()
        if not key or key in seen:
            continue
        seen.add(key)
        rows.append(
            {
                "network": network,
                "url": url,
                "username": username,
                "status": str(item.get("status") or "").strip(),
            }
        )
        if len(rows) >= limit:
            break
    return rows


def enrichment_hooks(lead: dict[str, Any], *, limit: int = 6) -> list[str]:
    """Short personalization hooks. Empty when OSINT/social never ran."""
    hooks: list[str] = []
    legal = str(lead.get("legal_name") or "").strip()
    if legal:
        hooks.append(f"legal name: {legal}")
    stack = as_string_list(lead.get("tech_stack"), limit=4)
    if stack:
        hooks.append("tech stack: " + ", ".join(stack))
    github = str(lead.get("github_org") or "").strip()
    if github:
        hooks.append(f"github: {github}")
    for row in social_profile_rows(lead, limit=3):
        label = row["network"] or "profile"
        target = row["url"] or row["username"]
        if target:
            hooks.append(f"{label}: {target}")
    return hooks[:limit]


def format_enrichment_block(lead: dict[str, Any]) -> str:
    """Plain-text block for EmailCraft prompts. Empty string if nothing extra."""
    lines: list[str] = []
    legal = str(lead.get("legal_name") or "").strip()
    if legal:
        lines.append(f"Legal name: {legal}")
    stack = as_string_list(lead.get("tech_stack"), limit=6)
    if stack:
        lines.append("Tech stack: " + ", ".join(stack))
    github = str(lead.get("github_org") or "").strip()
    if github:
        lines.append(f"GitHub org: {github}")
    profiles = social_profile_rows(lead, limit=6)
    if profiles:
        bits: list[str] = []
        for row in profiles:
            label = row["network"] or "profile"
            target = row["url"] or row["username"]
            bits.append(f"{label}={target}" if target else label)
        lines.append("Social profiles: " + "; ".join(bits))
    urls = as_string_list(lead.get("evidence_urls"), limit=5)
    if urls:
        lines.append("Evidence URLs: " + "; ".join(urls))
    return "\n".join(lines)
