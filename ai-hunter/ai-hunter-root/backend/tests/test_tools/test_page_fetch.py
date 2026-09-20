"""Tests for tools/page_fetch.py — direct HTML fallback after Jina."""

from unittest.mock import AsyncMock, patch

import httpx
import pytest

from tools.page_fetch import fetch_page_text


@pytest.mark.asyncio
async def test_fetch_page_text_strips_html():
    req = httpx.Request("GET", "https://acme.test/")
    html = "<html><head><title>Acme</title></head><body><p>Solar distributor in Berlin with OEM stock for European installers.</p></body></html>"
    resp = httpx.Response(200, text=html, request=req)

    with patch.object(httpx.AsyncClient, "get", new_callable=AsyncMock, return_value=resp):
        text = await fetch_page_text("https://acme.test/")

    assert "Solar distributor" in text
    assert "<p>" not in text


@pytest.mark.asyncio
async def test_fetch_page_text_empty_on_short_body():
    req = httpx.Request("GET", "https://acme.test/")
    resp = httpx.Response(200, text="<html>ok</html>", request=req)

    with patch.object(httpx.AsyncClient, "get", new_callable=AsyncMock, return_value=resp):
        text = await fetch_page_text("https://acme.test/")

    assert text == ""


@pytest.mark.asyncio
async def test_fetch_page_text_does_not_retry_direct_when_clash_is_set():
    from config.settings import Settings

    req = httpx.Request("GET", "https://acme.test/")
    get_mock = AsyncMock(side_effect=httpx.ConnectError("clash down", request=req))

    with patch("tools.page_fetch.get_settings", return_value=Settings(search_http_proxy="http://127.0.0.1:7897")), \
         patch.object(httpx.AsyncClient, "get", get_mock):
        text = await fetch_page_text("https://acme.test/")

    assert text == ""
    assert get_mock.await_count == 1


@pytest.mark.asyncio
async def test_fetch_page_text_retries_without_proxy_on_connect_error():
    from config.settings import Settings

    req = httpx.Request("GET", "https://acme.test/")
    ok = httpx.Response(
        200,
        text="<html><body>Official company website for Acme Solar Wholesale GmbH.</body></html>",
        request=req,
    )
    get_mock = AsyncMock(side_effect=[httpx.ConnectError("proxy down", request=req), ok])

    with patch("tools.page_fetch.get_settings", return_value=Settings(search_http_proxy="")), \
         patch.object(httpx.AsyncClient, "get", get_mock):
        text = await fetch_page_text("https://acme.test/")

    assert "Acme Solar Wholesale" in text
    assert get_mock.await_count == 2
