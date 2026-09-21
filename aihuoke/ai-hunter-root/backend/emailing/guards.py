"""Outbound email screens: freemail, role inboxes, send window, daily cap, suppression.

Python rewrite of ai-outreach-engine `server/src/compliance/guards.ts`
(MIT, Copyright (c) 2026 Rohit Malhotra). Logic ported, not copied as TypeScript.
Suppression / unsubscribe is original to this tree (outreach-engine footer is a stub).
"""

from __future__ import annotations

import re
from dataclasses import dataclass
from datetime import datetime, timedelta, timezone
from typing import Any
from zoneinfo import ZoneInfo

FREEMAIL_DOMAINS = {
    "gmail.com",
    "googlemail.com",
    "yahoo.com",
    "yahoo.co.uk",
    "yahoo.co.in",
    "hotmail.com",
    "outlook.com",
    "live.com",
    "msn.com",
    "aol.com",
    "icloud.com",
    "me.com",
    "mac.com",
    "proton.me",
    "protonmail.com",
    "gmx.com",
    "gmx.de",
    "mail.com",
    "yandex.ru",
    "zoho.com",
    "rediffmail.com",
    "qq.com",
    "163.com",
    "126.com",
    "sina.com",
    "yeah.net",
    "foxmail.com",
}

ROLE_LOCAL_PARTS = {
    "info", "support", "hello", "hi", "hey", "contact", "contacts",
    "admin", "administrator", "sales", "sale", "selling", "team", "teams",
    "help", "helpdesk", "press", "media", "marketing", "market", "markets",
    "pr", "careers", "career", "jobs", "job", "hiring", "recruit", "recruiting",
    "recruiter", "recruiters", "talent", "hr", "people", "peopleops", "recops",
    "billing", "finance", "accounts", "account", "accounting", "legal",
    "compliance", "noreply", "no-reply", "no_reply", "donotreply", "do-not-reply",
    "privacy", "security", "webmaster", "postmaster", "office", "enquiries",
    "enquiry", "inquiry", "inquiries", "reception", "general", "mail", "email",
    "newsletter", "news", "abuse", "root", "business", "biz", "enterprise",
    "commercial", "partnerships", "partnership", "partners", "partner",
    "affiliates", "affiliate", "vendors", "vendor", "suppliers", "supplier",
    "clients", "client", "customer", "customers", "success", "cs", "cx",
    "service", "services", "servicedesk", "ops", "operations", "product",
    "products", "engineering", "eng", "dev", "devs", "developers", "developer",
    "design", "designer", "designers", "founders", "founder", "ceo", "cto",
    "coo", "cfo", "cmo", "head", "heads", "lead", "leads", "manager", "managers",
    "director", "directors", "associate", "associates", "representative",
    "representatives", "rep", "reps", "agent", "agents", "desk", "inbox",
    "mailer", "updates", "update", "notify", "notifications", "alerts", "alert",
    "feedback", "community", "hello-world", "test", "testing", "demo", "demos",
    "example", "samples", "sample", "working", "work", "workers", "inside",
    "insider", "discover", "apply", "applications", "application", "join",
    "joinus", "welcome", "intro", "outreach", "growth", "revenue", "deals",
    "deal", "pipeline", "enablement", "onboarding", "implementation",
    "solutions", "solution", "consulting", "consultant", "edtech", "fintech",
    "healthtech", "saas", "analytics", "research", "labs", "studio", "ventures",
    "venture", "invest", "investors", "investor", "comms", "communications",
    "brand", "events", "event", "rsvp", "booking", "bookings", "orders", "order",
    "shipping", "returns", "refunds", "payments", "payment", "invoice",
    "invoices", "payroll", "sysadmin", "devops", "status", "null", "undefined",
    "unknown", "everyone", "anybody", "someone",
}

ROLE_STRONG_TOKENS = {token for token in ROLE_LOCAL_PARTS if len(token) >= 4}

_EMAIL_RE = re.compile(r"^[^@\s]+@[^@\s]+\.[a-z]{2,}$", re.I)


@dataclass(frozen=True)
class Verdict:
    ok: bool
    reason: str = ""


OK = Verdict(ok=True)


def _domain_brand(domain: str) -> str:
    host = domain.lower().removeprefix("www.")
    parts = [p for p in host.split(".") if p]
    if not parts:
        return ""
    return parts[0]


def is_role_or_generic_email(email: str) -> bool:
    trimmed = (email or "").lower().strip()
    at = trimmed.rfind("@")
    if at <= 0:
        return True
    local = trimmed[:at]
    domain = trimmed[at + 1 :]
    if not local or not domain:
        return True
    base = (local.split("+", 1)[0] or local).strip('"')
    if not base or len(base) <= 1:
        return True
    if base in ROLE_LOCAL_PARTS:
        return True
    tokens = [t for t in re.split(r"[._-]+", base) if t]
    if len(tokens) >= 2 and any(token in ROLE_STRONG_TOKENS for token in tokens):
        return True
    brand = _domain_brand(domain)
    if brand and (base == brand or base.replace("-", "") == brand.replace("-", "")):
        return True
    if not re.search(r"[a-z]", base, re.I):
        return True
    return False


def looks_like_personal_email(email: str) -> bool:
    if is_role_or_generic_email(email):
        return False
    local = ((email.split("@", 1)[0] if "@" in email else "") or "").lower().split("+", 1)[0]
    if re.match(r"^[a-z]{2,}[._-][a-z]{2,}([._-][a-z]{2,})?$", local):
        return True
    if re.match(r"^[a-z][._-][a-z]{2,}$", local):
        return True
    return False


def is_freemail_domain(domain: str) -> bool:
    return (domain or "").strip().lower() in FREEMAIL_DOMAINS


def screen_recipient(
    email: str,
    *,
    settings: Any | None = None,
    sender_email: str = "",
    user_supplied: bool = False,
    country: str = "",
    verify_status: str = "",
    store: Any | None = None,
) -> Verdict:
    normalised = (email or "").strip().lower()
    if not _EMAIL_RE.match(normalised):
        return Verdict(ok=False, reason="not a valid email address")
    if is_role_or_generic_email(normalised):
        return Verdict(ok=False, reason="role/generic inbox — skipped")
    domain = normalised.split("@", 1)[1]
    block_freemail = True if settings is None else bool(getattr(settings, "email_block_freemail", True))
    allow_generic = bool(getattr(settings, "email_allow_generic_company_email", False)) if settings else False
    if allow_generic and is_role_or_generic_email(normalised):
        pass
    if block_freemail and is_freemail_domain(domain) and not user_supplied:
        return Verdict(ok=False, reason=f"{domain} is a consumer mailbox; this tool is B2B only")
    sender_domain = ""
    if "@" in (sender_email or ""):
        sender_domain = sender_email.split("@", 1)[1].lower()
    if domain and domain == sender_domain and not is_freemail_domain(domain):
        return Verdict(ok=False, reason="recipient is on the sending domain")
    blocked = list(getattr(settings, "email_blocked_countries", []) or []) if settings else []
    if country and country.upper() in {str(item).upper() for item in blocked}:
        return Verdict(ok=False, reason=f"{country} is in blocked countries — skipped")
    if verify_status == "invalid":
        return Verdict(ok=False, reason="verification says the address is invalid")
    if store is not None and hasattr(store, "is_suppressed"):
        try:
            if store.is_suppressed(normalised):
                return Verdict(ok=False, reason="unsubscribed/suppressed")
        except Exception:
            return Verdict(ok=False, reason="suppression list unavailable")
    return OK


def _parse_hour(value: str, fallback: int) -> int:
    text = str(value or "").strip()
    if not text:
        return fallback
    hour_token = text.split(":", 1)[0]
    try:
        hour = int(hour_token)
    except ValueError:
        return fallback
    return max(0, min(23, hour))


def _parse_iso(value: str) -> datetime | None:
    text = str(value or "").strip()
    if not text:
        return None
    try:
        return datetime.fromisoformat(text.replace("Z", "+00:00"))
    except ValueError:
        return None


def screen_send_window(
    *,
    sent_today: int,
    daily_cap: int,
    now: datetime | None = None,
    timezone_name: str = "Asia/Shanghai",
    window_start: str = "09:00",
    window_end: str = "18:00",
    weekdays_only: bool = True,
    last_sent_at: str = "",
    min_minutes_between: int = 0,
) -> Verdict:
    if daily_cap > 0 and sent_today >= daily_cap:
        return Verdict(ok=False, reason=f"daily cap reached ({sent_today}/{daily_cap} in the last 24h)")
    current = now or datetime.now(timezone.utc)
    if current.tzinfo is None:
        current = current.replace(tzinfo=timezone.utc)
    try:
        tz = ZoneInfo(timezone_name or "UTC")
        local = current.astimezone(tz)
    except Exception:
        timezone_name = "UTC"
        local = current.astimezone(timezone.utc)
    start_hour = _parse_hour(window_start, 9)
    end_hour = _parse_hour(window_end, 18)
    if local.hour < start_hour or local.hour >= end_hour:
        return Verdict(
            ok=False,
            reason=f"outside the send window ({start_hour:02d}:00-{end_hour:02d}:00 {timezone_name}, now {local.hour:02d}:00)",
        )
    if weekdays_only and local.weekday() >= 5:
        return Verdict(ok=False, reason="weekend; outbound is paused")
    if min_minutes_between > 0:
        previous = _parse_iso(last_sent_at)
        if previous is not None:
            if previous.tzinfo is None:
                previous = previous.replace(tzinfo=timezone.utc)
            elapsed = (current - previous.astimezone(timezone.utc)).total_seconds() / 60.0
            if elapsed < min_minutes_between:
                wait = int(min_minutes_between - elapsed) + (0 if elapsed == int(elapsed) else 1)
                return Verdict(ok=False, reason=f"throttled; {max(wait, 1)} min until the next send")
    return OK


def screen_send_window_from_settings(
    settings: Any,
    *,
    sent_today: int,
    last_sent_at: str = "",
    now: datetime | None = None,
) -> Verdict:
    return screen_send_window(
        sent_today=sent_today,
        daily_cap=int(getattr(settings, "email_daily_send_limit", 50) or 50),
        now=now,
        timezone_name=str(getattr(settings, "email_timezone", "Asia/Shanghai") or "Asia/Shanghai"),
        window_start=str(getattr(settings, "email_business_hours_start", "09:00") or "09:00"),
        window_end=str(getattr(settings, "email_business_hours_end", "18:00") or "18:00"),
        weekdays_only=bool(getattr(settings, "email_weekdays_only", True)),
        last_sent_at=last_sent_at,
        min_minutes_between=int(getattr(settings, "email_min_minutes_between_sends", 0) or 0),
    )


def hours_ago_iso(hours: int = 24, now: datetime | None = None) -> str:
    current = now or datetime.now(timezone.utc)
    if current.tzinfo is None:
        current = current.replace(tzinfo=timezone.utc)
    return (current - timedelta(hours=hours)).isoformat()
