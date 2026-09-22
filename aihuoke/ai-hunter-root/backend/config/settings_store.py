"""Read and write the user .env configuration file."""

from __future__ import annotations

import platform
import sys
import hashlib
import os
from pathlib import Path

from config.settings import has_usable_api_key, sanitize_secret
from auth_sso import current_subject, has_configured_secret, managed as satellite_managed

_BACKEND_ROOT = Path(__file__).resolve().parent.parent


_LLM_KEYS = {
    "OPENAI_API_KEY",
    "ANTHROPIC_API_KEY",
    "OPENROUTER_API_KEY",
    "GROQ_API_KEY",
    "ZAI_API_KEY",
    "MOONSHOT_API_KEY",
    "MINIMAX_API_KEY",
}


def get_env_path() -> Path:
    """Return the effective .env file path for dev or packaged mode."""
    subject = current_subject().strip() if satellite_managed() else ""
    if getattr(sys, "frozen", False):
        system = platform.system()
        if system == "Darwin":
            base = Path.home() / "Library" / "Application Support" / "AIHunter"
        elif system == "Windows":
            import os

            base = Path(os.environ.get("APPDATA", str(Path.home()))) / "AIHunter"
        else:
            base = Path.home() / ".config" / "AIHunter"
        base.mkdir(parents=True, exist_ok=True)
        base = base / ".env"
        if subject:
            digest = hashlib.sha256(subject.encode("utf-8")).hexdigest()[:32]
            scoped_dir = base.parent / "user-settings"
            scoped_dir.mkdir(parents=True, exist_ok=True)
            return scoped_dir / f"{digest}.env"
        return base
    base = _BACKEND_ROOT / ".env"
    # Managed satellite users may customize search/email settings. Keep each
    # user's values in a separate file derived from the SSO subject instead of
    # sharing one process-wide .env (which would let the last writer affect
    # every tenant).
    if not subject:
        return base
    digest = hashlib.sha256(subject.encode("utf-8")).hexdigest()[:32]
    scoped_dir = base.parent / "data" / "user-settings"
    scoped_dir.mkdir(parents=True, exist_ok=True)
    return scoped_dir / f"{digest}.env"


def read_settings() -> dict[str, str]:
    """Parse the .env file into a KEY -> value mapping."""
    path = get_env_path()
    if not path.exists():
        return {}

    result: dict[str, str] = {}
    for line in path.read_text(encoding="utf-8").splitlines():
        line = line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, _, value = line.partition("=")
        result[key.strip()] = sanitize_secret(value)
    return result


def write_settings(data: dict[str, str]) -> None:
    """Overwrite the .env file with the provided mapping."""
    path = get_env_path()
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text("".join(f"{key}={value}\n" for key, value in data.items()), encoding="utf-8")


def update_settings(updates: dict[str, str]) -> None:
    """Merge updates into the existing .env file."""
    existing = read_settings()
    existing.update(updates)
    write_settings(existing)


def is_configured() -> bool:
    """Return True when at least one LLM key is configured."""
    if satellite_managed():
        return bool(
            has_configured_secret(os.getenv("SUB2API_APP_CREDENTIAL", ""))
            and current_subject().strip()
            and (os.getenv("SUB2API_RELAY_BASE_URL", "").strip() or os.getenv("LINK", "").strip())
        )
    settings = read_settings()
    return any(has_usable_api_key(settings.get(key, "")) for key in _LLM_KEYS)
