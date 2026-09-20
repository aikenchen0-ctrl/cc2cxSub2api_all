"""Tests for tools/search_backend_error.py."""

from tools.search_backend_error import collect_search_backend_error, round_search_is_blocked


def test_empty_stats():
    assert collect_search_backend_error({}) == ""
    assert collect_search_backend_error(None) == ""
    assert collect_search_backend_error("not-a-dict") == ""


def test_joins_unique_errors_in_order():
    stats = {
        "kw1": {"result_count": 0, "error": "ddg_html: 502 Bad Gateway"},
        "kw2": {"result_count": 0, "error": "ddg_html: 502 Bad Gateway"},
        "kw3": {"result_count": 0, "error": "ddg_lite: timeout"},
    }
    assert collect_search_backend_error(stats) == "ddg_html: 502 Bad Gateway; ddg_lite: timeout"


def test_skips_blank_and_non_dict_rows():
    stats = {
        "kw1": {"result_count": 3},
        "kw2": "legacy",
        "kw3": {"error": "   "},
        "kw4": {"error": "blocked"},
    }
    assert collect_search_backend_error(stats) == "blocked"


def test_collect_can_filter_to_given_keywords():
    stats = {
        "old": {"error": "historical timeout"},
        "new": {"result_count": 4},
    }
    assert collect_search_backend_error(stats, keywords=["new"]) == ""
    assert collect_search_backend_error(stats, keywords=["old"]) == "historical timeout"


def test_round_blocked_only_when_every_keyword_failed():
    stats = {
        "kw1": {"error": "ddg_html: 502"},
        "kw2": {"error": "ddg_html: 502"},
        "old": {"error": "stale"},
    }
    assert round_search_is_blocked(stats, ["kw1", "kw2"]) == "ddg_html: 502"
    assert round_search_is_blocked(stats, ["kw1", "kw2", "kw3"]) == ""
    assert round_search_is_blocked(stats, ["kw1", "ok"]) == ""
    assert round_search_is_blocked(stats, []) == ""
    assert round_search_is_blocked(stats, None) == ""
