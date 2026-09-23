from __future__ import annotations

import hmac
import os
from typing import Any

from fastapi import Depends, FastAPI, Header, HTTPException, Query

from service.manager import MonitorManager


def _service_token_from_env() -> str:
    return os.environ.get("BID_MONITOR_SERVICE_TOKEN", "").strip()


def create_app(manager: MonitorManager | Any | None = None, service_token: str | None = None) -> FastAPI:
    """创建只供 OpenBidKit Fastify 调用的内部应用。"""
    owned_manager = manager is None
    monitor_manager = manager or MonitorManager(
        os.environ.get("BID_MONITOR_DATA_ROOT", "data/users")
    )
    expected_token = service_token if service_token is not None else _service_token_from_env()
    app = FastAPI(title="BidMonitor Internal Service", version="1.0")

    async def require_service_token(
        provided: str | None = Header(default=None, alias="X-BidMonitor-Service-Token"),
    ) -> None:
        if not expected_token:
            raise HTTPException(status_code=503, detail="service token is not configured")
        if provided is None or not hmac.compare_digest(provided, expected_token):
            raise HTTPException(status_code=401, detail="invalid service token")

    def user_error(exc: Exception) -> HTTPException:
        if isinstance(exc, ValueError):
            return HTTPException(status_code=400, detail=str(exc))
        return HTTPException(status_code=500, detail="monitor service operation failed")

    @app.get("/internal/health", dependencies=[Depends(require_service_token)])
    async def health() -> dict[str, str]:
        return {"status": "ok", "service": "bid-monitor"}

    @app.get("/internal/users/{user_id}/status", dependencies=[Depends(require_service_token)])
    async def status(user_id: str) -> dict[str, Any]:
        try:
            return monitor_manager.status(user_id)
        except Exception as exc:
            raise user_error(exc) from exc

    @app.post("/internal/users/{user_id}/start", dependencies=[Depends(require_service_token)])
    async def start(user_id: str, body: dict[str, Any] | None = None) -> dict[str, bool]:
        try:
            runtime_config = body.get("runtime_config") if isinstance(body, dict) else None
            accepted = monitor_manager.start(user_id, runtime_config) if runtime_config is not None else monitor_manager.start(user_id)
            return {"accepted": bool(accepted)}
        except Exception as exc:
            raise user_error(exc) from exc

    @app.post("/internal/users/{user_id}/stop", dependencies=[Depends(require_service_token)])
    async def stop(user_id: str) -> dict[str, bool]:
        try:
            return {"accepted": bool(monitor_manager.stop(user_id))}
        except Exception as exc:
            raise user_error(exc) from exc

    @app.post("/internal/users/{user_id}/run-once", dependencies=[Depends(require_service_token)])
    async def run_once(user_id: str, body: dict[str, Any] | None = None) -> dict[str, bool]:
        try:
            runtime_config = body.get("runtime_config") if isinstance(body, dict) else None
            accepted = monitor_manager.run_once(user_id, runtime_config) if runtime_config is not None else monitor_manager.run_once(user_id)
            return {"accepted": bool(accepted)}
        except Exception as exc:
            raise user_error(exc) from exc

    @app.put("/internal/users/{user_id}/config", dependencies=[Depends(require_service_token)])
    async def update_config(user_id: str, config: dict[str, Any]) -> dict[str, Any]:
        try:
            return {"config": monitor_manager.update_config(user_id, config)}
        except Exception as exc:
            raise user_error(exc) from exc

    @app.get("/internal/users/{user_id}/sites", dependencies=[Depends(require_service_token)])
    async def sites(user_id: str) -> dict[str, Any]:
        try:
            return monitor_manager.sites(user_id)
        except Exception as exc:
            raise user_error(exc) from exc

    @app.put("/internal/users/{user_id}/sites", dependencies=[Depends(require_service_token)])
    async def update_sites(user_id: str, body: dict[str, Any]) -> dict[str, Any]:
        try:
            enabled_sites = body.get("enabled_sites") if isinstance(body, dict) else []
            custom_sites = body.get("custom_sites") if isinstance(body, dict) else []
            if not isinstance(enabled_sites, list) or not isinstance(custom_sites, list):
                raise ValueError("site configuration must contain arrays")
            return monitor_manager.update_sites(user_id, enabled_sites, custom_sites)
        except Exception as exc:
            raise user_error(exc) from exc

    @app.post("/internal/users/{user_id}/test-notification", dependencies=[Depends(require_service_token)])
    async def test_notification(user_id: str, body: dict[str, Any]) -> dict[str, Any]:
        try:
            return monitor_manager.test_notification(
                user_id,
                str(body.get("channel") or ""),
                str(body.get("target") or ""),
                body.get("runtime_config") if isinstance(body, dict) else None,
            )
        except Exception as exc:
            raise user_error(exc) from exc

    @app.post("/internal/users/{user_id}/test-ai", dependencies=[Depends(require_service_token)])
    async def test_ai(user_id: str, body: dict[str, Any] | None = None) -> dict[str, Any]:
        try:
            runtime_config = body.get("runtime_config") if isinstance(body, dict) else None
            return monitor_manager.test_ai(user_id, runtime_config)
        except Exception as exc:
            raise user_error(exc) from exc

    @app.get("/internal/users/{user_id}/results", dependencies=[Depends(require_service_token)])
    async def results(
        user_id: str,
        limit: int = Query(default=50, ge=1, le=200),
        offset: int = Query(default=0, ge=0),
    ) -> dict[str, Any]:
        try:
            return monitor_manager.results(user_id, limit, offset)
        except Exception as exc:
            raise user_error(exc) from exc

    @app.get("/internal/users/{user_id}/logs", dependencies=[Depends(require_service_token)])
    async def logs(
        user_id: str,
        limit: int = Query(default=100, ge=1, le=300),
    ) -> dict[str, list[str]]:
        try:
            return {"logs": monitor_manager.logs(user_id, limit)}
        except Exception as exc:
            raise user_error(exc) from exc

    @app.delete("/internal/users/{user_id}/history", dependencies=[Depends(require_service_token)])
    async def clear_history(user_id: str) -> dict[str, bool]:
        try:
            monitor_manager.clear_history(user_id)
            return {"success": True}
        except Exception as exc:
            raise user_error(exc) from exc

    if owned_manager:
        @app.on_event("shutdown")
        async def close_manager() -> None:
            monitor_manager.close()

    return app


app = create_app()


if __name__ == "__main__":
    import uvicorn

    uvicorn.run(
        "service.app:app",
        host=os.environ.get("BID_MONITOR_HOST", "127.0.0.1"),
        port=int(os.environ.get("BID_MONITOR_PORT", "8080")),
    )
