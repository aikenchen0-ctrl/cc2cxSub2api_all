"""Thin web-search router.

Picks Serper Google search when a key is present, otherwise the no-key
FreeSearchTool. Callers only depend on search()/close() — they do not
import DuckDuckGo parsing or Serper HTTP details.

Maps discovery stays in search_agent + GoogleMapsSearchTool. This module
is organic web search only.
"""

from __future__ import annotations

import logging
from typing import Protocol

from config.settings import Settings, get_settings, has_usable_api_key
from tools.free_search import FreeSearchTool
from tools.google_search import GoogleSearchTool

logger = logging.getLogger(__name__)


class WebSearcher(Protocol):
    """Organic web search: title/link/snippet rows."""

    async def search(
        self,
        query: str,
        *,
        num: int = 10,
        gl: str = "",
        hl: str = "",
    ) -> list[dict]:
        ...

    async def close(self) -> None:
        ...


class _EncyclopediaGatedSearcher:
    """FreeSearch wrapper that never falls back to Wikipedia Instant Answers."""

    def __init__(self, inner: FreeSearchTool) -> None:
        self._inner = inner

    async def search(
        self,
        query: str,
        *,
        num: int = 10,
        gl: str = "",
        hl: str = "",
    ) -> list[dict]:
        return await self._inner.search(
            query,
            num=num,
            gl=gl,
            hl=hl,
            allow_encyclopedia=False,
        )

    async def close(self) -> None:
        await self._inner.close()


def open_web_search(
    settings: Settings | None = None,
    *,
    allow_encyclopedia: bool = True,
) -> WebSearcher:
    """Return a web searcher. Serper when keyed, else free fallback.

    Insight may keep encyclopedia hits. Lead extract / company-site lookup
    must pass allow_encyclopedia=False so Wikipedia is not treated as a lead.
    """
    settings = settings or get_settings()
    backend = str(getattr(settings, "search_backend", "auto") or "auto").strip().lower()
    has_serper = has_usable_api_key(getattr(settings, "serper_api_key", ""))

    if backend == "serper" or (backend != "free" and has_serper):
        logger.info("[WebSearchRouter] backend=serper")
        return GoogleSearchTool(settings)

    logger.info("[WebSearchRouter] backend=free allow_encyclopedia=%s", allow_encyclopedia)
    free = FreeSearchTool(settings)
    if allow_encyclopedia:
        return free
    return _EncyclopediaGatedSearcher(free)
