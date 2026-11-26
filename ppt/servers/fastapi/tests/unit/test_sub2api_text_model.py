from utils.get_env import get_custom_model_env


def test_managed_text_model_replaces_stale_image_selection(monkeypatch):
    monkeypatch.setenv("SUB2API_APP_CREDENTIAL", "test-credential")
    monkeypatch.setenv("SUB2API_MODEL", "gpt-6.1-sol")
    for model in ("gpt-image-2", "kling-v3", "", "unknown-model"):
        monkeypatch.setenv("CUSTOM_MODEL", model)
        assert get_custom_model_env() == "gpt-6.1-sol"
    monkeypatch.setenv("CUSTOM_MODEL", "gpt-6.2-sol")
    assert get_custom_model_env() == "gpt-6.2-sol"


def test_legacy_text_defaults_migrate_to_sol(monkeypatch):
    monkeypatch.setenv("SUB2API_APP_CREDENTIAL", "test-credential")
    monkeypatch.delenv("SUB2API_MODEL", raising=False)
    for model in ("gpt-5.6-sol", "grok-4.7", "gpt-6-astra"):
        monkeypatch.setenv("CUSTOM_MODEL", model)
        assert get_custom_model_env() == "gpt-6.1-sol"
    monkeypatch.setenv("CUSTOM_MODEL", "gpt-6-sol")
    assert get_custom_model_env() == "gpt-6-sol"


def test_unmanaged_custom_model_is_preserved(monkeypatch):
    monkeypatch.delenv("SUB2API_APP_CREDENTIAL", raising=False)
    monkeypatch.setenv("CUSTOM_MODEL", "my-private-model")
    assert get_custom_model_env() == "my-private-model"
