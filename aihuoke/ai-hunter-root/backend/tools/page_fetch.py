"""Direct HTML fetch when Jina Reader returns nothing.

Lead extract still prefers Jina markdown. This module only runs after that
path is empty, so company sites remain usable without a Jina key.
"""

from __future__ import annotations

import logging

import httpx

from config.settings import get_settings
from tools.outbound_http import client_kwargs, explicit_proxy, is_retryable
from tools.site_scanner import FETCH_TIMEOUT, USER_AGENT, strip_html

logger = logging.getLogger(__name__)

MIN_CHARS = 50


async def fetch_page_text(url: str, *, min_chars: int = MIN_CHARS) -> str:
    """Fetch a URL and return stripped text, or empty string on failure."""
    target = str(url or "").strip()
    if not target:
        return ""
    try:
        text = await _get_text(target)
    except Exception as exc:
        logger.debug("[PageFetch] failed %s: %s", target, exc)
        return ""
    cleaned = (text or "").strip()
    return cleaned if len(cleaned) >= min_chars else ""


async def _get_text(url: str) -> str:
    headers = {"User-Agent": USER_AGENT}
    settings = get_settings()
    try:
        async with httpx.AsyncClient(
            **client_kwargs(timeout=FETCH_TIMEOUT, headers=headers, settings=settings)
        ) as client:
            resp = await client.get(url)
            resp.raise_for_status()
            return strip_html(resp.text)
    except (httpx.ProxyError, httpx.ConnectError, httpx.TimeoutException, httpx.HTTPStatusError) as exc:
        if not is_retryable(exc) or explicit_proxy(settings):
            raise
        logger.warning("[PageFetch] proxy/gateway failed, retrying direct: %s", exc)
        async with httpx.AsyncClient(
            **client_kwargs(
                timeout=FETCH_TIMEOUT,
                headers=headers,
                settings=settings,
                force_direct=True,
            )
        ) as direct:
            resp = await direct.get(url)
            resp.raise_for_status()
            return strip_html(resp.text)
