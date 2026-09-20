"""Local application sessions for the sub2api SSO handoff."""

from __future__ import annotations

import base64
import binascii
import hashlib
import hmac
import json
import os
import secrets
import sqlite3
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Any

from fastapi import HTTPException, Request, Response
from cryptography.hazmat.primitives.ciphers.aead import AESGCM

SESSION_COOKIE = "screen2code_session"
SECURE_SESSION_COOKIE = "__Host-screen2code_session"
CSRF_COOKIE = "screen2code_csrf"
SESSION_TTL_SECONDS = 3 * 24 * 60 * 60
TICKET_TTL_SKEW_SECONDS = 10


@dataclass(frozen=True)
class Identity:
    user_id: int
    subject: str
    email: str | None
    username: str | None
    display_name: str | None
    avatar_url: str | None
    relay_key_ciphertext: str | None = None

    def public(self) -> dict[str, Any]:
        return {
            "id": self.user_id,
            "subject": self.subject,
            "email": self.email,
            "username": self.username,
            "display_name": self.display_name,
            "avatar_url": self.avatar_url,
        }


def _db_path() -> Path:
    configured = os.environ.get("SCREEN2CODE_AUTH_DB", "")
    path = Path(configured) if configured else Path(__file__).parent / "data" / "auth.db"
    path.parent.mkdir(parents=True, exist_ok=True)
    return path


def _connect() -> sqlite3.Connection:
    conn = sqlite3.connect(_db_path(), timeout=5)
    conn.row_factory = sqlite3.Row
    conn.execute("PRAGMA journal_mode=WAL")
    conn.executescript(
        """
        CREATE TABLE IF NOT EXISTS identities (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            issuer TEXT NOT NULL,
            subject TEXT NOT NULL,
            email TEXT,
            username TEXT,
            display_name TEXT,
            avatar_url TEXT,
            created_at INTEGER NOT NULL,
            updated_at INTEGER NOT NULL,
            relay_key_ciphertext TEXT,
            UNIQUE (issuer, subject)
        );
        CREATE TABLE IF NOT EXISTS consumed_tickets (
            nonce TEXT PRIMARY KEY,
            expires_at INTEGER NOT NULL,
            consumed_at INTEGER NOT NULL
        );
        CREATE TABLE IF NOT EXISTS sessions (
            token_hash TEXT PRIMARY KEY,
            identity_id INTEGER NOT NULL REFERENCES identities(id),
            expires_at INTEGER NOT NULL,
            created_at INTEGER NOT NULL,
            csrf_token_hash TEXT
        );
        """
    )
    columns = {row[1] for row in conn.execute("PRAGMA table_info(identities)").fetchall()}
    if "relay_key_ciphertext" not in columns:
        conn.execute("ALTER TABLE identities ADD COLUMN relay_key_ciphertext TEXT")
    session_columns = {row[1] for row in conn.execute("PRAGMA table_info(sessions)").fetchall()}
    if "csrf_token_hash" not in session_columns:
        conn.execute("ALTER TABLE sessions ADD COLUMN csrf_token_hash TEXT")
    conn.commit()
    return conn


def _decode_part(value: str) -> bytes:
    return base64.urlsafe_b64decode(value + "=" * (-len(value) % 4))


def decrypt_relay_key(ciphertext: str | None) -> str | None:
    if not ciphertext:
        return None
    secret = os.environ.get("SUB2API_SSO_SECRET", "").strip()
    if len(secret) < 32:
        return None
    try:
        raw = _decode_part(ciphertext)
        if len(raw) < 12 + 16:
            return None
        key = hashlib.sha256(secret.encode()).digest()
        return AESGCM(key).decrypt(raw[:12], raw[12:], None).decode("utf-8")
    except (binascii.Error, UnicodeDecodeError, ValueError):
        return None


def verify_ticket(raw: str, expected_audience: str = "screen2code") -> dict[str, Any]:
    secret = os.environ.get("SUB2API_SSO_SECRET", "").strip()
    if len(secret) < 32:
        raise HTTPException(status_code=503, detail="SSO is not configured")
    try:
        encoded, signature = raw.split(".", 1)
        expected = hmac.new(secret.encode(), encoded.encode(), hashlib.sha256).digest()
        if not hmac.compare_digest(expected, _decode_part(signature)):
            raise ValueError("bad signature")
        payload = json.loads(_decode_part(encoded))
    except (ValueError, json.JSONDecodeError, UnicodeDecodeError, TypeError, binascii.Error) as exc:
        raise HTTPException(status_code=400, detail="Invalid SSO ticket") from exc
    if not isinstance(payload, dict):
        raise HTTPException(status_code=400, detail="Invalid SSO payload")
    now = int(time.time())
    if payload.get("iss") != "sub2api" or payload.get("aud") != expected_audience:
        raise HTTPException(status_code=400, detail="Invalid SSO audience")
    if not isinstance(payload.get("sub"), str) or not payload["sub"]:
        raise HTTPException(status_code=400, detail="Invalid SSO subject")
    if not isinstance(payload.get("jti"), str) or not payload["jti"]:
        raise HTTPException(status_code=400, detail="Invalid SSO nonce")
    try:
        issued_at = int(payload.get("iat", now))
        expires_at = int(payload.get("exp", 0))
    except (TypeError, ValueError) as exc:
        raise HTTPException(status_code=400, detail="Invalid SSO timestamps") from exc
    if expires_at < now - TICKET_TTL_SKEW_SECONDS:
        raise HTTPException(status_code=400, detail="SSO ticket expired")
    if issued_at > now + TICKET_TTL_SKEW_SECONDS or expires_at < issued_at:
        raise HTTPException(status_code=400, detail="Invalid SSO issue time")
    return payload


def _safe_next(value: str | None) -> str:
    value = (value or "/").strip()
    if not value.startswith("/") or value.startswith("//") or "\\" in value or len(value) > 2048 or any(ord(char) < 32 for char in value):
        return "/"
    return value


def consume_ticket(payload: dict[str, Any]) -> Identity:
    now = int(time.time())
    conn = _connect()
    try:
        conn.execute("DELETE FROM consumed_tickets WHERE expires_at < ?", (now,))
        try:
            conn.execute(
                "INSERT INTO consumed_tickets(nonce, expires_at, consumed_at) VALUES (?, ?, ?)",
                (payload["jti"], int(payload["exp"]), now),
            )
        except sqlite3.IntegrityError as exc:
            conn.rollback()
            raise HTTPException(status_code=400, detail="SSO ticket already used") from exc
        row = conn.execute(
            "SELECT * FROM identities WHERE issuer = ? AND subject = ?",
            (payload["iss"], payload["sub"]),
        ).fetchone()
        if row is None:
            conn.execute(
                "INSERT INTO identities(issuer, subject, email, username, display_name, avatar_url, relay_key_ciphertext, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
                (payload["iss"], payload["sub"], payload.get("email"), payload.get("username"), payload.get("displayName"), payload.get("avatarUrl"), payload.get("rk"), now, now),
            )
            identity_id = int(conn.execute("SELECT last_insert_rowid()").fetchone()[0])
        else:
            identity_id = int(row["id"])
            conn.execute(
                "UPDATE identities SET email = ?, username = ?, display_name = ?, avatar_url = ?, relay_key_ciphertext = COALESCE(?, relay_key_ciphertext), updated_at = ? WHERE id = ?",
                (payload.get("email"), payload.get("username"), payload.get("displayName"), payload.get("avatarUrl"), payload.get("rk"), now, identity_id),
            )
        conn.commit()
        identity = conn.execute("SELECT * FROM identities WHERE id = ?", (identity_id,)).fetchone()
        assert identity is not None
        return Identity(identity_id, identity["subject"], identity["email"], identity["username"], identity["display_name"], identity["avatar_url"], identity["relay_key_ciphertext"])
    finally:
        conn.close()


def create_session(identity: Identity, response: Response, request: Request) -> None:
    token = secrets.token_urlsafe(32)
    csrf_token = secrets.token_urlsafe(32)
    now = int(time.time())
    expires = now + SESSION_TTL_SECONDS
    conn = _connect()
    try:
        conn.execute("INSERT INTO sessions(token_hash, identity_id, expires_at, created_at, csrf_token_hash) VALUES (?, ?, ?, ?, ?)", (_hash_token(token), identity.user_id, expires, now, _hash_token(csrf_token)))
        conn.commit()
    finally:
        conn.close()
    forwarded_proto = request.headers.get("x-forwarded-proto", request.url.scheme).split(",", 1)[0].strip()
    secure = forwarded_proto == "https"
    response.set_cookie(SECURE_SESSION_COOKIE if secure else SESSION_COOKIE, token, max_age=SESSION_TTL_SECONDS, httponly=True, secure=secure, samesite="lax", path="/")
    response.set_cookie(CSRF_COOKIE, csrf_token, max_age=SESSION_TTL_SECONDS, httponly=False, secure=secure, samesite="lax", path="/")


def _session_token(request: Any) -> str | None:
    return request.cookies.get(SECURE_SESSION_COOKIE) or request.cookies.get(SESSION_COOKIE)


def get_identity(request: Any) -> Identity | None:
    token = _session_token(request)
    if not token:
        return None
    conn = _connect()
    try:
        row = conn.execute(
            "SELECT i.* FROM sessions s JOIN identities i ON i.id = s.identity_id WHERE s.token_hash = ? AND s.expires_at >= ?",
            (_hash_token(token), int(time.time())),
        ).fetchone()
        if row is None:
            return None
        return Identity(row["id"], row["subject"], row["email"], row["username"], row["display_name"], row["avatar_url"], row["relay_key_ciphertext"])
    finally:
        conn.close()


def require_identity(request: Request) -> Identity:
    identity = get_identity(request)
    if identity is None:
        raise HTTPException(status_code=401, detail="Authentication required")
    return identity


def logout(request: Request, response: Response) -> None:
    token = _session_token(request)
    if token:
        conn = _connect()
        try:
            conn.execute("DELETE FROM sessions WHERE token_hash = ?", (_hash_token(token),))
            conn.commit()
        finally:
            conn.close()
    response.delete_cookie(SESSION_COOKIE, path="/")
    response.delete_cookie(SECURE_SESSION_COOKIE, path="/")
    response.delete_cookie(CSRF_COOKIE, path="/")


def origin_is_allowed(request: Any) -> bool:
    origin = request.headers.get("origin", "").strip().rstrip("/")
    if not origin:
        return False
    from config import SCREEN2CODE_ALLOWED_ORIGINS, SCREEN2CODE_AUTH_REQUIRED

    configured_origins = SCREEN2CODE_ALLOWED_ORIGINS or (
        ("http://localhost:5173", "http://127.0.0.1:5173")
        if SCREEN2CODE_AUTH_REQUIRED
        else ()
    )
    if origin in configured_origins:
        return True
    forwarded_proto = request.headers.get("x-forwarded-proto", request.url.scheme).split(",", 1)[0].strip()
    forwarded_host = request.headers.get("x-forwarded-host", request.headers.get("host", "")).split(",", 1)[0].strip()
    return origin == f"{forwarded_proto}://{forwarded_host}".rstrip("/")


def require_csrf(request: Request) -> None:
    if not origin_is_allowed(request):
        raise HTTPException(status_code=403, detail="Invalid request origin")
    csrf_cookie = request.cookies.get(CSRF_COOKIE, "")
    csrf_header = request.headers.get("x-csrf-token", "")
    if not csrf_cookie or not csrf_header or not hmac.compare_digest(csrf_cookie, csrf_header):
        raise HTTPException(status_code=403, detail="CSRF validation failed")


def revoke_sessions_for_subject(subject: str) -> bool:
    conn = _connect()
    try:
        cursor = conn.execute(
            "DELETE FROM sessions WHERE identity_id IN (SELECT id FROM identities WHERE issuer = ? AND subject = ?)",
            ("sub2api", subject),
        )
        conn.commit()
        return cursor.rowcount >= 0
    finally:
        conn.close()


def _hash_token(token: str) -> str:
    return hashlib.sha256(token.encode()).hexdigest()
