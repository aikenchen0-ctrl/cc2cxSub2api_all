"""Tests for tools/search_readiness.py."""

from tools.search_readiness import search_readiness


def _settings(**overrides):
    defaults = {"search_backend": "auto", "serper_api_key": ""}
    defaults.update(overrides)
    return type("S", (), defaults)()


def test_paid_maps_when_serper_key_present():
    status = search_readiness(_settings(serper_api_key="k"))
    assert status["paid_maps"] is True
    assert status["discovery_backend"] == "maps"
    assert status["warning"] == ""


def test_free_web_warns_without_serper():
    status = search_readiness(_settings())
    assert status["paid_maps"] is False
    assert status["discovery_backend"] == "free_web"
    assert "Serper" in status["warning"]
    assert "Insight" in status["new_hunt_note"]
    assert "续挖" in status["resume_note"]


def test_placeholder_key_is_free_web():
    status = search_readiness(_settings(serper_api_key="xxx"))
    assert status["paid_maps"] is False
    assert status["discovery_backend"] == "free_web"


def test_forced_free_backend_warns_even_with_key():
    status = search_readiness(_settings(search_backend="free", serper_api_key="k"))
    assert status["paid_maps"] is False
    assert status["discovery_backend"] == "free_web"
    assert status["warning"]


def test_free_web_mentions_explicit_proxy():
    status = search_readiness(_settings(search_http_proxy="http://127.0.0.1:7897"))
    assert "7897" in status["warning"]
    assert "60376" in status["warning"]
    assert "不等于 DuckDuckGo" in status["warning"]
