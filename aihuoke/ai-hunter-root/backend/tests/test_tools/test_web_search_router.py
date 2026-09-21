"""Tests for tools/web_search_router.py — Serper vs free, no Maps mixing."""

from config.settings import Settings
from tools.free_search import FreeSearchTool
from tools.google_search import GoogleSearchTool
from tools.web_search_router import _EncyclopediaGatedSearcher, open_web_search


def _settings(**overrides) -> Settings:
    defaults = {"serper_api_key": "", "tavily_api_key": "", "search_backend": "auto"}
    defaults.update(overrides)
    return Settings(**defaults)


class TestOpenWebSearch:
    def test_auto_with_serper_uses_google(self):
        tool = open_web_search(_settings(serper_api_key="serper-test-key"))
        assert isinstance(tool, GoogleSearchTool)

    def test_auto_without_key_uses_free(self):
        tool = open_web_search(_settings(serper_api_key=""))
        assert isinstance(tool, FreeSearchTool)

    def test_auto_comment_placeholder_uses_free(self):
        tool = open_web_search(_settings(serper_api_key="# https://serper.dev"))
        assert isinstance(tool, FreeSearchTool)

    def test_free_overrides_key(self):
        tool = open_web_search(_settings(serper_api_key="serper-test-key", search_backend="free"))
        assert isinstance(tool, FreeSearchTool)

    def test_serper_forces_google_even_without_key(self):
        tool = open_web_search(_settings(serper_api_key="", search_backend="serper"))
        assert isinstance(tool, GoogleSearchTool)

    def test_lead_lookup_disables_encyclopedia(self):
        tool = open_web_search(_settings(serper_api_key=""), allow_encyclopedia=False)
        assert isinstance(tool, _EncyclopediaGatedSearcher)
