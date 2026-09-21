"""Drop encyclopedia / search-engine rows from lead discovery.

Free-search fallback often returns Wikipedia Instant Answers. Those pages
are useful for Insight, but they are not B2B leads. SearchAgent filters
with this module; Insight keeps using FreeSearchTool as-is.
"""

from __future__ import annotations

from urllib.parse import urlparse

_SKIP_HOST_SUFFIXES = (
    "wikipedia.org",
    "wikimedia.org",
    "britannica.com",
    "duckduckgo.com",
    "wikiwand.com",
    "baike.baidu.com",
    "baidu.com",
    "iciba.com",
    "youdao.com",
    "cambridge.org",
    "dictionary.com",
    "mdpi.com",
    "sciencedirect.com",
    "iqiyi.com",
    "iqiyi.cn",
    "douban.com",
    "fang.com",
    "anjuke.com",
    "58.com",
)


def is_usable_discovery_link(url: str) -> bool:
    """True when a search hit is worth sending to lead extraction."""
    raw = str(url or "").strip()
    if not raw:
        return False
    host = urlparse(raw).netloc.lower()
    if host.startswith("www."):
        host = host[4:]
    if not host:
        return False
    return not any(host == suffix or host.endswith("." + suffix) for suffix in _SKIP_HOST_SUFFIXES)
