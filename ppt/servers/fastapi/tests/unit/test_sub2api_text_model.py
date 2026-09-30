from utils.get_env import get_custom_model_env


def test_managed_text_model_replaces_stale_image_selection(monkeypatch):
    monkeypatch.setenv("SUB2API_APP_CREDENTIAL", "test-credential")
    monkeypatch.setenv("SUB2API_MODEL", "gpt-5.6-sol")
    for model in ("gpt-image-2", "kling-v3", "", "unknown-model"):
        monkeypatch.setenv("CUSTOM_MODEL", model)
        assert get_custom_model_env() == "gpt-5.6-sol"
    monkeypatch.setenv("CUSTOM_MODEL", "gpt-5.5")
    assert get_custom_model_env() == "gpt-5.5"


def test_unmanaged_custom_model_is_preserved(monkeypatch):
    monkeypatch.delenv("SUB2API_APP_CREDENTIAL", raising=False)
    monkeypatch.setenv("CUSTOM_MODEL", "my-private-model")
    assert get_custom_model_env() == "my-private-model"
