"""Lead product contract: reason, identity, Instantly/Smartlead CSV.

OpenOutreach (GPL-3.0) is a thought source only — column names and the
`reason` semantic are reimplemented here, no GPL code was copied.
"""

from __future__ import annotations

import csv
import hashlib
import io
import re
from datetime import datetime, timezone
from typing import Any, Iterable
from urllib.parse import urlparse

from emailing.guards import is_role_or_generic_email, looks_like_personal_email
from emailing.policy import expand_email_targets
from tools.email_patterns import parse_name_parts

CSV_COLUMNS = [
    "email",
    "first_name",
    "last_name",
    "company",
    "title",
    "website",
    "linkedin_url",
    "reason",
    "lead_id",
    "qualified_at",
]


def _now_iso() -> str:
    return datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")


def split_person_name(name: str) -> tuple[str, str]:
    parts = parse_name_parts(name or "")
    if not parts:
        tokens = [p for p in re.findall(r"[A-Za-z]+", name or "") if p]
        if not tokens:
            return "", ""
        first = tokens[0]
        last = tokens[-1] if len(tokens) > 1 else ""
        return first, last
    return parts["first"], parts["last"]


def compose_lead_reason(lead: dict[str, Any]) -> str:
    existing = str(lead.get("reason") or "").strip()
    if existing:
        return existing
    fit = lead.get("fit_reasons") or []
    if isinstance(fit, list):
        sentences = [str(item).strip() for item in fit if str(item).strip()]
        if sentences:
            return " ".join(sentences[:3])
    company = str(lead.get("company_name") or "").strip() or "This company"
    industry = str(lead.get("industry") or "").strip()
    role = str(lead.get("customer_role") or "").strip()
    bits = [company]
    if industry:
        bits.append(f"operates in {industry}")
    if role and role not in {"unknown", ""}:
        bits.append(f"as a {role.replace('_', ' ')}")
    bits.append("and is a plausible buyer or channel partner.")
    return " ".join(bits)


def make_lead_id(lead: dict[str, Any]) -> str:
    existing = str(lead.get("lead_id") or "").strip()
    if existing:
        return existing
    website = str(lead.get("website") or "").strip().lower()
    company = str(lead.get("company_name") or "").strip().lower()
    seed = website or company or str(lead.get("emails") or "")
    digest = hashlib.sha1(seed.encode("utf-8")).hexdigest()[:12]
    return f"lead_{digest}"


def _linkedin_url(lead: dict[str, Any]) -> str:
    social = lead.get("social_media") or {}
    if isinstance(social, dict):
        return str(social.get("linkedin") or social.get("LinkedIn") or "").strip()
    return ""


def _primary_person(lead: dict[str, Any]) -> tuple[str, str, str]:
    makers = lead.get("decision_makers") or []
    if isinstance(makers, list):
        for item in makers:
            if not isinstance(item, dict):
                continue
            name = str(item.get("name") or "").strip()
            if name:
                first, last = split_person_name(name)
                return first, last, str(item.get("title") or "").strip()
    contact = str(lead.get("contact_person") or "").strip()
    if contact:
        first, last = split_person_name(contact)
        return first, last, ""
    return (
        str(lead.get("first_name") or "").strip(),
        str(lead.get("last_name") or "").strip(),
        str(lead.get("title") or "").strip(),
    )


def finalize_lead_contract(lead: dict[str, Any]) -> dict[str, Any]:
    """Fill reason / identity fields in place and return the lead."""
    first, last, title = _primary_person(lead)
    lead["first_name"] = str(lead.get("first_name") or first)
    lead["last_name"] = str(lead.get("last_name") or last)
    if title and not lead.get("title"):
        lead["title"] = title
    lead["lead_id"] = make_lead_id(lead)
    lead["reason"] = compose_lead_reason(lead)
    if not str(lead.get("qualified_at") or "").strip():
        lead["qualified_at"] = _now_iso()
    if not str(lead.get("linkedin_url") or "").strip():
        lead["linkedin_url"] = _linkedin_url(lead)
    return lead


def _lead_emails(lead: dict[str, Any]) -> list[str]:
    emails: list[str] = []
    seen: set[str] = set()
    for target in expand_email_targets(lead):
        email = str(target.get("target_email") or "").strip().lower()
        if email and email not in seen:
            seen.add(email)
            emails.append(email)
    for raw in lead.get("emails") or []:
        email = str(raw or "").replace("(inferred)", "").strip().lower()
        if email and "@" in email and email not in seen:
            seen.add(email)
            emails.append(email)
    return emails


def lead_to_csv_rows(lead: dict[str, Any]) -> list[dict[str, str]]:
    finalized = finalize_lead_contract(dict(lead))
    emails = _lead_emails(finalized)
    if not emails:
        emails = [""]
    rows: list[dict[str, str]] = []
    for email in emails:
        first, last, title = _primary_person(finalized)
        if email and looks_like_personal_email(email):
            guessed = parse_name_parts(email.split("@", 1)[0].replace(".", " ").replace("_", " "))
            if guessed and not first:
                first, last = guessed["first"], guessed["last"]
        rows.append({
            "email": email,
            "first_name": str(finalized.get("first_name") or first),
            "last_name": str(finalized.get("last_name") or last),
            "company": str(finalized.get("company_name") or ""),
            "title": str(finalized.get("title") or title),
            "website": str(finalized.get("website") or ""),
            "linkedin_url": str(finalized.get("linkedin_url") or _linkedin_url(finalized)),
            "reason": str(finalized.get("reason") or ""),
            "lead_id": str(finalized.get("lead_id") or make_lead_id(finalized)),
            "qualified_at": str(finalized.get("qualified_at") or ""),
        })
    return rows


def leads_to_csv(leads: Iterable[dict[str, Any]]) -> str:
    buffer = io.StringIO()
    writer = csv.DictWriter(buffer, fieldnames=CSV_COLUMNS, extrasaction="ignore")
    writer.writeheader()
    for lead in leads:
        if not isinstance(lead, dict):
            continue
        for row in lead_to_csv_rows(lead):
            writer.writerow(row)
    return buffer.getvalue()


def domain_from_website(website: str) -> str:
    host = urlparse(str(website or "")).netloc.lower()
    if host.startswith("www."):
        host = host[4:]
    return host


def has_usable_contact(lead: dict[str, Any]) -> bool:
    """True when the lead already has a personal (non-role) inbox — resume, don't re-search."""
    for email in _lead_emails(lead):
        if email and not is_role_or_generic_email(email):
            return True
    makers = lead.get("decision_makers") or []
    if isinstance(makers, list):
        for item in makers:
            if not isinstance(item, dict):
                continue
            email = str(item.get("email") or "").replace("(inferred)", "").strip()
            if email and "@" in email and not is_role_or_generic_email(email):
                return True
    return False


def should_spend_verify_credits(lead: dict[str, Any], settings: Any | None = None) -> bool:
    """Spend MX / paid verify only on qualified leads."""
    min_fit = float(getattr(settings, "email_verify_min_fit_score", 0.0) or 0.0) if settings else 0.0
    if min_fit <= 0:
        return True
    fit = float(lead.get("fit_score") or lead.get("match_score") or 0.0)
    reason = str(lead.get("reason") or "").strip()
    fit_reasons = lead.get("fit_reasons") or []
    qualified = bool(reason) or (isinstance(fit_reasons, list) and any(fit_reasons))
    if not qualified and fit <= 0:
        return True
    return fit >= min_fit and (qualified or fit >= min_fit)
