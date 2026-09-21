"""Extract work emails from free-text pages.

Python rewrite of spiderfoot `modules/sfp_email.py`
(MIT, Copyright (c) Steve Micallef 2012). Logic ported, SpiderFoot
runtime was not copied.
"""

from __future__ import annotations

from tools.email_finder import extract_emails_from_text
from tools.lead_contract import domain_from_website


def extract_domain_emails(text: str, website: str = "", *, limit: int = 20) -> dict[str, list[str]]:
    """Split extracted inboxes into same-domain vs affiliate."""
    host = domain_from_website(website)
    same: list[str] = []
    affiliate: list[str] = []
    seen: set[str] = set()
    for email in extract_emails_from_text(text or ""):
        if email in seen:
            continue
        seen.add(email)
        domain = email.split("@", 1)[1]
        if host and (domain == host or domain.endswith("." + host)):
            same.append(email)
        else:
            affiliate.append(email)
        if len(same) + len(affiliate) >= limit:
            break
    return {"emails": same, "affiliate_emails": affiliate}
