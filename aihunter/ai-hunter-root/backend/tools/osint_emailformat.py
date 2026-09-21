"""Look up published inboxes for a company domain.

Python rewrite of spiderfoot `modules/sfp_emailformat.py`
(MIT, Copyright (c) bcoles 2018). Logic ported, SpiderFoot runtime
was not copied. Talks to the public email-format.com HTML page.
"""

from __future__ import annotations

import re

from tools.email_finder import extract_emails_from_text
from tools.email_patterns import detect_email_pattern, parse_name_parts

_MASKED = re.compile(r"^[0-9a-f]{8}\.[0-9]{7}@", re.I)
_FORMAT_HINT = re.compile(
    r"\b(first\.last|firstlast|flast|f\.last|first_last|first)\b",
    re.I,
)


def parse_emailformat_html(html: str, domain: str) -> dict[str, list[str]]:
    """Pull same-domain emails and format hints from an email-format page."""
    host = (domain or "").strip().lower().removeprefix("www.")
    emails: list[str] = []
    seen: set[str] = set()
    for email in extract_emails_from_text(html or ""):
        if _MASKED.match(email):
            continue
        mail_host = email.split("@", 1)[1]
        if host and mail_host != host and not mail_host.endswith("." + host):
            continue
        if email not in seen:
            seen.add(email)
            emails.append(email)
    patterns: list[str] = []
    for match in _FORMAT_HINT.findall(html or ""):
        key = match.lower()
        if key not in patterns:
            patterns.append(key)
    for email in emails:
        local = email.split("@", 1)[0]
        guessed_name = local.replace(".", " ").replace("_", " ")
        if not parse_name_parts(guessed_name):
            continue
        detected = detect_email_pattern(email, guessed_name, host)
        if detected and detected not in patterns:
            patterns.append(detected)
    return {"emails": emails, "patterns": patterns}


def emailformat_url(domain: str) -> str:
    host = (domain or "").strip().lower().removeprefix("www.")
    return f"https://www.email-format.com/d/{host}/"
