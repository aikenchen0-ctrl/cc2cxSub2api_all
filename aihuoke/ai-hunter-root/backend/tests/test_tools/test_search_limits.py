from tools.search_limits import (
    FREE_SEARCH_MAX_CONCURRENCY,
    FREE_SEARCH_MAX_KEYWORDS,
    effective_keywords_per_round,
    effective_search_concurrency,
    uses_paid_maps,
)


def _settings(**overrides):
    defaults = {"search_backend": "auto", "serper_api_key": "", "search_concurrency": 10}
    defaults.update(overrides)
    return type("S", (), defaults)()


class TestUsesPaidMaps:
    def test_auto_with_key(self):
        assert uses_paid_maps(_settings(serper_api_key="k")) is True

    def test_auto_without_key(self):
        assert uses_paid_maps(_settings()) is False

    def test_placeholder_is_not_a_key(self):
        assert uses_paid_maps(_settings(serper_api_key="xxx")) is False


class TestEffectiveSearchConcurrency:
    def test_maps_keeps_configured(self):
        assert effective_search_concurrency(_settings(serper_api_key="k", search_concurrency=10)) == 10

    def test_free_backend_is_capped(self):
        assert effective_search_concurrency(_settings(search_concurrency=10)) == FREE_SEARCH_MAX_CONCURRENCY


class TestEffectiveKeywordsPerRound:
    def test_maps_keeps_configured(self):
        assert effective_keywords_per_round(_settings(serper_api_key="k", default_keywords_per_round=8)) == 8

    def test_free_backend_is_capped(self):
        assert effective_keywords_per_round(_settings(default_keywords_per_round=8)) == FREE_SEARCH_MAX_KEYWORDS
