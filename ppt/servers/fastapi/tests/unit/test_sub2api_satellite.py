from utils.sub2api_satellite import (
    get_current_sub2api_subject,
    openai_base_url,
    reset_current_sub2api_subject,
    satellite_configured,
    satellite_headers,
    set_current_sub2api_subject,
)


def test_openai_base_url_prefixes_link(monkeypatch):
    monkeypatch.delenv("CUSTOM_LLM_URL", raising=False)
    monkeypatch.setenv("LINK", "localhost:18080")
    assert openai_base_url() == "http://localhost:18080/v1"


def test_satellite_headers_require_subject_and_credential(monkeypatch):
    monkeypatch.setenv("SUB2API_APP_CREDENTIAL", "sat-secret")
    monkeypatch.setenv("LINK", "localhost:18080")
    token = set_current_sub2api_subject("42")
    try:
        assert satellite_headers() == {
            "Authorization": "Bearer sat-secret",
            "X-Sub2API-On-Behalf-Of": "42",
            "X-Sub2API-Satellite": "ppt",
        }
        assert satellite_configured()
    finally:
        reset_current_sub2api_subject(token)
    assert satellite_headers() == {}
    assert get_current_sub2api_subject() == ""
