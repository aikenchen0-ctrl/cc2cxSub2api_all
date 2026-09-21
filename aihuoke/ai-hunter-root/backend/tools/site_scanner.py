"""High-value site scan for Insight.

Inspired by ai-outreach-engine (MIT, Copyright (c) 2026 Rohit Malhotra)
`providers/crawler.ts` + `pipeline/stages/scanSite.ts`: spend the page
budget on pricing / about / product, crawl with Firecrawl, fall back to
plain fetch. Python rewrite — no TypeScript copied.

Default off. Failures never abort the hunt. Never crawl a whole site.
"""

from __future__ import annotations

import asyncio
import logging
import re
from typing import Any
from urllib.parse import urljoin, urlparse

import httpx

logger = logging.getLogger(__name__)

FIRECRAWL_BASE = "https://api.firecrawl.dev/v2"
USER_AGENT = "AIHunter-SiteScan/0.1 (+personal research tool)"
DEFAULT_MAX_PAGES = 6
MAX_PAGES_HARD_CAP = 12
MAP_LIMIT_CAP = 50
PER_PAGE_MARKDOWN = 8000
PER_PAGE_BRIEF = 2500
TOTAL_BRIEF_CHARS = 10000
FETCH_TIMEOUT = 20.0
FIRECRAWL_TIMEOUT = 30.0
SCRAPE_CONCURRENCY = 2

# Pages that actually describe what a product is and who buys it.
HIGH_VALUE_PATHS: tuple[str, ...] = (
    "",
    "pricing",
    "plans",
    "features",
    "product",
    "products",
    "platform",
    "about",
    "about-us",
    "use-cases",
    "usecases",
    "solutions",
    "industries",
    "customers",
    "case-studies",
    "how-it-works",
    "why",
    "compare",
    "vs",
    "for",
    "demo",
    "faq",
    "company",
)

_LOW_VALUE_FIRST = re.compile(
    r"^(blog|news|changelog|docs|help|legal|privacy|terms|careers|jobs|"
    r"login|signup|cart|account|tag|category|author|wp-admin)$"
)
_TITLE_RE = re.compile(r"<title[^>]*>([^<]*)</title>", re.I)
_HREF_RE = re.compile(r"""href=["']([^"'#?]+)["']""", re.I)
_ANCHOR_RE = re.compile(
    r"""<a\b[^>]*href=["'](https?://[^"']+)["'][^>]*>([\s\S]*?)</a>""",
    re.I,
)


def clean_domain(value: str) -> str:
    """Host only, no scheme / path / www."""
    raw = (value or "").strip()
    if not raw:
        return ""
    if "://" not in raw:
        raw = "https://" + raw
    try:
        host = (urlparse(raw).hostname or "").lower()
    except Exception:
        return ""
    if host.startswith("www."):
        host = host[4:]
    return host


def origin_url(domain: str) -> str:
    return f"https://{domain}" if domain else ""


def normalize_page_url(url: str) -> str:
    raw = (url or "").strip()
    if not raw:
        return ""
    if "://" not in raw:
        raw = "https://" + raw
    try:
        parsed = urlparse(raw)
    except Exception:
        return ""
    if parsed.scheme not in {"http", "https"} or not parsed.netloc:
        return ""
    host = (parsed.hostname or "").strip()
    if not host or " " in host or " " in parsed.netloc:
        return ""
    path = parsed.path or "/"
    if path.endswith("/"):
        path = path.rstrip("/")
    return f"{parsed.scheme}://{parsed.netloc}{path}"


def path_score(url: str) -> int:
    """Rank a URL so the page budget is spent on product pages, not blogs."""
    try:
        path = urlparse(url).path.strip("/").lower()
    except Exception:
        return -1
    if path == "":
        return 100
    parts = [p for p in path.split("/") if p]
    if not parts:
        return 100
    first = parts[0]
    if _LOW_VALUE_FIRST.match(first):
        return -1
    try:
        index = HIGH_VALUE_PATHS.index(first)
        base = 90 - index * 3
    except ValueError:
        base = 30
    return base - len(parts) * 5


def rank_urls(
    urls: list[str],
    *,
    max_pages: int,
    bonus_url: str = "",
    domain: str = "",
) -> list[str]:
    """Dedupe, drop junk, keep the highest-scoring pages."""
    cap = _clamp_pages(max_pages)
    scored: list[tuple[int, str]] = []
    seen: set[str] = set()
    bonus = normalize_page_url(bonus_url)
    own = clean_domain(domain)
    for raw in urls:
        url = normalize_page_url(raw)
        if not url or url in seen:
            continue
        if own and clean_domain(url) != own:
            continue
        seen.add(url)
        score = path_score(url)
        if bonus and url == bonus:
            score = max(score, 95)
        if score <= 0:
            continue
        scored.append((score, url))
    scored.sort(key=lambda item: (-item[0], item[1]))
    return [url for _, url in scored[:cap]]


def _clamp_pages(max_pages: int | None) -> int:
    try:
        n = int(max_pages) if max_pages is not None else DEFAULT_MAX_PAGES
    except (TypeError, ValueError):
        n = DEFAULT_MAX_PAGES
    return max(1, min(n, MAX_PAGES_HARD_CAP))


def seed_urls(domain: str) -> list[str]:
    origin = origin_url(domain)
    if not origin:
        return []
    out = [origin]
    for path in HIGH_VALUE_PATHS:
        if path:
            out.append(f"{origin}/{path}")
    return out


def strip_html(html: str) -> str:
    """HTML → text, keeping anchors as markdown so compare/vs links survive."""
    with_links = _ANCHOR_RE.sub(
        lambda m: _anchor_to_md(m.group(1), m.group(2)),
        html or "",
    )
    text = re.sub(r"<script[\s\S]*?</script>", " ", with_links, flags=re.I)
    text = re.sub(r"<style[\s\S]*?</style>", " ", text, flags=re.I)
    text = re.sub(r"<nav[\s\S]*?</nav>", " ", text, flags=re.I)
    text = re.sub(r"<footer[\s\S]*?</footer>", " ", text, flags=re.I)
    text = re.sub(r"<[^>]+>", " ", text)
    text = text.replace("&nbsp;", " ").replace("&amp;", "&")
    text = re.sub(r"&#\d+;", " ", text)
    text = re.sub(r"[ \t]+", " ", text)
    text = re.sub(r"\n{3,}", "\n\n", text)
    return text.strip()


def _anchor_to_md(href: str, label: str) -> str:
    text = re.sub(r"<[^>]+>", " ", label or "")
    text = re.sub(r"\s+", " ", text).strip() or href
    return f"[{text}]({href})"


def extract_title(html: str, fallback: str = "") -> str:
    m = _TITLE_RE.search(html or "")
    if not m:
        return fallback
    return re.sub(r"\s+", " ", m.group(1)).strip() or fallback


def discover_same_origin_links(html: str, domain: str) -> list[str]:
    origin = origin_url(domain)
    found: list[str] = [origin] if origin else []
    seen = set(found)
    for match in _HREF_RE.finditer(html or ""):
        href = match.group(1)
        try:
            resolved = urlparse(urljoin(origin + "/", href))
        except Exception:
            continue
        host = (resolved.hostname or "").lower()
        if host.startswith("www."):
            host = host[4:]
        if host != domain:
            continue
        url = normalize_page_url(f"{resolved.scheme}://{resolved.netloc}{resolved.path}")
        if url and url not in seen:
            seen.add(url)
            found.append(url)
    return found


def format_brief_for_prompt(brief: dict | None) -> str:
    """Compact markdown for Insight's user prompt."""
    if not brief:
        return ""
    parts: list[str] = []
    total = 0
    for page in brief.get("pages") or []:
        url = str(page.get("url") or "")
        title = str(page.get("title") or url)
        md = str(page.get("markdown") or "").strip()
        if not md:
            continue
        chunk = md[:PER_PAGE_BRIEF]
        block = f"### {title}\nSource: {url}\n{chunk}"
        if total + len(block) > TOTAL_BRIEF_CHARS:
            remain = TOTAL_BRIEF_CHARS - total
            if remain < 200:
                break
            block = block[:remain]
        parts.append(block)
        total += len(block)
        if total >= TOTAL_BRIEF_CHARS:
            break
    return "\n\n".join(parts)


def empty_brief(*, skipped: bool, reason: str, domain: str = "", enabled: bool = False) -> dict[str, Any]:
    return {
        "enabled": enabled,
        "skipped": skipped,
        "reason": reason,
        "domain": domain,
        "provider": "",
        "pages": [],
        "brief_markdown": "",
    }


def _page_dict(url: str, title: str, markdown: str, score: int | None = None) -> dict[str, Any]:
    row: dict[str, Any] = {
        "url": url,
        "title": (title or url)[:180],
        "markdown": (markdown or "")[:PER_PAGE_MARKDOWN],
    }
    if score is not None:
        row["score"] = score
    return row


def _finalize(domain: str, provider: str, pages: list[dict[str, Any]]) -> dict[str, Any]:
    brief = {
        "enabled": True,
        "skipped": False,
        "reason": "",
        "domain": domain,
        "provider": provider,
        "pages": pages,
        "brief_markdown": "",
    }
    brief["brief_markdown"] = format_brief_for_prompt(brief)
    return brief


def _enabled(settings: Any | None) -> bool:
    return bool(getattr(settings, "site_scan_enabled", False)) if settings else False


def _api_key(settings: Any | None) -> str:
    if settings is None:
        return ""
    return str(getattr(settings, "firecrawl_api_key", "") or "").strip()


def _max_pages(settings: Any | None) -> int:
    if settings is None:
        return DEFAULT_MAX_PAGES
    return _clamp_pages(getattr(settings, "site_scan_max_pages", DEFAULT_MAX_PAGES))


async def _http_get(url: str, *, timeout: float = FETCH_TIMEOUT, headers: dict[str, str] | None = None) -> tuple[int, str]:
    async with httpx.AsyncClient(timeout=timeout, follow_redirects=True) as client:
        resp = await client.get(url, headers=headers or {"User-Agent": USER_AGENT})
        return resp.status_code, resp.text


async def _http_post_json(
    url: str,
    *,
    headers: dict[str, str],
    payload: dict[str, Any],
    timeout: float = FIRECRAWL_TIMEOUT,
) -> tuple[int, dict[str, Any]]:
    async with httpx.AsyncClient(timeout=timeout) as client:
        resp = await client.post(url, headers=headers, json=payload)
        if not resp.content:
            return resp.status_code, {}
        try:
            data = resp.json()
        except Exception:
            data = {}
        if not isinstance(data, dict):
            data = {}
        return resp.status_code, data


def _firecrawl_headers(api_key: str) -> dict[str, str]:
    return {
        "Authorization": f"Bearer {api_key}",
        "Content-Type": "application/json",
        "Accept": "application/json",
    }


def _links_from_map_payload(payload: dict[str, Any]) -> list[str]:
    raw = payload.get("links")
    if raw is None and isinstance(payload.get("data"), dict):
        raw = payload["data"].get("links")
    if not isinstance(raw, list):
        return []
    out: list[str] = []
    for item in raw:
        if isinstance(item, str):
            out.append(item)
        elif isinstance(item, dict):
            url = item.get("url") or item.get("href") or ""
            if url:
                out.append(str(url))
    return out


async def firecrawl_map(domain: str, api_key: str, *, limit: int = 40) -> list[str]:
    status, payload = await _http_post_json(
        f"{FIRECRAWL_BASE}/map",
        headers=_firecrawl_headers(api_key),
        payload={"url": origin_url(domain), "limit": max(1, min(int(limit), MAP_LIMIT_CAP))},
    )
    if status >= 400:
        raise RuntimeError(f"Firecrawl map HTTP {status}")
    return _links_from_map_payload(payload)


async def firecrawl_scrape(url: str, api_key: str) -> dict[str, Any] | None:
    status, payload = await _http_post_json(
        f"{FIRECRAWL_BASE}/scrape",
        headers=_firecrawl_headers(api_key),
        payload={"url": url, "formats": ["markdown"], "onlyMainContent": True},
    )
    if status >= 400:
        raise RuntimeError(f"Firecrawl scrape HTTP {status}")
    data = payload.get("data") if isinstance(payload.get("data"), dict) else payload
    markdown = str((data or {}).get("markdown") or "").strip()
    if not markdown:
        return None
    meta = data.get("metadata") if isinstance(data.get("metadata"), dict) else {}
    title = str(meta.get("title") or url)
    return _page_dict(url, title, markdown, path_score(url))


async def plain_fetch(url: str) -> dict[str, Any] | None:
    try:
        status, html = await _http_get(url)
    except Exception as exc:
        logger.debug("[site_scanner] fetch failed %s: %s", url, exc)
        return None
    if status >= 400 or not html:
        return None
    text = strip_html(html)
    if len(text) < 200:
        return None
    return _page_dict(url, extract_title(html, url), text, path_score(url))


async def _map_limit(urls: list[str], limit: int, fn) -> list[dict[str, Any] | None]:
    sem = asyncio.Semaphore(limit)

    async def one(url: str) -> dict[str, Any] | None:
        async with sem:
            try:
                return await fn(url)
            except Exception as exc:
                logger.debug("[site_scanner] page failed %s: %s", url, exc)
                return None

    if not urls:
        return []
    return list(await asyncio.gather(*[one(u) for u in urls]))


async def discover_links(domain: str) -> list[str]:
    origin = origin_url(domain)
    try:
        status, html = await _http_get(origin)
    except Exception:
        return seed_urls(domain)
    if status >= 400 or not html:
        return seed_urls(domain)
    found = discover_same_origin_links(html, domain)
    for url in seed_urls(domain):
        if url not in found:
            found.append(url)
    return found


async def _read_with(
    provider: str,
    domain: str,
    *,
    max_pages: int,
    api_key: str = "",
    bonus_url: str = "",
) -> tuple[list[dict[str, Any]], int]:
    if provider == "firecrawl":
        mapped = await firecrawl_map(domain, api_key, limit=min(40, max(20, max_pages * 5)))
        candidates = mapped + seed_urls(domain)
        if bonus_url:
            candidates.append(bonus_url)
    else:
        candidates = await discover_links(domain)
        if bonus_url:
            candidates.append(bonus_url)

    targets = rank_urls(candidates, max_pages=max_pages, bonus_url=bonus_url)
    if not targets:
        targets = [origin_url(domain)]

    async def scrape(url: str) -> dict[str, Any] | None:
        if provider == "firecrawl":
            return await firecrawl_scrape(url, api_key)
        return await plain_fetch(url)

    scraped = await _map_limit(targets, SCRAPE_CONCURRENCY, scrape)
    pages = [p for p in scraped if p]
    return pages, len(targets)


async def peek_homepage(domain_or_url: str) -> dict[str, str] | None:
    """Cheap homepage peek — plain fetch only, never burns Firecrawl credits."""
    domain = clean_domain(domain_or_url)
    if not domain:
        return None
    for host in (domain, f"www.{domain}"):
        page = await plain_fetch(origin_url(host if not host.startswith("www.") else host))
        if page:
            return {
                "title": page["title"][:180],
                "excerpt": page["markdown"][:1200],
                "url": page["url"],
            }
    return None


async def scan_site(
    website_url: str,
    *,
    settings: Any | None = None,
) -> dict[str, Any]:
    """Crawl high-value pages of the seller's own site.

    Never raises. Disabled / missing URL / empty crawl → skipped brief.
    """
    domain = clean_domain(website_url)
    if not _enabled(settings):
        return empty_brief(skipped=True, reason="disabled", domain=domain, enabled=False)
    if not domain:
        return empty_brief(skipped=True, reason="no_url", enabled=True)

    api_key = _api_key(settings)
    max_pages = _max_pages(settings)
    provider = "firecrawl" if api_key else "fetch"
    bonus = normalize_page_url(website_url)

    try:
        pages, requested = await _read_with(
            provider,
            domain,
            max_pages=max_pages,
            api_key=api_key,
            bonus_url=bonus,
        )
        if provider == "firecrawl" and not pages:
            raise RuntimeError("Firecrawl returned no readable pages")
    except Exception as exc:
        if provider != "firecrawl":
            logger.warning("[site_scanner] fetch crawl failed for %s: %s", domain, exc)
            return empty_brief(
                skipped=True,
                reason=f"error: {exc}",
                domain=domain,
                enabled=True,
            )
        logger.info("[site_scanner] Firecrawl failed for %s (%s) — falling back to fetch", domain, exc)
        provider = "fetch"
        try:
            pages, requested = await _read_with(
                "fetch",
                domain,
                max_pages=max_pages,
                bonus_url=bonus,
            )
        except Exception as fetch_exc:
            logger.warning("[site_scanner] fetch fallback failed for %s: %s", domain, fetch_exc)
            return empty_brief(
                skipped=True,
                reason=f"error: {fetch_exc}",
                domain=domain,
                enabled=True,
            )

    if not pages:
        return empty_brief(
            skipped=True,
            reason="no_content",
            domain=domain,
            enabled=True,
        )

    logger.info(
        "[site_scanner] %s via %s — requested=%s retrieved=%s",
        domain,
        provider,
        requested,
        len(pages),
    )
    return _finalize(domain, provider, pages)
