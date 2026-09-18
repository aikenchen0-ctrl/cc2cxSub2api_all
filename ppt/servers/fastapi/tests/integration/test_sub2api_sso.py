import asyncio
import base64
import hashlib
import hmac
import json
import logging
import time
import uuid

import pytest
from sqlalchemy import select
from sqlalchemy.ext.asyncio import async_sessionmaker

from api.v1.auth.config import SESSION_COOKIE_NAME
from api.v1.auth.sub2api_sso import verify_ticket, safe_next
from models.sql.key_value import KeyValueSqlModel
from models.sql.user import User
from tests.integration.test_auth_endpoints import _build_client
from utils.sso_logging import SSOTicketLogFilter

SECRET = "integration-sso-secret-" * 2


def ticket(**overrides):
    now = int(time.time())
    payload = {"aud": "presenton", "sub": "7", "jti": str(uuid.uuid4()), "iat": now, "exp": now + 120, "next": "/upload", **overrides}
    body = base64.urlsafe_b64encode(json.dumps(payload).encode()).rstrip(b"=")
    signature = base64.urlsafe_b64encode(hmac.digest(SECRET.encode(), body, "sha256")).rstrip(b"=")
    return (body + b"." + signature).decode()


@pytest.fixture
def sso_client(monkeypatch, tmp_path):
    monkeypatch.setenv("USER_CONFIG_PATH", str(tmp_path / "userConfig.json"))
    monkeypatch.setenv("SUB2API_SSO_SECRET", SECRET)
    monkeypatch.delenv("DISABLE_AUTH", raising=False)
    client, engine = _build_client(tmp_path)
    async def create_table():
        async with engine.begin() as conn:
            await conn.run_sync(KeyValueSqlModel.__table__.create)
    asyncio.run(create_table())
    assert client.post("/api/v1/auth/setup", json={"username": "admin", "password": "secret123"}).status_code == 200
    client.cookies.clear()
    yield client, engine
    client.close()
    asyncio.run(engine.dispose())


def callback(client, raw):
    return client.get("/api/v1/auth/sso/callback", params={"ticket": raw}, follow_redirects=False)


def test_sso_creates_ordinary_account_and_reuses_identity(sso_client):
    client, engine = sso_client
    response = callback(client, ticket(username="admin"))
    assert response.status_code == 303
    assert response.headers["location"] == "/upload"
    assert "HttpOnly" in response.headers["set-cookie"]
    assert "SameSite=lax" in response.headers["set-cookie"]
    assert response.headers["cache-control"] == "no-store"
    assert response.headers["referrer-policy"] == "no-referrer"
    status = client.get("/api/v1/auth/status").json()
    assert status["authenticated"] and status["role"] == "user"
    assert status["username"] != "admin"
    assert client.get("/api/v1/admin/provider-settings").status_code == 403
    client.cookies.clear()
    assert callback(client, ticket()).headers["location"] == "/upload"
    assert client.get("/api/v1/auth/status").json()["user_id"] == status["user_id"]
    client.cookies.clear()
    callback(client, ticket(sub="8"))
    assert client.get("/api/v1/auth/status").json()["user_id"] != status["user_id"]


def test_ticket_cannot_be_replayed_after_new_session(sso_client):
    client, engine = sso_client
    raw = ticket()
    assert callback(client, raw).headers["location"] == "/upload"
    client.cookies.clear()
    asyncio.run(engine.dispose())
    response = callback(client, raw)
    assert response.headers["location"] == "/login?error=SSO_failed"
    assert SESSION_COOKIE_NAME not in response.cookies
    assert client.get("/api/v1/auth/status").json()["authenticated"] is False


@pytest.mark.parametrize("changes", [
    {"aud": "canvas"}, {"aud": None}, {"sub": ""}, {"sub": 7}, {"jti": ""},
    {"exp": 1}, {"exp": int(time.time()) + 300}, {"iat": int(time.time()) + 60},
    {"iat": True}, {"sub": "a\nb"},
])
def test_bad_tickets_do_not_authenticate(sso_client, changes):
    client, _ = sso_client
    response = callback(client, ticket(**changes))
    assert response.headers["location"] == "/login?error=SSO_failed"
    assert SESSION_COOKIE_NAME not in response.cookies


@pytest.mark.parametrize("raw", ["", ".", "not.signed", "a.b.c", "a" * 9000])
def test_malformed_ticket_fails_closed(sso_client, raw):
    client, _ = sso_client
    assert callback(client, raw).headers["location"] == "/login?error=SSO_failed"


@pytest.mark.parametrize("next_path", ["//evil.example", "https://evil.example", "/\\evil.example", "/a\nb", None])
def test_next_is_local(next_path):
    assert safe_next(next_path) == "/upload"


def test_missing_secret_disables_sso(sso_client, monkeypatch):
    client, _ = sso_client
    monkeypatch.delenv("SUB2API_SSO_SECRET")
    assert callback(client, ticket()).headers["location"] == "/login?error=SSO_failed"


def test_disabled_or_promoted_identity_is_rejected(sso_client):
    client, engine = sso_client
    callback(client, ticket())
    user_id = uuid.UUID(client.get("/api/v1/auth/status").json()["user_id"])
    async def update_user(active, admin):
        async with async_sessionmaker(engine, expire_on_commit=False)() as session:
            user = await session.get(User, user_id)
            user.is_active = active
            user.is_superuser = admin
            await session.commit()
    for active, admin in [(False, False), (True, True)]:
        asyncio.run(update_user(active, admin))
        client.cookies.clear()
        assert callback(client, ticket()).headers["location"] == "/login?error=SSO_failed"


def test_ticket_tampering_and_wrong_signer():
    raw = ticket()
    with pytest.raises(ValueError):
        verify_ticket(raw, "wrong-secret" * 4)
    body, signature = raw.split(".")
    with pytest.raises(ValueError):
        verify_ticket(body[:-1] + "A." + signature, SECRET)


def test_access_log_strips_ticket():
    record = logging.LogRecord("uvicorn.access", logging.INFO, "", 0, "%s - %s %s HTTP/%s %s", ("client", "GET", "/api/v1/auth/sso/callback?ticket=secret", "1.1", 303), None)
    assert SSOTicketLogFilter().filter(record)
    assert "secret" not in record.getMessage()
    assert "/api/v1/auth/sso/callback" in record.getMessage()
