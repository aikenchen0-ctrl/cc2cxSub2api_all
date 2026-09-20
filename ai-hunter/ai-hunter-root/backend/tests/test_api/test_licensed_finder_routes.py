"""API tests for the licensed-finder bypass (isolated app, no langgraph)."""

from unittest.mock import AsyncMock, patch

import pytest
from fastapi import FastAPI
from httpx import ASGITransport, AsyncClient

from api.licensed_finder_routes import router
from tools.licensed_finder import LicensedFinderError


def _settings(**kwargs):
    defaults = {
        "licensed_finder_enabled": True,
        "bettercontact_api_key": "bc-test",
        "licensed_finder_base_url": "https://app.bettercontact.rocks/api/v2",
        "licensed_finder_poll_seconds": 0.01,
        "licensed_finder_timeout_seconds": 5,
        "api_access_token": "",
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
async def test_status_never_spends(client):
    with patch("api.licensed_finder_routes.get_settings", return_value=_settings()):
        resp = await client.get("/api/v1/licensed-finder/status")
    assert resp.status_code == 200
    data = resp.json()
    assert data["in_graph"] is False
    assert data["enabled"] is True
    assert "next_action" in data


@pytest.mark.asyncio
async def test_search_disabled_conflict(client):
    with patch(
        "api.licensed_finder_routes.get_settings",
        return_value=_settings(licensed_finder_enabled=False),
    ):
        resp = await client.post(
            "/api/v1/licensed-finder/search",
            json={"industries": ["Automotive"], "max_leads": 3},
        )
    assert resp.status_code == 409
    assert resp.json()["detail"]["type"] == "disabled"


@pytest.mark.asyncio
async def test_search_returns_leads_json(client):
    payload = {
        "request_id": "abc",
        "credits_consumed": 0,
        "credits_left": 40,
        "leads_found": 1,
        "leads": [{
            "company_name": "Acme Tools",
            "website": "https://acme.com",
            "emails": [],
            "first_name": "Jane",
            "last_name": "Doe",
            "reason": "Purchasing manager at Acme Tools from a licensed contact source.",
            "lead_id": "lead_abc",
            "qualified_at": "2026-09-17T00:00:00Z",
        }],
        "raw_status": "terminated",
    }
    with (
        patch("api.licensed_finder_routes.get_settings", return_value=_settings()),
        patch("api.licensed_finder_routes.search_people", new=AsyncMock(return_value=payload)) as mocked,
    ):
        resp = await client.post(
            "/api/v1/licensed-finder/search",
            json={"industries": ["Industrial machinery"], "max_leads": 5, "enrich_emails": False},
        )
    assert resp.status_code == 200
    data = resp.json()
    assert data["credits_consumed"] == 0
    assert data["leads"][0]["company_name"] == "Acme Tools"
    assert mocked.await_args.kwargs["enrich_emails"] is False


@pytest.mark.asyncio
async def test_search_csv_export(client):
    payload = {
        "request_id": "abc",
        "credits_consumed": 0,
        "leads_found": 1,
        "leads": [{
            "company_name": "Acme Tools",
            "website": "https://acme.com",
            "emails": ["jane.doe@acme.com"],
            "first_name": "Jane",
            "last_name": "Doe",
            "title": "Buyer",
            "reason": "Buyer at Acme Tools from a licensed contact source.",
            "lead_id": "lead_abc",
            "qualified_at": "2026-09-17T00:00:00Z",
        }],
        "raw_status": "terminated",
    }
    with (
        patch("api.licensed_finder_routes.get_settings", return_value=_settings()),
        patch("api.licensed_finder_routes.search_people", new=AsyncMock(return_value=payload)),
    ):
        resp = await client.post(
            "/api/v1/licensed-finder/search?format=csv",
            json={"industries": ["Industrial machinery"]},
        )
    assert resp.status_code == 200
    assert "text/csv" in resp.headers["content-type"]
    text = resp.text
    assert text.splitlines()[0].startswith("email,first_name,last_name,company")
    assert "jane.doe@acme.com" in text
    assert "score" not in text.splitlines()[0]


@pytest.mark.asyncio
async def test_search_auth_error_mapped(client):
    with (
        patch("api.licensed_finder_routes.get_settings", return_value=_settings()),
        patch(
            "api.licensed_finder_routes.search_people",
            new=AsyncMock(side_effect=LicensedFinderError("provider_auth", "bad key")),
        ),
    ):
        resp = await client.post(
            "/api/v1/licensed-finder/search",
            json={"industries": ["Automotive"]},
        )
    assert resp.status_code == 401
    assert resp.json()["detail"]["type"] == "provider_auth"
