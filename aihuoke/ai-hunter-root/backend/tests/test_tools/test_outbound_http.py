"""Tests for tools/outbound_http.py — Clash 7897 vs sandbox 60376."""

import httpx
import pytest

from config.settings import Settings
from tools.outbound_http import client_kwargs, explicit_proxy, format_http_error, is_retryable


def test_explicit_proxy_uses_clash_and_skips_env():
    settings = Settings(search_http_proxy="http://127.0.0.1:7897")
    kwargs = client_kwargs(timeout=20.0, settings=settings)
    assert kwargs["proxy"] == "http://127.0.0.1:7897"
    assert kwargs["trust_env"] is False
    assert kwargs["follow_redirects"] is True


def test_empty_proxy_follows_env():
    settings = Settings(search_http_proxy="")
    kwargs = client_kwargs(timeout=20.0, settings=settings)
    assert "proxy" not in kwargs
    assert "trust_env" not in kwargs


def test_force_direct_ignores_explicit_proxy():
    settings = Settings(search_http_proxy="http://127.0.0.1:7897")
    kwargs = client_kwargs(timeout=20.0, settings=settings, force_direct=True)
    assert "proxy" not in kwargs
    assert kwargs["trust_env"] is False


def test_explicit_proxy_helper():
    assert explicit_proxy(Settings(search_http_proxy=" http://127.0.0.1:7897 ")) == "http://127.0.0.1:7897"
    assert explicit_proxy(Settings(search_http_proxy="")) == ""


def test_is_retryable():
    req = httpx.Request("GET", "https://html.duckduckgo.com/html/")
    assert is_retryable(httpx.ConnectError("down", request=req))
    assert is_retryable(httpx.ConnectTimeout("timed out"))
    resp = httpx.Response(502, request=req)
    assert is_retryable(httpx.HTTPStatusError("bad gateway", request=req, response=resp))
    resp_404 = httpx.Response(404, request=req)
    assert is_retryable(httpx.HTTPStatusError("missing", request=req, response=resp_404)) is False
