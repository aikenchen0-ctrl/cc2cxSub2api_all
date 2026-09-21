"""Social presence lookup after LeadExtract.

Inspired by maigret (MIT, Copyright (c) 2020-2026 Soxoj): username →
site existence, tags, and a bounded site list. We call the maigret
library when it is installed; we do not copy `checking.py`. Default
off. Failures never abort the hunt. Never scan the full 3000-site
database — whitelist + country tags, capped per lead.
"""

from __future__ import annotations

import logging
import re
from typing import Any, Awaitable, Callable, Iterable
from urllib.parse import urlparse

import httpx

logger = logging.getLogger(__name__)

_USER_AGENT = "AIHunter-Social/0.1 (+https://localhost)"
_USERNAME_RE = re.compile(r"^[A-Za-z0-9._-]{2,32}$")
_EMAIL_RE = re.compile(r"^[^@\s]+@[^@\s]+\.[^@\s]+$")
DEFAULT_MAX_SITES = 20

# Whitelist first. Keys are canonical network names used in social_profiles.
DEFAULT_ALLOWED_SITES: tuple[str, ...] = (
    "linkedin",
    "twitter",
    "github",
    "facebook",
    "instagram",
    "youtube",
)

SITE_CATALOG: dict[str, dict[str, Any]] = {
    "linkedin": {
        "url": "https://www.linkedin.com/in/{username}",
        "tags": ["us", "global", "social"],
        "aliases": ("linkedin", "li"),
    },
    "twitter": {
        "url": "https://x.com/{username}",
        "tags": ["us", "global", "social"],
        "aliases": ("twitter", "x", "x.com"),
    },
    "github": {
        "url": "https://github.com/{username}",
        "tags": ["global", "coding"],
        "aliases": ("github", "gh"),
    },
    "facebook": {
        "url": "https://www.facebook.com/{username}",
        "tags": ["us", "global", "social"],
        "aliases": ("facebook", "fb"),
    },
    "instagram": {
        "url": "https://www.instagram.com/{username}",
        "tags": ["global", "social"],
        "aliases": ("instagram", "ig"),
    },
    "youtube": {
        "url": "https://www.youtube.com/@{username}",
        "tags": ["global", "video"],
        "aliases": ("youtube", "yt"),
    },
    "xing": {
        "url": "https://www.xing.com/profile/{username}",
        "tags": ["de", "at", "ch", "eu"],
        "aliases": ("xing",),
    },
    "weibo": {
        "url": "https://weibo.com/n/{username}",
        "tags": ["cn"],
        "aliases": ("weibo",),
    },
    "bilibili": {
        "url": "https://space.bilibili.com/{username}",
        "tags": ["cn"],
        "aliases": ("bilibili", "bili"),
    },
}

_HOST_TO_SITE = {
    "linkedin.com": "linkedin",
    "www.linkedin.com": "linkedin",
    "twitter.com": "twitter",
    "www.twitter.com": "twitter",
    "x.com": "twitter",
    "www.x.com": "twitter",
    "github.com": "github",
    "www.github.com": "github",
    "facebook.com": "facebook",
    "www.facebook.com": "facebook",
    "fb.com": "facebook",
    "instagram.com": "instagram",
    "www.instagram.com": "instagram",
    "youtube.com": "youtube",
    "www.youtube.com": "youtube",
    "youtu.be": "youtube",
    "xing.com": "xing",
    "www.xing.com": "xing",
    "weibo.com": "weibo",
    "www.weibo.com": "weibo",
    "bilibili.com": "bilibili",
    "space.bilibili.com": "bilibili",
}

ProbeFn = Callable[[str], Awaitable[bool]]


def normalize_username(value: str) -> str:
    """Return a maigret-safe username or empty string."""
    raw = str(value or "").strip()
    if not raw or "://" in raw or _EMAIL_RE.match(raw):
        return ""
    raw = raw.lstrip("@").strip().strip("/")
    raw = raw.replace(" ", "")
    if not _USERNAME_RE.match(raw):
        return ""
    if raw.replace(".", "").replace("_", "").replace("-", "") == "":
        return ""
    return raw


def username_from_url(url: str) -> str:
    """Pull a username out of a known social URL."""
    parsed = urlparse(str(url or "").strip())
    host = parsed.netloc.lower()
    if host.startswith("www."):
        host = host[4:]
    path = [p for p in parsed.path.split("/") if p]
    if not path:
        return ""
    first = path[0]
    if host.endswith("linkedin.com"):
        if first in {"in", "company"} and len(path) > 1:
            return normalize_username(path[1])
        return ""
    if host.endswith("github.com"):
        if first in {"orgs", "search", "topics", "about", "login"}:
            return ""
        return normalize_username(first)
    if host.endswith("youtube.com"):
        handle = first[1:] if first.startswith("@") else first
        if handle in {"channel", "c", "user", "watch", "results"}:
            return normalize_username(path[1]) if len(path) > 1 else ""
        return normalize_username(handle)
    if first in {"profile", "n", "people"} and len(path) > 1:
        return normalize_username(path[1])
    return normalize_username(first)


def usernames_from_lead(lead: dict[str, Any]) -> list[str]:
    """Collect candidate usernames without inventing ones from CJK full names."""
    found: list[str] = []
    seen: set[str] = set()

    def _push(value: str) -> None:
        name = normalize_username(value)
        key = name.lower()
        if name and key not in seen:
            seen.add(key)
            found.append(name)

    _push(str(lead.get("github_org") or ""))
    social = lead.get("social_media") or {}
    if isinstance(social, dict):
        for url in social.values():
            _push(username_from_url(str(url or "")))
    _push(username_from_url(str(lead.get("linkedin_url") or "")))
    for profile in lead.get("social_profiles") or []:
        if isinstance(profile, dict):
            _push(str(profile.get("username") or ""))
            _push(username_from_url(str(profile.get("url") or "")))

    first = str(lead.get("first_name") or "").strip()
    last = str(lead.get("last_name") or "").strip()
    if first and last and first.isascii() and last.isascii():
        _push(f"{first}{last}")
        _push(f"{first}.{last}")
        _push(f"{first}_{last}")

    contact = str(lead.get("contact_person") or "").strip()
    if contact and contact.isascii() and " " in contact:
        parts = [p for p in re.split(r"\s+", contact) if p]
        if len(parts) >= 2:
            _push("".join(parts[:2]))
            _push(".".join(parts[:2]))

    for email in lead.get("emails") or []:
        local = str(email or "").split("@", 1)[0]
        _push(local)

    return found


def _canonical_site(name: str) -> str:
    raw = str(name or "").strip().lower()
    if raw in SITE_CATALOG:
        return raw
    for key, spec in SITE_CATALOG.items():
        if raw in {a.lower() for a in spec.get("aliases") or ()}:
            return key
    return ""


def select_sites(
    allowed_sites: Iterable[str] | None = None,
    country_tags: Iterable[str] | None = None,
    *,
    max_sites: int = DEFAULT_MAX_SITES,
) -> list[str]:
    """Whitelist first, then country-tagged extras, hard-capped per lead."""
    cap = max(1, min(int(max_sites or DEFAULT_MAX_SITES), DEFAULT_MAX_SITES))
    wanted = [_canonical_site(name) for name in (allowed_sites or DEFAULT_ALLOWED_SITES)]
    ordered: list[str] = []
    for name in wanted:
        if name and name not in ordered:
            ordered.append(name)

    tags = {str(tag or "").strip().lower() for tag in (country_tags or []) if str(tag or "").strip()}
    if tags:
        for name, spec in SITE_CATALOG.items():
            site_tags = {str(t).lower() for t in spec.get("tags") or []}
            if name not in ordered and site_tags & tags:
                ordered.append(name)
    return ordered[:cap]


def profile_url(site: str, username: str) -> str:
    spec = SITE_CATALOG.get(_canonical_site(site) or site, {})
    template = str(spec.get("url") or "")
    name = normalize_username(username)
    if not template or not name:
        return ""
    return template.format(username=name)


def parse_maigret_results(raw: Any, *, username: str = "") -> list[dict[str, str]]:
    """Normalize maigret SiteResult dicts/objects into social_profiles rows."""
    if not isinstance(raw, dict):
        return []
    rows: list[dict[str, str]] = []
    for site_name, payload in raw.items():
        status = ""
        url = ""
        if hasattr(payload, "status"):
            inner = getattr(payload, "status", None)
            status = str(getattr(inner, "status", inner) or "")
            url = str(getattr(payload, "url_user", "") or getattr(payload, "url", "") or "")
        elif isinstance(payload, dict):
            inner = payload.get("status")
            if hasattr(inner, "status"):
                status = str(getattr(inner, "status") or "")
            elif isinstance(inner, dict):
                status = str(inner.get("status") or inner.get("name") or "")
            else:
                status = str(inner or payload.get("claimed") or "")
            url = str(payload.get("url_user") or payload.get("url") or "")
        status_l = status.lower()
        claimed = status_l in {"claimed", "true", "1"} or "claimed" in status_l
        if not claimed:
            continue
        site = _canonical_site(str(site_name)) or str(site_name).strip().lower()
        url = url or profile_url(site, username)
        if not site or not url:
            continue
        rows.append(
            {
                "network": site,
                "url": url,
                "username": normalize_username(username),
                "status": "claimed",
            }
        )
    return rows


def merge_social_profiles(lead: dict[str, Any], profiles: list[dict[str, str]]) -> dict[str, Any]:
    """Write social_profiles[] and mirror into social_media / linkedin_url."""
    bucket = lead.setdefault("social_profiles", [])
    if not isinstance(bucket, list):
        bucket = []
        lead["social_profiles"] = bucket
    seen = {
        (str(item.get("network") or "").lower(), str(item.get("url") or "").rstrip("/").lower())
        for item in bucket
        if isinstance(item, dict)
    }
    social = lead.setdefault("social_media", {})
    if not isinstance(social, dict):
        social = {}
        lead["social_media"] = social
    for profile in profiles:
        network = str(profile.get("network") or "").strip().lower()
        url = str(profile.get("url") or "").strip()
        if not network or not url:
            continue
        key = (network, url.rstrip("/").lower())
        if key in seen:
            continue
        seen.add(key)
        row = {
            "network": network,
            "url": url,
            "username": str(profile.get("username") or ""),
            "status": str(profile.get("status") or "claimed"),
        }
        bucket.append(row)
        if not social.get(network):
            social[network] = url
        if network == "linkedin" and not str(lead.get("linkedin_url") or "").strip():
            lead["linkedin_url"] = url
        if network == "github" and not str(lead.get("github_org") or "").strip():
            handle = str(profile.get("username") or username_from_url(url))
            if handle:
                lead["github_org"] = handle
    return lead


def _country_tags_from_settings(settings: Any | None, lead: dict[str, Any]) -> list[str]:
    tags: list[str] = []
    raw = ""
    if settings is not None:
        raw = str(getattr(settings, "social_presence_country_tags", "") or "")
    for part in raw.split(","):
        item = part.strip().lower()
        if item:
            tags.append(item)
    code = str(lead.get("country_code") or "").strip().lower()
    if code and code not in tags:
        tags.append(code)
    return tags


def _max_sites_from_settings(settings: Any | None) -> int:
    if settings is None:
        return DEFAULT_MAX_SITES
    try:
        value = int(getattr(settings, "social_presence_max_sites", DEFAULT_MAX_SITES) or DEFAULT_MAX_SITES)
    except (TypeError, ValueError):
        value = DEFAULT_MAX_SITES
    return max(1, min(value, DEFAULT_MAX_SITES))


async def _default_probe(url: str, *, client: httpx.AsyncClient | None = None) -> bool:
    own = client is None
    http = client or httpx.AsyncClient(headers={"User-Agent": _USER_AGENT}, timeout=8.0)
    try:
        resp = await http.get(url, timeout=8.0, follow_redirects=True)
        if resp.status_code in {404, 410}:
            return False
        if resp.status_code >= 400:
            return False
        text = (resp.text or "")[:4000].lower()
        if any(marker in text for marker in ("page not found", "user not found", "doesn't exist", "does not exist", "404")):
            return False
        return True
    except Exception as exc:
        logger.debug("[social] probe failed %s: %s", url, exc)
        return False
    finally:
        if own:
            await http.aclose()


async def _scan_with_maigret(username: str, sites: list[str]) -> list[dict[str, str]] | None:
    """Call maigret if installed. Return None when the library is absent."""
    try:
        from maigret.checking import maigret as maigret_search  # type: ignore
        from maigret.sites import MaigretDatabase  # type: ignore
    except Exception:
        return None
    try:
        db = MaigretDatabase()
        if hasattr(db, "load_from_http") and not getattr(db, "sites", None):
            # Offline-safe: some installs ship a bundled data file.
            loader = getattr(db, "load_from_path", None) or getattr(db, "load_from_json", None)
            if callable(loader):
                try:
                    loader()
                except Exception:
                    pass
        site_dict = {}
        raw_sites = getattr(db, "sites_dict", None) or getattr(db, "sites", {}) or {}
        if isinstance(raw_sites, dict):
            items = raw_sites.items()
        else:
            items = ((getattr(site, "name", str(site)), site) for site in raw_sites)
        wanted = {name.lower() for name in sites}
        for name, site in items:
            key = _canonical_site(str(name)) or str(name).strip().lower()
            if key in wanted and key not in site_dict:
                site_dict[str(name)] = site
            if len(site_dict) >= len(sites):
                break
        if not site_dict:
            return []
        raw = await maigret_search(
            username,
            site_dict,
            logger,
            timeout=5,
            no_progressbar=True,
            is_parsing_enabled=False,
            is_enrich_enabled=False,
            max_connections=min(10, len(site_dict)),
        )
        return parse_maigret_results(raw, username=username)
    except Exception as exc:
        logger.debug("[social] maigret scan failed for %s: %s", username, exc)
        return None


async def scan_username(
    username: str,
    *,
    allowed_sites: Iterable[str] | None = None,
    country_tags: Iterable[str] | None = None,
    max_sites: int = DEFAULT_MAX_SITES,
    probe: ProbeFn | None = None,
    use_maigret: bool = True,
) -> list[dict[str, str]]:
    """Scan one username against the bounded whitelist. Never exceeds max_sites."""
    handle = normalize_username(username)
    if not handle:
        return []
    sites = select_sites(allowed_sites, country_tags, max_sites=max_sites)
    if not sites:
        return []
    if use_maigret:
        maigret_rows = await _scan_with_maigret(handle, sites)
        if maigret_rows is not None:
            return maigret_rows[: max(1, min(int(max_sites), DEFAULT_MAX_SITES))]

    rows: list[dict[str, str]] = []
    checker = probe or _default_probe
    for site in sites:
        url = profile_url(site, handle)
        if not url:
            continue
        try:
            claimed = await checker(url)
        except Exception as exc:
            logger.debug("[social] probe error %s %s: %s", site, handle, exc)
            continue
        if not claimed:
            continue
        rows.append({"network": site, "url": url, "username": handle, "status": "claimed"})
    return rows


async def enrich_lead_social(
    lead: dict[str, Any],
    *,
    settings: Any | None = None,
    probe: ProbeFn | None = None,
) -> dict[str, Any]:
    """Enrich one lead in place. Always returns the lead."""
    enabled = bool(getattr(settings, "social_presence_enabled", False)) if settings else False
    if not enabled:
        return lead
    try:
        names = usernames_from_lead(lead)
        if not names:
            return lead
        tags = _country_tags_from_settings(settings, lead)
        max_sites = _max_sites_from_settings(settings)
        allowed = DEFAULT_ALLOWED_SITES
        use_maigret = True if settings is None else bool(getattr(settings, "social_presence_use_maigret", True))
        merged: list[dict[str, str]] = []
        seen_urls: set[str] = set()
        for name in names[:3]:
            found = await scan_username(
                name,
                allowed_sites=allowed,
                country_tags=tags,
                max_sites=max_sites,
                probe=probe,
                use_maigret=use_maigret,
            )
            for row in found:
                url = str(row.get("url") or "").rstrip("/").lower()
                if url in seen_urls:
                    continue
                seen_urls.add(url)
                merged.append(row)
            if len(merged) >= max_sites:
                break
        merge_social_profiles(lead, merged[:max_sites])
    except Exception as exc:
        logger.debug("[social] enrich skipped: %s", exc)
    return lead


async def enrich_leads_social(
    leads: list[dict[str, Any]],
    *,
    settings: Any | None = None,
    probe: ProbeFn | None = None,
) -> list[dict[str, Any]]:
    """Batch wrapper. Disabled / failed enrichment leaves leads untouched."""
    if not leads:
        return leads
    enabled = bool(getattr(settings, "social_presence_enabled", False)) if settings else False
    if not enabled:
        return leads
    out: list[dict[str, Any]] = []
    for lead in leads:
        try:
            out.append(await enrich_lead_social(lead, settings=settings, probe=probe))
        except Exception as exc:
            logger.warning("[social] lead skipped: %s", exc)
            out.append(lead)
    return out
