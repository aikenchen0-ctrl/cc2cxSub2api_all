from __future__ import annotations

import json
import math
import os
from dataclasses import dataclass
from pathlib import Path
from typing import Any
from urllib.parse import urlsplit, urlunsplit

try:
    from dotenv import load_dotenv
except ImportError:  # Docker may inject environment variables directly.
    load_dotenv = None


def _env(name: str, default: str = "") -> str:
    return os.environ.get(name, default).strip()


def _base_url(raw: str) -> str:
    value = raw.strip()
    if not value:
        return ""
    if "://" not in value:
        value = "http://" + value
    return value.rstrip("/")


def _parse_json_object(name: str) -> dict[str, Any]:
    raw = _env(name, "{}")
    try:
        value = json.loads(raw)
    except json.JSONDecodeError as exc:
        raise ValueError(f"{name} must be valid JSON") from exc
    if not isinstance(value, dict):
        raise ValueError(f"{name} must be a JSON object")
    return value


def _admin_origin(value: str) -> str:
    value = _base_url(value)
    if not value:
        return ""
    parsed = urlsplit(value)
    path = parsed.path.rstrip("/")
    if path.endswith("/v1"):
        path = path[:-3]
    return urlunsplit((parsed.scheme, parsed.netloc, path, "", "")).rstrip("/")


@dataclass(frozen=True)
class Settings:
    link: str
    admin_base_url: str
    gateway_base_url: str
    admin_api_key: str
    gateway_api_keys: dict[int, str]
    admin_jwt: str
    group_profiles: dict[int, dict[str, Any]]
    sso_secret: str
    session_secret: str
    cookie_secure: bool
    allowed_origins: tuple[str, ...]
    db_path: Path
    interval_minutes: int
    schedule_enabled: bool
    run_on_start: bool
    max_group_calibrations: int
    max_parallel_accounts: int
    request_timeout_seconds: float
    auto_stop_enabled: bool
    multiplier_order_of_magnitude: float
    trace_min_probability: float
    trace_min_profile_similarity: float
    trace_min_margin: float

    @property
    def configured(self) -> bool:
        return bool(self.admin_base_url and (self.admin_api_key or self.admin_jwt) and self.gateway_api_keys)


def load_settings() -> Settings:
    if load_dotenv is not None:
        load_dotenv(Path(__file__).resolve().parents[1] / ".env", override=False)
    link = _base_url(_env("LINK", "localhost:18080"))
    relay = _base_url(_env("SUB2API_RELAY_BASE_URL"))
    gateway_root = relay or link
    gateway_base = gateway_root if gateway_root.endswith("/v1") else gateway_root + "/v1"
    admin_root = _admin_origin(_env("SUB2API_ADMIN_BASE_URL") or relay or link)

    raw_keys = _parse_json_object("SUB2API_GATEWAY_API_KEYS")
    gateway_keys: dict[int, str] = {}
    for group_id, key in raw_keys.items():
        try:
            parsed_id = int(group_id)
        except (TypeError, ValueError) as exc:
            raise ValueError("SUB2API_GATEWAY_API_KEYS keys must be group IDs") from exc
        if parsed_id <= 0 or not isinstance(key, str) or not key.strip():
            raise ValueError("SUB2API_GATEWAY_API_KEYS must map positive group IDs to nonempty keys")
        normalized_key = key.strip()
        if normalized_key.lower().startswith("sk-super-"):
            raise ValueError("SUB2API_GATEWAY_API_KEYS must contain ordinary user API keys, never SuperKeys")
        gateway_keys[parsed_id] = normalized_key

    raw_profiles = _parse_json_object("MODELAUDIT_GROUP_PROFILES")
    profiles: dict[int, dict[str, Any]] = {}
    for group_id, profile in raw_profiles.items():
        try:
            parsed_id = int(group_id)
        except (TypeError, ValueError) as exc:
            raise ValueError("MODELAUDIT_GROUP_PROFILES keys must be group IDs") from exc
        if parsed_id <= 0 or not isinstance(profile, dict):
            raise ValueError("MODELAUDIT_GROUP_PROFILES values must be objects keyed by positive group IDs")
        profiles[parsed_id] = profile

    interval = int(_env("MODELAUDIT_INTERVAL_MINUTES", "60"))
    if interval not in (30, 60):
        raise ValueError("MODELAUDIT_INTERVAL_MINUTES must be 30 or 60")
    calibrations = int(_env("MODELAUDIT_MAX_GROUP_CALIBRATIONS", "5"))
    if not 1 <= calibrations <= 20:
        raise ValueError("MODELAUDIT_MAX_GROUP_CALIBRATIONS must be between 1 and 20")
    parallel_accounts = int(_env("MODELAUDIT_MAX_PARALLEL_ACCOUNTS", "4"))
    if not 1 <= parallel_accounts <= 32:
        raise ValueError("MODELAUDIT_MAX_PARALLEL_ACCOUNTS must be between 1 and 32")
    multiplier_magnitude = float(_env("MODELAUDIT_MULTIPLIER_ORDER_OF_MAGNITUDE", "10"))
    if multiplier_magnitude <= 1:
        raise ValueError("MODELAUDIT_MULTIPLIER_ORDER_OF_MAGNITUDE must be greater than 1")
    trace_probability = float(_env("MODELAUDIT_MODELTRACE_MIN_PROBABILITY", "0.95"))
    trace_similarity_floor = float(_env("MODELAUDIT_MODELTRACE_MIN_PROFILE_SIMILARITY", "0"))
    trace_margin = float(_env("MODELAUDIT_MODELTRACE_MIN_MARGIN", "0.20"))
    if any(
        not math.isfinite(value) or value < 0 or value > 1
        for value in (trace_probability, trace_similarity_floor, trace_margin)
    ):
        raise ValueError("ModelTrace confidence thresholds must be finite numbers between 0 and 1")

    allowed_origins = tuple(
        origin.strip().rstrip("/")
        for origin in _env("MODELAUDIT_ALLOWED_ORIGINS").split(",")
        if origin.strip()
    )
    data_path = Path(_env("MODELAUDIT_DB_PATH", "./data/modelaudit.sqlite3"))

    return Settings(
        link=link,
        admin_base_url=admin_root,
        gateway_base_url=gateway_base,
        admin_api_key=_env("SUB2API_ADMIN_API_KEY"),
        admin_jwt=_env("SUB2API_ADMIN_JWT"),
        gateway_api_keys=gateway_keys,
        group_profiles=profiles,
        sso_secret=_env("SUB2API_SSO_SECRET"),
        session_secret=_env("MODELAUDIT_SESSION_SECRET"),
        cookie_secure=_env("MODELAUDIT_COOKIE_SECURE", "true").lower() not in {"0", "false", "no"},
        allowed_origins=allowed_origins,
        db_path=data_path,
        interval_minutes=interval,
        schedule_enabled=_env("MODELAUDIT_SCHEDULE_ENABLED", "true").lower() not in {"0", "false", "no"},
        run_on_start=_env("MODELAUDIT_RUN_ON_START", "false").lower() in {"1", "true", "yes"},
        max_group_calibrations=calibrations,
        max_parallel_accounts=parallel_accounts,
        request_timeout_seconds=float(_env("MODELAUDIT_REQUEST_TIMEOUT_SECONDS", "240")),
        auto_stop_enabled=_env("MODELAUDIT_AUTO_STOP_ENABLED", "false").lower() in {"1", "true", "yes"},
        multiplier_order_of_magnitude=multiplier_magnitude,
        trace_min_probability=trace_probability,
        # This is an additional operator floor. The default 0 uses the
        # bank-hash-bound per-candidate calibration profile.
        trace_min_profile_similarity=trace_similarity_floor,
        trace_min_margin=trace_margin,
    )
