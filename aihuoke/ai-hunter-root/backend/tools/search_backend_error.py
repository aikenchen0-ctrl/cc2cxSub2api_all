"""Errors from no-key search backends.

Empty SERP pages are not errors. This is raised only when every discovery
backend failed to respond, so a hunt can show “blocked” instead of
“completed with 0 leads”.
"""

from __future__ import annotations

from typing import Any, Iterable


class SearchBackendError(RuntimeError):
    """DuckDuckGo / lite HTML could not be reached."""


def collect_search_backend_error(
    keyword_search_stats: Any,
    keywords: Iterable[str] | None = None,
) -> str:
    """Join unique per-keyword search errors, preserving first-seen order.

    When `keywords` is given, only those rows are read. Returns "" when stats
    are missing or no selected keyword recorded an error.
    """
    if not isinstance(keyword_search_stats, dict):
        return ""
    if keywords is None:
        items = keyword_search_stats.items()
    else:
        items = ((kw, keyword_search_stats.get(kw)) for kw in keywords)
    errors = [
        str(stats.get("error") or "").strip()
        for _, stats in items
        if isinstance(stats, dict) and str(stats.get("error") or "").strip()
    ]
    return "; ".join(dict.fromkeys(errors))


def round_search_is_blocked(keyword_search_stats: Any, keywords: Any) -> str:
    """Return joined errors only when every current-round keyword failed.

    A mixed round (some hits, some timeouts) is not a blocked backend.
    Empty keyword lists are not blocked.
    """
    if not isinstance(keywords, list) or not keywords:
        return ""
    if not isinstance(keyword_search_stats, dict):
        return ""
    errors: list[str] = []
    for kw in keywords:
        stats = keyword_search_stats.get(kw)
        err = str(stats.get("error") or "").strip() if isinstance(stats, dict) else ""
        if not err:
            return ""
        errors.append(err)
    return "; ".join(dict.fromkeys(errors))
