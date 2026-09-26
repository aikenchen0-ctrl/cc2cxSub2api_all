from __future__ import annotations

import json
from dataclasses import replace
from types import SimpleNamespace

import pytest

from modelaudit.app import db as database
from modelaudit.app.config import load_settings
from modelaudit.app.main import create_app
from modelaudit.app.options import defaults, get_options, merge_options
from modelaudit.app.pricing import PRICE_TABLE_COMMIT, resolve_group_pricing
from modelaudit.app.probes.cache import run_cache
from modelaudit.app.probes.common import (
    AccountSnapshot,
    ProbeCall,
    ProbeContext,
    account_snapshot,
    account_metadata_user_id,
    correlated_chat,
    probe_user_agent,
    result,
    stable_account_session_id,
)
from modelaudit.app.probes import modeltrace as modeltrace_probe
from modelaudit.app.probes import trace_calibration
from modelaudit.app.probes.multiplier import _expected_cost, run_multiplier
from modelaudit.app.probes.token_delta import _input_usage, run_token_delta
from modelaudit.app.probes.usage import normalize_usage, supplier_money_counter, supplier_token_delta
from modelaudit.app.report import build_report
from modelaudit.app.scheduler import (
    ScanCoordinator,
    _active_schedulable_group_counts,
    _exclusive_group_ids,
    _profile,
)
from modelaudit.app.storage import save_account, save_probe_calls, save_probe_result
from modelaudit.app.sub2api import Sub2APIClient, Sub2APIError
from modelaudit.tools.evaluate_modeltrace_thresholds import environment_disjoint_folds


def make_settings(monkeypatch: pytest.MonkeyPatch, tmp_path, profiles: dict | None = None):
    monkeypatch.setenv("LINK", "http://sub2api.test")
    monkeypatch.setenv("SUB2API_ADMIN_API_KEY", "admin-server-secret")
    monkeypatch.setenv("SUB2API_GATEWAY_API_KEYS", '{"7":"ordinary-user-key"}')
    monkeypatch.setenv("MODELAUDIT_GROUP_PROFILES", json.dumps(profiles or {
        "7": {
            "model": "gpt-4o",
            "expected_model": "gpt-4o",
            "cache_min_prefix_tokens": 128,
            "cache_protocol": "openai",
            "pricing": {
                "input_per_million": 1,
                "output_per_million": 2,
                "cache_read_per_million": 0.25,
                "cache_write_per_million": 1.1,
            },
            "gateway_cost_multiplier": 1,
            "expected_provider_cost_multiplier": 1,
        }
    }))
    monkeypatch.setenv("MODELAUDIT_DB_PATH", str(tmp_path / "modelaudit.sqlite3"))
    monkeypatch.setenv("MODELAUDIT_SCHEDULE_ENABLED", "false")
    monkeypatch.setenv("MODELAUDIT_COOKIE_SECURE", "false")
    return load_settings()


def test_auto_stop_starts_with_no_selected_rules(monkeypatch, tmp_path):
    settings = make_settings(monkeypatch, tmp_path)
    options = defaults(settings)
    assert options["auto_stop_enabled"] is False
    assert options["stop_rules"] == []
    assert merge_options(options, {"auto_stop_enabled": True})["stop_rules"] == []


def test_account_snapshot_applies_sub2api_default_rate_multiplier():
    base = {"id": 41, "status": "active", "schedulable": True}
    assert account_snapshot(base).rate_multiplier == pytest.approx(1.0)
    assert account_snapshot({**base, "rate_multiplier": None}).rate_multiplier == pytest.approx(1.0)
    assert account_snapshot({**base, "rate_multiplier": 0.25}).rate_multiplier == pytest.approx(0.25)
    assert account_snapshot({**base, "rate_multiplier": -1}).rate_multiplier is None


def test_account_snapshot_preserves_unlimited_concurrency():
    account = account_snapshot({"id": 41, "concurrency": 0, "current_concurrency": 3})
    assert account.concurrency == 0
    assert account.current_concurrency == 3
    assert account_snapshot({"id": 42, "concurrency": -1}).concurrency == 1


def test_litellm_price_snapshot_is_pinned_and_manual_overrides_win():
    rates, source, price_model, table_status = resolve_group_pricing("gpt-4o", "", {})
    assert source == "litellm_snapshot"
    assert price_model == "gpt-4o"
    assert table_status == "verified"
    assert rates["input_per_million"] == pytest.approx(2.5)
    assert rates["output_per_million"] == pytest.approx(10.0)
    assert rates["cache_read_per_million"] == pytest.approx(1.25)
    assert rates["cache_write_per_million"] == pytest.approx(2.5)
    assert len(PRICE_TABLE_COMMIT) == 40

    alias_rates, alias_source, alias_model, alias_status = resolve_group_pricing(
        "public-chat-alias", "gpt-4o", {"input_per_million": 3.0}
    )
    assert alias_source == "litellm_snapshot_with_manual_overrides"
    assert alias_model == "gpt-4o"
    assert alias_status == "verified"
    assert alias_rates["input_per_million"] == pytest.approx(3.0)
    assert alias_rates["output_per_million"] == pytest.approx(10.0)

    missing_rates, missing_source, missing_model, missing_status = resolve_group_pricing(
        "provider-private-alias", "", {}
    )
    assert missing_rates == {}
    assert missing_source == "unavailable"
    assert missing_model == "provider-private-alias"
    assert missing_status == "model_not_found"


@pytest.mark.asyncio
async def test_auto_stop_migrates_legacy_implicitly_selected_rules(monkeypatch, tmp_path):
    settings = make_settings(monkeypatch, tmp_path)
    await database.initialize(settings.db_path)
    db = await database.connect(settings.db_path)
    legacy = {
        "interval_minutes": 60,
        "scheduler_enabled": False,
        "auto_stop_enabled": True,
        "stop_rules": ["modeltrace_red", "token_delta_red", "cache_red", "multiplier_red"],
    }
    await db.execute(
        "INSERT INTO settings(key, value_json, updated_at) VALUES('runtime_options', ?, ?)",
        (database.json_dump(legacy), database.utc_now()),
    )
    await db.commit()
    try:
        value = await get_options(db, settings)
        stored = await database.fetch_one(db, "SELECT value_json FROM settings WHERE key='runtime_options'")
    finally:
        await db.close()

    assert value["auto_stop_enabled"] is True
    assert value["stop_rules"] == []
    assert value["options_version"] == 3
    assert json.loads(stored["value_json"])["stop_rules"] == []


def test_only_confirmed_modeltrace_is_allowed_to_trigger_auto_stop(monkeypatch, tmp_path):
    from fastapi import HTTPException

    settings = make_settings(monkeypatch, tmp_path)
    options = defaults(settings)
    assert options["stop_rules"] == []
    assert merge_options(options, {"stop_rules": ["modeltrace_red"]})["stop_rules"] == ["modeltrace_red"]
    with pytest.raises(HTTPException) as caught:
        merge_options(options, {"stop_rules": ["token_delta_red"]})
    assert caught.value.detail == "stop_rules_invalid"


@pytest.mark.asyncio
async def test_multiple_stop_alerts_submit_only_one_schedulable_update(monkeypatch, tmp_path):
    settings = make_settings(monkeypatch, tmp_path)
    db_path = tmp_path / "modelaudit.sqlite3"
    await database.initialize(db_path)
    db = await database.connect(db_path)

    class StopClient:
        def __init__(self):
            self.calls: list[tuple[int, bool]] = []

        async def set_schedulable(self, account_id: int, schedulable: bool):
            self.calls.append((account_id, schedulable))

    stop_client = StopClient()
    run_id = "run-stop-deduplication"
    await db.execute(
        "INSERT INTO probe_runs(id, trigger, status, started_at) VALUES(?, 'manual', 'running', ?)",
        (run_id, database.utc_now()),
    )
    await db.commit()
    coordinator = ScanCoordinator(db, settings, stop_client)
    context = make_context(settings, _profile(settings, 7), FakeGateway([]))
    outcomes = [
        result("modeltrace", "alert", "trace confirmed red", severity="red", metrics={"auto_stop_eligible": True}),
        result("modeltrace", "alert", "duplicate trace red", severity="red", metrics={"auto_stop_eligible": True}),
    ]

    try:
        await coordinator._evaluate_alerts(
            run_id,
            context.account,
            outcomes,
            {
                "auto_stop_enabled": True,
                "stop_rules": ["modeltrace_red"],
            },
        )
        alerts = await database.fetch_all(db, "SELECT action_taken FROM alerts WHERE run_id=?", (run_id,))
        audit_rows = await database.fetch_all(db, "SELECT result FROM audit_actions WHERE account_id=?", (context.account.account_id,))
    finally:
        await db.close()

    assert stop_client.calls == [(context.account.account_id, False)]
    assert sorted(row["action_taken"] for row in alerts) == [
        "auto_stop_already_attempted_for_run",
        "schedulable_set_false",
    ]
    assert len(audit_rows) == 1
    assert audit_rows[0]["result"] == "success"


@pytest.mark.asyncio
async def test_modeltrace_stop_requires_independent_confirmation(monkeypatch, tmp_path):
    settings = make_settings(monkeypatch, tmp_path)
    db_path = tmp_path / "modelaudit.sqlite3"
    await database.initialize(db_path)
    db = await database.connect(db_path)

    class StopClient:
        def __init__(self):
            self.calls: list[tuple[int, bool]] = []

        async def set_schedulable(self, account_id: int, schedulable: bool):
            self.calls.append((account_id, schedulable))

    stop_client = StopClient()
    context = make_context(settings, _profile(settings, 7), FakeGateway([]))
    coordinator = ScanCoordinator(db, settings, stop_client)
    options = {"auto_stop_enabled": True, "stop_rules": ["modeltrace_red"]}

    try:
        for run_id in ("run-unconfirmed-trace", "run-confirmed-trace"):
            await db.execute(
                "INSERT INTO probe_runs(id, trigger, status, started_at) VALUES(?, 'manual', 'running', ?)",
                (run_id, database.utc_now()),
            )
        await db.commit()
        await coordinator._evaluate_alerts(
            "run-unconfirmed-trace",
            context.account,
            [result("modeltrace", "alert", "mismatch", severity="red", metrics={"auto_stop_eligible": False})],
            options,
        )
        assert stop_client.calls == []

        await coordinator._evaluate_alerts(
            "run-confirmed-trace",
            context.account,
            [result("modeltrace", "alert", "confirmed mismatch", severity="red", metrics={"auto_stop_eligible": True})],
            options,
        )
        rows = await database.fetch_all(db, "SELECT action_taken FROM alerts WHERE account_id=? ORDER BY run_id", (context.account.account_id,))
    finally:
        await db.close()

    assert stop_client.calls == [(context.account.account_id, False)]
    assert {row["action_taken"] for row in rows} == {"", "schedulable_set_false"}


@pytest.mark.asyncio
async def test_only_unknown_fingerprint_mismatch_creates_warning(monkeypatch, tmp_path):
    settings = make_settings(monkeypatch, tmp_path)
    db_path = tmp_path / "modelaudit.sqlite3"
    await database.initialize(db_path)
    db = await database.connect(db_path)

    class NoopClient:
        def __init__(self):
            self.calls: list[tuple[int, bool]] = []

        async def set_schedulable(self, account_id: int, schedulable: bool):
            self.calls.append((account_id, schedulable))

    client = NoopClient()
    context = make_context(settings, _profile(settings, 7), FakeGateway([]))
    coordinator = ScanCoordinator(db, settings, client)
    run_id = "run-unknown-warning-filter"
    await db.execute(
        "INSERT INTO probe_runs(id, trigger, status, started_at) VALUES(?, 'manual', 'running', ?)",
        (run_id, database.utc_now()),
    )
    await db.commit()

    try:
        await coordinator._evaluate_alerts(
            run_id,
            context.account,
            [
                result(
                    "modeltrace",
                    "unknown",
                    "期望吻合但证据不足",
                    severity="none",
                    metrics={"closed_set_candidate_differs_from_expected": False},
                ),
                result(
                    "modeltrace",
                    "unknown",
                    "偏离期望但候选名隐藏",
                    severity="yellow",
                    metrics={"closed_set_candidate_differs_from_expected": True},
                ),
            ],
            {"auto_stop_enabled": True, "stop_rules": ["modeltrace_red"]},
        )
        alerts = await database.fetch_all(
            db,
            "SELECT status, severity, message FROM alerts WHERE run_id=?",
            (run_id,),
        )
    finally:
        await db.close()

    assert client.calls == []
    assert len(alerts) == 1
    assert alerts[0]["status"] == "warning"
    assert alerts[0]["severity"] == "yellow"
    assert alerts[0]["message"] == "偏离期望但候选名隐藏"


class FakeGateway:
    def __init__(self, payloads: list[dict]):
        self.payloads = list(payloads)
        self.bodies: list[bytes] = []
        self.sessions: dict[str, str] = {}
        self.metadata_user_ids: dict[str, str] = {}
        self.user_agents: dict[str, str | None] = {}
        self.messages: dict[str, list[dict]] = {}
        self.routes: dict[str, str] = {}

    async def _gateway_request(self, route, *, key, model, session_id, request_id, metadata_user_id, user_agent, messages, max_tokens, extra_body=None, serialized_body=None):
        self.sessions[request_id] = session_id
        self.metadata_user_ids[request_id] = metadata_user_id
        self.user_agents[request_id] = user_agent
        self.messages[request_id] = messages
        self.routes[request_id] = route
        if serialized_body is not None:
            self.bodies.append(serialized_body)
        payload = self.payloads.pop(0)
        return 200, payload, ""

    async def gateway_chat(self, **kwargs):
        return await self._gateway_request("chat/completions", **kwargs)

    async def gateway_messages(self, **kwargs):
        return await self._gateway_request("messages", **kwargs)

def make_context(settings, profile, fake, calls=None):
    account = AccountSnapshot(
        account_id=41,
        name="provider",
        platform="openai",
        account_type="oauth",
        status="active",
        schedulable=True,
        concurrency=2,
        current_concurrency=0,
        rate_multiplier=1.0,
        group_ids=(7,),
    )
    return ProbeContext(
        account=account,
        profile=profile,
        group_id=7,
        session_id="stable-session-for-test",
        api_key="ordinary-user-key",
        run_id="run-test",
        settings=settings,
        sub2api=fake,
        exclusive_group=True,
        calls=calls if calls is not None else [],
    )


def test_account_session_id_is_stable_unique_and_secret_bound():
    first = stable_account_session_id("a" * 32, 41)
    assert first == stable_account_session_id("a" * 32, 41)
    assert len(first) == 36
    assert first != stable_account_session_id("a" * 32, 42)
    assert first != stable_account_session_id("b" * 32, 41)


def test_metadata_user_id_is_valid_stable_and_does_not_expose_account_id():
    session_id = stable_account_session_id("a" * 32, 41)
    raw = account_metadata_user_id("a" * 32, 41, session_id)
    value = json.loads(raw)
    assert value["session_id"] == session_id
    assert len(value["device_id"]) == 64
    assert "account_id" not in value
    assert raw == account_metadata_user_id("a" * 32, 41, session_id)


def test_only_exclusive_schedulable_groups_can_be_used_to_pin_an_account():
    accounts = [
        {"id": 41, "status": "active", "schedulable": True, "group_ids": [7, 8]},
        {"id": 42, "status": "active", "schedulable": True, "group_ids": [7]},
        {"id": 43, "status": "active", "schedulable": False, "group_ids": [9]},
        {"id": 44, "status": "disabled", "schedulable": True, "group_ids": [10]},
    ]
    counts = _active_schedulable_group_counts(accounts)
    account = AccountSnapshot(41, "provider", "openai", "oauth", "active", True, 1, 0, 1.0, (7, 8))

    assert counts == {7: 2, 8: 1}
    assert _exclusive_group_ids(account, counts) == (8,)


@pytest.mark.asyncio
async def test_shared_group_is_reported_unpinned_without_sending_a_probe(monkeypatch, tmp_path):
    settings = make_settings(monkeypatch, tmp_path)
    await database.initialize(settings.db_path)
    db = await database.connect(settings.db_path)

    class AccountLookupOnly:
        gateway_calls = 0

        async def get_account(self, account_id):
            return {
                "id": account_id,
                "name": "provider",
                "platform": "openai",
                "type": "oauth",
                "status": "active",
                "schedulable": True,
                "group_ids": [7],
                "concurrency": 1,
            }

        async def gateway_chat(self, **kwargs):
            self.gateway_calls += 1
            raise AssertionError("shared group must be rejected before probing")

    sub2api = AccountLookupOnly()
    try:
        await db.execute(
            "INSERT INTO probe_runs(id, trigger, status, started_at) VALUES('run-shared', 'manual', 'running', ?)",
            (database.utc_now(),),
        )
        await db.commit()
        coordinator = ScanCoordinator(db, settings, sub2api)
        result_count, error_count = await coordinator._scan_account(
            "run-shared",
            {"id": 41, "status": "active", "schedulable": True, "group_ids": [7]},
            {7: 2},
        )
        rows = await database.fetch_all(db, "SELECT probe, status FROM probe_results WHERE run_id='run-shared'")
        assert result_count == 4
        assert error_count == 0
        assert {row["status"] for row in rows} == {"unpinned"}
        assert len(rows) == 4
        assert sub2api.gateway_calls == 0
    finally:
        await db.close()


@pytest.mark.asyncio
async def test_manual_scan_writes_separate_results_and_report_with_fake_gateway(monkeypatch, tmp_path):
    settings = make_settings(monkeypatch, tmp_path)
    await database.initialize(settings.db_path)
    db = await database.connect(settings.db_path)
    account = {
        "id": 41,
        "name": "provider",
        "platform": "openai",
        "type": "oauth",
        "status": "active",
        "schedulable": True,
        "concurrency": 2,
        "current_concurrency": 0,
        "rate_multiplier": 1.0,
        "group_ids": [7],
    }

    class FakeSub2API:
        def __init__(self):
            self.gateway_calls = []
            self.stop_calls = []
            self.responses = [
                {"choices": [{"message": {"content": "hello"}}], "usage": {"prompt_tokens": 6, "completion_tokens": 1}},
                {"choices": [{"message": {"content": "hi"}}], "usage": {"prompt_tokens": 6, "completion_tokens": 1}},
                {"choices": [{"message": {"content": "cached"}}], "usage": {"prompt_tokens": 300, "completion_tokens": 2, "prompt_tokens_details": {"cached_tokens": 0}}},
                {"choices": [{"message": {"content": "cached"}}], "usage": {"prompt_tokens": 300, "completion_tokens": 2, "prompt_tokens_details": {"cached_tokens": 220}}},
            ]

        async def list_active_accounts(self):
            return [account]

        async def get_account(self, account_id):
            assert account_id == 41
            return dict(account)

        async def get_account_usage(self, account_id):
            assert account_id == 41
            return {"provider_usage": {}}

        async def gateway_chat(self, **kwargs):
            self.gateway_calls.append(kwargs)
            return 200, self.responses.pop(0), ""

        async def set_schedulable(self, account_id: int, schedulable: bool):
            self.stop_calls.append((account_id, schedulable))

    sub2api = FakeSub2API()

    async def successful_modeltrace(_context):
        return result(
            "modeltrace",
            "ok",
            "mocked offline fingerprint result",
            severity="normal",
            model="gpt-4o",
            expected_model="gpt-4o",
            metrics={
                "estimated_modeltrace_cost_usd": 0.000012,
                "estimated_modeltrace_cost_status": "estimated",
                "estimated_modeltrace_challenges": 3,
            },
        )

    monkeypatch.setattr("modelaudit.app.scheduler.run_modeltrace_with_confirmation", successful_modeltrace)
    coordinator = ScanCoordinator(db, settings, sub2api)
    try:
        run = await coordinator.run_once("manual")
        report = await build_report(db, run["id"])
    finally:
        await db.close()

    assert run["status"] == "completed"
    assert run["account_count"] == 1
    assert run["result_count"] == 4
    assert len(sub2api.gateway_calls) == 4
    assert len({call["session_id"] for call in sub2api.gateway_calls}) == 1
    assert all(
        all(message.get("role") != "system" for message in call["messages"])
        for call in sub2api.gateway_calls
    )
    assert sub2api.stop_calls == []
    assert report is not None
    checks = report["accounts"][0]["checks"]
    assert set(checks) == {"modeltrace", "token_delta", "cache", "multiplier"}
    assert checks["modeltrace"]["metrics"]["estimated_modeltrace_cost_usd"] == pytest.approx(0.000012)
    assert checks["modeltrace"]["metrics"]["estimated_modeltrace_challenges"] == 3
    assert checks["cache"]["metrics"]["cache_check"]["second_cache_read_tokens"] == 220
    assert checks["multiplier"]["status"] == "inconclusive"
    serialized_report = json.dumps(report)
    assert "ordinary-user-key" not in serialized_report
    assert "secret" not in serialized_report.lower()
    report_path = settings.db_path.parent / "reports" / f"{run['id']}.json"
    assert report_path.is_file()


@pytest.mark.asyncio
async def test_gateway_chat_emits_metadata_and_headers_on_regular_json_request(monkeypatch, tmp_path):
    import httpx

    monkeypatch.setenv("MODELAUDIT_SESSION_SECRET", "test-session-secret-with-at-least-32-bytes")
    settings = make_settings(monkeypatch, tmp_path)
    client = Sub2APIClient(settings)
    captured: dict[str, object] = {}

    def handle(request: httpx.Request) -> httpx.Response:
        captured["request"] = request
        captured["body"] = json.loads(request.content)
        return httpx.Response(200, json={"usage": {"prompt_tokens": 5, "completion_tokens": 1}})

    await client.gateway.aclose()
    client.gateway = httpx.AsyncClient(
        base_url=settings.gateway_base_url,
        headers={"Accept": "application/json", "Content-Type": "application/json"},
        follow_redirects=False,
        transport=httpx.MockTransport(handle),
    )
    session_id = stable_account_session_id(settings.session_secret, 41)
    metadata_user_id = account_metadata_user_id(settings.session_secret, 41, session_id)
    try:
        status, payload, error = await client.gateway_chat(
            key="ordinary-user-key",
            model="claude-sonnet-4",
            session_id=session_id,
            request_id="probe-request-id",
            metadata_user_id=metadata_user_id,
            user_agent="claude-cli/2.1.161 (modelaudit probe)",
            messages=[{"role": "user", "content": "Say hi"}],
            max_tokens=8,
        )
    finally:
        await client.close()

    request = captured["request"]
    body = captured["body"]
    assert isinstance(request, httpx.Request)
    assert isinstance(body, dict)
    assert request.url.path == "/v1/chat/completions"
    assert request.headers["Authorization"] == "Bearer ordinary-user-key"
    assert request.headers["X-Session-Id"] == session_id
    assert request.headers["X-Request-ID"] == "probe-request-id"
    assert request.headers["User-Agent"] == "claude-cli/2.1.161 (modelaudit probe)"
    assert body["metadata"] == {"user_id": metadata_user_id}
    assert body["messages"] == [{"role": "user", "content": "Say hi"}]
    assert status == 200
    assert payload == {"usage": {"prompt_tokens": 5, "completion_tokens": 1}}
    assert error == ""


@pytest.mark.asyncio
async def test_gateway_messages_keeps_anthropic_probe_body_without_system_prompt(monkeypatch, tmp_path):
    import httpx

    monkeypatch.setenv("MODELAUDIT_SESSION_SECRET", "test-session-secret-with-at-least-32-bytes")
    settings = make_settings(monkeypatch, tmp_path)
    client = Sub2APIClient(settings)
    captured: dict[str, object] = {}

    def handle(request: httpx.Request) -> httpx.Response:
        captured["request"] = request
        captured["body"] = json.loads(request.content)
        return httpx.Response(
            200,
            json={"content": [{"type": "text", "text": "ok"}], "usage": {"input_tokens": 5, "output_tokens": 1}},
        )

    await client.gateway.aclose()
    client.gateway = httpx.AsyncClient(
        base_url=settings.gateway_base_url,
        headers={"Accept": "application/json", "Content-Type": "application/json"},
        follow_redirects=False,
        transport=httpx.MockTransport(handle),
    )
    session_id = stable_account_session_id(settings.session_secret, 41)
    metadata_user_id = account_metadata_user_id(settings.session_secret, 41, session_id)
    try:
        status, payload, error = await client.gateway_messages(
            key="ordinary-user-key",
            model="claude-sonnet-4-5",
            session_id=session_id,
            request_id="probe-messages-request-id",
            metadata_user_id=metadata_user_id,
            user_agent="claude-cli/2.1.161 (modelaudit probe)",
            messages=[{"role": "user", "content": "Say hi"}],
            max_tokens=8,
        )
    finally:
        await client.close()

    request = captured["request"]
    body = captured["body"]
    assert isinstance(request, httpx.Request)
    assert isinstance(body, dict)
    assert request.url.path == "/v1/messages"
    assert request.headers["Authorization"] == "Bearer ordinary-user-key"
    assert request.headers["X-Session-Id"] == session_id
    assert request.headers["X-Request-ID"] == "probe-messages-request-id"
    assert request.headers["anthropic-version"] == "2023-06-01"
    assert request.headers["User-Agent"] == "claude-cli/2.1.161 (modelaudit probe)"
    assert body["metadata"] == {"user_id": metadata_user_id}
    assert body["messages"] == [{"role": "user", "content": "Say hi"}]
    assert "system" not in body
    assert status == 200
    assert payload == {"content": [{"type": "text", "text": "ok"}], "usage": {"input_tokens": 5, "output_tokens": 1}}
    assert error == ""


@pytest.mark.asyncio
async def test_scan_admin_reads_stay_within_account_endpoints(monkeypatch, tmp_path):
    import httpx

    settings = make_settings(monkeypatch, tmp_path)
    client = Sub2APIClient(settings)
    seen: list[tuple[str, str]] = []

    def handle(request: httpx.Request) -> httpx.Response:
        seen.append((request.method, request.url.path))
        if request.url.path == "/api/v1/admin/accounts":
            return httpx.Response(200, json={"data": {"items": [], "total": 0}})
        if request.url.path == "/api/v1/admin/accounts/41":
            return httpx.Response(200, json={"data": {"id": 41}})
        if request.url.path == "/api/v1/admin/accounts/41/usage":
            return httpx.Response(200, json={"data": {"provider_usage": {}}})
        raise AssertionError(f"unexpected admin API request: {request.method} {request.url.path}")

    await client.admin.aclose()
    client.admin = httpx.AsyncClient(
        base_url=settings.admin_base_url,
        headers={"Accept": "application/json", "x-api-key": settings.admin_api_key},
        follow_redirects=False,
        transport=httpx.MockTransport(handle),
    )
    try:
        assert await client.list_active_accounts() == []
        assert await client.get_account(41) == {"id": 41}
        assert await client.get_account_usage(41) == {"provider_usage": {}}
    finally:
        await client.close()

    assert seen == [
        ("GET", "/api/v1/admin/accounts"),
        ("GET", "/api/v1/admin/accounts/41"),
        ("GET", "/api/v1/admin/accounts/41/usage"),
    ]


@pytest.mark.asyncio
async def test_cache_probe_replays_identical_bytes_and_accounts_openai_cache_once(monkeypatch, tmp_path):
    settings = make_settings(monkeypatch, tmp_path)
    profile = _profile(settings, 7)
    assert profile is not None
    fake = FakeGateway(
        [
            {"usage": {"prompt_tokens": 1000, "completion_tokens": 2, "prompt_tokens_details": {"cached_tokens": 0}}},
            {"usage": {"prompt_tokens": 1000, "completion_tokens": 2, "prompt_tokens_details": {"cached_tokens": 500}}},
        ],
    )
    outcome = await run_cache(make_context(settings, profile, fake))
    assert fake.bodies[0] == fake.bodies[1]
    assert len(set(fake.sessions.values())) == 1
    cache_wire_body = json.loads(fake.bodies[0])
    assert cache_wire_body["metadata"]["user_id"] in set(fake.metadata_user_ids.values())
    assert outcome.status == "inconclusive"
    assert outcome.metrics["cache_check"]["second_cache_read_tokens"] == 500
    assert outcome.metrics["cost_check"]["status"] == "inconclusive"
    assert outcome.metrics["cost_check"]["reason"] == "per_request_gateway_cost_unavailable_from_allowed_endpoints"


@pytest.mark.asyncio
async def test_anthropic_oauth_cache_probe_uses_messages_cache_semantics(monkeypatch, tmp_path):
    settings = make_settings(monkeypatch, tmp_path)
    profile = _profile(settings, 7)
    assert profile is not None
    # The default profile says OpenAI, but the actual upstream account dictates
    # both the native Messages route and Anthropic cache token semantics.
    fake = FakeGateway(
        [
            {"usage": {"input_tokens": 0, "cache_creation_input_tokens": 900, "cache_read_input_tokens": 0, "output_tokens": 2}},
            {"usage": {"input_tokens": 0, "cache_creation_input_tokens": 0, "cache_read_input_tokens": 900, "output_tokens": 2}},
        ]
    )
    context = make_context(settings, profile, fake)
    context = replace(
        context,
        account=replace(context.account, platform="anthropic", account_type="oauth"),
    )

    outcome = await run_cache(context)

    assert set(fake.routes.values()) == {"messages"}
    assert fake.bodies[0] == fake.bodies[1]
    body = json.loads(fake.bodies[0])
    assert "system" not in body
    assert body["messages"][0]["content"][0]["cache_control"] == {"type": "ephemeral"}
    assert outcome.metrics["cache_check"]["protocol"] == "anthropic"
    assert outcome.metrics["cache_check"]["configured_protocol_matches_route"] is False
    assert outcome.metrics["cache_check"]["second_cache_read_tokens"] == 900


@pytest.mark.asyncio
async def test_different_probe_prompts_share_sub2api_metadata_routing_anchor(monkeypatch, tmp_path):
    settings = make_settings(monkeypatch, tmp_path)
    profile = _profile(settings, 7)
    assert profile is not None
    fake = FakeGateway([{"usage": {"prompt_tokens": 5, "completion_tokens": 1}} for _ in range(3)])
    context = make_context(settings, profile, fake)

    for prompt in ("challenge one", "challenge two", "challenge three"):
        outcome = await correlated_chat(
            context,
            probe="modeltrace",
            messages=[{"role": "user", "content": prompt}],
            max_tokens=16,
        )
        assert outcome.pinned

    assert len(set(fake.sessions.values())) == 1
    assert len(set(fake.metadata_user_ids.values())) == 1
    assert all("system" not in messages for messages in fake.messages.values())


@pytest.mark.asyncio
async def test_modeltrace_calls_do_not_read_or_record_response_usage(monkeypatch, tmp_path):
    settings = make_settings(monkeypatch, tmp_path)
    profile = _profile(settings, 7)
    assert profile is not None
    fake = FakeGateway(
        [{"choices": [{"message": {"content": "1 2 3"}}], "usage": {"prompt_tokens": 123, "completion_tokens": 4}}]
    )
    context = make_context(settings, profile, fake)

    outcome = await correlated_chat(
        context,
        probe="modeltrace",
        messages=[{"role": "user", "content": "challenge"}],
        max_tokens=16,
    )

    assert outcome.pinned
    call = context.calls[0]
    assert (
        call.input_tokens,
        call.output_tokens,
        call.cache_creation_tokens,
        call.cache_read_tokens,
    ) == (None, None, None, None)


@pytest.mark.asyncio
async def test_anthropic_oauth_probe_avoids_gateway_prompt_injection_path(monkeypatch, tmp_path):
    settings = make_settings(monkeypatch, tmp_path)
    profile = _profile(settings, 7)
    assert profile is not None
    fake = FakeGateway([{"usage": {"prompt_tokens": 5, "completion_tokens": 1}}])
    context = make_context(settings, profile, fake)
    context = replace(
        context,
        account=replace(context.account, platform="anthropic", account_type="setup-token"),
    )
    outcome = await correlated_chat(
        context,
        probe="modeltrace",
        messages=[{"role": "user", "content": "challenge"}],
        max_tokens=16,
    )

    assert outcome.pinned
    assert next(iter(fake.user_agents.values())) == "claude-cli/2.1.161 (modelaudit probe)"
    assert set(fake.routes.values()) == {"messages"}
    assert all("system" not in messages for messages in fake.messages.values())


@pytest.mark.asyncio
async def test_anthropic_api_key_probe_uses_native_messages_without_cli_identity(monkeypatch, tmp_path):
    settings = make_settings(monkeypatch, tmp_path)
    profile = _profile(settings, 7)
    assert profile is not None
    fake = FakeGateway([{"usage": {"input_tokens": 5, "output_tokens": 1}}])
    context = make_context(settings, profile, fake)
    context = replace(context, account=replace(context.account, platform="anthropic", account_type="apikey"))

    outcome = await correlated_chat(
        context,
        probe="token_delta",
        messages=[{"role": "user", "content": "Say hi"}],
        max_tokens=8,
    )

    assert outcome.pinned
    assert set(fake.routes.values()) == {"messages"}
    assert set(fake.user_agents.values()) == {None}
    assert context.calls[0].input_tokens == 5


@pytest.mark.asyncio
async def test_cache_cost_is_inconclusive_when_reference_prices_are_zero(monkeypatch, tmp_path):
    settings = make_settings(
        monkeypatch,
        tmp_path,
        profiles={
            "7": {
                "model": "gpt-4o",
                "expected_model": "gpt-4o",
                "cache_min_prefix_tokens": 128,
                "cache_protocol": "openai",
                "pricing": {
                    "input_per_million": 0,
                    "output_per_million": 0,
                    "cache_read_per_million": 0,
                    "cache_write_per_million": 0,
                },
                "gateway_cost_multiplier": 1,
                "expected_provider_cost_multiplier": 1,
            }
        },
    )
    profile = _profile(settings, 7)
    assert profile is not None
    fake = FakeGateway(
        [
            {"usage": {"prompt_tokens": 1000, "completion_tokens": 2, "prompt_tokens_details": {"cached_tokens": 0}}},
            {"usage": {"prompt_tokens": 1000, "completion_tokens": 2, "prompt_tokens_details": {"cached_tokens": 500}}},
        ],
    )

    outcome = await run_cache(make_context(settings, profile, fake))
    assert outcome.metrics["cache_check"]["status"] == "ok"
    assert outcome.metrics["cost_check"]["status"] == "inconclusive"
    assert outcome.metrics["cost_check"]["reason"] == "configured_expected_cost_is_zero"


def test_token_usage_does_not_add_openai_cached_tokens_twice_and_handles_anthropic():
    openai_total, openai_meta = _input_usage(
        {"usage": {"prompt_tokens": 100, "prompt_tokens_details": {"cached_tokens": 30}}},
        "gpt-4o",
    )
    assert openai_total == 100
    assert openai_meta["cached_tokens_included_in_total"] == 30
    anthropic_total, anthropic_meta = _input_usage(
        {"usage": {"input_tokens": 40, "cache_creation_input_tokens": 10, "cache_read_input_tokens": 5}},
        "claude-sonnet",
    )
    assert anthropic_total == 55
    assert anthropic_meta["usage_semantics"] == "anthropic_disjoint_input_cache_tokens"


def test_gateway_configuration_rejects_superkey(monkeypatch, tmp_path):
    make_settings(monkeypatch, tmp_path)
    monkeypatch.setenv("SUB2API_GATEWAY_API_KEYS", '{"7":"sk-super-never-an-ordinary-api-key"}')
    with pytest.raises(ValueError, match="never SuperKeys"):
        load_settings()


@pytest.mark.asyncio
async def test_upstream_http_errors_never_propagate_response_body(monkeypatch, tmp_path):
    import httpx

    settings = make_settings(monkeypatch, tmp_path)
    client = Sub2APIClient(settings)
    try:
        async def admin_error(*args, **kwargs):
            return httpx.Response(403, json={"reason": "secret-admin-detail", "token": "must-not-leak"})

        async def gateway_error(*args, **kwargs):
            return httpx.Response(429, json={"error": {"code": "secret-upstream-detail"}, "token": "must-not-leak"})

        monkeypatch.setattr(client.admin, "request", admin_error)
        with pytest.raises(Sub2APIError) as caught:
            await client._admin_json("GET", "/private")
        assert caught.value.status_code == 403
        assert caught.value.code == "admin_api_http_403"
        assert "secret" not in str(caught.value).lower()

        monkeypatch.setattr(client.gateway, "post", gateway_error)
        status, payload, code = await client.gateway_chat(
            key="ordinary-user-key",
            model="gpt-4o",
            session_id="session",
            request_id="request",
            metadata_user_id=account_metadata_user_id("a" * 32, 41, "session"),
            user_agent=None,
            messages=[{"role": "user", "content": "hello"}],
            max_tokens=4,
        )
        assert (status, payload, code) == (429, None, "gateway_http_429")
    finally:
        await client.close()


def test_panel_api_requires_admin_session(monkeypatch, tmp_path):
    make_settings(monkeypatch, tmp_path)
    from fastapi.testclient import TestClient

    with TestClient(create_app()) as client:
        assert client.get("/health").status_code == 200
        assert client.get("/").status_code == 200
        assert client.get("/api/session").status_code == 401


def test_modeltrace_vendor_snapshot_is_available_for_docker():
    from pathlib import Path
    import importlib.util

    vendor = Path(__file__).resolve().parents[1] / "vendor" / "modeltrace"
    module_spec = importlib.util.spec_from_file_location("modelaudit_test_vendor_fingerprint", vendor / "fingerprint.py")
    assert module_spec is not None and module_spec.loader is not None
    module = importlib.util.module_from_spec(module_spec)
    module_spec.loader.exec_module(module)
    bank = module.load_bank(vendor / "data" / "unified_bank.json")
    assert len(bank["models"]) == 16
    calibration, status = trace_calibration.load_candidate_calibration(
        __import__("hashlib").sha256((vendor / "data" / "unified_bank.json").read_bytes()).hexdigest(),
        __import__("hashlib").sha256((vendor / "fingerprint.py").read_bytes()).hexdigest(),
    )
    assert status == "verified"
    assert calibration is not None
    assert set(calibration["candidate_minimum_profile_similarity"]) == {
        item["id"] for item in bank["models"]
    }


def test_modeltrace_candidate_calibration_is_bound_to_bank_hash(monkeypatch, tmp_path):
    calibration_path = tmp_path / "candidate_thresholds.json"
    calibration_path.write_text(
        json.dumps(
            {
                "schema_version": 1,
                "bank_sha256": "expected-bank-hash",
                "fingerprint_sha256": "expected-fingerprint-hash",
                "method": "test",
                "minimum_probability": 0.95,
                "minimum_margin": 0.2,
                "similarity_margin": 0.1,
                "candidate_minimum_profile_similarity": {"gpt-4o": 0.91},
            }
        ),
        encoding="utf-8",
    )
    monkeypatch.setattr(trace_calibration, "CALIBRATION_PATH", calibration_path)

    loaded, status = trace_calibration.load_candidate_calibration(
        "expected-bank-hash", "expected-fingerprint-hash"
    )
    assert status == "verified"
    assert loaded["candidate_minimum_profile_similarity"]["gpt-4o"] == pytest.approx(0.91)

    mismatched, mismatch_status = trace_calibration.load_candidate_calibration(
        "different-bank-hash", "expected-fingerprint-hash"
    )
    assert mismatched is None
    assert mismatch_status == "calibration_bank_hash_mismatch"
    mismatched, mismatch_status = trace_calibration.load_candidate_calibration(
        "expected-bank-hash", "different-fingerprint-hash"
    )
    assert mismatched is None
    assert mismatch_status == "calibration_fingerprint_hash_mismatch"


def test_modeltrace_calibration_folds_keep_training_calibration_validation_disjoint():
    folds = environment_disjoint_folds([f"environment-{index:02d}" for index in range(1, 13)])

    assert len(folds) == 4
    validation_environments = set()
    for fold in folds:
        training = set(fold["training"])
        calibration = set(fold["calibration"])
        validation = set(fold["validation"])
        assert len(training) == 6
        assert len(calibration) == 3
        assert len(validation) == 3
        assert training.isdisjoint(calibration)
        assert training.isdisjoint(validation)
        assert calibration.isdisjoint(validation)
        validation_environments.update(validation)
    assert validation_environments == {f"environment-{index:02d}" for index in range(1, 13)}


def test_modeltrace_uses_candidate_threshold_and_rejects_lower_runtime_floors(monkeypatch, tmp_path):
    settings = make_settings(monkeypatch, tmp_path)
    profile = _profile(settings, 7)
    assert profile is not None
    context = make_context(settings, profile, FakeGateway([]))
    calibration = {
        "minimum_probability": 0.95,
        "minimum_margin": 0.2,
        "similarity_margin": 0.1,
        "method": "test calibration",
        "candidate_minimum_profile_similarity": {"gpt-4o": 0.82},
    }
    analysis = {
        "results": [
            {"model": "gpt-4o", "probability": 0.99, "profile_similarity": 0.83},
            {"model": "gpt-4.1", "probability": 0.01, "profile_similarity": 0.4},
        ]
    }

    unknown, reason, metrics = modeltrace_probe._ood_decision(
        analysis, context, calibration, "verified", "bank-hash", "fingerprint-hash"
    )
    assert unknown is False
    assert reason == ""
    assert metrics["thresholds"]["minimum_profile_similarity"] == pytest.approx(0.82)
    assert metrics["threshold_source"] == "candidate_omitted_model_calibration"

    low_context = replace(context, settings=replace(settings, trace_min_probability=0.90))
    unknown, reason, _ = modeltrace_probe._ood_decision(
        analysis, low_context, calibration, "verified", "bank-hash", "fingerprint-hash"
    )
    assert unknown is True
    assert reason == "runtime_confidence_threshold_below_calibrated_floor"


def test_modeltrace_extracts_native_anthropic_messages_text_and_truncation():
    assert modeltrace_probe._message_text(
        {
            "content": [
                {"type": "text", "text": "1 2 "},
                {"type": "thinking", "thinking": "not part of the visible answer"},
                {"type": "text", "text": "3 4"},
            ],
            "stop_reason": "end_turn",
        }
    ) == ("1 2 3 4", "end_turn")
    assert modeltrace_probe._message_text(
        {"content": [{"type": "text", "text": "short"}], "stop_reason": "max_tokens"}
    ) == ("short", "length")


@pytest.mark.asyncio
@pytest.mark.parametrize(
    ("bank_models", "results", "expected_reason", "expected_differs"),
    [
        (
            [{"id": "gpt-4o"}, {"id": "gpt-5.4"}],
            [
                {"model": "gpt-5.4", "probability": 0.4, "profile_similarity": 0.5},
                {"model": "gpt-4o", "probability": 0.3, "profile_similarity": 0.4},
            ],
            "low_probability",
            True,
        ),
        (
            [{"id": "gpt-4o"}, {"id": "gpt-5.4"}],
            [
                {"model": "gpt-4o", "probability": 0.4, "profile_similarity": 0.5},
                {"model": "gpt-5.4", "probability": 0.3, "profile_similarity": 0.4},
            ],
            "low_probability",
            False,
        ),
        (
            [{"id": "gpt-5.4"}, {"id": "gpt-5.5"}],
            [
                {"model": "gpt-5.4", "probability": 0.99, "profile_similarity": 0.95},
                {"model": "gpt-5.5", "probability": 0.01, "profile_similarity": 0.2},
            ],
            "expected_model_not_in_reference_bank",
            None,
        ),
    ],
)
async def test_modeltrace_unknown_result_does_not_name_closed_set_candidate(
    monkeypatch, tmp_path, bank_models, results, expected_reason, expected_differs
):
    settings = make_settings(monkeypatch, tmp_path)
    profile = _profile(settings, 7)
    assert profile is not None
    context = make_context(settings, profile, FakeGateway([]))
    bank_path = tmp_path / "bank.json"
    bank_path.write_text("{}", encoding="utf-8")

    class FakeFingerprint:
        @staticmethod
        def load_bank(path):
            return {"built_at": "test", "models": bank_models}

        @staticmethod
        def generate_challenges(count):
            return [{"prompt": f"challenge-{index}", "expected_count": 80} for index in range(count)]

        @staticmethod
        def parse_numbers(text):
            return list(range(80))

        @staticmethod
        def analyze_global_outputs(outputs, bank):
            return {
                "prediction": results[0]["model"],
                "family_prediction": "gpt",
                "family_probability": 0.99,
                "method": "synthetic-test",
                "calibration": {"temperature": 1.0},
                "results": results,
            }

    calls = 0

    async def fake_correlated_chat(*args, **kwargs):
        nonlocal calls
        calls += 1
        return SimpleNamespace(
            request_id=f"request-{calls}",
            pinned=True,
            actual_account_id=41,
            http_status=200,
            payload={
                "choices": [{"finish_reason": "stop", "message": {"content": "numbers"}}],
                "usage": {"prompt_tokens": 999_999, "completion_tokens": 999_999},
            },
            error_code="",
            pin_reason="verified",
        )

    monkeypatch.setattr(modeltrace_probe, "_load_fingerprint_module", lambda: FakeFingerprint())
    monkeypatch.setattr(modeltrace_probe, "_bank_path", lambda: bank_path)
    monkeypatch.setattr(
        modeltrace_probe,
        "load_candidate_calibration",
        lambda _bank_hash, _fingerprint_hash: (
            {
                "minimum_probability": 0.95,
                "minimum_margin": 0.2,
                "similarity_margin": 0.1,
                "fingerprint_sha256": "test-fingerprint-hash",
                "method": "test calibration",
                "candidate_minimum_profile_similarity": {
                    "gpt-4o": 0.9,
                    "gpt-5.4": 0.9,
                    "gpt-5.5": 0.9,
                },
            },
            "verified",
        ),
    )
    monkeypatch.setattr(modeltrace_probe, "correlated_chat", fake_correlated_chat)

    outcome = await modeltrace_probe.run_modeltrace_async(context)
    assert outcome.status == "unknown"
    assert "gpt-5.4" not in json.dumps(outcome.as_dict())
    assert "gpt-5.5" not in json.dumps(outcome.as_dict())
    assert outcome.metrics["candidate_count"] == 2
    assert expected_reason in outcome.evidence["reason"]
    if expected_reason == "expected_model_not_in_reference_bank":
        assert calls == 0
    if expected_differs is not None:
        assert outcome.metrics["closed_set_candidate_differs_from_expected"] is expected_differs
        assert outcome.severity == ("yellow" if expected_differs else "none")
        assert outcome.metrics["estimated_modeltrace_challenges"] == 3
        assert outcome.metrics["estimated_modeltrace_cost_status"] == "estimated"
        assert 0 < outcome.metrics["estimated_modeltrace_input_tokens"] < 1_000
        assert 0 < outcome.metrics["estimated_modeltrace_output_tokens"] < 100
        assert 0 < outcome.metrics["estimated_modeltrace_cost_usd"] < 0.001
        assert "numbers" not in json.dumps(outcome.as_dict())
        if expected_differs:
            assert "候选型号名已隐藏" in outcome.summary


@pytest.mark.asyncio
async def test_modeltrace_stale_calibration_is_unknown_without_spending_probe_calls(
    monkeypatch, tmp_path
):
    settings = make_settings(monkeypatch, tmp_path)
    profile = _profile(settings, 7)
    assert profile is not None
    context = make_context(settings, profile, FakeGateway([]))
    bank_path = tmp_path / "bank.json"
    bank_path.write_text("{}", encoding="utf-8")

    class FakeFingerprint:
        @staticmethod
        def load_bank(_path):
            return {"built_at": "test", "models": [{"id": "gpt-4o"}]}

        @staticmethod
        def generate_challenges(_count):
            raise AssertionError("stale calibration must not spend upstream calls")

    async def unexpected_probe(*_args, **_kwargs):
        raise AssertionError("stale calibration must not send challenges")

    monkeypatch.setattr(modeltrace_probe, "_load_fingerprint_module", lambda: FakeFingerprint())
    monkeypatch.setattr(modeltrace_probe, "_bank_path", lambda: bank_path)
    monkeypatch.setattr(
        modeltrace_probe,
        "load_candidate_calibration",
        lambda _bank_hash, _fingerprint_hash: (None, "calibration_bank_hash_mismatch"),
    )
    monkeypatch.setattr(modeltrace_probe, "correlated_chat", unexpected_probe)

    outcome = await modeltrace_probe.run_modeltrace_async(context)

    assert outcome.status == "unknown"
    assert outcome.severity == "yellow"
    assert outcome.metrics["calibration_status"] == "calibration_bank_hash_mismatch"
    assert outcome.request_ids == ()
    assert outcome.actual_account_id is None


@pytest.mark.asyncio
@pytest.mark.parametrize(
    ("second_status", "second_candidate", "confirmed"),
    [
        ("alert", "gpt-4o", True),
        ("alert", "gpt-4.1", False),
        ("unknown", "gpt-4o", False),
    ],
)
async def test_modeltrace_mismatch_needs_same_candidate_in_second_challenge(
    monkeypatch, tmp_path, second_status, second_candidate, confirmed
):
    settings = make_settings(monkeypatch, tmp_path)
    profile = _profile(settings, 7)
    assert profile is not None
    context = make_context(settings, profile, FakeGateway([]))
    outcomes = [
        result(
            "modeltrace",
            "alert",
            "expected mismatch",
            severity="red",
            model=profile.model,
            expected_model=profile.expected_model,
            request_ids=("first-request",),
            metrics={"observed_candidate": "gpt-4o"},
        ),
        result(
            "modeltrace",
            second_status,
            "second challenge result",
            severity="red" if second_status == "alert" else "yellow",
            model=profile.model,
            expected_model=profile.expected_model,
            request_ids=("second-request",),
            metrics={"observed_candidate": second_candidate},
        ),
    ]
    calls = 0

    async def fake_run(_context):
        nonlocal calls
        value = outcomes[calls]
        calls += 1
        return value

    monkeypatch.setattr(modeltrace_probe, "run_modeltrace_async", fake_run)
    outcome = await modeltrace_probe.run_modeltrace_with_confirmation(context)

    assert calls == 2
    assert outcome.status == "alert"
    assert outcome.severity == "red"
    assert outcome.metrics["auto_stop_eligible"] is confirmed
    assert outcome.metrics["confirmation"]["confirmed_same_mismatch"] is confirmed
    assert outcome.request_ids == ("first-request", "second-request")


@pytest.mark.asyncio
async def test_modeltrace_confirmation_aggregates_local_cost_estimates(monkeypatch, tmp_path):
    settings = make_settings(monkeypatch, tmp_path)
    context = make_context(settings, _profile(settings, 7), FakeGateway([]))
    outcomes = iter(
        [
            result(
                "modeltrace",
                "alert",
                "first mismatch",
                severity="red",
                model="gpt-4o",
                expected_model="gpt-4o",
                request_ids=("first-1", "first-2", "first-3"),
                metrics={
                    "observed_candidate": "gpt-4.1",
                    "estimated_modeltrace_cost_usd": 0.0002,
                    "estimated_modeltrace_cost_status": "estimated",
                    "estimated_modeltrace_challenges": 3,
                    "estimated_modeltrace_attributed_challenges": 3,
                    "estimated_modeltrace_input_tokens": 120,
                    "estimated_modeltrace_output_tokens": 90,
                },
            ),
            result(
                "modeltrace",
                "alert",
                "confirmed mismatch",
                severity="red",
                model="gpt-4o",
                expected_model="gpt-4o",
                request_ids=("second-1", "second-2", "second-3"),
                metrics={
                    "observed_candidate": "gpt-4.1",
                    "estimated_modeltrace_cost_usd": 0.0003,
                    "estimated_modeltrace_cost_status": "estimated",
                    "estimated_modeltrace_challenges": 3,
                    "estimated_modeltrace_attributed_challenges": 3,
                    "estimated_modeltrace_input_tokens": 130,
                    "estimated_modeltrace_output_tokens": 95,
                },
            ),
        ]
    )

    async def fake_modeltrace(_context):
        return next(outcomes)

    monkeypatch.setattr(modeltrace_probe, "run_modeltrace_async", fake_modeltrace)
    outcome = await modeltrace_probe.run_modeltrace_with_confirmation(context)

    assert outcome.metrics["auto_stop_eligible"] is True
    assert outcome.metrics["estimated_modeltrace_cost_usd"] == pytest.approx(0.0005)
    assert outcome.metrics["estimated_modeltrace_challenges"] == 6
    assert outcome.metrics["estimated_modeltrace_input_tokens"] == 250
    assert outcome.metrics["estimated_modeltrace_output_tokens"] == 185


@pytest.mark.asyncio
async def test_modeltrace_confirmation_preserves_unavailable_cost_reason(monkeypatch, tmp_path):
    settings = make_settings(monkeypatch, tmp_path)
    context = make_context(settings, _profile(settings, 7), FakeGateway([]))
    outcomes = iter(
        [
            result(
                "modeltrace",
                "alert",
                "first mismatch",
                severity="red",
                model="gpt-4o",
                expected_model="gpt-4o",
                metrics={
                    "observed_candidate": "gpt-4.1",
                    "estimated_modeltrace_cost_usd": None,
                    "estimated_modeltrace_cost_status": "price_or_multiplier_unavailable",
                },
            ),
            result(
                "modeltrace",
                "alert",
                "confirmed mismatch",
                severity="red",
                model="gpt-4o",
                expected_model="gpt-4o",
                metrics={
                    "observed_candidate": "gpt-4.1",
                    "estimated_modeltrace_cost_usd": None,
                    "estimated_modeltrace_cost_status": "price_or_multiplier_unavailable",
                },
            ),
        ]
    )

    async def fake_modeltrace(_context):
        return next(outcomes)

    monkeypatch.setattr(modeltrace_probe, "run_modeltrace_async", fake_modeltrace)
    outcome = await modeltrace_probe.run_modeltrace_with_confirmation(context)

    assert outcome.metrics["estimated_modeltrace_cost_status"] == "price_or_multiplier_unavailable"
    assert outcome.metrics["estimated_modeltrace_cost_usd"] is None


@pytest.mark.asyncio
async def test_request_is_unpinned_without_exclusive_group_routing(monkeypatch, tmp_path):
    settings = make_settings(monkeypatch, tmp_path)
    profile = _profile(settings, 7)
    assert profile is not None
    fake = FakeGateway([{"usage": {"prompt_tokens": 3, "completion_tokens": 1}}])
    context = replace(make_context(settings, profile, fake), exclusive_group=False)
    outcome = await correlated_chat(
        context,
        probe="token_delta",
        messages=[{"role": "user", "content": "Say hi"}],
        max_tokens=8,
    )
    assert not outcome.pinned
    assert outcome.pin_reason == "exclusive_group_not_confirmed"


@pytest.mark.asyncio
@pytest.mark.parametrize(
    ("protocol", "usage", "expected"),
    [
        (
            "openai",
            {"prompt_tokens": 75, "completion_tokens": 4, "prompt_tokens_details": {"cached_tokens": 10}},
            (65, 4, 0, 10),
        ),
        (
            "anthropic",
            {
                "input_tokens": 75,
                "output_tokens": 4,
                "cache_creation_input_tokens": 8,
                "cache_read_input_tokens": 7,
            },
            (75, 4, 8, 7),
        ),
    ],
)
async def test_response_usage_is_logged_without_admin_usage_lookup(monkeypatch, tmp_path, protocol, usage, expected):
    settings = make_settings(monkeypatch, tmp_path)
    profile = _profile(settings, 7)
    assert profile is not None
    profile = replace(profile, cache_protocol=protocol)

    fake = FakeGateway([{"usage": usage}])
    context = make_context(settings, profile, fake)
    outcome = await correlated_chat(
        context,
        probe="token_delta",
        messages=[{"role": "user", "content": "Say hi"}],
        max_tokens=8,
    )

    assert outcome.pinned
    assert outcome.pin_reason == "exclusive_group_single_schedulable_account"
    assert context.calls[0].pinned is True
    assert (
        context.calls[0].input_tokens,
        context.calls[0].output_tokens,
        context.calls[0].cache_creation_tokens,
        context.calls[0].cache_read_tokens,
    ) == expected


@pytest.mark.asyncio
async def test_openai_response_usage_splits_cached_input_without_admin_lookup(monkeypatch, tmp_path):
    settings = make_settings(monkeypatch, tmp_path)
    profile = _profile(settings, 7)
    assert profile is not None

    fake = FakeGateway(
        [
            {
                "usage": {
                    "prompt_tokens": 100,
                    "completion_tokens": 5,
                    "prompt_tokens_details": {"cached_tokens": 30},
                }
            }
        ]
    )
    context = make_context(settings, profile, fake)
    outcome = await correlated_chat(
        context,
        probe="cache",
        messages=[{"role": "user", "content": "cached prompt"}],
        max_tokens=8,
    )

    assert outcome.pinned
    call = context.calls[0]
    assert call.input_tokens == 70
    assert call.cache_creation_tokens == 0
    assert call.cache_read_tokens == 30
    assert call.input_tokens + call.cache_creation_tokens + call.cache_read_tokens == 100


@pytest.mark.asyncio
async def test_token_delta_flags_negative_delta_for_hidden_input_additions(monkeypatch, tmp_path):
    settings = make_settings(monkeypatch, tmp_path)
    profile = _profile(settings, 7)
    assert profile is not None
    fake = FakeGateway(
        [{"usage": {"prompt_tokens": 250, "completion_tokens": 2, "prompt_tokens_details": {"cached_tokens": 0}}}],
    )
    outcome = await run_token_delta(make_context(settings, profile, fake))
    assert outcome.status == "alert"
    assert outcome.severity == "red"
    assert outcome.metrics["delta_expected_minus_actual"] < -100
    assert outcome.metrics["direction"] == "reported_input_exceeds_local_estimate"


def test_supplier_usage_counters_are_whitelisted_and_token_delta_drives_multiplier(monkeypatch, tmp_path):
    normalized = normalize_usage(
        {
            "source": "active",
            "secret": "sk-upstream-must-not-persist",
            "error_code": "sk-error-code-must-not-persist",
            "grok_quota_snapshot_state": "observed",
            "grok_last_headers_seen_at": "2026-09-25T10:00:00Z",
            "grok_token_quota": {
                "limit": 100000,
                "remaining": 50000,
                "reset_unix": 1790000000,
                "credential": "must-not-persist",
            },
            "grok_billing": {
                "status_code": 200,
                "monthly_used": 10.0,
                "billing_period_start": "2026-09-01",
                "billing_period_end": "2026-10-01",
            },
        }
    )
    assert "secret" not in normalized
    assert normalized["error_code"] == "upstream_usage_error"
    assert "sk-error-code-must-not-persist" not in json.dumps(normalized)
    assert supplier_money_counter(normalized) == ("grok_monthly_usd", 10.0, "2026-09-01")
    assert normalized["grok_token_quota"] == {
        "limit": 100000,
        "remaining": 50000,
        "reset_unix": 1790000000,
    }
    assert "credential" not in json.dumps(normalized)

    settings = make_settings(monkeypatch, tmp_path)
    profile = _profile(settings, 7)
    assert profile is not None
    empty = FakeGateway([])
    context = make_context(settings, profile, empty)
    before = {
        "grok_quota_snapshot_state": "observed",
        "grok_last_headers_seen_at": "2026-09-25T10:00:00Z",
        "grok_token_quota": {"limit": 100000, "remaining": 50000, "reset_unix": 1790000000},
        "grok_billing": {"status_code": 200, "monthly_used_usd": 10.0, "period_start": "2026-09-01"},
    }
    after = {
        "grok_quota_snapshot_state": "observed",
        "grok_last_headers_seen_at": "2026-09-25T10:01:00Z",
        "grok_token_quota": {"limit": 100000, "remaining": 49000, "reset_unix": 1790000000},
        "grok_billing": {"status_code": 200, "monthly_used_usd": 10.001, "period_start": "2026-09-01"},
    }
    assert supplier_token_delta(normalize_usage(before), normalize_usage(after)) == (1000, "")
    stale_headers = {**after, "grok_last_headers_seen_at": before["grok_last_headers_seen_at"]}
    changed_window = {
        **after,
        "grok_token_quota": {"limit": 200000, "remaining": 49000, "reset_unix": 1790000000},
    }
    assert supplier_token_delta(normalize_usage(before), normalize_usage(stale_headers)) == (
        None,
        "upstream_token_headers_not_refreshed",
    )
    assert supplier_token_delta(normalize_usage(before), normalize_usage(changed_window)) == (
        None,
        "token_quota_window_changed",
    )

    context = replace(
        context,
        account=replace(context.account, rate_multiplier=1.0),
        profile=replace(context.profile, expected_provider_cost_multiplier=1.0),
    )
    context.calls.append(
        ProbeCall(
            request_id="calibration-request",
            probe="pin_calibration",
            http_status=200,
            error_code="",
            actual_account_id=41,
            session_matches=True,
            pinned=True,
            input_tokens=100,
            output_tokens=0,
            cache_creation_tokens=0,
            cache_read_tokens=0,
            pin_reason="verified",
        )
    )
    context.calls.append(
        ProbeCall(
            request_id="request-1",
            probe="token_delta",
            http_status=200,
            error_code="",
            actual_account_id=41,
            session_matches=True,
            pinned=True,
            input_tokens=1000,
            output_tokens=0,
            cache_creation_tokens=0,
            cache_read_tokens=0,
            pin_reason="verified",
        )
    )
    result = run_multiplier(context, before, after)
    assert result.status == "ok"
    assert result.metrics["provider_token_delta"] == 1000
    assert result.metrics["reported_probe_tokens"] == 1000
    assert result.metrics["estimated_probe_cost_usd"] == pytest.approx(0.001)
    assert result.metrics["estimated_probe_cost_measured_calls"] == 1
    assert result.metrics["estimated_pin_calibration_cost_usd"] == pytest.approx(0.0001)
    assert result.metrics["estimated_total_usage_aware_probe_cost_usd"] == pytest.approx(0.0011)
    assert result.metrics["observed_usage_multiplier"] == pytest.approx(1)
    assert result.metrics["billing_multiplier_comparison"] == "token_quota_ratio_compared_separately_from_invoice_usd_ratio"
    assert result.metrics["token_quota_multiplier_comparisons"]["configured_account_rate_multiplier"]["status"] == "within_threshold"
    assert result.metrics["token_quota_multiplier_comparisons"]["expected_provider_cost_multiplier"]["status"] == "within_threshold"
    assert result.metrics["invoice_diagnostic"]["used_for_alert"] is False
    assert result.metrics["invoice_diagnostic"]["observed_cost_multiplier"] == pytest.approx(1)
    assert result.metrics["invoice_diagnostic"]["comparisons"]["configured_account_rate_multiplier"]["status"] == "within_threshold"
    assert result.metrics["invoice_diagnostic"]["comparisons"]["expected_provider_cost_multiplier"]["status"] == "within_threshold"

    rate_mismatch = run_multiplier(
        replace(context, account=replace(context.account, rate_multiplier=20.0)), before, after
    )
    assert rate_mismatch.status == "alert"
    assert rate_mismatch.metrics["token_quota_multiplier_comparisons"]["configured_account_rate_multiplier"]["status"] == "mismatch"

    partial_baseline_context = replace(
        context,
        profile=replace(context.profile, expected_provider_cost_multiplier=None),
    )
    partial_baseline = run_multiplier(partial_baseline_context, before, after)
    assert partial_baseline.status == "inconclusive"
    assert partial_baseline.metrics["invoice_diagnostic"]["status"] == "partially_comparable"

    exaggerated = {
        **after,
        "grok_last_headers_seen_at": "2026-09-25T10:02:00Z",
        "grok_token_quota": {"limit": 100000, "remaining": 39000, "reset_unix": 1790000000},
    }
    result = run_multiplier(context, before, exaggerated)
    assert result.status == "alert"
    assert result.metrics["observed_usage_multiplier"] > 10

    bill_only = run_multiplier(
        context,
        {"grok_billing": {"status_code": 200, "monthly_used_usd": 10.0, "period_start": "2026-09-01"}},
        {"grok_billing": {"status_code": 200, "monthly_used_usd": 11.0, "period_start": "2026-09-01"}},
    )
    assert bill_only.status == "alert"
    assert bill_only.severity == "yellow"
    assert bill_only.metrics["provider_token_delta"] is None
    assert bill_only.metrics["invoice_diagnostic"]["status"] == "mismatch"
    assert bill_only.metrics["invoice_diagnostic"]["observed_cost_multiplier"] > 10
    assert bill_only.metrics["invoice_multiplier_used_for_alert"] is True
    assert bill_only.evidence["supplier_invoice_alert_is_an_account_level_screening_signal"] is True


def test_multiplier_expected_cost_excludes_modeltrace_and_pin_calibration(monkeypatch, tmp_path):
    settings = make_settings(monkeypatch, tmp_path)
    profile = _profile(settings, 7)
    assert profile is not None
    context = make_context(settings, profile, FakeGateway([]))

    def call(probe: str, input_tokens: int, output_tokens: int, cache_read_tokens: int = 0):
        return ProbeCall(
            request_id=f"request-{probe}",
            probe=probe,
            http_status=200,
            error_code="",
            actual_account_id=41,
            session_matches=True,
            pinned=True,
            input_tokens=input_tokens,
            output_tokens=output_tokens,
            cache_creation_tokens=0,
            cache_read_tokens=cache_read_tokens,
            pin_reason="verified",
        )

    context.calls.extend(
        [
            call("pin_calibration", 50_000, 8),
            call("modeltrace", 20_000, 4_000),
            call("token_delta", 1_000, 2),
            call("cache", 1_000, 2, 500),
        ]
    )

    expected_cost, measured_calls = _expected_cost(context)
    assert expected_cost == pytest.approx(0.002133)
    assert measured_calls == 2
    calibration_cost, calibration_calls = _expected_cost(context, frozenset({"pin_calibration"}))
    assert calibration_cost == pytest.approx(0.050016)
    assert calibration_calls == 1


@pytest.mark.asyncio
async def test_sqlite_report_keeps_checks_separate_and_hashes_session(monkeypatch, tmp_path):
    settings = make_settings(monkeypatch, tmp_path)
    await database.initialize(settings.db_path)
    db = await database.connect(settings.db_path)
    try:
        await db.execute(
            "INSERT INTO probe_runs(id, trigger, status, started_at, finished_at, account_count, result_count) "
            "VALUES('run-report', 'manual', 'completed', ?, ?, 1, 1)",
            (database.utc_now(), database.utc_now()),
        )
        account = AccountSnapshot(41, "provider", "openai", "oauth", "active", True, 1, 0, 1.0, (7,))
        await save_account(db, account)
        profile = _profile(settings, 7)
        assert profile is not None
        await save_probe_result(
            db,
            run_id="run-report",
            account_id=41,
            profile=profile,
            item=result("token_delta", "ok", "delta checked", metrics={"delta_expected_minus_actual": 1}),
            session_id="session-must-be-hashed",
        )
        call = ProbeCall(
            request_id="request-report",
            probe="token_delta",
            http_status=200,
            error_code="",
            actual_account_id=41,
            session_matches=True,
            pinned=True,
            input_tokens=10,
            output_tokens=1,
            cache_creation_tokens=0,
            cache_read_tokens=0,
            pin_reason="verified",
        )
        await save_probe_calls(
            db,
            run_id="run-report",
            account_id=41,
            call_rows=[(call, 7, "session-must-be-hashed")],
        )
        row = await database.fetch_one(db, "SELECT session_id_hash FROM probe_calls WHERE request_id='request-report'")
        assert row["session_id_hash"] != "session-must-be-hashed"
        columns = await database.fetch_all(db, "PRAGMA table_info(probe_calls)")
        assert not {"total_cost", "actual_cost", "upstream_model_mismatch"} & {column["name"] for column in columns}
        report = await build_report(db, "run-report")
        encoded = json.dumps(report)
        assert report["report_version"] == 2
        assert set(report["accounts"][0]["checks"]) == {"token_delta"}
        gateway_calls = report["accounts"][0]["gateway_calls"]
        assert len(gateway_calls) == 1
        assert gateway_calls[0]["request_id"] == "request-report"
        assert gateway_calls[0]["input_tokens"] == 10
        assert "request_costs" not in report["accounts"][0]
        assert "actual_cost" not in encoded
        assert "total_cost" not in encoded
        assert "session-must-be-hashed" not in encoded
        assert "sk-upstream-must-not-persist" not in encoded
    finally:
        await db.close()
