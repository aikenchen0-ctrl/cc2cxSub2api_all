"""Tests for tools/lead_enrichment_view.py — P2/P3 fields for UI and EmailCraft."""

from emailing.template_pipeline import build_fallback_template_profile
from tools.lead_enrichment_view import (
    enrichment_hooks,
    format_enrichment_block,
    social_profile_rows,
)


def _enriched_lead() -> dict:
    return {
        "company_name": "Acme Tools",
        "industry": "Industrial Supply",
        "legal_name": "Acme Tools GmbH",
        "tech_stack": ["Shopify", "Cloudflare"],
        "github_org": "acme-tools",
        "social_profiles": [
            {"network": "linkedin", "url": "https://www.linkedin.com/company/acme", "username": "acme"},
            {"network": "github", "url": "https://github.com/acme-tools", "username": "acme-tools"},
            {"network": "linkedin", "url": "https://www.linkedin.com/company/acme", "username": "dup"},
        ],
        "evidence_urls": ["https://acme.example/about", "https://acme.example/about"],
        "reason": "Operator-only: do not paste into outreach.",
    }


def test_social_profile_rows_dedupes_and_caps():
    rows = social_profile_rows(_enriched_lead())
    assert len(rows) == 2
    assert rows[0]["network"] == "linkedin"
    assert rows[1]["url"] == "https://github.com/acme-tools"


def test_format_enrichment_block_includes_osint_and_social_not_reason():
    block = format_enrichment_block(_enriched_lead())
    assert "Acme Tools GmbH" in block
    assert "Shopify" in block
    assert "github.com/acme-tools" in block
    assert "linkedin.com/company/acme" in block
    assert "acme.example/about" in block
    assert "Operator-only" not in block


def test_format_enrichment_block_empty_when_modules_did_not_run():
    assert format_enrichment_block({"company_name": "Bare"}) == ""
    assert enrichment_hooks({"company_name": "Bare"}) == []


def test_enrichment_hooks_are_short_and_actionable():
    hooks = enrichment_hooks(_enriched_lead())
    assert any("legal name" in item for item in hooks)
    assert any("Shopify" in item for item in hooks)
    assert any("github" in item for item in hooks)


def test_fallback_template_profile_mentions_enrichment():
    profile = build_fallback_template_profile(
        examples=[],
        lead=_enriched_lead(),
        insight={"products": ["switch"]},
    )
    assert "Acme Tools GmbH" in profile["template_notes"]
    assert "Shopify" in profile["template_notes"]
