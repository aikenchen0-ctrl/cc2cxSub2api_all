"""HTTP client options for DuckDuckGo / page fetch.

SEARCH_HTTP_PROXY (e.g. Clash Verge http://127.0.0.1:7897) is used
explicitly and skips WorkBuddy sandbox HTTP_PROXY (60376). Without an
explicit proxy, callers may still retry once with trust_env=False.
"""

from __future__ import annotations

from typing import Any

import httpx

from config.settings import get_settings


def explicit_proxy(settings: Any | None = None) -> str:
    """Return SEARCH_HTTP_PROXY if set, else empty."""
    raw = getattr(settings or get_settings(), "search_http_proxy", "") or ""
    return str(raw).strip()


def client_kwargs(
    *,
    timeout: float,
    headers: dict[str, str] | None = None,
    settings: Any | None = None,
    force_direct: bool = False,
) -> dict[str, Any]:
    """Build httpx.AsyncClient kwargs.

    Explicit proxy → use it and ignore env (sandbox 60376).
    force_direct → ignore env and ignore SEARCH_HTTP_PROXY.
    Otherwise let httpx follow HTTP(S)_PROXY.
    """
    kwargs: dict[str, Any] = {
        "timeout": timeout,
        "follow_redirects": True,
        "headers": headers or {},
    }
    if force_direct:
        kwargs["trust_env"] = False
        return kwargs
    proxy = explicit_proxy(settings)
    if proxy:
        kwargs["proxy"] = proxy
        kwargs["trust_env"] = False
    return kwargs


def is_retryable(exc: BaseException) -> bool:
    """True for proxy/gateway failures that may work without env proxy."""
    if isinstance(exc, (httpx.ProxyError, httpx.ConnectError, httpx.TimeoutException)):
        return True
    status = getattr(getattr(exc, "response", None), "status_code", None)
    return status in {502, 503}


def format_http_error(exc: BaseException, *, proxy: str = "") -> str:
    """One-line error. httpx ConnectTimeout/ConnectError often have empty str()."""
    name = type(exc).__name__
    detail = str(exc or "").strip()
    status = getattr(getattr(exc, "response", None), "status_code", None)
    parts: list[str] = [name]
    if status:
        parts.append(str(status))
    if detail:
        parts.append(detail)
    elif isinstance(exc, httpx.TimeoutException):
        parts.append("timed out")
    elif isinstance(exc, (httpx.ProxyError, httpx.ConnectError, httpx.RequestError)):
        parts.append("failed")
    if proxy:
        parts.append(f"via {proxy}")
    return " ".join(parts)
