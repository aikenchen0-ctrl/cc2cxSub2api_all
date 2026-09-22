"""Application settings managed via pydantic-settings."""

import os
import platform
import sys
from functools import lru_cache
from pathlib import Path

from pydantic import model_validator
from pydantic_settings import BaseSettings, SettingsConfigDict


_BACKEND_ROOT = Path(__file__).resolve().parent.parent


_PLACEHOLDER_SECRETS = {
    "your-key",
    "your_key",
    "your-api-key",
    "changeme",
    "replace-me",
    "xxx",
    "sk-xxx",
    "none",
    "null",
    "undefined",
}


def sanitize_secret(value: object) -> str:
    """Treat empty values, placeholders, and inline comments as missing secrets."""
    text = str(value or "").strip().strip('"').strip("'")
    if not text or text.startswith("#"):
        return ""
    for sep in (" #", "\t#"):
        if sep in text:
            text = text.split(sep, 1)[0].strip()
            break
    return text


def has_usable_api_key(value: object) -> bool:
    """True when a configured secret looks like a real key, not a comment."""
    cleaned = sanitize_secret(value)
    if not cleaned:
        return False
    return cleaned.lower() not in _PLACEHOLDER_SECRETS


def _app_data_dir() -> Path | None:
    """Return the user's writable app-data directory when running packaged.

    Returns None in dev mode so callers fall back to relative paths.
    """
    if not getattr(sys, "frozen", False):
        return None
    system = platform.system()
    if system == "Windows":
        base = Path(os.environ.get("APPDATA", str(Path.home())))
    elif system == "Darwin":
        base = Path.home() / "Library" / "Application Support"
    else:
        base = Path.home() / ".config"
    app_dir = base / "AIHunter"
    app_dir.mkdir(parents=True, exist_ok=True)
    return app_dir


def _resolve_env_file() -> str:
    """Return the .env path to use.

    - Packaged (PyInstaller frozen binary): user app-data dir so the file
      survives app updates and is writable by the user.
    - Dev / bare Python: local .env next to the current working directory.
    """
    d = _app_data_dir()
    if d is not None:
        return str(d / ".env")
    return str(_BACKEND_ROOT / ".env")


def _resolve_dir(relative: str) -> str:
    """Resolve a writable directory path (created if missing in packaged mode)."""
    d = _app_data_dir()
    if d is not None:
        resolved = d / relative
        resolved.mkdir(parents=True, exist_ok=True)
        return str(resolved)
    resolved = _BACKEND_ROOT / relative
    resolved.mkdir(parents=True, exist_ok=True)
    return str(resolved)


def _resolve_file(relative: str) -> str:
    """Resolve a writable file path (parent dir created if missing in packaged mode)."""
    d = _app_data_dir()
    if d is not None:
        resolved = d / relative
        resolved.parent.mkdir(parents=True, exist_ok=True)
        return str(resolved)
    resolved = _BACKEND_ROOT / relative
    resolved.parent.mkdir(parents=True, exist_ok=True)
    return str(resolved)


class Settings(BaseSettings):
    """AI Hunter configuration — loaded from environment variables or .env file."""

    model_config = SettingsConfigDict(
        env_file=_resolve_env_file(),
        env_file_encoding="utf-8",
        case_sensitive=False,
        extra="ignore",
        protected_namespaces=("model_",),
    )

    # --- LLM (litellm model format: "provider/model") ---
    # Active model — uses litellm naming, e.g.:
    #   "gpt-4o"                        (OpenAI)
    #   "anthropic/claude-3-5-sonnet-20241022"  (Anthropic)
    #   "openrouter/google/gemini-pro"  (OpenRouter)
    #   "groq/llama-3.3-70b-versatile"  (Groq)
    #   "zai/glm-4.7"                   (GLM / 智谱 Z.AI)
    #   "moonshot/moonshot-v1-128k"      (Kimi / Moonshot)
    #   "minimax/MiniMax-Text-01"        (MiniMax)
    llm_model: str = "gpt-4o-mini"
    llm_temperature: float = 0.3
    llm_max_tokens: int = 4096
    llm_requests_per_minute: int = 0
    # GPT-5.x reasoning effort: none | low | medium | high | xhigh | max. Empty = provider default.
    llm_reasoning_effort: str = ""

    # Reasoning model — used for ReAct agent decision-making (stronger reasoning)
    #   e.g. "gpt-4o", "anthropic/claude-3-5-sonnet-20241022", "openrouter/deepseek/deepseek-r1"
    reasoning_model: str = "gpt-4o"
    reasoning_temperature: float = 0.2
    reasoning_max_tokens: int = 4096
    reasoning_requests_per_minute: int = 0

    # --- LLM Provider API Keys ---
    openai_api_key: str = ""
    openai_api_base: str = ""  # OpenAI-compatible gateway, e.g. http://api.example.com/v1
    openai_api_mode: str = "auto"  # auto | chat | responses
    anthropic_api_key: str = ""
    openrouter_api_key: str = ""
    groq_api_key: str = ""
    zai_api_key: str = ""         # GLM / 智谱 Z.AI
    moonshot_api_key: str = ""    # Kimi / Moonshot AI
    minimax_api_key: str = ""
    minimax_api_base: str = "https://api.minimax.io/v1"
    email_openai_api_key: str = ""
    email_anthropic_api_key: str = ""
    email_openrouter_api_key: str = ""
    email_groq_api_key: str = ""
    email_zai_api_key: str = ""
    email_moonshot_api_key: str = ""
    email_minimax_api_key: str = ""

    # --- Search ---
    serper_api_key: str = ""
    tavily_api_key: str = ""      # supports multiple keys: "key1,key2"
    jina_api_key: str = ""
    # auto = Serper Maps when SERPER_API_KEY is set, else DuckDuckGo (encyclopedia is not a lead)
    # serper = always Google Maps via Serper (raises if key missing)
    # free = always no-key web search
    search_backend: str = "auto"
    # Explicit HTTP proxy for DuckDuckGo / page fetch. Skips WorkBuddy's
    # sandbox HTTP_PROXY (60376). Example: http://127.0.0.1:7897 (Clash Verge)
    search_http_proxy: str = ""

    # --- OSINT enrichment (spiderfoot ports; default off) ---
    osint_enrichment_enabled: bool = False
    builtwith_api_key: str = ""

    # --- Social presence (maigret-inspired; default off, ≤20 sites / lead) ---
    social_presence_enabled: bool = False
    social_presence_use_maigret: bool = True
    social_presence_max_sites: int = 20
    social_presence_country_tags: str = ""

    # --- Licensed finder bypass (BetterContact; not in the LangGraph path) ---
    licensed_finder_enabled: bool = False
    bettercontact_api_key: str = ""
    licensed_finder_base_url: str = "https://app.bettercontact.rocks/api/v2"
    licensed_finder_poll_seconds: float = 5.0
    licensed_finder_timeout_seconds: int = 300

    # --- Site scan for Insight (Firecrawl high-value pages; default off) ---
    site_scan_enabled: bool = False
    firecrawl_api_key: str = ""
    site_scan_max_pages: int = 6

    # --- Email ---
    email_provider_type: str = "smtp"
    email_dry_run: bool = True
    email_block_freemail: bool = True
    email_min_minutes_between_sends: int = 0
    email_verify_min_fit_score: float = 0.0
    email_from_name: str = "B2Binsights"
    email_from_address: str = ""
    email_reply_to: str = ""
    email_smtp_host: str = ""
    email_smtp_port: int = 587
    email_smtp_username: str = ""
    email_smtp_password: str = ""
    email_smtp_last_test_at: str = ""
    email_imap_host: str = ""
    email_imap_port: int = 993
    email_imap_username: str = ""
    email_imap_password: str = ""
    email_imap_last_test_at: str = ""
    email_use_tls: bool = True
    email_sequence_enabled: bool = False
    email_auto_send_enabled: bool = False
    email_step1_delay_days: int = 0
    email_step2_delay_days: int = 3
    email_step3_delay_days: int = 3
    email_business_hours_start: str = "09:00"
    email_business_hours_end: str = "18:00"
    email_weekdays_only: bool = True
    email_timezone: str = "Asia/Shanghai"
    email_daily_send_limit: int = 50
    email_hourly_send_limit: int = 10
    email_language_mode: str = "auto_by_region"
    email_default_language: str = "en"
    email_fallback_language: str = "en"
    email_tone: str = "professional"
    email_signature_block: str = ""
    email_public_base_url: str = ""
    email_unsubscribe_secret: str = ""
    email_llm_model: str = ""
    email_reasoning_model: str = ""
    email_llm_requests_per_minute: int = 0
    email_reasoning_requests_per_minute: int = 0
    email_min_fit_score_to_send: float = 0.6
    email_min_contactability_score_to_send: float = 0.45
    email_allow_inferred_target: bool = True
    email_allow_generic_company_email: bool = False
    email_require_approval_before_send: bool = True
    email_reply_detection_enabled: bool = False
    email_reply_check_interval_seconds: int = 180
    email_template_max_send_count: int = 100
    email_template_underperforming_min_assigned: int = 10
    email_template_underperforming_min_reply_rate: float = 1.0
    email_review_min_score: int = 75
    email_review_max_blocking_issues: int = 0
    email_validation_max_revisions: int = 2
    email_review_auto_fix_rounds: int = 2

    # --- Langfuse (observability) ---
    langfuse_public_key: str = ""
    langfuse_secret_key: str = ""
    langfuse_host: str = "http://localhost:3000"
    langfuse_enabled: bool = False

    # --- Hunt defaults ---
    default_target_lead_count: int = 200
    default_max_rounds: int = 10
    default_keywords_per_round: int = 8
    min_new_leads_threshold: int = 5  # stop if fewer new leads per round

    # --- Concurrency ---
    search_concurrency: int = 10  # max concurrent Serper API calls
    scrape_concurrency: int = 5   # max concurrent Jina Reader calls
    email_gen_concurrency: int = 3  # max concurrent LLM calls for email generation
    react_max_iterations: int = 5   # max ReAct loop iterations per URL

    # --- API ---
    api_host: str = "0.0.0.0"
    api_port: int = 8000
    cors_origins: list[str] = ["http://localhost:3000", "http://localhost:3001"]
    api_access_token: str = ""
    settings_api_enabled: bool = True

    # --- Database (checkpointer) ---
    # In packaged mode, redirected to ~/Library/Application Support/AIHunter/
    checkpoint_db_path: str = _resolve_file("hunt_sessions.db")
    email_db_path: str = _resolve_file("email_automation.db")
    automation_queue_db_path: str = _resolve_file("automation_queue.db")
    template_seed_cache_path: str = _resolve_file("template_seed_cache.json")
    automation_feishu_webhook_url: str = ""
    automation_summary_enabled: bool = False
    automation_summary_interval_seconds: int = 7200
    automation_alerts_enabled: bool = False
    automation_alert_interval_seconds: int = 1800
    automation_alert_backlog_threshold: int = 20
    automation_alert_failed_messages_threshold: int = 10
    automation_event_notifications_enabled: bool = True
    automation_discovery_batch_size: int = 5
    automation_send_batch_size: int = 10
    automation_event_flush_interval_seconds: int = 600
    automation_embedded_consumer_enabled: bool = True
    automation_template_seed_prewarm_enabled: bool = True
    automation_consumer_poll_seconds: int = 5
    automation_consumer_retry_delay_seconds: int = 120
    automation_consumer_max_attempts: int = 3
    automation_consumer_status_poll_seconds: int = 15
    automation_consumer_request_timeout_seconds: int = 60
    automation_consumer_auto_start_campaign: bool = True

    # --- Hunt persistence ---
    hunts_dir: str = _resolve_dir("data/hunts")  # directory for JSON hunt files

    # --- File upload ---
    upload_dir: str = _resolve_dir("uploads")
    max_upload_size_mb: int = 50

    @model_validator(mode="after")
    def _drop_comment_secrets(self):
        for name, value in self.__dict__.items():
            if name.endswith("_api_key") or name.endswith("_api_base") or name.endswith("_password"):
                cleaned = sanitize_secret(value)
                if cleaned != value:
                    setattr(self, name, cleaned)
        return self


@lru_cache
def _get_settings_for_subject(subject: str) -> Settings:
    """Build one cached settings object per authenticated subject."""
    base = Settings()
    if not subject:
        return base
    try:
        # Imported lazily to avoid a module import cycle during startup.
        from config.settings_store import read_settings

        overrides = read_settings()
    except Exception:
        overrides = {}
    if not overrides:
        return base
    values = base.model_dump()
    for key, value in overrides.items():
        field = str(key).strip().lower()
        if field in values:
            values[field] = value
    try:
        return Settings.model_validate(values)
    except Exception:
        # A malformed per-user value must not take the whole satellite down;
        # retain the validated process defaults and let the UI correct it.
        return base


def get_settings() -> Settings:
    """Return settings scoped to the current SSO subject when managed."""
    try:
        from auth_sso import current_subject, managed as satellite_managed

        subject = current_subject().strip() if satellite_managed() else ""
    except Exception:
        subject = ""
    return _get_settings_for_subject(subject)


# Preserve the cache invalidation API used by the existing settings routes and
# tests while keeping the public zero-argument call unchanged.
get_settings.cache_clear = _get_settings_for_subject.cache_clear
