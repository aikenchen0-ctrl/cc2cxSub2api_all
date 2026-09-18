import asyncio

from models.sql.user import User
from services.provider_settings import (
    merge_provider_settings,
    migrate_provider_settings_from_file,
    sanitize_provider_settings,
    with_sub2api_defaults,
)
from utils.user_config_store import read_user_config_file, update_user_config_file


class ProviderSettingsSession:
    def __init__(self):
        self.row = None

    async def get(self, _model, _key):
        return self.row

    def add(self, row):
        self.row = row

    async def commit(self):
        return None

    async def refresh(self, _row):
        return None


def test_sub2api_bootstrap_preserves_existing_provider(monkeypatch):
    monkeypatch.setenv("SUB2API_API_KEY", "server-only-key")
    original = {"LLM": "openai", "OPENAI_MODEL": "existing-model"}
    assert with_sub2api_defaults(original) == original


def test_sub2api_bootstrap_requires_key(monkeypatch):
    monkeypatch.delenv("SUB2API_API_KEY", raising=False)
    assert with_sub2api_defaults({}) == {}


def test_sub2api_bootstrap_normalizes_url_and_preserves_options(monkeypatch):
    monkeypatch.setenv("SUB2API_API_KEY", "server-only-key")
    monkeypatch.setenv("SUB2API_MODEL", "test-model")
    for base in ("http://relay:8080", "http://relay:8080/", "http://relay:8080/v1/"):
        monkeypatch.setenv("SUB2API_BASE_URL", base)
        config = with_sub2api_defaults({"DISABLE_IMAGE_GENERATION": False})
        assert config["CUSTOM_LLM_URL"] == "http://relay:8080/v1"
        assert config["CUSTOM_LLM_API_KEY"] == "server-only-key"
        assert config["CUSTOM_MODEL"] == "test-model"
        assert config["DISABLE_IMAGE_GENERATION"] is False


def test_sub2api_defaults_persist_and_do_not_reset_on_restart(monkeypatch, tmp_path):
    monkeypatch.setenv("USER_CONFIG_PATH", str(tmp_path / "userConfig.json"))
    monkeypatch.setenv("SUB2API_API_KEY", "server-only-key")
    monkeypatch.setenv("SUB2API_BASE_URL", "http://relay:8080/v1")
    session = ProviderSettingsSession()
    config = asyncio.run(migrate_provider_settings_from_file(session))
    assert config["LLM"] == "custom"
    assert read_user_config_file(str(tmp_path / "userConfig.json")) == config
    monkeypatch.setenv("SUB2API_API_KEY", "changed-env-key")
    assert asyncio.run(migrate_provider_settings_from_file(session)) == config


def test_user_table_has_username_and_no_email_column():
    columns = set(User.__table__.columns.keys())

    assert "username" in columns
    assert "email" not in columns


def test_provider_settings_exclude_all_legacy_auth_fields():
    assert sanitize_provider_settings(
        {
            "LLM": "openai",
            "AUTH_USERNAME": "admin",
            "AUTH_PASSWORD": "plain",
            "AUTH_PASSWORD_HASH": "hash",
            "AUTH_SECRET_KEY": "jwt-secret",
        }
    ) == {"LLM": "openai"}


def test_reset_advanced_settings_removes_only_optional_overrides():
    existing = {
        "LLM": "openrouter",
        "OPENROUTER_API_KEY": "key",
        "OPENROUTER_MODEL": "openai/gpt-4o",
        "LLM_MAX_OUTPUT_TOKENS": 65536,
        "LLM_REASONING_MODE": "enabled",
        "OPENROUTER_PROVIDER_ORDER": ["groq"],
    }

    merged = merge_provider_settings(
        existing,
        {
            "LLM_MAX_OUTPUT_TOKENS": "",
            "LLM_REASONING_MODE": "",
            "OPENROUTER_PROVIDER_ORDER": [],
        },
    )

    assert merged == {
        "LLM": "openrouter",
        "OPENROUTER_API_KEY": "key",
        "OPENROUTER_MODEL": "openai/gpt-4o",
    }


def test_startup_migrates_user_config_and_rewrites_compatibility_file(
    monkeypatch, tmp_path
):
    path = tmp_path / "userConfig.json"
    monkeypatch.setenv("USER_CONFIG_PATH", str(path))
    update_user_config_file(
        str(path),
        lambda _: {
            "AUTH_USERNAME": "admin",
            "AUTH_PASSWORD_HASH": "legacy-hash",
            "AUTH_SECRET_KEY": "jwt-secret",
            "LLM": "openai",
            "OPENAI_API_KEY": "provider-key",
        },
    )
    session = ProviderSettingsSession()

    migrated = asyncio.run(migrate_provider_settings_from_file(session))

    assert migrated == {
        "LLM": "openai",
        "OPENAI_API_KEY": "provider-key",
    }
    assert session.row.config == migrated
    assert read_user_config_file(str(path)) == {
        "LLM": "openai",
        "OPENAI_API_KEY": "provider-key",
        "AUTH_USERNAME": "admin",
        "AUTH_PASSWORD_HASH": "legacy-hash",
        "AUTH_SECRET_KEY": "jwt-secret",
    }
