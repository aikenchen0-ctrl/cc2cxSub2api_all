"""Tests for tools/email_patterns.py — format waterfall + domain memory."""

from tools.email_patterns import (
    apply_email_pattern,
    candidates_for_person,
    detect_email_pattern,
    ordered_email_candidates,
    parse_name_parts,
    ranked_keys_for_domain,
    remember_domain_pattern,
    set_pattern_store_path,
)


def test_parse_name_parts_strips_accents():
    parts = parse_name_parts("José García")
    assert parts == {"first": "jose", "last": "garcia"}


def test_apply_email_pattern_first_last():
    assert apply_email_pattern("first.last", "Jane Doe", "acme.com") == "jane.doe@acme.com"
    assert apply_email_pattern("flast", "Jane Doe", "acme.com") == "jdoe@acme.com"
    assert apply_email_pattern("firstlast", "Jane Doe", "acme.com") == "janedoe@acme.com"


def test_detect_email_pattern_matches_known_format():
    assert detect_email_pattern("jane.doe@acme.com", "Jane Doe", "acme.com") == "first.last"
    assert detect_email_pattern("jdoe@acme.com", "Jane Doe") == "flast"


def test_ordered_candidates_put_ranked_keys_first():
    emails = ordered_email_candidates("Jane Doe", "acme.com", ranked_keys=["flast"])
    assert emails[0] == "jdoe@acme.com"
    assert "jane.doe@acme.com" in emails


def test_remember_domain_pattern_persists_ranked_keys(tmp_path):
    from tools import email_patterns

    store = tmp_path / "domain_email_patterns.json"
    set_pattern_store_path(store)
    email_patterns._PATTERN_HITS = {}
    try:
        remember_domain_pattern("acme.com", "flast")
        remember_domain_pattern("acme.com", "flast")
        remember_domain_pattern("acme.com", "first.last")
        assert ranked_keys_for_domain("acme.com")[0] == "flast"
        assert candidates_for_person("Jane Doe", "acme.com")[0] == "jdoe@acme.com"
    finally:
        set_pattern_store_path(None)
        email_patterns._PATTERN_HITS = {}
