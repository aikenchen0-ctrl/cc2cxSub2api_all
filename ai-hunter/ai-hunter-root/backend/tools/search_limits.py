"""Search pacing limits.

Maps/Serper can take the configured concurrency. Free DuckDuckGo cannot —
hammering it returns empty pages and looks like “0 leads”.
"""

from __future__ import annotations

from config.settings import has_usable_api_key

FREE_SEARCH_MAX_CONCURRENCY = 2
FREE_SEARCH_MAX_KEYWORDS = 4


def uses_paid_maps(settings) -> bool:
    """True when discovery should call Serper Maps."""
    backend = str(getattr(settings, "search_backend", "auto") or "auto").strip().lower()
    if backend == "free":
        return False
    if backend == "serper":
        return True
    return has_usable_api_key(getattr(settings, "serper_api_key", ""))


def effective_search_concurrency(settings) -> int:
    """Cap concurrent searches for the active backend."""
    configured = max(1, int(getattr(settings, "search_concurrency", 1) or 1))
    if uses_paid_maps(settings):
        return configured
    return max(1, min(configured, FREE_SEARCH_MAX_CONCURRENCY))


def effective_keywords_per_round(settings, requested: int | None = None) -> int:
    """Cap keyword batch size so free web search is not flooded."""
    configured = int(requested or getattr(settings, "default_keywords_per_round", 8) or 8)
    configured = max(1, configured)
    if uses_paid_maps(settings):
        return configured
    return max(1, min(configured, FREE_SEARCH_MAX_KEYWORDS))
