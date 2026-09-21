"""Tests for maigret-inspired social presence helpers (offline, no live HTTP)."""

import pytest

from tools.social_presence import (
    DEFAULT_ALLOWED_SITES,
    DEFAULT_MAX_SITES,
    enrich_lead_social,
    enrich_leads_social,
    merge_social_profiles,
    normalize_username,
    parse_maigret_results,
    profile_url,
    scan_username,
    select_sites,
    username_from_url,
    usernames_from_lead,
)


def test_normalize_username_rejects_urls_and_emails():
    assert normalize_username("jane.doe") == "jane.doe"
    assert normalize_username("@jane_doe") == "jane_doe"
    assert normalize_username("https://github.com/jane") == ""
    assert normalize_username("jane@acme.com") == ""
    assert normalize_username("张三") == ""
    assert normalize_username("a") == ""


def test_username_from_known_urls():
    assert username_from_url("https://github.com/acme-tools") == "acme-tools"
    assert username_from_url("https://www.linkedin.com/in/jane-doe") == "jane-doe"
    assert username_from_url("https://x.com/acme") == "acme"
    assert username_from_url("https://www.youtube.com/@acme") == "acme"
    assert username_from_url("https://github.com/orgs/acme") == ""


def test_usernames_from_lead_prefers_handles_over_cjk_names():
    lead = {
        "first_name": "李",
        "last_name": "雷",
        "contact_person": "李雷",
        "github_org": "acme-tools",
        "social_media": {"twitter": "https://x.com/acme_io"},
        "emails": ["jane.doe@acme.com"],
    }
    names = usernames_from_lead(lead)
    assert "acme-tools" in names
    assert "acme_io" in names
    assert "jane.doe" in names
    assert all(name.isascii() for name in names)


def test_select_sites_whitelist_then_country_cap():
    sites = select_sites(country_tags=["de"], max_sites=8)
    assert sites[0] in DEFAULT_ALLOWED_SITES
    assert "xing" in sites
    assert len(sites) <= 8
    assert len(select_sites(max_sites=99)) <= DEFAULT_MAX_SITES


def test_parse_maigret_results_keeps_claimed_only():
    class Status:
        status = "Claimed"

    class Result:
        def __init__(self, status, url):
            self.status = status
            self.url_user = url

    raw = {
        "GitHub": Result(Status(), "https://github.com/acme"),
        "MissingSite": {"status": {"status": "available"}, "url_user": "https://example.com/x"},
    }
    rows = parse_maigret_results(raw, username="acme")
    assert rows == [
        {
            "network": "github",
            "url": "https://github.com/acme",
            "username": "acme",
            "status": "claimed",
        }
    ]


def test_merge_social_profiles_fills_linkedin_and_github():
    lead = {"social_media": {}, "linkedin_url": "", "github_org": ""}
    merge_social_profiles(
        lead,
        [
            {
                "network": "linkedin",
                "url": "https://www.linkedin.com/in/jane",
                "username": "jane",
                "status": "claimed",
            },
            {
                "network": "github",
                "url": "https://github.com/jane",
                "username": "jane",
                "status": "claimed",
            },
        ],
    )
    assert lead["linkedin_url"].endswith("/jane")
    assert lead["github_org"] == "jane"
    assert lead["social_media"]["github"] == "https://github.com/jane"
    assert len(lead["social_profiles"]) == 2


@pytest.mark.asyncio
async def test_scan_username_respects_probe_and_cap():
    seen: list[str] = []

    async def probe(url: str) -> bool:
        seen.append(url)
        return "github.com" in url or "x.com" in url

    rows = await scan_username(
        "acme",
        allowed_sites=("github", "twitter", "facebook"),
        max_sites=2,
        probe=probe,
        use_maigret=False,
    )
    assert len(seen) <= 2
    networks = {row["network"] for row in rows}
    assert networks <= {"github", "twitter"}
    assert all(row["username"] == "acme" for row in rows)


@pytest.mark.asyncio
async def test_enrich_lead_social_disabled_is_noop():
    lead = {
        "company_name": "Acme",
        "website": "https://acme.com",
        "github_org": "acme",
        "social_profiles": [],
    }
    settings = type("S", (), {"social_presence_enabled": False})()
    out = await enrich_lead_social(lead, settings=settings)
    assert out["social_profiles"] == []


@pytest.mark.asyncio
async def test_enrich_leads_social_writes_profiles():
    async def probe(url: str) -> bool:
        return "github.com" in url

    lead = {
        "company_name": "Acme",
        "website": "https://acme.com",
        "github_org": "acme",
        "social_media": {},
        "social_profiles": [],
        "country_code": "us",
    }
    settings = type(
        "S",
        (),
        {
            "social_presence_enabled": True,
            "social_presence_use_maigret": False,
            "social_presence_max_sites": 6,
            "social_presence_country_tags": "",
        },
    )()
    out = await enrich_leads_social([lead], settings=settings, probe=probe)
    assert any(row["network"] == "github" for row in out[0]["social_profiles"])
    assert out[0]["social_media"]["github"].startswith("https://github.com/")


def test_profile_url_templates():
    assert profile_url("github", "acme") == "https://github.com/acme"
    assert profile_url("youtube", "acme") == "https://www.youtube.com/@acme"
    assert profile_url("unknown", "acme") == ""
