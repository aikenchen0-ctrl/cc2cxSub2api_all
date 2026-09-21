"""Find a public GitHub org that matches the company.

Python rewrite of spiderfoot `modules/sfp_github.py`
(MIT, Copyright (c) Steve Micallef 2015). Logic ported, SpiderFoot
runtime was not copied. Uses the public GitHub search API.
"""

from __future__ import annotations

import re
from typing import Any
from urllib.parse import urlparse


def github_username_from_url(url: str) -> str:
    host = urlparse(url or "").netloc.lower()
    if host not in {"github.com", "www.github.com"}:
        return ""
    parts = [p for p in urlparse(url).path.split("/") if p]
    if not parts or parts[0] in {"orgs", "search", "topics", "about"}:
        return ""
    return parts[0]


def company_github_query(company_name: str, domain: str = "") -> str:
    slug = (domain or "").split(".")[0].strip()
    if slug and slug not in {"www", "com", "net", "org"}:
        return slug
    name = re.sub(r"[^A-Za-z0-9]+", " ", company_name or "").strip()
    return name.split(" ")[0] if name else ""


def parse_github_search(payload: Any, query: str, *, names_only: bool = True) -> list[dict[str, str]]:
    items = []
    if isinstance(payload, dict):
        raw_items = payload.get("items") or []
    elif isinstance(payload, list):
        raw_items = payload
    else:
        raw_items = []
    needle = (query or "").lower()
    for item in raw_items:
        if not isinstance(item, dict):
            continue
        login = str(item.get("login") or item.get("name") or "").strip()
        html_url = str(item.get("html_url") or "").strip()
        if not login or not html_url:
            continue
        if names_only and needle and needle not in login.lower() and needle not in str(item.get("name") or "").lower():
            continue
        items.append({
            "login": login,
            "html_url": html_url,
            "type": str(item.get("type") or ""),
            "description": str(item.get("description") or item.get("bio") or ""),
        })
        if len(items) >= 5:
            break
    return items
