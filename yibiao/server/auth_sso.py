"""Sub2API SSO receiver and local session storage for 自动招标.

The browser only receives ``yibiao_session``.  The Sub2API application
credential and on-behalf-of identity are used by the server when it calls the
model gateway; neither is accepted from browser input.
"""

from __future__ import annotations

import base64
import hashlib
import hmac
import json
import os
import secrets
import sqlite3
import time
from contextvars import ContextVar
from dataclasses import dataclass
from pathlib import Path
from typing import Any
from urllib.parse import urlsplit

from fastapi import HTTPException, Request, Response

SESSION_COOKIE = "yibiao_session"
SESSION_TTL_SECONDS = 3 * 24 * 60 * 60
_CURRENT_SUBJECT: ContextVar[str] = ContextVar("yibiao_sub2api_subject", default="")


def _load_local_env() -> None:
    """Load ``server/.env`` for standalone launches without overriding env.

    Docker/systemd deployments inject their environment directly; keeping
    those values authoritative avoids surprising credential changes while the
    checked-in ``.env.example`` remains useful for ``python app.py``.
    """
    env_path = Path(__file__).resolve().parent / ".env"
    try:
        lines = env_path.read_text(encoding="utf-8").splitlines()
    except OSError:
        return
    for line in lines:
        text = line.strip()
        if not text or text.startswith("#") or "=" not in text:
            continue
        key, value = text.split("=", 1)
        key = key.strip()
        value = value.strip().strip('"').strip("'")
        if key and key not in os.environ:
            os.environ[key] = value


_load_local_env()


def _has_configured_secret(value: object) -> bool:
    normalized = str(value or "").strip().lower()
    if not normalized:
        return False
    return not (
        normalized in {"changeme", "change-me", "replace-me", "your-key", "your_api_key", "your-api-key", "xxx", "sk-xxx"}
        or normalized.startswith(("your_", "your-", "replace-with-", "example-", "sk-super-"))
    )


@dataclass(frozen=True)
class Identity:
    id: int
    subject: str
    username: str = ""
    display_name: str = ""


def _db_path() -> Path:
    raw = os.getenv("YIBIAO_AUTH_DB", "").strip()
    path = Path(raw) if raw else Path(__file__).resolve().parent / "data" / "auth.db"
    path.parent.mkdir(parents=True, exist_ok=True)
    return path


def _connect() -> sqlite3.Connection:
    conn = sqlite3.connect(_db_path(), timeout=5)
    conn.row_factory = sqlite3.Row
    conn.executescript(
        """
        CREATE TABLE IF NOT EXISTS identities (
          id INTEGER PRIMARY KEY AUTOINCREMENT,
          issuer TEXT NOT NULL,
          subject TEXT NOT NULL,
          username TEXT NOT NULL DEFAULT '',
          display_name TEXT NOT NULL DEFAULT '',
          created_at INTEGER NOT NULL,
          updated_at INTEGER NOT NULL,
          UNIQUE (issuer, subject)
        );
        CREATE TABLE IF NOT EXISTS consumed_tickets (
          jti TEXT PRIMARY KEY,
          expires_at INTEGER NOT NULL,
          consumed_at INTEGER NOT NULL
        );
        CREATE TABLE IF NOT EXISTS sessions (
          token_hash TEXT PRIMARY KEY,
          identity_id INTEGER NOT NULL REFERENCES identities(id),
          expires_at INTEGER NOT NULL,
          created_at INTEGER NOT NULL
        );
        """
    )
    conn.commit()
    return conn


def _decode_part(value: str) -> bytes:
    return base64.urlsafe_b64decode(value + "=" * (-len(value) % 4))


def verify_ticket(raw: str, now: int | None = None) -> dict[str, Any]:
    secret = os.getenv("SUB2API_SSO_SECRET", "").strip()
    if not _has_configured_secret(secret) or len(secret) < 32:
        raise ValueError("SSO is not configured")
    if not isinstance(raw, str) or not raw or len(raw) > 8192:
        raise ValueError("invalid ticket")
    try:
        encoded, supplied = raw.split(".", 1)
        expected = hmac.new(secret.encode(), encoded.encode(), hashlib.sha256).digest()
        if not hmac.compare_digest(expected, _decode_part(supplied)):
            raise ValueError("invalid signature")
        payload = json.loads(_decode_part(encoded))
    except (ValueError, TypeError, UnicodeDecodeError, json.JSONDecodeError, base64.binascii.Error) as exc:
        raise ValueError("invalid ticket") from exc
    if not isinstance(payload, dict) or payload.get("iss") != "sub2api" or payload.get("aud") != "yibiao":
        raise ValueError("invalid audience")
    # Legacy tickets carrying relay/SuperKey material are rejected.
    if "rk" in payload or "api_key" in payload or "super_key" in payload:
        raise ValueError("credential material is not allowed")
    subject = payload.get("sub")
    jti = payload.get("jti")
    if not isinstance(subject, str) or not subject.strip() or len(subject) > 160:
        raise ValueError("invalid subject")
    if not isinstance(jti, str) or not jti.strip() or len(jti) > 200:
        raise ValueError("invalid nonce")
    issued, expires = payload.get("iat"), payload.get("exp")
    current = int(time.time()) if now is None else int(now)
    if type(issued) is not int or type(expires) is not int or issued <= 0 or expires <= issued or expires <= current or expires - issued > 120 or issued > current + 30:
        raise ValueError("expired ticket")
    return payload


def safe_next(value: object) -> str:
    if not isinstance(value, str) or not value.startswith("/") or value.startswith("//") or "\\" in value or len(value) > 2048 or any(ord(c) < 32 for c in value):
        return "/"
    return value


def consume_ticket(payload: dict[str, Any]) -> Identity:
    now = int(time.time())
    conn = _connect()
    try:
        conn.execute("DELETE FROM consumed_tickets WHERE expires_at < ?", (now,))
        try:
            conn.execute("INSERT INTO consumed_tickets(jti, expires_at, consumed_at) VALUES (?, ?, ?)", (payload["jti"], int(payload["exp"]), now))
        except sqlite3.IntegrityError as exc:
            conn.rollback()
            raise ValueError("ticket already used") from exc
        row = conn.execute("SELECT * FROM identities WHERE issuer = ? AND subject = ?", ("sub2api", payload["sub"])).fetchone()
        if row is None:
            conn.execute(
                "INSERT INTO identities(issuer, subject, username, display_name, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
                ("sub2api", payload["sub"], str(payload.get("username") or ""), str(payload.get("displayName") or ""), now, now),
            )
            identity_id = int(conn.execute("SELECT last_insert_rowid()").fetchone()[0])
        else:
            identity_id = int(row["id"])
            conn.execute("UPDATE identities SET username = ?, display_name = ?, updated_at = ? WHERE id = ?", (str(payload.get("username") or row["username"]), str(payload.get("displayName") or row["display_name"]), now, identity_id))
        conn.commit()
        row = conn.execute("SELECT * FROM identities WHERE id = ?", (identity_id,)).fetchone()
        assert row is not None
        return Identity(int(row["id"]), str(row["subject"]), str(row["username"]), str(row["display_name"]))
    finally:
        conn.close()


def create_session(identity: Identity, response: Response, request: Request) -> None:
    token = secrets.token_urlsafe(32)
    now = int(time.time())
    conn = _connect()
    try:
        conn.execute("INSERT INTO sessions(token_hash, identity_id, expires_at, created_at) VALUES (?, ?, ?, ?)", (_hash(token), identity.id, now + SESSION_TTL_SECONDS, now))
        conn.commit()
    finally:
        conn.close()
    forwarded = request.headers.get("x-forwarded-proto", request.url.scheme).split(",", 1)[0].strip().lower()
    secure = forwarded == "https"
    response.set_cookie(SESSION_COOKIE, token, max_age=SESSION_TTL_SECONDS, httponly=True, secure=secure, samesite="lax", path="/")


def get_identity(request: Request) -> Identity | None:
    token = request.cookies.get(SESSION_COOKIE, "").strip()
    if not token:
        return None
    conn = _connect()
    try:
        row = conn.execute("SELECT i.* FROM sessions s JOIN identities i ON i.id = s.identity_id WHERE s.token_hash = ? AND s.expires_at >= ?", (_hash(token), int(time.time()))).fetchone()
        if row is None:
            return None
        return Identity(int(row["id"]), str(row["subject"]), str(row["username"]), str(row["display_name"]))
    finally:
        conn.close()


def require_identity(request: Request) -> Identity:
    identity = get_identity(request)
    if identity is None:
        raise HTTPException(status_code=401, detail="Authentication required")
    return identity


def set_current_subject(subject: str):
    return _CURRENT_SUBJECT.set(str(subject or "").strip())


def reset_current_subject(token: object) -> None:
    _CURRENT_SUBJECT.reset(token)  # type: ignore[arg-type]


def current_subject() -> str:
    return _CURRENT_SUBJECT.get()


def satellite_headers(*, required: bool = True) -> dict[str, str]:
    subject = current_subject()
    credential = os.getenv("SUB2API_APP_CREDENTIAL", "").strip()
    if required and (not subject or not _has_configured_secret(credential)):
        raise HTTPException(status_code=503, detail="Sub2API satellite credentials are not configured")
    if not subject or not _has_configured_secret(credential):
        return {}
    return {
        "Authorization": f"Bearer {credential}",
        "X-Sub2API-On-Behalf-Of": subject,
        "X-Sub2API-Satellite": "yibiao",
    }


def is_managed() -> bool:
    """Whether this process is running as a Sub2API-managed satellite.

    Standalone/local deployments do not have an SSO secret or application
    credential and should retain the original local API behaviour.  The
    Docker deployment supplies both values, which enables the session and
    per-user isolation middleware in ``server.app``.
    """
    # Presence of an integration variable opts into managed mode.  A readable
    # placeholder must not turn into an unauthenticated standalone fallback;
    # the actual ticket/credential validators will reject it instead.
    return bool(
        str(os.getenv("SUB2API_SSO_SECRET", "") or "").strip()
        or str(os.getenv("SUB2API_APP_CREDENTIAL", "") or "").strip()
    )


def relay_base_url() -> str:
    raw = (os.getenv("SUB2API_RELAY_BASE_URL") or os.getenv("LINK") or "").strip()
    if raw and "://" not in raw:
        raw = "http://" + raw
    raw = raw.rstrip("/")
    return raw if raw.endswith("/v1") else (raw + "/v1" if raw else "")


def is_same_origin(request: Request) -> bool:
    origin = request.headers.get("origin", "").strip()
    if not origin:
        return True
    parsed = urlsplit(origin)
    expected_scheme = request.headers.get("x-forwarded-proto", request.url.scheme).split(",", 1)[0].strip()
    expected_host = request.headers.get("x-forwarded-host", request.headers.get("host", "")).split(",", 1)[0].strip()
    return parsed.scheme == expected_scheme and parsed.netloc.lower() == expected_host.lower()


def _hash(value: str) -> str:
    return hashlib.sha256(value.encode()).hexdigest()
