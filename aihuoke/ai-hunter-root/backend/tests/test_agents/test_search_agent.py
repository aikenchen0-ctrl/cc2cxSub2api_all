"""Tests for agents/search_agent.py — Google Maps-only routing, dedup, stats."""

import asyncio
from unittest.mock import AsyncMock, patch

import pytest

from agents.search_agent import (
    _build_maps_snippet,
    _free_search_keyword,
    _is_china_region,
    _maps_search_keyword,
    _result_identity_key,
    _use_paid_maps,
    search_node,
)
from tools.search_backend_error import SearchBackendError
from tools.search_circuit import SearchCircuit


def _base_state(**overrides):
    base = {
        "website_url": "https://solartech.de",
        "product_keywords": ["solar inverter"],
        "target_regions": ["Europe"],
        "uploaded_files": [],
        "target_lead_count": 200,
        "max_rounds": 10,
        "insight": {
            "products": ["solar inverter"],
            "industries": ["Renewable Energy"],
        },
        "keywords": ["solar inverter distributor", "PV panel wholesale"],
        "used_keywords": ["solar inverter distributor", "PV panel wholesale"],
        "search_results": [],
        "seen_urls": [],
        "matched_platforms": [],
        "keyword_search_stats": {},
        "leads": [],
        "email_sequences": [],
        "hunt_round": 1,
        "prev_round_lead_count": 0,
        "round_feedback": None,
        "current_stage": "keyword_gen",
        "messages": [],
    }
    base.update(overrides)
    return base


class TestMapsSearchKeyword:
    @pytest.mark.asyncio
    async def test_returns_maps_results_and_fields(self):
        sem = asyncio.Semaphore(5)
        mock_tool = AsyncMock()
        mock_tool.search = AsyncMock(return_value=[
            {
                "title": "Solar Co",
                "website": "https://solar.com",
                "address": "Berlin",
                "phone_number": "+49 123",
                "rating": 4.5,
                "rating_count": 100,
                "type": "Solar company",
                "types": ["Solar company"],
                "description": "Distributor",
                "email": "sales@solar.com",
                "place_id": "abc",
            },
        ])

        result = await _maps_search_keyword("solar", mock_tool, sem)
        assert result["keyword"] == "solar"
        assert result["result_count"] == 1
        assert result["source"] == "google_maps"
        assert result["results"][0]["link"] == "https://solar.com"
        assert result["results"][0]["maps_data"]["phoneNumber"] == "+49 123"
        assert result["error"] is None

    @pytest.mark.asyncio
    async def test_keeps_maps_rows_without_website(self):
        sem = asyncio.Semaphore(5)
        mock_tool = AsyncMock()
        mock_tool.search = AsyncMock(return_value=[
            {"title": "No Website Co", "address": "Berlin", "place_id": "pid-1"},
            {"title": "Has Website", "website": "https://has.com", "address": "Munich", "place_id": "pid-2"},
        ])

        result = await _maps_search_keyword("test", mock_tool, sem)
        assert result["result_count"] == 2
        assert result["results"][0]["link"] == ""
        assert result["results"][0]["maps_data"]["place_id"] == "pid-1"

    @pytest.mark.asyncio
    async def test_handles_maps_error(self):
        sem = asyncio.Semaphore(5)
        mock_tool = AsyncMock()
        mock_tool.search = AsyncMock(side_effect=Exception("Maps API error"))

        result = await _maps_search_keyword("test", mock_tool, sem)
        assert result["result_count"] == 0
        assert result["source"] == "google_maps"
        assert result["error"] == "Maps API error"


class TestBuildMapsSnippet:
    def test_full_snippet(self):
        place = {"type": "Solar", "address": "Berlin", "phone_number": "+49", "rating": 4.5, "rating_count": 10}
        assert _build_maps_snippet(place) == "Solar | Berlin | +49 | Rating: 4.5/5 (10 reviews)"

    def test_partial_snippet(self):
        place = {"type": "Solar", "address": "Berlin"}
        assert _build_maps_snippet(place) == "Solar | Berlin"

    def test_empty_snippet(self):
        assert _build_maps_snippet({}) == ""


class TestResultIdentityKey:
    def test_uses_url_when_present(self):
        key = _result_identity_key({"link": "https://example.com/a"})
        assert key == "url:https://example.com/a"

    def test_uses_place_id_without_url(self):
        key = _result_identity_key({"link": "", "maps_data": {"place_id": "PID-1"}})
        assert key == "place:pid-1"

    def test_falls_back_to_title_and_address(self):
        key = _result_identity_key({"title": "ACME", "maps_data": {"address": "Berlin"}})
        assert key == "maps:acme|berlin"


class TestSearchNode:
    @pytest.mark.asyncio
    async def test_maps_only_routing(self):
        state = _base_state(keywords=["solar berlin"])

        maps_places = [
            {
                "title": "Maps Place",
                "website": "https://maps-place.com",
                "address": "Berlin",
                "type": "Solar",
                "types": ["Solar"],
                "place_id": "xyz",
            },
        ]

        with patch("agents.search_agent.GoogleMapsSearchTool") as MockMaps, \
             patch("agents.search_agent.get_settings") as mock_settings:

            mock_settings.return_value.search_concurrency = 5
            mock_settings.return_value.serper_api_key = "serper-test-key"
            mock_settings.return_value.search_backend = "auto"
            maps_inst = AsyncMock()
            maps_inst.search = AsyncMock(return_value=maps_places)
            maps_inst.close = AsyncMock()
            MockMaps.return_value = maps_inst

            result = await search_node(state)

        assert result["current_stage"] == "search"
        assert len(result["search_results"]) == 1
        assert result["search_results"][0]["source"] == "google_maps"
        maps_inst.search.assert_called_once()
        maps_inst.close.assert_called_once()

    @pytest.mark.asyncio
    async def test_deduplicates_by_place_without_website(self):
        state = _base_state(
            keywords=["k1", "k2"],
            search_results=[],
            seen_urls=[],
        )

        maps_places = [{"title": "No Website Co", "address": "Berlin", "place_id": "pid-1"}]

        with patch("agents.search_agent.GoogleMapsSearchTool") as MockMaps, \
             patch("agents.search_agent.get_settings") as mock_settings:

            mock_settings.return_value.search_concurrency = 5
            maps_inst = AsyncMock()
            maps_inst.search = AsyncMock(return_value=maps_places)
            maps_inst.close = AsyncMock()
            MockMaps.return_value = maps_inst

            result = await search_node(state)

        assert len(result["search_results"]) == 1
        assert result["seen_urls"] == ["place:pid-1"]

    @pytest.mark.asyncio
    async def test_accumulates_keyword_stats(self):
        state = _base_state(keywords=["kw1"])

        maps_places = [
            {"title": "R1", "website": "https://r1.com", "place_id": "p1"},
            {"title": "R2", "address": "Berlin", "place_id": "p2"},
        ]

        with patch("agents.search_agent.GoogleMapsSearchTool") as MockMaps, \
             patch("agents.search_agent.get_settings") as mock_settings:

            mock_settings.return_value.search_concurrency = 5
            mock_settings.return_value.serper_api_key = "serper-test-key"
            mock_settings.return_value.search_backend = "auto"
            maps_inst = AsyncMock()
            maps_inst.search = AsyncMock(return_value=maps_places)
            maps_inst.close = AsyncMock()
            MockMaps.return_value = maps_inst

            result = await search_node(state)

        assert "kw1" in result["keyword_search_stats"]
        assert result["keyword_search_stats"]["kw1"]["result_count"] == 2

    @pytest.mark.asyncio
    async def test_empty_keywords_returns_early(self):
        state = _base_state(keywords=[])
        result = await search_node(state)
        assert result["current_stage"] == "search"

    @pytest.mark.asyncio
    async def test_free_backend_when_no_serper_key(self):
        state = _base_state(keywords=["solar berlin"])

        with patch("agents.search_agent.GoogleMapsSearchTool") as MockMaps, \
             patch("agents.search_agent.FreeSearchTool") as MockFree, \
             patch("agents.search_agent.get_settings") as mock_settings:

            mock_settings.return_value.search_concurrency = 5
            mock_settings.return_value.serper_api_key = ""
            mock_settings.return_value.search_backend = "auto"
            free_inst = AsyncMock()
            free_inst.search = AsyncMock(return_value=[
                {
                    "title": "Free Co",
                    "link": "https://free-co.example",
                    "snippet": "A distributor",
                    "position": 1,
                    "source": "free_web",
                }
            ])
            free_inst.close = AsyncMock()
            MockFree.return_value = free_inst

            result = await search_node(state)

        MockMaps.assert_not_called()
        free_inst.search.assert_called_once()
        free_inst.close.assert_called_once()
        assert result["search_results"][0]["source"] == "free_web"
        assert result["search_results"][0]["link"] == "https://free-co.example"

    @pytest.mark.asyncio
    async def test_search_backend_free_skips_maps_even_with_key(self):
        state = _base_state(keywords=["solar berlin"])

        with patch("agents.search_agent.GoogleMapsSearchTool") as MockMaps, \
             patch("agents.search_agent.FreeSearchTool") as MockFree, \
             patch("agents.search_agent.get_settings") as mock_settings:

            mock_settings.return_value.search_concurrency = 5
            mock_settings.return_value.serper_api_key = "serper-test-key"
            mock_settings.return_value.search_backend = "free"
            free_inst = AsyncMock()
            free_inst.search = AsyncMock(return_value=[
                {"title": "Forced Free", "link": "https://forced.example", "snippet": "", "position": 1}
            ])
            free_inst.close = AsyncMock()
            MockFree.return_value = free_inst

            result = await search_node(state)

        MockMaps.assert_not_called()
        assert result["search_results"][0]["source"] == "free_web"

    @pytest.mark.asyncio
    async def test_free_backend_trips_circuit_after_first_blocked_keyword(self):
        state = _base_state(keywords=["kw1", "kw2", "kw3"])

        with patch("agents.search_agent.GoogleMapsSearchTool") as MockMaps, \
             patch("agents.search_agent.FreeSearchTool") as MockFree, \
             patch("agents.search_agent.get_settings") as mock_settings:

            mock_settings.return_value.search_concurrency = 1
            mock_settings.return_value.serper_api_key = ""
            mock_settings.return_value.search_backend = "auto"
            free_inst = AsyncMock()
            free_inst.search = AsyncMock(side_effect=SearchBackendError("ddg_html: 502"))
            free_inst.close = AsyncMock()
            MockFree.return_value = free_inst

            result = await search_node(state)

        MockMaps.assert_not_called()
        assert free_inst.search.call_count == 1
        for kw in ("kw1", "kw2", "kw3"):
            assert result["keyword_search_stats"][kw]["error"] == "ddg_html: 502"
        assert result["search_results"] == []


class TestUsePaidMaps:
    def test_auto_with_key(self):
        settings = type("S", (), {"search_backend": "auto", "serper_api_key": "k"})()
        assert _use_paid_maps(settings) is True

    def test_auto_without_key(self):
        settings = type("S", (), {"search_backend": "auto", "serper_api_key": ""})()
        assert _use_paid_maps(settings) is False

    def test_auto_ignores_comment_placeholder(self):
        settings = type(
            "S",
            (),
            {"search_backend": "auto", "serper_api_key": "# https://serper.dev"},
        )()
        assert _use_paid_maps(settings) is False

    def test_free_overrides_key(self):
        settings = type("S", (), {"search_backend": "free", "serper_api_key": "k"})()
        assert _use_paid_maps(settings) is False

    def test_serper_forces_maps(self):
        settings = type("S", (), {"search_backend": "serper", "serper_api_key": ""})()
        assert _use_paid_maps(settings) is True


class TestFreeSearchKeyword:
    @pytest.mark.asyncio
    async def test_normalizes_rows(self):
        sem = asyncio.Semaphore(5)
        mock_tool = AsyncMock()
        mock_tool.search = AsyncMock(return_value=[
            {"title": "A", "link": "https://a.example", "snippet": "s", "position": 1, "source": "free_web"},
        ])
        result = await _free_search_keyword("solar", mock_tool, sem)
        assert result["source"] == "free_web"
        assert result["result_count"] == 1
        assert result["error"] is None

    @pytest.mark.asyncio
    async def test_drops_wikipedia_rows(self):
        sem = asyncio.Semaphore(5)
        mock_tool = AsyncMock()
        mock_tool.search = AsyncMock(return_value=[
            {"title": "Wiki", "link": "https://en.wikipedia.org/wiki/Solar", "snippet": "s", "position": 1, "source": "free_web"},
            {"title": "Co", "link": "https://solarco.example", "snippet": "s", "position": 2, "source": "free_web"},
        ])
        result = await _free_search_keyword("solar", mock_tool, sem)
        assert result["result_count"] == 1
        assert result["results"][0]["link"] == "https://solarco.example"

    @pytest.mark.asyncio
    async def test_handles_error(self):
        sem = asyncio.Semaphore(5)
        mock_tool = AsyncMock()
        mock_tool.search = AsyncMock(side_effect=Exception("blocked"))
        result = await _free_search_keyword("solar", mock_tool, sem)
        assert result["result_count"] == 0
        assert result["error"] == "blocked"

    @pytest.mark.asyncio
    async def test_backend_error_trips_circuit_and_skips_later_keywords(self):
        sem = asyncio.Semaphore(1)
        circuit = SearchCircuit()
        mock_tool = AsyncMock()
        mock_tool.search = AsyncMock(side_effect=SearchBackendError("ddg_html: timeout"))

        first = await _free_search_keyword("kw1", mock_tool, sem, circuit=circuit)
        second = await _free_search_keyword("kw2", mock_tool, sem, circuit=circuit)

        assert first["error"] == "ddg_html: timeout"
        assert second["error"] == "ddg_html: timeout"
        mock_tool.search.assert_called_once()

    @pytest.mark.asyncio
    async def test_ordinary_error_does_not_trip_circuit(self):
        sem = asyncio.Semaphore(1)
        circuit = SearchCircuit()
        mock_tool = AsyncMock()
        mock_tool.search = AsyncMock(side_effect=[Exception("parse fail"), [
            {"title": "Co", "link": "https://co.example", "snippet": "s", "position": 1, "source": "free_web"},
        ]])

        first = await _free_search_keyword("kw1", mock_tool, sem, circuit=circuit)
        second = await _free_search_keyword("kw2", mock_tool, sem, circuit=circuit)

        assert first["error"] == "parse fail"
        assert second["error"] is None
        assert second["result_count"] == 1
        assert mock_tool.search.call_count == 2


class TestIsChinaRegion:
    def test_china_english(self):
        assert _is_china_region(["China"]) is True

    def test_china_chinese(self):
        assert _is_china_region(["中国"]) is True

    def test_non_china(self):
        assert _is_china_region(["Germany", "Poland"]) is False
