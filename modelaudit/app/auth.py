from __future__ import annotations

import base64
import hashlib
import hmac
import json
import secrets
import time
from dataclasses import dataclass
from typing import Any
from urllib.parse import urlsplit

import aiosqlite
from fastapi import HTTPException, Request, Response

from . import db as database
from .config import Settings
from .sub2api import Sub2APIClient, Sub2APIError


SESSION_COOKIE = "modelaudit_session"
SESSION_TTL_SECONDS = 3 * 24 * 60 * 60
TICKET_TTL_SECONDS = 120
_ADMIN_CACHE: dict[str, tuple[float, bool]] = {}


@dataclass(frozen=True)
class AdminIdentity:
    sub: str
    email: str
    display_name: str


def _decode_base64url(value: str) -> bytes:
    padded = value + "=" * ((4 - len(value) % 4) % 4)
    return base64.urlsafe_b64decode(padded.encode("ascii"))


def verify_ticket(raw: str, secret: str, now: int | None = None) -> tuple[dict[str, Any], str] | None:
    if not secret or len(secret.encode("utf-8")) < 32 or len(raw) > 8192:
        return None
    parts = raw.split(".")
    if len(parts) != 2 or not parts[0] or not parts[1]:
        return None
    encoded, supplied_signature = parts
    expected_signature = base64.urlsafe_b64encode(
        hmac.new(secret.encode("utf-8"), encoded.encode("ascii"), hashlib.sha256).digest()
    ).rstrip(b"=").decode("ascii")
    if not hmac.compare_digest(supplied_signature, expected_signature):
        return None
    try:
        payload = json.loads(_decode_base64url(encoded))
    except (ValueError, UnicodeDecodeError, json.JSONDecodeError):
        return None
    if not isinstance(payload, dict):
        return None
    current = int(time.time()) if now is None else now
    sub = payload.get("sub")
    jti = payload.get("jti")
    issued_at = payload.get("iat")
    expires_at = payload.get("exp")
    if payload.get("iss") != "sub2api" or payload.get("aud") != "modelaudit":
        return None
    if not isinstance(sub, str) or not sub.strip() or not sub.isdecimal():
        return None
    if not isinstance(jti, str) or not jti.strip() or len(jti) > 256:
        return None
    if isinstance(issued_at, bool) or not isinstance(issued_at, (int, float)):
        return None
    if isinstance(expires_at, bool) or not isinstance(expires_at, (int, float)):
        return None
    if expires_at <= current or issued_at > current + 30 or expires_at - issued_at > TICKET_TTL_SECONDS:
        return None
    next_path = payload.get("next")
    if (
        not isinstance(next_path, str)
        or not next_path.startswith("/")
        or next_path.startswith("//")
        or "\\" in next_path
        or "\r" in next_path
        or "\n" in next_path
    ):
        next_path = "/"
    return payload, next_path


def _session_hash(token: str, secret: str) -> str:
    return hmac.new(secret.encode("utf-8"), token.encode("ascii"), hashlib.sha256).hexdigest()


def _cookie_options(settings: Settings) -> dict[str, Any]:
    return {
        "key": SESSION_COOKIE,
        "httponly": True,
        "secure": settings.cookie_secure,
        "samesite": "lax",
        "max_age": SESSION_TTL_SECONDS,
        "path": "/",
    }


async def exchange_ticket(
    *,
    request: Request,
    response: Response,
    db: aiosqlite.Connection,
    settings: Settings,
    sub2api: Sub2APIClient,
    ticket: str,
) -> str:
    verified = verify_ticket(ticket, settings.sso_secret)
    if verified is None:
        raise HTTPException(status_code=401, detail="invalid_sso_ticket")
    payload, next_path = verified
    sub = str(payload["sub"])
    jti = str(payload["jti"])
    now = int(time.time())

    await db.execute("DELETE FROM sso_tickets WHERE expires_at <= ?", (now,))
    try:
        await db.execute(
            "INSERT INTO sso_tickets(jti, expires_at) VALUES(?, ?)",
            (jti, int(payload["exp"])),
        )
    except aiosqlite.IntegrityError as exc:
        raise HTTPException(status_code=401, detail="sso_ticket_replayed") from exc

    try:
        profile = await sub2api.get_user(sub)
    except Sub2APIError as exc:
        await db.rollback()
        raise HTTPException(status_code=503, detail="admin_identity_check_unavailable") from exc
    if profile.get("role") != "admin" or profile.get("status", "active") != "active":
        await db.commit()
        raise HTTPException(status_code=403, detail="admin_access_required")

    display_name = str(profile.get("username") or profile.get("name") or profile.get("email") or sub)[:160]
    email = str(profile.get("email") or "")[:254]
    token = secrets.token_urlsafe(32)
    expires_at = now + SESSION_TTL_SECONDS
    await db.execute(
        "INSERT INTO sessions(token_hash, sub, email, display_name, expires_at, created_at) VALUES(?, ?, ?, ?, ?, ?)",
        (_session_hash(token, settings.session_secret), sub, email, display_name, expires_at, database.utc_now()),
    )
    await db.execute(
        "INSERT INTO users_seen(sub, email, display_name, role, last_seen_at) VALUES(?, ?, ?, 'admin', ?) "
        "ON CONFLICT(sub) DO UPDATE SET email=excluded.email, display_name=excluded.display_name, role='admin', last_seen_at=excluded.last_seen_at",
        (sub, email, display_name, database.utc_now()),
    )
    await db.commit()
    response.set_cookie(value=token, **_cookie_options(settings))
    response.headers["Cache-Control"] = "no-store"
    request.state.sso_next = next_path
    return next_path


async def require_admin(request: Request) -> AdminIdentity:
    settings: Settings = request.app.state.settings
    db: aiosqlite.Connection = request.app.state.db
    sub2api: Sub2APIClient = request.app.state.sub2api
    token = request.cookies.get(SESSION_COOKIE, "")
    if not token or len(token) > 256:
        raise HTTPException(status_code=401, detail="authentication_required")
    row = await database.fetch_one(
        "SELECT sub, email, display_name, expires_at FROM sessions WHERE token_hash=?",
        (_session_hash(token, settings.session_secret),),
    )
    now = int(time.time())
    if not row or int(row["expires_at"]) <= now:
        raise HTTPException(status_code=401, detail="session_expired")

    cached = _ADMIN_CACHE.get(row["sub"])
    is_admin = cached[1] if cached and cached[0] > time.monotonic() else False
    if not cached or cached[0] <= time.monotonic():
        try:
            profile = await sub2api.get_user(row["sub"])
        except Sub2APIError as exc:
            raise HTTPException(status_code=503, detail="admin_identity_check_unavailable") from exc
        is_admin = profile.get("role") == "admin" and profile.get("status", "active") == "active"
        _ADMIN_CACHE[row["sub"]] = (time.monotonic() + 30, is_admin)
        if not is_admin:
            await db.execute("DELETE FROM sessions WHERE token_hash=?", (_session_hash(token, settings.session_secret),))
            await db.commit()
    if not is_admin:
        raise HTTPException(status_code=403, detail="admin_access_required")
    return AdminIdentity(sub=row["sub"], email=row["email"], display_name=row["display_name"])


def require_same_origin(request: Request, settings: Settings) -> None:
    origin = request.headers.get("origin", "").strip().rstrip("/")
    if not origin:
        raise HTTPException(status_code=403, detail="origin_required")
    if settings.allowed_origins:
        allowed = settings.allowed_origins
    else:
        parsed = urlsplit(str(request.base_url))
        allowed = (f"{parsed.scheme}://{parsed.netloc}".rstrip("/"),)
    if origin not in allowed:
        raise HTTPException(status_code=403, detail="origin_forbidden")


async def clear_session(request: Request, response: Response) -> None:
    token = request.cookies.get(SESSION_COOKIE, "")
    settings: Settings = request.app.state.settings
    if token:
        db: aiosqlite.Connection = request.app.state.db
        await db.execute("DELETE FROM sessions WHERE token_hash=?", (_session_hash(token, settings.session_secret),))
        await db.commit()
    options = _cookie_options(settings)
    options["max_age"] = 0
    response.set_cookie(value="", **options)
    response.headers["Cache-Control"] = "no-store"
