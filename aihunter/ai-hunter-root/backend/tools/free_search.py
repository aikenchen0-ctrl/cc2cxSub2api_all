"""Free (no-key) web search fallback.

Used when SERPER/Tavily keys are missing. Quality and rate limits are
lower than paid backends. Primary source is DuckDuckGo HTML, then lite
HTML. Instant Answer / Wikipedia are optional encyclopedia fallbacks
for Insight — lead discovery must pass allow_encyclopedia=False. Direct
URL/domain queries are fetched as a single result.

No third-party HTML parser: regex only, matching site_scanner style.
"""

from __future__ import annotations

import html as html_lib
import logging
import re
from typing import Optional
from urllib.parse import parse_qs, unquote, urlparse

import httpx

from config.settings import Settings, get_settings
from tools.outbound_http import client_kwargs, explicit_proxy, format_http_error, is_retryable
from tools.search_backend_error import SearchBackendError

logger = logging.getLogger(__name__)

DDG_HTML_URL = "https://html.duckduckgo.com/html/"
DDG_LITE_URL = "https://lite.duckduckgo.com/lite/"
DDG_IA_URL = "https://api.duckduckgo.com/"
WIKI_OPENSEARCH_URL = "https://en.wikipedia.org/w/api.php"
USER_AGENT = (
    "Mozilla/5.0 (compatible; AIHunter-FreeSearch/0.1; +personal research tool)"
)
DEFAULT_NUM = 8
MAX_NUM = 10
FETCH_TIMEOUT = 20.0

_TAG_RE = re.compile(r"<[^>]+>")
_WS_RE = re.compile(r"\s+")
_RESULT_A_RE = re.compile(
    r'<a[^>]*class="[^"]*result__a[^"]*"[^>]*href="([^"]+)"[^>]*>(.*?)</a>'
    r'|<a[^>]*href="([^"]+)"[^>]*class="[^"]*result__a[^"]*"[^>]*>(.*?)</a>',
    re.I | re.S,
)
_RESULT_LITE_RE = re.compile(
    r'<a[^>]*class="[^"]*result-link[^"]*"[^>]*href="([^"]+)"[^>]*>(.*?)</a>'
    r'|<a[^>]*href="([^"]+)"[^>]*class="[^"]*result-link[^"]*"[^>]*>(.*?)</a>',
    re.I | re.S,
)
_SNIPPET_RE = re.compile(
    r'<a[^>]*class="[^"]*result__snippet[^"]*"[^>]*>(.*?)</a>'
    r'|<div[^>]*class="[^"]*result__snippet[^"]*"[^>]*>(.*?)</div>',
    re.I | re.S,
)
_TITLE_RE = re.compile(r"<title[^>]*>([^<]*)</title>", re.I)
_URLISH_RE = re.compile(
    r"^(https?://)?[a-z0-9][a-z0-9.-]*\.[a-z]{2,}(/[^\s]*)?$",
    re.I,
)
_SKIP_HOSTS = {
    "duckduckgo.com",
    "www.duckduckgo.com",
    "html.duckduckgo.com",
    "lite.duckduckgo.com",
}


def strip_tags(value: str) -> str:
    """Remove HTML tags and collapse whitespace."""
    text = html_lib.unescape(_TAG_RE.sub(" ", value or ""))
    return _WS_RE.sub(" ", text).strip()


def unwrap_ddg_link(href: str) -> str:
    """Turn a DDG redirect href into the destination URL."""
    raw = html_lib.unescape((href or "").strip())
    if raw.startswith("//"):
        raw = "https:" + raw
    parsed = urlparse(raw)
    query = parse_qs(parsed.query)
    if "uddg" in query and query["uddg"]:
        return unquote(query["uddg"][0])
    if parsed.netloc.lower() in _SKIP_HOSTS:
        return ""
    return raw


def looks_like_url(query: str) -> bool:
    """True when the query is already a URL or bare domain."""
    q = (query or "").strip()
    if not q or " " in q:
        return False
    return bool(_URLISH_RE.match(q))


def normalize_direct_url(query: str) -> str:
    """Add https:// when the query is a bare domain."""
    q = (query or "").strip()
    if q.startswith("http://") or q.startswith("https://"):
        return q
    return "https://" + q


def ddg_region(gl: str = "", hl: str = "") -> str:
    """Map Serper-style gl onto DuckDuckGo kl.

    DuckDuckGo wants country-country (de-de). Mixing gl=de with hl=en
    produces de-en, which returns HTTP 202 and an empty HTML page.
    """
    gl = (gl or "").strip().lower()
    if gl:
        return f"{gl}-{gl}"
    return "wt-wt"


def parse_ddg_html(html: str, *, limit: int = DEFAULT_NUM) -> list[dict]:
    """Parse DuckDuckGo HTML result cards into title/link/snippet rows."""
    results: list[dict] = []
    seen: set[str] = set()
    for match in _RESULT_A_RE.finditer(html or ""):
        link = unwrap_ddg_link(match.group(1) or match.group(3) or "")
        title = strip_tags(match.group(2) or match.group(4) or "")
        if not link or not title:
            continue
        host = urlparse(link).netloc.lower()
        if host in _SKIP_HOSTS:
            continue
        if link in seen:
            continue
        window = (html or "")[match.end() : match.end() + 1600]
        snippet_match = _SNIPPET_RE.search(window)
        snippet = ""
        if snippet_match:
            snippet = strip_tags(snippet_match.group(1) or snippet_match.group(2) or "")
        seen.add(link)
        results.append(
            {
                "title": title,
                "link": link,
                "snippet": snippet,
                "position": len(results) + 1,
                "source": "free_web",
            }
        )
        if len(results) >= limit:
            break
    return results


def parse_ddg_lite(html: str, *, limit: int = DEFAULT_NUM) -> list[dict]:
    """Parse DuckDuckGo lite result links into title/link rows."""
    results: list[dict] = []
    seen: set[str] = set()
    for match in _RESULT_LITE_RE.finditer(html or ""):
        href = match.group(1) or match.group(3) or ""
        title = strip_tags(match.group(2) or match.group(4) or "")
        link = unwrap_ddg_link(href)
        if not link or not title:
            continue
        host = urlparse(link).netloc.lower()
        if host in _SKIP_HOSTS or link in seen:
            continue
        seen.add(link)
        results.append(
            {
                "title": title,
                "link": link,
                "snippet": "",
                "position": len(results) + 1,
                "source": "free_web",
            }
        )
        if len(results) >= limit:
            break
    return results


def parse_ddg_instant_answer(payload: dict, *, limit: int = DEFAULT_NUM) -> list[dict]:
    """Flatten DuckDuckGo Instant Answer JSON into search-like rows."""
    results: list[dict] = []
    seen: set[str] = set()

    def _add(title: str, link: str, snippet: str) -> None:
        if not link or link in seen:
            return
        host = urlparse(link).netloc.lower()
        if host in _SKIP_HOSTS:
            return
        seen.add(link)
        results.append(
            {
                "title": title or link,
                "link": link,
                "snippet": snippet,
                "position": len(results) + 1,
                "source": "free_web",
            }
        )

    abstract_url = str(payload.get("AbstractURL") or "").strip()
    abstract = str(payload.get("AbstractText") or payload.get("Abstract") or "").strip()
    heading = str(payload.get("Heading") or "").strip()
    if abstract_url:
        _add(heading or abstract_url, abstract_url, abstract)

    def _walk(items: list) -> None:
        for item in items:
            if not isinstance(item, dict):
                continue
            if "Topics" in item:
                _walk(item.get("Topics") or [])
                continue
            url = str(item.get("FirstURL") or "").strip()
            text = str(item.get("Text") or "").strip()
            if url:
                _add(text.split(" - ", 1)[0] if text else url, url, text)
            if len(results) >= limit:
                return

    _walk(payload.get("RelatedTopics") or [])
    return results[:limit]


def parse_wikipedia_opensearch(payload: list, *, limit: int = DEFAULT_NUM) -> list[dict]:
    """Parse Wikipedia action=opensearch JSON into search-like rows."""
    if not isinstance(payload, list) or len(payload) < 4:
        return []
    titles = payload[1] if isinstance(payload[1], list) else []
    snippets = payload[2] if isinstance(payload[2], list) else []
    urls = payload[3] if isinstance(payload[3], list) else []
    results: list[dict] = []
    for i, url in enumerate(urls):
        link = str(url or "").strip()
        if not link:
            continue
        title = str(titles[i] if i < len(titles) else link)
        snippet = str(snippets[i] if i < len(snippets) else "")
        results.append(
            {
                "title": title,
                "link": link,
                "snippet": snippet,
                "position": len(results) + 1,
                "source": "free_web",
            }
        )
        if len(results) >= limit:
            break
    return results


class FreeSearchTool:
    """No-key web search. DuckDuckGo HTML/lite first; encyclopedia is optional."""

    def __init__(self, settings: Settings | None = None) -> None:
        self._settings = settings or get_settings()
        self._client: Optional[httpx.AsyncClient] = None

    def _client_headers(self) -> dict[str, str]:
        return {"User-Agent": USER_AGENT, "Accept-Language": "en,zh;q=0.8"}

    async def _get_client(self) -> httpx.AsyncClient:
        if self._client is None:
            self._client = httpx.AsyncClient(
                **client_kwargs(
                    timeout=FETCH_TIMEOUT,
                    headers=self._client_headers(),
                    settings=self._settings,
                )
            )
        return self._client

    async def _request(self, method: str, url: str, **kwargs) -> httpx.Response:
        """Issue HTTP. Explicit SEARCH_HTTP_PROXY skips sandbox 60376.

        Without an explicit proxy, 502 / ConnectError / Timeout still retry
        once with trust_env=False. Direct retry is skipped when Clash is set,
        because mainland direct connect times out.
        """
        client = await self._get_client()
        try:
            resp = await getattr(client, method)(url, **kwargs)
            resp.raise_for_status()
            return resp
        except (httpx.ProxyError, httpx.ConnectError, httpx.TimeoutException, httpx.HTTPStatusError) as exc:
            if not is_retryable(exc):
                raise
            if explicit_proxy(self._settings):
                raise
            logger.warning("[FreeSearch] proxy/gateway failed, retrying direct: %s", exc)
            async with httpx.AsyncClient(
                **client_kwargs(
                    timeout=FETCH_TIMEOUT,
                    headers=self._client_headers(),
                    settings=self._settings,
                    force_direct=True,
                )
            ) as direct:
                resp = await getattr(direct, method)(url, **kwargs)
                resp.raise_for_status()
                return resp

    async def search(
        self,
        query: str,
        *,
        num: int = DEFAULT_NUM,
        gl: str = "",
        hl: str = "",
        allow_encyclopedia: bool = True,
    ) -> list[dict]:
        """Search without API keys. Returns title/link/snippet/position/source."""
        q = (query or "").strip()
        if not q:
            return []
        limit = max(1, min(int(num or DEFAULT_NUM), MAX_NUM))

        if looks_like_url(q):
            direct = await self._fetch_direct(normalize_direct_url(q))
            if direct:
                return [direct]

        errors: list[str] = []
        saw_empty_page = False

        try:
            html_rows = await self._ddg_html(q, limit=limit, gl=gl, hl=hl)
            if html_rows:
                return html_rows
            saw_empty_page = True
        except Exception as exc:
            html_err = format_http_error(exc, proxy=explicit_proxy(self._settings))
            errors.append(f"ddg_html: {html_err}")
            logger.warning("[FreeSearch] DDG HTML failed query=%r: %s", q, html_err)

        try:
            lite_rows = await self._ddg_lite(q, limit=limit, gl=gl, hl=hl)
            if lite_rows:
                logger.info("[FreeSearch] HTML empty — lite returned %d", len(lite_rows))
                return lite_rows
            saw_empty_page = True
        except Exception as exc:
            lite_err = format_http_error(exc, proxy=explicit_proxy(self._settings))
            errors.append(f"ddg_lite: {lite_err}")
            logger.warning("[FreeSearch] DDG lite failed query=%r: %s", q, lite_err)

        if not allow_encyclopedia:
            if errors and not saw_empty_page:
                raise SearchBackendError("; ".join(errors))
            logger.warning("[FreeSearch] no company-site results for query=%r", q)
            return []

        ia_rows = await self._ddg_instant_answer(q, limit=limit)
        if ia_rows:
            logger.info("[FreeSearch] HTML empty — Instant Answer returned %d", len(ia_rows))
            return ia_rows

        wiki_rows = await self._wikipedia(q, limit=limit)
        if wiki_rows:
            logger.info("[FreeSearch] DDG empty — Wikipedia returned %d", len(wiki_rows))
            return wiki_rows

        logger.warning("[FreeSearch] no results for query=%r", q)
        return []

    async def _ddg_html(self, query: str, *, limit: int, gl: str, hl: str) -> list[dict]:
        # Do not send kl. DuckDuckGo HTML returns HTTP 202 / anomaly page for
        # country codes like de-de; the query text already carries the region.
        del gl, hl
        resp = await self._request(
            "post",
            DDG_HTML_URL,
            data={"q": query},
        )
        return parse_ddg_html(resp.text, limit=limit)

    async def _ddg_lite(self, query: str, *, limit: int, gl: str, hl: str) -> list[dict]:
        del gl, hl
        resp = await self._request(
            "get",
            DDG_LITE_URL,
            params={"q": query},
        )
        return parse_ddg_lite(resp.text, limit=limit)

    async def _ddg_instant_answer(self, query: str, *, limit: int) -> list[dict]:
        try:
            resp = await self._request(
                "get",
                DDG_IA_URL,
                params={
                    "q": query,
                    "format": "json",
                    "no_html": "1",
                    "skip_disambig": "1",
                },
            )
            payload = resp.json()
            if not isinstance(payload, dict):
                return []
            return parse_ddg_instant_answer(payload, limit=limit)
        except Exception as exc:
            logger.warning("[FreeSearch] Instant Answer failed query=%r: %s", query, exc)
            return []

    async def _wikipedia(self, query: str, *, limit: int) -> list[dict]:
        try:
            resp = await self._request(
                "get",
                WIKI_OPENSEARCH_URL,
                params={
                    "action": "opensearch",
                    "search": query,
                    "limit": min(limit, 8),
                    "namespace": 0,
                    "format": "json",
                },
            )
            payload = resp.json()
            if not isinstance(payload, list):
                return []
            return parse_wikipedia_opensearch(payload, limit=limit)
        except Exception as exc:
            logger.warning("[FreeSearch] Wikipedia failed query=%r: %s", query, exc)
            return []

    async def _fetch_direct(self, url: str) -> dict | None:
        try:
            resp = await self._request("get", url)
            title_match = _TITLE_RE.search(resp.text or "")
            title = strip_tags(title_match.group(1) if title_match else url)
            snippet = strip_tags((resp.text or "")[:1200])[:280]
            return {
                "title": title or url,
                "link": str(resp.url) if resp.url else url,
                "snippet": snippet,
                "position": 1,
                "source": "free_web",
            }
        except Exception as exc:
            logger.warning("[FreeSearch] direct fetch failed url=%r: %s", url, exc)
            return None

    async def close(self) -> None:
        if self._client:
            await self._client.aclose()
            self._client = None
