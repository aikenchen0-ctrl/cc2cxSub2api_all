"""Offline tests for the Firecrawl-inspired high-value site scanner."""

import pytest

from tools.site_scanner import (
    DEFAULT_MAX_PAGES,
    HIGH_VALUE_PATHS,
    MAX_PAGES_HARD_CAP,
    clean_domain,
    discover_same_origin_links,
    empty_brief,
    format_brief_for_prompt,
    normalize_page_url,
    path_score,
    peek_homepage,
    rank_urls,
    scan_site,
    seed_urls,
    strip_html,
)


def _settings(**kwargs):
    defaults = {
        "site_scan_enabled": True,
        "firecrawl_api_key": "",
        "site_scan_max_pages": 4,
    }
    defaults.update(kwargs)
    return type("S", (), defaults)()


def test_clean_domain_and_normalize():
    assert clean_domain("https://www.Acme.com/about") == "acme.com"
    assert clean_domain("acme.com/pricing") == "acme.com"
    assert clean_domain("") == ""
    assert normalize_page_url("https://acme.com/about/") == "https://acme.com/about"
    assert normalize_page_url("not a url") == ""


def test_path_score_prefers_product_pages():
    assert path_score("https://acme.com/") == 100
    assert path_score("https://acme.com/pricing") > path_score("https://acme.com/random")
    assert path_score("https://acme.com/about") > 0
    assert path_score("https://acme.com/blog/hello") < 0
    assert path_score("https://acme.com/careers") < 0
    assert path_score("https://acme.com/legal/privacy") < 0
    assert "pricing" in HIGH_VALUE_PATHS


def test_rank_urls_caps_and_drops_junk():
    urls = [
        "https://acme.com/blog/post",
        "https://acme.com/pricing",
        "https://acme.com/about",
        "https://acme.com/",
        "https://other.com/pricing",
        "https://acme.com/pricing",
        "https://acme.com/docs/api",
    ]
    ranked = rank_urls(urls, max_pages=3, domain="acme.com")
    assert ranked[0] == "https://acme.com"
    assert "https://acme.com/pricing" in ranked
    assert "https://acme.com/about" in ranked
    assert all("blog" not in u and "docs" not in u for u in ranked)
    assert all(clean_domain(u) == "acme.com" for u in ranked)
    assert len(ranked) <= 3
    assert len(rank_urls(urls, max_pages=99, domain="acme.com")) <= MAX_PAGES_HARD_CAP


def test_strip_html_keeps_markdown_links():
    html = (
        "<html><head><title>Acme</title></head><body>"
        "<nav>ignore</nav>"
        "<p>We sell inverters.</p>"
        '<a href="https://rival.com">Rival Tools</a>'
        "<script>alert(1)</script>"
        "</body></html>"
    )
    text = strip_html(html)
    assert "We sell inverters." in text
    assert "[Rival Tools](https://rival.com)" in text
    assert "alert" not in text
    assert "ignore" not in text


def test_discover_same_origin_links():
    html = """
    <a href="/about">About</a>
    <a href="https://acme.com/pricing">Pricing</a>
    <a href="https://other.com/x">Out</a>
    <a href="#top">Skip</a>
    """
    links = discover_same_origin_links(html, "acme.com")
    assert "https://acme.com" in links
    assert "https://acme.com/about" in links
    assert "https://acme.com/pricing" in links
    assert all("other.com" not in u for u in links)


def test_format_brief_for_prompt_truncates():
    brief = {
        "pages": [
            {"url": "https://acme.com", "title": "Home", "markdown": "Hello world " * 400},
            {"url": "https://acme.com/about", "title": "About", "markdown": "About us " * 400},
        ]
    }
    md = format_brief_for_prompt(brief)
    assert "Source: https://acme.com" in md
    assert "Home" in md
    assert len(md) <= 10000


@pytest.mark.asyncio
async def test_scan_site_disabled_is_noop():
    brief = await scan_site("https://acme.com", settings=_settings(site_scan_enabled=False))
    assert brief["skipped"] is True
    assert brief["reason"] == "disabled"
    assert brief["pages"] == []
    assert brief["enabled"] is False


@pytest.mark.asyncio
async def test_scan_site_no_url():
    brief = await scan_site("", settings=_settings())
    assert brief["skipped"] is True
    assert brief["reason"] == "no_url"


@pytest.mark.asyncio
async def test_scan_site_fetch_fallback(monkeypatch):
    html_home = (
        "<html><head><title>Acme Tools</title></head><body>"
        + ("We manufacture 5-50kW solar inverters for distributors. " * 20)
        + '<a href="/about">About</a>'
        + '<a href="/pricing">Pricing</a>'
        + "</body></html>"
    )
    html_about = (
        "<html><head><title>About Acme</title></head><body>"
        + ("Factory in Shenzhen, CE certified, MOQ 10. " * 20)
        + "</body></html>"
    )
    html_pricing = (
        "<html><head><title>Pricing</title></head><body>"
        + ("Distributor pricing starts at factory-direct. " * 20)
        + "</body></html>"
    )
    pages = {
        "https://acme.com": html_home,
        "https://acme.com/about": html_about,
        "https://acme.com/pricing": html_pricing,
    }

    async def fake_get(url, *, timeout=20.0, headers=None):
        body = pages.get(url.rstrip("/"), pages.get(url, ""))
        if not body:
            return 404, ""
        return 200, body

    monkeypatch.setattr("tools.site_scanner._http_get", fake_get)
    brief = await scan_site("https://acme.com", settings=_settings(firecrawl_api_key=""))
    assert brief["skipped"] is False
    assert brief["provider"] == "fetch"
    assert brief["domain"] == "acme.com"
    assert len(brief["pages"]) >= 1
    urls = [p["url"] for p in brief["pages"]]
    assert any(u.rstrip("/") == "https://acme.com" for u in urls)
    assert "inverters" in brief["brief_markdown"].lower() or "inverters" in brief["pages"][0]["markdown"].lower()


@pytest.mark.asyncio
async def test_scan_site_firecrawl_then_fetch_fallback(monkeypatch):
    async def boom_map(*_a, **_k):
        raise RuntimeError("Firecrawl map HTTP 401")

    html = (
        "<html><head><title>Acme</title></head><body>"
        + ("Product homepage content for distributors worldwide. " * 20)
        + "</body></html>"
    )

    async def fake_get(url, *, timeout=20.0, headers=None):
        return 200, html

    monkeypatch.setattr("tools.site_scanner.firecrawl_map", boom_map)
    monkeypatch.setattr("tools.site_scanner._http_get", fake_get)
    brief = await scan_site(
        "https://acme.com",
        settings=_settings(firecrawl_api_key="fc-test"),
    )
    assert brief["skipped"] is False
    assert brief["provider"] == "fetch"
    assert brief["pages"]


@pytest.mark.asyncio
async def test_scan_site_never_raises(monkeypatch):
    async def boom(*_a, **_k):
        raise RuntimeError("network down")

    monkeypatch.setattr("tools.site_scanner._read_with", boom)
    brief = await scan_site("https://acme.com", settings=_settings())
    assert brief["skipped"] is True
    assert "error" in brief["reason"]


@pytest.mark.asyncio
async def test_peek_homepage_never_uses_firecrawl(monkeypatch):
    called = {"post": 0}

    async def no_post(*_a, **_k):
        called["post"] += 1
        raise AssertionError("Firecrawl must not be called")

    html = "<html><head><title>Peek</title></head><body>" + ("Hello company " * 40) + "</body></html>"

    async def fake_get(url, *, timeout=20.0, headers=None):
        return 200, html

    monkeypatch.setattr("tools.site_scanner._http_post_json", no_post)
    monkeypatch.setattr("tools.site_scanner._http_get", fake_get)
    peek = await peek_homepage("acme.com")
    assert peek is not None
    assert peek["title"] == "Peek"
    assert called["post"] == 0


def test_empty_brief_shape():
    brief = empty_brief(skipped=True, reason="disabled", domain="acme.com", enabled=False)
    assert brief["pages"] == []
    assert brief["brief_markdown"] == ""
    assert DEFAULT_MAX_PAGES >= 1
    assert seed_urls("acme.com")[0] == "https://acme.com"
    assert "https://acme.com/pricing" in seed_urls("acme.com")
