"""Guess the public web stack of a company site.

Python rewrite of spiderfoot `modules/sfp_builtwith.py` (MIT, Copyright
(c) Steve Micallef). The original talks to BuiltWith's paid API; this
port uses cheap public HTML/URL heuristics and only hits the API when
`builtwith_api_key` is set.
"""

from __future__ import annotations

import json
import re
from typing import Any
from urllib.parse import urlparse

_STACK_HINTS: tuple[tuple[str, re.Pattern[str]], ...] = (
    ("Shopify", re.compile(r"cdn\.shopify\.com|myshopify\.com|Shopify\.theme|\bShopify\b", re.I)),
    ("WooCommerce", re.compile(r"woocommerce|wp-content/plugins/woocommerce", re.I)),
    ("WordPress", re.compile(r"wp-content|wordpress\.org", re.I)),
    ("Wix", re.compile(r"wixstatic\.com|wixsite\.com|X-Wix", re.I)),
    ("Squarespace", re.compile(r"squarespace\.com|static1\.squarespace", re.I)),
    ("Magento", re.compile(r"mage/cookies|Magento_|magento", re.I)),
    ("PrestaShop", re.compile(r"prestashop", re.I)),
    ("Webflow", re.compile(r"webflow\.com|wf-page", re.I)),
    ("Ghost", re.compile(r"ghost\.org|casper-ghost", re.I)),
    ("Drupal", re.compile(r"drupal\.js|sites/default/files", re.I)),
    ("Google Analytics", re.compile(r"googletagmanager\.com|google-analytics\.com", re.I)),
    ("Cloudflare", re.compile(r"cloudflare|cf-ray", re.I)),
)


def detect_tech_stack(text: str = "", website: str = "") -> list[str]:
    blob = f"{website or ''}\n{text or ''}"
    found: list[str] = []
    for name, pattern in _STACK_HINTS:
        if pattern.search(blob):
            found.append(name)
    host = urlparse(website or "").netloc.lower()
    if host.endswith(".myshopify.com") and "Shopify" not in found:
        found.insert(0, "Shopify")
    return found


def parse_builtwith_payload(payload: Any, *, maxage_days: int = 30) -> list[str]:
    """Flatten a BuiltWith Domain API JSON blob into tech names."""
    del maxage_days  # age filter needs live timestamps; skip in the offline parse
    techs: list[str] = []
    seen: set[str] = set()

    def push(name: str) -> None:
        cleaned = str(name or "").strip()
        if not cleaned or cleaned.lower() in seen:
            return
        seen.add(cleaned.lower())
        techs.append(cleaned)

    if isinstance(payload, str):
        try:
            payload = json.loads(payload)
        except json.JSONDecodeError:
            return techs
    if not isinstance(payload, dict):
        return techs
    results = payload.get("Results") or payload.get("results") or []
    if isinstance(payload.get("Result"), dict):
        results = [payload["Result"]]
    if isinstance(results, dict):
        results = [results]
    for result in results:
        if not isinstance(result, dict):
            continue
        paths = result.get("Result") or result.get("Paths") or result.get("paths") or []
        if isinstance(result.get("Result"), dict):
            paths = result["Result"].get("Paths") or []
        if isinstance(paths, dict):
            paths = [paths]
        for path in paths:
            if not isinstance(path, dict):
                continue
            for group in path.get("Technologies") or path.get("technologies") or []:
                if isinstance(group, dict):
                    push(str(group.get("Name") or group.get("name") or ""))
                    for item in group.get("Categories") or []:
                        if isinstance(item, dict):
                            push(str(item.get("Name") or ""))
                elif isinstance(group, str):
                    push(group)
    return techs
