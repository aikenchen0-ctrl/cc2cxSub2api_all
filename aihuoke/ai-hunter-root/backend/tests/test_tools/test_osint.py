"""Tests for spiderfoot-inspired OSINT helpers (offline, no live HTTP)."""

import pytest

from tools.osint_builtwith import detect_tech_stack, parse_builtwith_payload
from tools.osint_company import extract_company_names
from tools.osint_email import extract_domain_emails
from tools.osint_emailformat import parse_emailformat_html
from tools.osint_enrichment import enrich_lead, enrich_leads
from tools.osint_github import company_github_query, github_username_from_url, parse_github_search


def test_extract_domain_emails_splits_affiliate():
    text = "Contact jane.doe@acme.com or press@media.org"
    parsed = extract_domain_emails(text, "https://www.acme.com")
    assert parsed["emails"] == ["jane.doe@acme.com"]
    assert parsed["affiliate_emails"] == ["press@media.org"]


def test_extract_company_names_legal_suffix():
    names = extract_company_names("Welcome to Acme Tools Ltd, a hardware distributor.")
    assert any("Acme Tools" in name and "Ltd" in name for name in names)


def test_detect_shopify_and_wordpress():
    html = '<script src="https://cdn.shopify.com/s/files/1/theme.js"></script>'
    assert "Shopify" in detect_tech_stack(html, "https://shop.example.com")
    assert "WordPress" in detect_tech_stack("wp-content/themes/foo", "https://blog.example.com")


def test_parse_builtwith_payload_names():
    payload = {
        "Results": [
            {"Result": {"Paths": [{"Technologies": [{"Name": "Cloudflare"}, {"Name": "Shopify"}]}]}}
        ]
    }
    assert parse_builtwith_payload(payload) == ["Cloudflare", "Shopify"]


def test_parse_emailformat_html_skips_masked():
    html = """
    <tbody>
      <tr><td>jane.doe@acme.com</td></tr>
      <tr><td>aabbccdd.1234567@acme.com</td></tr>
    </tbody>
    Most common format: first.last
    """
    parsed = parse_emailformat_html(html, "acme.com")
    assert parsed["emails"] == ["jane.doe@acme.com"]
    assert "first.last" in parsed["patterns"]


def test_github_helpers():
    assert github_username_from_url("https://github.com/acme-tools") == "acme-tools"
    assert company_github_query("Acme Tools", "acme.com") == "acme"
    payload = {
        "items": [
            {"login": "acme", "html_url": "https://github.com/acme", "type": "Organization"},
            {"login": "other", "html_url": "https://github.com/other", "type": "User"},
        ]
    }
    orgs = parse_github_search(payload, "acme")
    assert orgs[0]["login"] == "acme"


@pytest.mark.asyncio
async def test_enrich_lead_offline_page_text(monkeypatch):
    async def no_fetch(*_args, **_kwargs):
        return ""

    async def no_json(*_args, **_kwargs):
        return None

    monkeypatch.setattr("tools.osint_enrichment._fetch_text", no_fetch)
    monkeypatch.setattr("tools.osint_enrichment._fetch_json", no_json)
    lead = {
        "company_name": "Acme Tools",
        "website": "https://acme.com",
        "page_text": "Acme Tools Ltd sells hardware. Email jane.doe@acme.com. Built with Shopify.",
        "emails": [],
        "evidence": [],
    }
    settings = type("S", (), {"osint_enrichment_enabled": True, "builtwith_api_key": ""})()
    enriched = await enrich_lead(lead, settings=settings)
    assert "jane.doe@acme.com" in enriched["emails"]
    assert enriched["legal_name"]
    assert "Shopify" in enriched["tech_stack"]
    assert any("acme.com" in url for url in enriched["evidence_urls"])


@pytest.mark.asyncio
async def test_enrich_leads_noop_when_disabled():
    lead = {"company_name": "Acme", "website": "https://acme.com", "emails": []}
    out = await enrich_leads([lead], settings=type("S", (), {"osint_enrichment_enabled": False})())
    assert out[0]["emails"] == []


@pytest.mark.asyncio
async def test_enrich_lead_skips_emailformat_when_usable_contact(monkeypatch):
    fetched: list[str] = []

    async def capture_fetch(_client, url, *, timeout=12.0):
        fetched.append(url)
        return ""

    async def no_json(*_args, **_kwargs):
        return None

    monkeypatch.setattr("tools.osint_enrichment._fetch_text", capture_fetch)
    monkeypatch.setattr("tools.osint_enrichment._fetch_json", no_json)
    lead = {
        "company_name": "Acme Tools",
        "website": "https://acme.com",
        "page_text": "Acme Tools Ltd",
        "emails": ["jane.doe@acme.com"],
        "evidence": [],
    }
    settings = type("S", (), {"osint_enrichment_enabled": True, "builtwith_api_key": ""})()
    await enrich_lead(lead, settings=settings)
    assert not any("email-format.com" in url for url in fetched)
