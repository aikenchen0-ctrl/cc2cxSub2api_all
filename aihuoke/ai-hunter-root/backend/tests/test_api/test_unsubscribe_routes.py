"""API tests for public unsubscribe (isolated app, no langgraph)."""

from unittest.mock import patch

import pytest
from fastapi import FastAPI
from httpx import ASGITransport, AsyncClient

from api.unsubscribe_routes import router
from emailing.store import EmailStore
from emailing.suppression import unsubscribe_token


def _settings(tmp_path, **kwargs):
    defaults = {
        "email_db_path": str(tmp_path / "email.db"),
        "email_unsubscribe_secret": "test-secret",
        "email_public_base_url": "http://127.0.0.1:8000",
        "api_access_token": "",
        "api_host": "127.0.0.1",
        "api_port": 8000,
    }
    defaults.update(kwargs)
    return type("S", (), defaults)()


@pytest.fixture
def app():
    app = FastAPI()
    app.include_router(router)
    return app


@pytest.fixture
async def client(app):
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as c:
        yield c


@pytest.mark.asyncio
async def test_unsubscribe_rejects_bad_token(client, tmp_path):
    with patch("api.unsubscribe_routes.get_settings", return_value=_settings(tmp_path)):
        resp = await client.get(
            "/api/v1/unsubscribe",
            params={"email": "buyer@acme.com", "token": "nope"},
        )
    assert resp.status_code == 403


@pytest.mark.asyncio
async def test_unsubscribe_one_click_and_list(client, tmp_path):
    settings = _settings(tmp_path)
    token = unsubscribe_token("buyer@acme.com", secret="test-secret")
    with patch("api.unsubscribe_routes.get_settings", return_value=settings):
        resp = await client.get(
            "/api/v1/unsubscribe",
            params={"email": "buyer@acme.com", "token": token},
        )
        listed = await client.get("/api/v1/suppressions")
    assert resp.status_code == 200
    data = resp.json()
    assert data["ok"] is True
    assert data["email"] == "buyer@acme.com"
    assert data["suppressed"] is True
    assert listed.status_code == 200
    emails = [item["email"] for item in listed.json()["items"]]
    assert "buyer@acme.com" in emails
    store = EmailStore(settings.email_db_path)
    assert store.is_suppressed("buyer@acme.com") is True


@pytest.mark.asyncio
async def test_unsubscribe_one_click_form_post(client, tmp_path):
    settings = _settings(tmp_path)
    token = unsubscribe_token("buyer@acme.com", secret="test-secret")
    with patch("api.unsubscribe_routes.get_settings", return_value=settings):
        resp = await client.post(
            "/api/v1/unsubscribe",
            params={"email": "buyer@acme.com", "token": token},
            data={"List-Unsubscribe": "One-Click"},
        )
    assert resp.status_code == 200
    assert resp.json()["suppressed"] is True


@pytest.mark.asyncio
async def test_operator_suppression_add(client, tmp_path):
    settings = _settings(tmp_path)
    with patch("api.unsubscribe_routes.get_settings", return_value=settings):
        resp = await client.post(
            "/api/v1/suppressions",
            json={"email": "jane.doe@acme.com", "reason": "operator"},
        )
    assert resp.status_code == 200
    assert resp.json()["email"] == "jane.doe@acme.com"
    store = EmailStore(settings.email_db_path)
    record = store.get_suppression("jane.doe@acme.com")
    assert record is not None
    assert record["source"] == "operator"
