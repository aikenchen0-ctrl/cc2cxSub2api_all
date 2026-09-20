"""Offline tests for the BetterContact licensed-finder adapter."""

import pytest

from tools.lead_contract import CSV_COLUMNS, leads_to_csv
from tools.licensed_finder import (
    LicensedFinderError,
    build_search_filters,
    email_status_is_usable,
    enrich_people,
    enrichment_payload_from_lead,
    filters_are_empty,
    include_exclude,
    licensed_row_to_lead,
    parse_submit_id,
    readiness,
    rows_from_search_result,
    search_people,
)


def _settings(**kwargs):
    defaults = {
        "licensed_finder_enabled": True,
        "bettercontact_api_key": "bc-test",
        "licensed_finder_base_url": "https://app.bettercontact.rocks/api/v2",
        "licensed_finder_poll_seconds": 0.01,
        "licensed_finder_timeout_seconds": 5,
    }
    defaults.update(kwargs)
    return type("S", (), defaults)()


def test_include_exclude_and_empty_filters():
    packed = include_exclude(["Automotive"], "Consumer")
    assert packed["include"] == ["Automotive"]
    assert packed["exclude"] == ["Consumer"]
    filters = build_search_filters(industries=["Automotive"], job_titles=["Purchasing Manager"])
    assert filters["company_industry"]["include"] == ["Automotive"]
    assert filters["contact_job_title"]["include"] == ["Purchasing Manager"]
    assert filters_are_empty({}) is True
    assert filters_are_empty(filters) is False


def test_licensed_row_to_lead_maps_contract():
    row = {
        "contact_first_name": "Jane",
        "contact_last_name": "Doe",
        "contact_job_title": "Purchasing Manager",
        "contact_linkedin_profile_url": "https://www.linkedin.com/in/jane-doe",
        "contact_email_address": "jane.doe@acme.com",
        "contact_email_address_status": "deliverable",
        "company_name": "Acme Tools",
        "company_domain": "acme.com",
        "company_industry": "Industrial machinery",
        "contact_location_country": "Germany",
    }
    lead = licensed_row_to_lead(row)
    assert lead["company_name"] == "Acme Tools"
    assert lead["website"] == "https://acme.com"
    assert lead["emails"] == ["jane.doe@acme.com"]
    assert lead["first_name"] == "Jane"
    assert lead["last_name"] == "Doe"
    assert lead["linkedin_url"].endswith("/jane-doe")
    assert "reason" in lead and lead["reason"]
    assert lead["lead_id"].startswith("lead_")
    csv_body = leads_to_csv([lead])
    assert csv_body.splitlines()[0] == ",".join(CSV_COLUMNS)
    assert "jane.doe@acme.com" in csv_body
    assert "score" not in csv_body.splitlines()[0]


def test_email_status_gate():
    assert email_status_is_usable("deliverable") is True
    assert email_status_is_usable("catch_all_safe") is True
    assert email_status_is_usable("catch_all_not_safe") is False
    assert email_status_is_usable("undeliverable") is False


def test_enrichment_payload_requires_identity():
    assert enrichment_payload_from_lead({"company_name": "Acme"}) is None
    row = enrichment_payload_from_lead({
        "first_name": "Jane",
        "last_name": "Doe",
        "website": "https://acme.com",
        "lead_id": "lead_abc",
    })
    assert row["first_name"] == "Jane"
    assert row["company_domain"] == "acme.com"
    assert row["custom_fields"]["lead_id"] == "lead_abc"


def test_parse_submit_id_kinds():
    assert parse_submit_id({"id": "enr-1"}, kind="enrich") == "enr-1"
    assert parse_submit_id({"request_id": "src-1"}, kind="search") == "src-1"
    with pytest.raises(LicensedFinderError) as exc:
        parse_submit_id({}, kind="search")
    assert exc.value.code == "bad_response"


def test_readiness_never_spends():
    off = readiness(_settings(licensed_finder_enabled=False, bettercontact_api_key=""))
    assert off["enabled"] is False
    assert off["in_graph"] is False
    assert off["blocked"] == "disabled"
    missing = readiness(_settings(bettercontact_api_key=""))
    assert missing["blocked"] == "no_credential"


def test_rows_from_search_result():
    payload = {"status": "terminated", "leads": [{"company_name": "Acme"}]}
    assert rows_from_search_result(payload)[0]["company_name"] == "Acme"


class _FakeResponse:
    def __init__(self, status_code, body, content=b"{}"):
        self.status_code = status_code
        self._body = body
        self.content = content if content is not None else b""
        self.text = ""

    def json(self):
        return self._body


class _FakeClient:
    def __init__(self, calls):
        self._calls = list(calls)
        self.seen = []

    async def request(self, method, url, headers=None, json=None):
        self.seen.append((method, url, json))
        if not self._calls:
            raise AssertionError(f"unexpected {method} {url}")
        status, body = self._calls.pop(0)
        content = b"" if body == {} and status == 202 else b"{}"
        return _FakeResponse(status, body, content)


@pytest.mark.asyncio
async def test_search_people_polls_until_terminated():
    client = _FakeClient([
        (202, {"success": True, "request_id": "abc123"}),
        (202, {"status": "processing"}),
        (200, {
            "status": "terminated",
            "credits_consumed": 0,
            "credits_left": 40,
            "summary": {"leads_found": 1},
            "leads": [{
                "contact_first_name": "Jane",
                "contact_last_name": "Doe",
                "contact_job_title": "Buyer",
                "company_name": "Acme Tools",
                "company_domain": "acme.com",
                "contact_linkedin_profile_url": "https://www.linkedin.com/in/jane-doe",
            }],
        }),
    ])

    async def no_sleep(_):
        return None

    out = await search_people(
        build_search_filters(industries=["Industrial machinery"]),
        settings=_settings(),
        max_leads=5,
        enrich_emails=False,
        client=client,
        sleep=no_sleep,
    )
    assert out["request_id"] == "abc123"
    assert out["credits_consumed"] == 0
    assert out["leads"][0]["company_name"] == "Acme Tools"
    assert out["leads"][0]["emails"] == []
    assert "Buyer" in out["leads"][0]["reason"]
    assert client.seen[0][1].endswith("/lead_finder/async")
    assert client.seen[0][2]["enrich_email_address"] is False


@pytest.mark.asyncio
async def test_search_people_disabled_does_not_call():
    client = _FakeClient([])
    with pytest.raises(LicensedFinderError) as exc:
        await search_people(
            build_search_filters(industries=["Automotive"]),
            settings=_settings(licensed_finder_enabled=False),
            client=client,
        )
    assert exc.value.code == "disabled"
    assert client.seen == []


@pytest.mark.asyncio
async def test_search_people_empty_filters_rejected():
    with pytest.raises(LicensedFinderError) as exc:
        await search_people({}, settings=_settings())
    assert exc.value.code == "bad_config"


@pytest.mark.asyncio
async def test_enrich_people_posts_async_and_maps():
    client = _FakeClient([
        (201, {"success": True, "id": "enr-9"}),
        (200, {
            "status": "terminated",
            "credits_consumed": 1,
            "data": [{
                "contact_first_name": "Jane",
                "contact_last_name": "Doe",
                "contact_email_address": "jane.doe@acme.com",
                "contact_email_address_status": "deliverable",
                "company_name": "Acme Tools",
                "company_domain": "acme.com",
            }],
        }),
    ])

    async def no_sleep(_):
        return None

    out = await enrich_people(
        [{"first_name": "Jane", "last_name": "Doe", "website": "https://acme.com"}],
        settings=_settings(),
        client=client,
        sleep=no_sleep,
    )
    assert out["credits_consumed"] == 1
    assert out["leads"][0]["emails"] == ["jane.doe@acme.com"]
    assert client.seen[0][1].endswith("/async")


@pytest.mark.asyncio
async def test_search_people_auth_error():
    client = _FakeClient([(401, {"error": "Unauthorized"})])
    with pytest.raises(LicensedFinderError) as exc:
        await search_people(
            build_search_filters(industries=["Automotive"]),
            settings=_settings(),
            client=client,
        )
    assert exc.value.code == "provider_auth"
