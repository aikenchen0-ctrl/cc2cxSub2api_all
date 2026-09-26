"""Sub2API SSO and three-day HttpOnly sessions for AI获客."""

from __future__ import annotations

import base64
import binascii
import hashlib
import hmac
import json
import math
import os
import secrets
import sqlite3
import time
from contextvars import ContextVar
from dataclasses import dataclass
from pathlib import Path
from typing import Any

import httpx
from fastapi import APIRouter, HTTPException, Request, Response
from fastapi.responses import RedirectResponse
from urllib.parse import urlsplit, urlunsplit

SESSION_COOKIE = "aihuoke_session"
SESSION_TTL_SECONDS = 3 * 24 * 60 * 60
_CURRENT_SUBJECT: ContextVar[str] = ContextVar("aihuoke_sub2api_subject", default="")
router = APIRouter(prefix="/api/auth", tags=["auth"])
balance_router = APIRouter(prefix="/api/sub2api", tags=["sub2api"])


def _load_local_env() -> None:
    """Load ``backend/.env`` for non-Docker launches without overriding env.

    Docker/systemd deployments inject their environment directly.  Reading
    the local file here keeps the SSO and relay variables consistent with the
    settings layer when the README's ``cp .env.example .env`` workflow is
    used, while preserving explicitly injected deployment values.
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


def has_configured_secret(value: object) -> bool:
    """Return whether a secret is real rather than an example placeholder.

    Keep the placeholder policy in one place so status endpoints do not claim
    that the checked-in ``.env.example`` values have enabled the relay.
    """
    return _has_configured_secret(value)


@dataclass(frozen=True)
class Identity:
    id: int
    subject: str
    username: str = ""
    display_name: str = ""


def _db_path() -> Path:
    raw = os.getenv("AIHUOKE_AUTH_DB", "").strip()
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


def _decode(value: str) -> bytes:
    return base64.urlsafe_b64decode(value + "=" * (-len(value) % 4))


def verify_ticket(raw: str, now: int | None = None) -> dict[str, Any]:
    secret = os.getenv("SUB2API_SSO_SECRET", "").strip()
    if not _has_configured_secret(secret) or len(secret) < 32:
        raise ValueError("SSO is not configured")
    if not isinstance(raw, str) or not raw or len(raw) > 8192:
        raise ValueError("invalid ticket")
    try:
        encoded, signature = raw.split(".", 1)
        expected = hmac.new(secret.encode(), encoded.encode(), hashlib.sha256).digest()
        if not hmac.compare_digest(expected, _decode(signature)):
            raise ValueError("invalid signature")
        payload = json.loads(_decode(encoded))
    except (ValueError, TypeError, UnicodeDecodeError, json.JSONDecodeError, binascii.Error) as exc:
        raise ValueError("invalid ticket") from exc
    if not isinstance(payload, dict) or payload.get("iss") != "sub2api" or payload.get("aud") != "aihuoke":
        raise ValueError("invalid audience")
    if any(key in payload for key in ("rk", "api_key", "super_key")):
        raise ValueError("credential material is not allowed")
    subject, jti = payload.get("sub"), payload.get("jti")
    if not isinstance(subject, str) or not subject.strip() or len(subject) > 160 or not isinstance(jti, str) or not jti.strip() or len(jti) > 200:
        raise ValueError("invalid identity")
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
            conn.execute("INSERT INTO identities(issuer, subject, username, display_name, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)", ("sub2api", payload["sub"], str(payload.get("username") or ""), str(payload.get("displayName") or ""), now, now))
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
    response.set_cookie(SESSION_COOKIE, token, max_age=SESSION_TTL_SECONDS, httponly=True, secure=forwarded == "https", samesite="lax", path="/")


def get_identity(request: Request) -> Identity | None:
    token = request.cookies.get(SESSION_COOKIE, "").strip()
    if not token:
        return None
    conn = _connect()
    try:
        row = conn.execute("SELECT i.* FROM sessions s JOIN identities i ON i.id = s.identity_id WHERE s.token_hash = ? AND s.expires_at >= ?", (_hash(token), int(time.time()))).fetchone()
        return None if row is None else Identity(int(row["id"]), str(row["subject"]), str(row["username"]), str(row["display_name"]))
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


def managed() -> bool:
    # Fail closed when a deployment explicitly supplied either integration
    # variable, even if it still contains the readable value from an example
    # file.  Treating that value as "local mode" would silently disable SSO
    # and expose the standalone, unauthenticated surface.
    return bool(
        str(os.getenv("SUB2API_SSO_SECRET", "") or "").strip()
        or str(os.getenv("SUB2API_APP_CREDENTIAL", "") or "").strip()
    )


def scoped_email_db_path(base_path: str, subject: str | None = None) -> str:
    """Return the email database owned by one Sub2API subject.

    Local mode deliberately keeps the historical single database path.  In a
    managed deployment every authenticated subject gets a separate SQLite
    file, so account credentials, suppression lists, campaigns and worker
    state cannot cross an identity boundary.  The subject is represented only
    by a one-way directory name and is never written to a browser-visible
    value or a filename.
    """
    raw_path = str(base_path or "email_automation.db").strip() or "email_automation.db"
    owner = str(subject if subject is not None else current_subject()).strip()
    if not managed() or not owner:
        return raw_path
    path = Path(raw_path)
    digest = hashlib.sha256(owner.encode("utf-8")).hexdigest()[:32]
    directory = path.parent / f"{path.stem}.users"
    return str(directory / f"{digest}{path.suffix or '.db'}")


def default_email_account_id(subject: str | None = None) -> str:
    """Return a collision-free default account id for a managed subject."""
    owner = str(subject if subject is not None else current_subject()).strip()
    if not managed() or not owner:
        return "default"
    return f"default-{hashlib.sha256(owner.encode('utf-8')).hexdigest()[:24]}"


def known_subjects() -> set[str]:
    """Read known SSO subjects for maintenance/public unsubscribe workers."""
    if not managed():
        return set()
    try:
        conn = _connect()
        try:
            rows = conn.execute("SELECT subject FROM identities WHERE issuer = 'sub2api'").fetchall()
        finally:
            conn.close()
        return {str(row["subject"] or "").strip() for row in rows if str(row["subject"] or "").strip()}
    except sqlite3.Error:
        return set()


def relay_base_url() -> str:
    raw = (os.getenv("SUB2API_RELAY_BASE_URL") or os.getenv("LINK") or "").strip()
    if raw and "://" not in raw:
        raw = "http://" + raw
    raw = raw.rstrip("/")
    return raw if raw.endswith("/v1") else (raw + "/v1" if raw else "")


def satellite_headers(*, required: bool = True) -> dict[str, str]:
    subject = current_subject()
    credential = os.getenv("SUB2API_APP_CREDENTIAL", "").strip()
    if required and (not subject or not _has_configured_secret(credential)):
        raise RuntimeError("Sub2API satellite credential and session subject are required")
    if not subject or not _has_configured_secret(credential):
        return {}
    return {"Authorization": f"Bearer {credential}", "X-Sub2API-On-Behalf-Of": subject, "X-Sub2API-Satellite": "aihuoke"}


def sub2api_purchase_url() -> str | None:
    raw = os.getenv("LINK", "").strip()
    if not raw:
        return None
    if "://" not in raw:
        raw = "http://" + raw
    try:
        parts = urlsplit(raw)
        if parts.scheme not in {"http", "https"} or not parts.hostname or parts.username or parts.password or parts.query or parts.fragment:
            return None
        return urlunsplit((parts.scheme, parts.netloc, "/purchase", "", ""))
    except ValueError:
        return None


@balance_router.get("/balance")
async def sub2api_balance(response: Response):
    response.headers["Cache-Control"] = "no-store"
    subject = current_subject().strip()
    if not managed() or not subject:
        raise HTTPException(status_code=401, detail="Authentication required")
    base = relay_base_url()
    if not base:
        raise HTTPException(status_code=503, detail="Sub2API is not configured")
    try:
        headers = satellite_headers()
    except RuntimeError as exc:
        raise HTTPException(status_code=503, detail="Sub2API satellite is not configured") from exc
    try:
        async with httpx.AsyncClient(timeout=10) as client:
            upstream = await client.get(f"{base}/sub2api/balance", headers=headers)
        if upstream.status_code >= 400:
            status = upstream.status_code if upstream.status_code in {401, 503} else 502
            raise HTTPException(status_code=status, detail="Balance is unavailable")
        data = upstream.json()
        balance = float(data["balance"])
        if not math.isfinite(balance):
            raise ValueError("invalid balance")
    except HTTPException:
        raise
    except (httpx.HTTPError, ValueError, KeyError, TypeError) as exc:
        raise HTTPException(status_code=502, detail="Balance is unavailable") from exc
    return {"balance": balance, "recharge_url": sub2api_purchase_url()}


@router.get("/sso/callback")
async def sso_callback(request: Request):
    try:
        payload = verify_ticket(request.query_params.get("ticket", ""))
        identity = consume_ticket(payload)
    except (ValueError, TypeError) as exc:
        if str(exc) == "SSO is not configured":
            raise HTTPException(status_code=503, detail="SSO is not configured") from exc
        return RedirectResponse("/?error=sso_failed", status_code=303, headers={"Cache-Control": "no-store", "Referrer-Policy": "no-referrer"})
    response = RedirectResponse(safe_next(payload.get("next")), status_code=303, headers={"Cache-Control": "no-store", "Referrer-Policy": "no-referrer"})
    create_session(identity, response, request)
    return response


@router.get("/me")
async def me(request: Request):
    identity = require_identity(request)
    return {"id": identity.id, "subject": identity.subject, "username": identity.username, "display_name": identity.display_name}


def _hash(value: str) -> str:
    return hashlib.sha256(value.encode()).hexdigest()
