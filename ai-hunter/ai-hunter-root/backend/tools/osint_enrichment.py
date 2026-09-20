"""Optional OSINT enrichment after LeadExtract.

Rewrites selected spiderfoot modules as plain functions (MIT, Steve
Micallef / bcoles). Default off. Failures never abort the hunt.
"""

from __future__ import annotations

import logging
from typing import Any
from urllib.parse import urlparse

import httpx

from emailing.guards import is_role_or_generic_email
from tools.email_patterns import remember_domain_pattern
from tools.lead_contract import domain_from_website, has_usable_contact
from tools.osint_builtwith import detect_tech_stack, parse_builtwith_payload
from tools.osint_company import extract_company_names
from tools.osint_email import extract_domain_emails
from tools.osint_emailformat import emailformat_url, parse_emailformat_html
from tools.osint_github import company_github_query, github_username_from_url, parse_github_search

logger = logging.getLogger(__name__)

_USER_AGENT = "AIHunter-OSINT/0.1 (+https://localhost)"


def _push_evidence(lead: dict[str, Any], claim: str, source_url: str) -> None:
    evidence = lead.setdefault("evidence", [])
    if not isinstance(evidence, list):
        evidence = []
        lead["evidence"] = evidence
    urls = lead.setdefault("evidence_urls", [])
    if not isinstance(urls, list):
        urls = []
        lead["evidence_urls"] = urls
    if source_url and source_url not in urls:
        urls.append(source_url)
    item = {"claim": claim, "source_url": source_url}
    if item not in evidence:
        evidence.append(item)


def _merge_emails(lead: dict[str, Any], emails: list[str]) -> None:
    bucket = lead.setdefault("emails", [])
    if not isinstance(bucket, list):
        bucket = []
        lead["emails"] = bucket
    seen = {str(item).replace("(inferred)", "").strip().lower() for item in bucket}
    for email in emails:
        cleaned = str(email or "").strip().lower()
        if not cleaned or cleaned in seen:
            continue
        seen.add(cleaned)
        bucket.append(cleaned)


def _merge_social(lead: dict[str, Any], network: str, url: str) -> None:
    social = lead.setdefault("social_media", {})
    if not isinstance(social, dict):
        social = {}
        lead["social_media"] = social
    if url and not social.get(network):
        social[network] = url


async def _fetch_text(client: httpx.AsyncClient, url: str, *, timeout: float = 12.0) -> str:
    try:
        resp = await client.get(url, timeout=timeout, follow_redirects=True)
        resp.raise_for_status()
        return resp.text
    except Exception as exc:
        logger.debug("[OSINT] fetch failed %s: %s", url, exc)
        return ""


async def _fetch_json(client: httpx.AsyncClient, url: str, *, timeout: float = 12.0) -> Any:
    try:
        resp = await client.get(url, timeout=timeout, follow_redirects=True)
        resp.raise_for_status()
        return resp.json()
    except Exception as exc:
        logger.debug("[OSINT] json fetch failed %s: %s", url, exc)
        return None


async def enrich_lead(
    lead: dict[str, Any],
    *,
    settings: Any | None = None,
    client: httpx.AsyncClient | None = None,
) -> dict[str, Any]:
    """Enrich one lead in place. Always returns the lead."""
    enabled = bool(getattr(settings, "osint_enrichment_enabled", False)) if settings else False
    if not enabled:
        return lead

    website = str(lead.get("website") or "")
    domain = domain_from_website(website)
    page_text = str(lead.get("page_text") or lead.get("description") or "")
    own_client = client is None
    http = client or httpx.AsyncClient(headers={"User-Agent": _USER_AGENT}, timeout=12.0)
    try:
        if not page_text and website:
            page_text = await _fetch_text(http, website)

        extracted = extract_domain_emails(page_text, website)
        if extracted["emails"]:
            _merge_emails(lead, extracted["emails"])
            _push_evidence(lead, f"Page emails: {', '.join(extracted['emails'][:5])}", website)

        names = extract_company_names(page_text)
        if names and not str(lead.get("legal_name") or "").strip():
            lead["legal_name"] = names[0]
            _push_evidence(lead, f"Legal name: {names[0]}", website)

        stack = detect_tech_stack(page_text, website)
        api_key = str(getattr(settings, "builtwith_api_key", "") or "") if settings else ""
        if api_key and domain:
            payload = await _fetch_json(
                http,
                f"https://api.builtwith.com/rv1/api.json?LOOKUP={domain}&KEY={api_key}",
            )
            if payload:
                stack = parse_builtwith_payload(payload) or stack
        if stack:
            lead["tech_stack"] = stack
            _push_evidence(lead, "Tech stack: " + ", ".join(stack[:6]), website)

        skip_emailformat = has_usable_contact(lead)
        if domain and not skip_emailformat:
            html = await _fetch_text(http, emailformat_url(domain))
            if html:
                parsed = parse_emailformat_html(html, domain)
                personal = [e for e in parsed["emails"] if not is_role_or_generic_email(e)]
                _merge_emails(lead, personal)
                for key in parsed["patterns"][:2]:
                    remember_domain_pattern(domain, key)
                if personal or parsed["patterns"]:
                    _push_evidence(
                        lead,
                        "email-format.com: "
                        + (", ".join(personal[:3]) or "patterns " + ", ".join(parsed["patterns"][:3])),
                        emailformat_url(domain),
                    )

        existing_github = ""
        social = lead.get("social_media") or {}
        if isinstance(social, dict):
            existing_github = str(social.get("github") or social.get("GitHub") or "")
        login = github_username_from_url(existing_github) or company_github_query(
            str(lead.get("company_name") or ""), domain
        )
        if login:
            payload = await _fetch_json(
                http,
                f"https://api.github.com/search/users?q={login}+type:org",
            )
            orgs = parse_github_search(payload, login)
            if orgs:
                org = orgs[0]
                _merge_social(lead, "github", org["html_url"])
                lead["github_org"] = org["login"]
                _push_evidence(lead, f"GitHub org: {org['login']}", org["html_url"])
    except Exception as exc:
        logger.warning("[OSINT] enrichment skipped for %s: %s", domain or website, exc)
    finally:
        if own_client:
            await http.aclose()
    return lead


async def enrich_leads(leads: list[dict[str, Any]], *, settings: Any | None = None) -> list[dict[str, Any]]:
    if not leads:
        return leads
    enabled = bool(getattr(settings, "osint_enrichment_enabled", False)) if settings else False
    if not enabled:
        return leads
    async with httpx.AsyncClient(headers={"User-Agent": _USER_AGENT}, timeout=12.0) as client:
        out: list[dict[str, Any]] = []
        for lead in leads:
            if not isinstance(lead, dict):
                continue
            out.append(await enrich_lead(lead, settings=settings, client=client))
        return out


def homepage_url(website: str) -> str:
    parsed = urlparse(website or "")
    if parsed.scheme and parsed.netloc:
        return f"{parsed.scheme}://{parsed.netloc}"
    host = domain_from_website(website)
    return f"https://{host}" if host else ""
