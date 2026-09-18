# Load environment variables first
from dotenv import load_dotenv

load_dotenv()


from fastapi import FastAPI, Request
from fastapi.responses import JSONResponse
from fastapi.middleware.cors import CORSMiddleware
from config import IS_DEBUG_ENABLED, SCREEN2CODE_ALLOWED_ORIGINS, SCREEN2CODE_AUTH_REQUIRED
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


@app.middleware("http")
async def require_hosted_session(request: Request, call_next):
    """Keep hosted API routes behind the local session established by SSO."""
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
                )
    return await call_next(request)


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
