"""Unified email sending entrypoint."""

from __future__ import annotations

import asyncio
import email.utils
import smtplib
import ssl
from email.message import EmailMessage
from typing import Any
from uuid import uuid4

from datetime import datetime, timezone

from config.settings import get_settings
from emailing.body_format import format_plaintext_email_body
from emailing.guards import screen_recipient
from emailing.store import EmailStore
from emailing.suppression import unsubscribe_headers, unsubscribe_url, with_unsubscribe_footer
from auth_sso import scoped_email_db_path


def _send_via_smtp_sync(
    account: dict[str, Any],
    *,
    to_email: str,
    subject: str,
    body_text: str,
    reply_to: str | None = None,
    thread_key: str | None = None,
    extra_headers: dict[str, str] | None = None,
) -> dict[str, Any]:
    msg = EmailMessage()
    msg["From"] = email.utils.formataddr((account.get("from_name", ""), account.get("from_email", "")))
    msg["To"] = to_email
    msg["Subject"] = subject
    if reply_to:
        msg["Reply-To"] = reply_to
    for header, value in (extra_headers or {}).items():
        if header and value:
            msg[header] = value
    message_id = f"<{uuid4()}@{(account.get('from_email') or 'localhost').split('@')[-1] or 'localhost'}>"
    msg["Message-ID"] = message_id
    msg["Date"] = email.utils.formatdate(localtime=True)
    msg.set_content(format_plaintext_email_body(body_text))

    host = str(account.get("smtp_host", "") or "").strip()
    port = int(account.get("smtp_port", 587) or 587)
    username = str(account.get("smtp_username", "") or "").strip()
    password = str(account.get("smtp_secret_encrypted", "") or "")
    use_tls = bool(account.get("use_tls", True))

    if not host or not username or not password:
        return {
            "ok": False,
            "provider": "smtp",
            "provider_message_id": "",
            "thread_key": thread_key or subject,
            "sent_at": "",
            "error": "smtp_account_incomplete",
            "error_type": "auth_error",
        }

    context = ssl.create_default_context()
    with smtplib.SMTP(host, port, timeout=20) as server:
        server.ehlo()
        if use_tls:
            server.starttls(context=context)
            server.ehlo()
        server.login(username, password)
        server.send_message(msg)

    return {
        "ok": True,
        "provider": "smtp",
        "provider_message_id": message_id,
        "thread_key": thread_key or subject,
        "sent_at": email.utils.formatdate(localtime=False),
        "error": "",
        "error_type": "",
    }


async def send_email(
    account: dict[str, Any],
    *,
    to_email: str,
    subject: str,
    body_text: str,
    reply_to: str | None = None,
    thread_key: str | None = None,
    owner_subject: str | None = None,
) -> dict[str, Any]:
    """Send one email via the configured provider."""
    if not to_email.strip():
        return {
            "ok": False,
            "provider": str(account.get("provider_type", "smtp") or "smtp"),
            "provider_message_id": "",
            "thread_key": thread_key or subject,
            "sent_at": "",
            "error": "missing_recipient",
            "error_type": "invalid_recipient",
        }

    settings = get_settings()
    sender_email = str(account.get("from_email", "") or "")
    store = EmailStore(
        scoped_email_db_path(
            str(getattr(settings, "email_db_path", "") or "email_automation.db"),
            owner_subject,
        )
    )
    store.init_db()
    recipient_verdict = screen_recipient(
        to_email,
        settings=settings,
        sender_email=sender_email,
        store=store,
    )
    if not recipient_verdict.ok:
        return {
            "ok": False,
            "provider": str(account.get("provider_type", "smtp") or "smtp"),
            "provider_message_id": "",
            "thread_key": thread_key or subject,
            "sent_at": "",
            "error": recipient_verdict.reason,
            "error_type": "blocked_recipient",
        }

    unsub_url = unsubscribe_url(to_email, store=store, settings=settings)
    headers = unsubscribe_headers(
        to_email,
        store=store,
        settings=settings,
        sender_email=sender_email,
    )
    outbound_body = with_unsubscribe_footer(format_plaintext_email_body(body_text), unsub_url)

    dry_run = bool(getattr(settings, "email_dry_run", True)) or str(account.get("provider_type", "")).lower() == "dry_run"
    if dry_run:
        return {
            "ok": True,
            "provider": "dry_run",
            "provider_message_id": f"<dry-run-{uuid4()}@localhost>",
            "thread_key": thread_key or subject,
            "sent_at": datetime.now(timezone.utc).isoformat(),
            "error": "",
            "error_type": "",
            "dry_run": True,
        }

    provider = str(account.get("provider_type", "smtp") or "smtp").lower()
    try:
        if provider == "smtp":
            return await asyncio.to_thread(
                _send_via_smtp_sync,
                account,
                to_email=to_email,
                subject=subject,
                body_text=outbound_body,
                reply_to=reply_to,
                thread_key=thread_key,
                extra_headers=headers,
            )
        return {
            "ok": False,
            "provider": provider,
            "provider_message_id": "",
            "thread_key": thread_key or subject,
            "sent_at": "",
            "error": f"unsupported_provider:{provider}",
            "error_type": "permanent_failure",
        }
    except smtplib.SMTPAuthenticationError as exc:
        return {"ok": False, "provider": provider, "provider_message_id": "", "thread_key": thread_key or subject, "sent_at": "", "error": str(exc), "error_type": "auth_error"}
    except smtplib.SMTPRecipientsRefused as exc:
        return {"ok": False, "provider": provider, "provider_message_id": "", "thread_key": thread_key or subject, "sent_at": "", "error": str(exc), "error_type": "invalid_recipient"}
    except smtplib.SMTPResponseException as exc:
        error_type = "temporary_failure" if 400 <= exc.smtp_code < 500 else "permanent_failure"
        return {"ok": False, "provider": provider, "provider_message_id": "", "thread_key": thread_key or subject, "sent_at": "", "error": str(exc), "error_type": error_type}
    except (TimeoutError, OSError) as exc:
        return {"ok": False, "provider": provider, "provider_message_id": "", "thread_key": thread_key or subject, "sent_at": "", "error": str(exc), "error_type": "network_error"}
