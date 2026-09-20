"""Tests for tools/free_search.py — no-key DuckDuckGo / Wikipedia fallback."""

from unittest.mock import AsyncMock, patch

import httpx
import pytest

from config.settings import Settings
from tools.free_search import (
    FreeSearchTool,
    ddg_region,
    looks_like_url,
    normalize_direct_url,
    parse_ddg_html,
    parse_ddg_instant_answer,
    parse_ddg_lite,
    parse_wikipedia_opensearch,
    unwrap_ddg_link,
)
from tools.search_backend_error import SearchBackendError


DDG_HTML = """
<html><body>
  <a rel="nofollow" class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fenergiedist.de%2Fabout&amp;rut=abc">EnergieDist GmbH</a>
  <a class="result__snippet" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fenergiedist.de%2Fabout">German solar distributor based in Berlin.</a>
  <a rel="nofollow" class="result__a" href="https://pvwholesale.example/">PV Wholesale</a>
  <div class="result__snippet">Panels and inverters for EU installers.</div>
  <a rel="nofollow" class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fenergiedist.de%2Fabout">Duplicate</a>
</body></html>
"""


class TestParsers:
    def test_unwrap_uddg_redirect(self):
        href = "//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2Fa&rut=x"
        assert unwrap_ddg_link(href) == "https://example.com/a"

    def test_unwrap_skips_ddg_host_without_uddg(self):
        assert unwrap_ddg_link("https://duckduckgo.com/about") == ""

    def test_parse_ddg_lite_extracts_links(self):
        html = """
        <html><body>
          <a class="result-link" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fenergiedist.de%2F">EnergieDist</a>
          <a href="https://pvwholesale.example/" class="result-link">PV Wholesale</a>
        </body></html>
        """
        rows = parse_ddg_lite(html, limit=10)
        assert [r["link"] for r in rows] == [
            "https://energiedist.de/",
            "https://pvwholesale.example/",
        ]

    def test_parse_ddg_html_reversed_class_order(self):
        html = """
        <html><body>
          <a href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fenergiedist.de%2F" class="result__a">EnergieDist</a>
        </body></html>
        """
        rows = parse_ddg_html(html, limit=5)
        assert rows[0]["link"] == "https://energiedist.de/"

    def test_parse_ddg_html_extracts_unique_links(self):
        rows = parse_ddg_html(DDG_HTML, limit=10)
        assert [r["link"] for r in rows] == [
            "https://energiedist.de/about",
            "https://pvwholesale.example/",
        ]
        assert rows[0]["title"] == "EnergieDist GmbH"
        assert "solar distributor" in rows[0]["snippet"]
        assert rows[0]["source"] == "free_web"
        assert rows[0]["position"] == 1

    def test_parse_ddg_instant_answer(self):
        payload = {
            "Heading": "Solar inverter",
            "AbstractURL": "https://en.wikipedia.org/wiki/Solar_inverter",
            "AbstractText": "A solar inverter converts DC to AC.",
            "RelatedTopics": [
                {"FirstURL": "https://en.wikipedia.org/wiki/Photovoltaics", "Text": "Photovoltaics - solar cells"},
                {"Topics": [
                    {"FirstURL": "https://en.wikipedia.org/wiki/Grid-tie_inverter", "Text": "Grid-tie inverter"},
                ]},
            ],
        }
        rows = parse_ddg_instant_answer(payload, limit=10)
        assert rows[0]["link"] == "https://en.wikipedia.org/wiki/Solar_inverter"
        assert any(r["link"].endswith("Photovoltaics") for r in rows)
        assert any("Grid-tie" in r["title"] for r in rows)

    def test_parse_wikipedia_opensearch(self):
        payload = [
            "solar",
            ["Solar inverter", "Solar panel"],
            ["Converts DC", "Converts light"],
            ["https://en.wikipedia.org/wiki/Solar_inverter", "https://en.wikipedia.org/wiki/Solar_panel"],
        ]
        rows = parse_wikipedia_opensearch(payload, limit=1)
        assert len(rows) == 1
        assert rows[0]["title"] == "Solar inverter"

    def test_looks_like_url(self):
        assert looks_like_url("energiedist.de") is True
        assert looks_like_url("https://energiedist.de/about") is True
        assert looks_like_url("solar inverter Germany") is False

    def test_normalize_direct_url(self):
        assert normalize_direct_url("energiedist.de") == "https://energiedist.de"
        assert normalize_direct_url("https://x.test") == "https://x.test"

    def test_ddg_region(self):
        assert ddg_region("de", "de") == "de-de"
        assert ddg_region("de", "en") == "de-de"
        assert ddg_region("", "") == "wt-wt"


class TestFreeSearchTool:
    @pytest.mark.asyncio
    async def test_search_uses_ddg_html(self):
        tool = FreeSearchTool(settings=Settings(serper_api_key="", tavily_api_key=""))
        fake_req = httpx.Request("POST", "https://html.duckduckgo.com/html/")
        resp = httpx.Response(200, text=DDG_HTML, request=fake_req)

        with patch.object(httpx.AsyncClient, "post", new_callable=AsyncMock, return_value=resp):
            results = await tool.search("solar distributor Germany", gl="de", hl="de")

        assert len(results) == 2
        assert results[0]["link"] == "https://energiedist.de/about"
        await tool.close()

    @pytest.mark.asyncio
    async def test_search_falls_back_to_instant_answer(self):
        tool = FreeSearchTool(settings=Settings())
        html_req = httpx.Request("POST", "https://html.duckduckgo.com/html/")
        ia_req = httpx.Request("GET", "https://api.duckduckgo.com/")
        empty_html = httpx.Response(200, text="<html></html>", request=html_req)
        ia_json = httpx.Response(
            200,
            json={
                "Heading": "Solar",
                "AbstractURL": "https://en.wikipedia.org/wiki/Solar_energy",
                "AbstractText": "Radiant light and heat from the Sun.",
                "RelatedTopics": [],
            },
            request=ia_req,
        )

        with patch.object(httpx.AsyncClient, "post", new_callable=AsyncMock, return_value=empty_html), \
             patch.object(httpx.AsyncClient, "get", new_callable=AsyncMock, return_value=ia_json):
            results = await tool.search("solar energy")

        assert results[0]["link"] == "https://en.wikipedia.org/wiki/Solar_energy"
        await tool.close()

    @pytest.mark.asyncio
    async def test_search_skips_encyclopedia_when_disabled(self):
        tool = FreeSearchTool(settings=Settings())
        html_req = httpx.Request("POST", "https://html.duckduckgo.com/html/")
        lite_req = httpx.Request("GET", "https://lite.duckduckgo.com/lite/")
        empty_html = httpx.Response(200, text="<html></html>", request=html_req)
        empty_lite = httpx.Response(200, text="<html></html>", request=lite_req)
        get_mock = AsyncMock(return_value=empty_lite)

        with patch.object(httpx.AsyncClient, "post", new_callable=AsyncMock, return_value=empty_html), \
             patch.object(httpx.AsyncClient, "get", get_mock):
            results = await tool.search("solar energy", allow_encyclopedia=False)

        assert results == []
        assert get_mock.await_count == 1
        await tool.close()

    @pytest.mark.asyncio
    async def test_search_raises_when_discovery_backends_fail(self):
        tool = FreeSearchTool(settings=Settings())
        with patch.object(httpx.AsyncClient, "post", new_callable=AsyncMock, side_effect=httpx.ConnectTimeout("timed out")), \
             patch.object(httpx.AsyncClient, "get", new_callable=AsyncMock, side_effect=httpx.ConnectTimeout("timed out")):
            with pytest.raises(SearchBackendError) as excinfo:
                await tool.search("solar energy", allow_encyclopedia=False)
        assert "ddg_html:" in str(excinfo.value)
        assert "ConnectTimeout" in str(excinfo.value)
        await tool.close()

    @pytest.mark.asyncio
    async def test_empty_timeout_message_is_still_readable(self):
        tool = FreeSearchTool(settings=Settings(search_http_proxy="http://127.0.0.1:7897"))
        with patch.object(httpx.AsyncClient, "post", new_callable=AsyncMock, side_effect=httpx.ConnectTimeout("")), \
             patch.object(httpx.AsyncClient, "get", new_callable=AsyncMock, side_effect=httpx.ConnectTimeout("")):
            with pytest.raises(SearchBackendError) as excinfo:
                await tool.search("solar energy", allow_encyclopedia=False)
        text = str(excinfo.value)
        assert "ddg_html: ConnectTimeout timed out via http://127.0.0.1:7897" in text
        assert "ddg_lite: ConnectTimeout timed out via http://127.0.0.1:7897" in text
        await tool.close()

    @pytest.mark.asyncio
    async def test_explicit_clash_proxy_does_not_retry_direct(self):
        tool = FreeSearchTool(settings=Settings(search_http_proxy="http://127.0.0.1:7897"))
        post_mock = AsyncMock(side_effect=httpx.ConnectTimeout("clash timeout"))
        get_mock = AsyncMock(side_effect=httpx.ConnectTimeout("clash timeout"))
        with patch.object(httpx.AsyncClient, "post", post_mock), \
             patch.object(httpx.AsyncClient, "get", get_mock):
            with pytest.raises(SearchBackendError):
                await tool.search("solar energy", allow_encyclopedia=False)
        assert post_mock.await_count == 1
        assert get_mock.await_count == 1
        await tool.close()

    @pytest.mark.asyncio
    async def test_empty_query_returns_empty(self):
        tool = FreeSearchTool(settings=Settings())
        assert await tool.search("  ") == []
        await tool.close()
