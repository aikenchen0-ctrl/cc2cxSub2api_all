"""Outbound suppression list and unsubscribe helpers.

Original to this tree. OpenOutreach's LEGAL_NOTICE is a thought source only
(GPL-3.0 — do not copy its files). ai-outreach-engine `compliance/footer.ts`
is an empty stub and is not used.

Every live send must:
- refuse a suppressed recipient
- append a visible opt-out line
- set List-Unsubscribe (+ List-Unsubscribe-Post) headers
"""

from __future__ import annotations

import hashlib
import hmac
import re
from typing import Any
from urllib.parse import quote

from emailing.store import EmailStore

_EMAIL_RE = re.compile(r"^[^@\s]+@[^@\s]+\.[a-z]{2,}$", re.I)

UNSUBSCRIBE_FOOTER = (
    "If you would rather not receive these emails, reply with UNSUBSCRIBE "
    "or use: {url}"
)

_UNSUBSCRIBE_SNIPPET_MARKERS = (
    "unsubscribe",
    "opt out",
    "opt-out",
    "remove me",
    "stop emailing",
    "do not contact",
    "don't contact",
    "take me off",
    "退订",
    "取消订阅",
    "不要再发",
)


def normalize_email(email: str) -> str:
    return str(email or "").strip().lower()


def is_valid_email(email: str) -> bool:
    return bool(_EMAIL_RE.match(normalize_email(email)))


def _secret(store: EmailStore | None, settings: Any | None) -> str:
    configured = str(getattr(settings, "email_unsubscribe_secret", "") or "").strip() if settings else ""
    if configured:
        return configured
    if store is not None:
        return store.unsubscribe_secret()
    return ""


def unsubscribe_token(email: str, *, secret: str) -> str:
    normalised = normalize_email(email)
    if not normalised or not secret:
        return ""
    digest = hmac.new(secret.encode("utf-8"), normalised.encode("utf-8"), hashlib.sha256)
    return digest.hexdigest()[:32]


def verify_unsubscribe_token(email: str, token: str, *, secret: str) -> bool:
    expected = unsubscribe_token(email, secret=secret)
    provided = str(token or "").strip()
    if not expected or not provided:
        return False
    return hmac.compare_digest(expected, provided)


def public_base_url(settings: Any | None = None) -> str:
    configured = str(getattr(settings, "email_public_base_url", "") or "").strip() if settings else ""
    if configured:
        return configured.rstrip("/")
    host = str(getattr(settings, "api_host", "") or "").strip() if settings else ""
    port = int(getattr(settings, "api_port", 8000) or 8000) if settings else 8000
    if host in {"", "0.0.0.0", "::"}:
        host = "127.0.0.1"
    return f"http://{host}:{port}"


def unsubscribe_url(email: str, *, store: EmailStore | None = None, settings: Any | None = None) -> str:
    normalised = normalize_email(email)
    secret = _secret(store, settings)
    token = unsubscribe_token(normalised, secret=secret)
    if not normalised or not token:
        return ""
    base = public_base_url(settings)
    return f"{base}/api/v1/unsubscribe?email={quote(normalised)}&token={token}"


def mailto_unsubscribe(sender_email: str) -> str:
    address = str(sender_email or "").strip()
    if not address or "@" not in address:
        return ""
    return f"mailto:{address}?subject=unsubscribe"


def looks_like_unsubscribe_request(text: str) -> bool:
    raw = str(text or "").replace("\r\n", "\n")
    if not raw.strip():
        return False
    lines: list[str] = []
    for line in raw.split("\n"):
        stripped = line.strip()
        if not stripped:
            continue
        if stripped.startswith(">"):
            continue
        if stripped.lower().startswith("on ") and " wrote:" in stripped.lower():
            break
        if stripped.startswith("-----original message-----"):
            break
        lines.append(stripped)
    snippet = "\n".join(lines).lower()
    if not snippet:
        return False
    return any(marker in snippet for marker in _UNSUBSCRIBE_SNIPPET_MARKERS)


def with_unsubscribe_footer(body_text: str, url: str) -> str:
    body = str(body_text or "").rstrip()
    link = str(url or "").strip()
    if not link:
        return body
    if "would rather not receive these emails" in body.lower() or "/api/v1/unsubscribe" in body:
        return body
    footer = UNSUBSCRIBE_FOOTER.format(url=link)
    if not body:
        return footer
    return f"{body}\n\n{footer}"


def unsubscribe_headers(
    email: str,
    *,
    store: EmailStore | None = None,
    settings: Any | None = None,
    sender_email: str = "",
) -> dict[str, str]:
    url = unsubscribe_url(email, store=store, settings=settings)
    if not url:
        return {}
    mailto = mailto_unsubscribe(sender_email)
    value = f"<{url}>"
    if mailto:
        value = f"<{mailto}>, {value}"
    return {
        "List-Unsubscribe": value,
        "List-Unsubscribe-Post": "List-Unsubscribe=One-Click",
    }


def is_suppressed(email: str, *, store: EmailStore | None) -> bool:
    if store is None:
        return False
    return store.is_suppressed(email)


def add_suppression(
    store: EmailStore,
    email: str,
    *,
    reason: str = "unsubscribed",
    source: str = "recipient",
    created_at: str,
) -> dict[str, Any]:
    return store.add_suppression(email, reason=reason, source=source, created_at=created_at)


def apply_unsubscribe(
    store: EmailStore,
    email: str,
    *,
    reason: str = "unsubscribed",
    source: str = "recipient",
    created_at: str,
) -> dict[str, Any]:
    """Record a suppression and stop every open sequence for that address."""
    record = add_suppression(
        store,
        email,
        reason=reason,
        source=source,
        created_at=created_at,
    )
    stopped = 0
    for sequence in store.list_sequences_for_email(email):
        status = str(sequence.get("status", "") or "")
        if status in {"replied", "stopped", "completed", "failed"}:
            store.cancel_future_pending_messages(str(sequence["id"]), updated_at=created_at)
            continue
        store.update_sequence_status(
            str(sequence["id"]),
            status="stopped",
            updated_at=created_at,
            stop_reason="unsubscribed",
            next_scheduled_at="",
        )
        store.cancel_future_pending_messages(str(sequence["id"]), updated_at=created_at)
        stopped += 1
    record["sequences_stopped"] = stopped
    return record
