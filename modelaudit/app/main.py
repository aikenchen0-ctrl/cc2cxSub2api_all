from __future__ import annotations

from contextlib import asynccontextmanager
from pathlib import Path
from typing import Any

import aiosqlite
from fastapi import FastAPI, HTTPException, Request
from fastapi.responses import FileResponse, JSONResponse, RedirectResponse
from fastapi.staticfiles import StaticFiles

from . import db as database
from .auth import clear_session, exchange_ticket, require_admin, require_same_origin
from .config import Settings, load_settings
from .options import get_options, merge_options, set_options
from .report import build_report
from .scheduler import ScanAlreadyRunning, ScanCoordinator, _profile
from .storage import audit_action
from .sub2api import Sub2APIClient, Sub2APIError


def _parse_json(raw: str | None) -> Any:
    if not raw:
        return {}
    try:
        import json

        return json.loads(raw)
    except (TypeError, ValueError):
        return {}


def _configured(settings: Settings) -> bool:
    return (
        settings.configured
        and len(settings.sso_secret.encode("utf-8")) >= 32
        and len(settings.session_secret.encode("utf-8")) >= 32
    )


@asynccontextmanager
async def lifespan(app: FastAPI):
    settings = load_settings()
    await database.initialize(settings.db_path)
    db = await database.connect(settings.db_path)
    sub2api = Sub2APIClient(settings)
    coordinator = ScanCoordinator(db, settings, sub2api)
    app.state.settings = settings
    app.state.db = db
    app.state.sub2api = sub2api
    app.state.coordinator = coordinator
    await get_options(db, settings)
    await coordinator.start_scheduler()
    if settings.run_on_start:
        coordinator.launch_manual_scan()
    try:
        yield
    finally:
        await coordinator.stop_scheduler()
        await sub2api.close()
        await db.close()


def create_app() -> FastAPI:
    app = FastAPI(title="Sub2API Upstream Audit", docs_url=None, redoc_url=None, lifespan=lifespan)
    assets = Path(__file__).resolve().parent / "static"
    app.mount("/static", StaticFiles(directory=assets), name="static")

    @app.middleware("http")
    async def no_store_api(request: Request, call_next):
        response = await call_next(request)
        if request.url.path.startswith("/api/"):
            response.headers["Cache-Control"] = "no-store"
            response.headers["Pragma"] = "no-cache"
        return response

    @app.get("/health")
    async def health(request: Request) -> dict[str, Any]:
        settings: Settings = request.app.state.settings
        return {
            "status": "ok",
            "configured": _configured(settings),
            "scan_running": request.app.state.coordinator.is_running,
        }

    @app.get("/")
    async def index() -> FileResponse:
        return FileResponse(assets / "index.html", headers={"Cache-Control": "no-store"})

    @app.get("/api/auth/sso/callback")
    async def sso_callback(ticket: str, request: Request) -> RedirectResponse:
        settings: Settings = request.app.state.settings
        if (
            len(settings.sso_secret.encode("utf-8")) < 32
            or len(settings.session_secret.encode("utf-8")) < 32
            or not (settings.admin_api_key or settings.admin_jwt)
        ):
            raise HTTPException(status_code=503, detail="sso_not_configured")
        response = RedirectResponse(url="/", status_code=303, headers={"Cache-Control": "no-store"})
        next_path = await exchange_ticket(
            request=request,
            response=response,
            db=request.app.state.db,
            settings=settings,
            sub2api=request.app.state.sub2api,
            ticket=ticket,
        )
        response.headers["Location"] = next_path
        return response

    @app.get("/api/session")
    async def session(request: Request) -> dict[str, Any]:
        identity = await require_admin(request)
        return {"sub": identity.sub, "email": identity.email, "display_name": identity.display_name}

    @app.post("/api/logout")
    async def logout(request: Request) -> JSONResponse:
        require_same_origin(request, request.app.state.settings)
        response = JSONResponse({"ok": True}, headers={"Cache-Control": "no-store"})
        await clear_session(request, response)
        return response

    @app.get("/api/dashboard")
    async def dashboard(request: Request) -> dict[str, Any]:
        await require_admin(request)
        db: aiosqlite.Connection = request.app.state.db
        settings: Settings = request.app.state.settings
        runs = await database.fetch_all(
            db,
            "SELECT id, trigger, status, started_at, finished_at, account_count, result_count, error_count, note "
            "FROM probe_runs ORDER BY started_at DESC LIMIT 20",
        )
        latest_run = runs[0] if runs else None
        if latest_run:
            accounts = await database.fetch_all(
                db,
                """SELECT account_id, name, platform, account_type, status, schedulable, concurrency,
                          current_concurrency, rate_multiplier, group_ids_json, last_seen_at
                   FROM accounts
                   WHERE account_id IN (SELECT DISTINCT account_id FROM probe_results WHERE run_id=?)
                   ORDER BY name COLLATE NOCASE, account_id""",
                (latest_run["id"],),
            )
        else:
            accounts = await database.fetch_all(
                db,
                """SELECT account_id, name, platform, account_type, status, schedulable, concurrency,
                          current_concurrency, rate_multiplier, group_ids_json, last_seen_at
                   FROM accounts ORDER BY name COLLATE NOCASE, account_id""",
            )
        by_account: dict[int, list[dict[str, Any]]] = {}
        alerts: list[dict[str, Any]] = []
        if latest_run:
            result_rows = await database.fetch_all(
                db,
                "SELECT account_id, probe, status, severity, summary, model, expected_model, metrics_json, evidence_json "
                "FROM probe_results WHERE run_id=? ORDER BY account_id, probe",
                (latest_run["id"],),
            )
            for row in result_rows:
                by_account.setdefault(int(row["account_id"]), []).append(
                    {
                        "probe": row["probe"],
                        "status": row["status"],
                        "severity": row["severity"],
                        "summary": row["summary"],
                        "model": row["model"],
                        "expected_model": row["expected_model"],
                        "metrics": _parse_json(row["metrics_json"]),
                        "evidence": _parse_json(row["evidence_json"]),
                    }
                )
            alert_rows = await database.fetch_all(
                db,
                "SELECT id, account_id, rule_id, severity, status, message, action_taken, created_at "
                "FROM alerts WHERE run_id=? ORDER BY created_at DESC",
                (latest_run["id"],),
            )
            alerts = alert_rows
        group_configs = []
        for group_id in sorted(set(settings.gateway_api_keys) | set(settings.group_profiles)):
            profile = _profile(settings, group_id)
            prices = profile.pricing if profile else {}
            prices_configured = bool(
                profile
                and "input_per_million" in prices
                and ("output_per_million" in prices or profile.declared_completion_ratio is not None)
                and ("cache_read_per_million" in prices or profile.declared_cache_ratio is not None)
                and (
                    profile.cache_protocol == "openai"
                    or "cache_write_per_million" in prices
                )
            )
            group_configs.append(
                {
                    "group_id": group_id,
                    "key_configured": group_id in settings.gateway_api_keys,
                    "model": profile.model if profile else "",
                    "expected_model": profile.expected_model if profile else "",
                    "cache_protocol": profile.cache_protocol if profile else "",
                    "cache_min_prefix_tokens": profile.cache_min_prefix_tokens if profile else None,
                    "prices_configured": prices_configured,
                }
            )
        options = await get_options(db, settings)
        return {
            "configured": _configured(settings),
            "scan_running": request.app.state.coordinator.is_running,
            "settings": options,
            "latest_run": latest_run,
            "runs": runs,
            "accounts": [
                {
                    "id": int(row["account_id"]),
                    "name": row["name"],
                    "platform": row["platform"],
                    "account_type": row["account_type"],
                    "status": row["status"],
                    "schedulable": bool(row["schedulable"]),
                    "concurrency": row["concurrency"],
                    "current_concurrency": row["current_concurrency"],
                    "rate_multiplier": row["rate_multiplier"],
                    "group_ids": _parse_json(row["group_ids_json"]),
                    "last_seen_at": row["last_seen_at"],
                    "checks": by_account.get(int(row["account_id"]), []),
                }
                for row in accounts
            ],
            "alerts": alerts,
            "groups": group_configs,
        }

    @app.get("/api/settings")
    async def read_settings(request: Request) -> dict[str, Any]:
        await require_admin(request)
        return await get_options(request.app.state.db, request.app.state.settings)

    @app.get("/api/models")
    async def available_models(request: Request) -> dict[str, Any]:
        await require_admin(request)
        models = sorted({profile.get("model", "").strip() for profile in request.app.state.settings.group_profiles.values() if isinstance(profile, dict) and isinstance(profile.get("model"), str) and profile.get("model", "").strip()})
        return {"models": models}

    @app.post("/api/settings")
    async def update_settings(request: Request) -> dict[str, Any]:
        await require_admin(request)
        require_same_origin(request, request.app.state.settings)
        try:
            body = await request.json()
        except ValueError as exc:
            raise HTTPException(status_code=422, detail="settings_json_invalid") from exc
        if not isinstance(body, dict):
            raise HTTPException(status_code=422, detail="settings_must_be_object")
        allowed = {"interval_minutes", "scheduler_enabled", "auto_stop_enabled", "stop_rules", "target_model"}
        if set(body) - allowed:
            raise HTTPException(status_code=422, detail="settings_field_not_allowed")
        db: aiosqlite.Connection = request.app.state.db
        current = await get_options(db, request.app.state.settings)
        updated = merge_options(current, body)
        await set_options(db, updated)
        request.app.state.coordinator.wake_scheduler()
        return updated

    @app.post("/api/scan")
    async def scan_now(request: Request) -> JSONResponse:
        await require_admin(request)
        require_same_origin(request, request.app.state.settings)
        try:
            body = await request.json()
        except ValueError:
            body = {}
        if not isinstance(body, dict):
            raise HTTPException(status_code=422, detail="scan_body_invalid")
        if "target_model" in body:
            current = await get_options(request.app.state.db, request.app.state.settings)
            await set_options(request.app.state.db, merge_options(current, {"target_model": body["target_model"]}))
        try:
            request.app.state.coordinator.launch_manual_scan()
        except ScanAlreadyRunning as exc:
            raise HTTPException(status_code=409, detail="scan_already_running") from exc
        return JSONResponse({"status": "scheduled"}, status_code=202, headers={"Cache-Control": "no-store"})

    @app.get("/api/runs/{run_id}")
    async def run_report(run_id: str, request: Request) -> dict[str, Any]:
        await require_admin(request)
        report = await build_report(request.app.state.db, run_id)
        if report is None:
            raise HTTPException(status_code=404, detail="run_not_found")
        return report

    @app.get("/api/report/{run_id}")
    async def download_report(run_id: str, request: Request) -> JSONResponse:
        await require_admin(request)
        report = await build_report(request.app.state.db, run_id)
        if report is None:
            raise HTTPException(status_code=404, detail="run_not_found")
        return JSONResponse(
            report,
            headers={
                "Cache-Control": "no-store",
                "Content-Disposition": f'attachment; filename="modelaudit-{run_id}.json"',
            },
        )

    @app.post("/api/accounts/{account_id}/stop")
    async def stop_account(account_id: int, request: Request) -> dict[str, Any]:
        identity = await require_admin(request)
        require_same_origin(request, request.app.state.settings)
        if account_id <= 0:
            raise HTTPException(status_code=422, detail="account_id_invalid")
        sub2api: Sub2APIClient = request.app.state.sub2api
        try:
            await sub2api.set_schedulable(account_id, False)
        except Sub2APIError as exc:
            raise HTTPException(status_code=502, detail=exc.code) from exc
        db: aiosqlite.Connection = request.app.state.db
        await db.execute(
            "UPDATE accounts SET schedulable=0, last_seen_at=? WHERE account_id=?",
            (database.utc_now(), account_id),
        )
        await db.commit()
        await audit_action(
            db,
            account_id=account_id,
            actor_sub=identity.sub,
            action="manual_stop",
            result="success",
            details={"schedulable": False},
        )
        return {"account_id": account_id, "schedulable": False}

    return app


app = create_app()
