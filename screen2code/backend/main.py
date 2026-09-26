# Load environment variables first
import math
import os
from urllib.parse import urlsplit, urlunsplit

import httpx
from dotenv import load_dotenv

load_dotenv()


from fastapi import FastAPI, Request
from fastapi.responses import JSONResponse
from fastapi.middleware.cors import CORSMiddleware
from config import IS_DEBUG_ENABLED, SCREEN2CODE_ALLOWED_ORIGINS, SCREEN2CODE_AUTH_REQUIRED, SUB2API_RELAY_BASE_URL
from routes import (
    auth,
    capabilities,
    screenshot,
    generate_code,
    home,
    evals,
    export,
    design_systems,
    prompt_reports,
    agent_runs,
    eval_sets,
)
from uploaded_assets import configure_uploaded_asset_routes

app = FastAPI(openapi_url=None, docs_url=None, redoc_url=None)
configure_uploaded_asset_routes(app)


def _sub2api_purchase_url() -> str:
    raw = os.environ.get("LINK", "").strip()
    if not raw:
        return ""
    if "://" not in raw:
        raw = "http://" + raw
    try:
        parsed = urlsplit(raw)
        if parsed.scheme not in {"http", "https"} or not parsed.hostname or parsed.username or parsed.password:
            return ""
        if parsed.query or parsed.fragment:
            return ""
        return urlunsplit((parsed.scheme, parsed.netloc, "/purchase", "", ""))
    except ValueError:
        return ""


@app.get("/api/sub2api/balance")
async def sub2api_user_balance(request: Request):
    from auth import require_identity

    identity = require_identity(request)
    credential = os.environ.get("SUB2API_APP_CREDENTIAL", "").strip()
    base_url = (SUB2API_RELAY_BASE_URL or "").rstrip("/")
    if not credential or not base_url or not identity.subject.strip():
        return JSONResponse(status_code=503, content={"error": "Sub2API balance is unavailable"}, headers={"Cache-Control": "no-store"})
    headers = {
        "Authorization": f"Bearer {credential}",
        "X-Sub2API-On-Behalf-Of": identity.subject.strip(),
        "X-Sub2API-Satellite": "screen2code",
    }
    try:
        async with httpx.AsyncClient(timeout=httpx.Timeout(12.0)) as client:
            upstream = await client.get(f"{base_url}/sub2api/balance", headers=headers)
        if upstream.status_code != 200:
            status = upstream.status_code if upstream.status_code in {401, 503} else 502
            return JSONResponse(status_code=status, content={"error": "Sub2API balance is unavailable"}, headers={"Cache-Control": "no-store"})
        balance = float(upstream.json().get("balance"))
        if not math.isfinite(balance):
            raise ValueError("balance is not finite")
    except (httpx.HTTPError, ValueError, TypeError, AttributeError):
        return JSONResponse(status_code=502, content={"error": "Sub2API balance is unavailable"}, headers={"Cache-Control": "no-store"})
    return JSONResponse(content={"balance": balance, "recharge_url": _sub2api_purchase_url()}, headers={"Cache-Control": "no-store"})


@app.middleware("http")
async def require_hosted_session(request: Request, call_next):
    """Keep hosted API routes behind the local session established by SSO."""
    balance_path = request.url.path == "/api/sub2api/balance"
    if SCREEN2CODE_AUTH_REQUIRED and request.url.path.startswith("/api/"):
        public_paths = {
            "/api/auth/sso/callback",
            "/api/auth/sso/logout",
            "/api/auth/me",
            "/api/auth/csrf",
            "/api/account/service-status",
        }
        if request.url.path not in public_paths:
            from auth import get_identity

            if get_identity(request) is None:
                return JSONResponse(
                    status_code=401,
                    content={"error": {"code": "AUTH_REQUIRED", "message": "Authentication required"}},
                    headers={"Cache-Control": "no-store"} if balance_path else None,
                )
    response = await call_next(request)
    if balance_path:
        response.headers["Cache-Control"] = "no-store"
    return response


@app.on_event("startup")
async def log_debug_mode() -> None:
    debug_status = "ENABLED" if IS_DEBUG_ENABLED else "DISABLED"
    print(f"Backend startup complete. Debug mode is {debug_status}.")


@app.on_event("startup")
async def probe_screenshot_preview_on_startup() -> None:
    # Detect (and warm up) headless Chromium so the screenshot_preview tool is
    # only offered when it can actually run. Logs the outcome.
    from preview_screenshot import probe_screenshot_preview

    await probe_screenshot_preview()

# Configure CORS settings
app.add_middleware(
    CORSMiddleware,
    allow_origins=list(SCREEN2CODE_ALLOWED_ORIGINS) or (["http://localhost:5173", "http://127.0.0.1:5173"] if SCREEN2CODE_AUTH_REQUIRED else ["*"]),
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

# Add routes
app.include_router(generate_code.router)
app.include_router(screenshot.router)
app.include_router(home.router)
app.include_router(capabilities.router)
app.include_router(evals.router)
app.include_router(export.router)
app.include_router(design_systems.router)
app.include_router(prompt_reports.router)
app.include_router(agent_runs.router)
app.include_router(eval_sets.router)
app.include_router(auth.router)
app.include_router(auth.account_router)
