from fastapi import APIRouter, Request
from fastapi.responses import RedirectResponse
from pydantic import BaseModel

import os

from auth import consume_ticket, create_session, get_identity, logout, require_csrf, revoke_sessions_for_subject, _safe_next, verify_ticket
from config import SCREEN2CODE_AUTH_REQUIRED, SUB2API_RELAY_BASE_URL

router = APIRouter(prefix="/api/auth", tags=["auth"])
account_router = APIRouter(prefix="/api/account", tags=["account"])


class SSOLogoutRequest(BaseModel):
    ticket: str


@router.get("/sso/callback")
async def sso_callback(request: Request, ticket: str):
    payload = verify_ticket(ticket)
    identity = consume_ticket(payload)
    target = _safe_next(payload.get("next") or request.query_params.get("next"))
    response = RedirectResponse(target, status_code=303)
    create_session(identity, response, request)
    response.headers["Cache-Control"] = "no-store"
    response.headers["Referrer-Policy"] = "no-referrer"
    return response


@router.get("/me")
async def me(request: Request):
    identity = get_identity(request)
    return {"authenticated": identity is not None, "user": identity.public() if identity else None}


@router.get("/csrf")
async def csrf(request: Request):
    identity = get_identity(request)
    if identity is None:
        return {"authenticated": False}
    token = request.cookies.get("screen2code_csrf")
    return {"authenticated": True, "csrf_token": token}


@account_router.get("/service-status")
async def service_status(request: Request):
    identity = get_identity(request)
    if identity is None:
        return {"status": "unauthenticated"}
    if not SCREEN2CODE_AUTH_REQUIRED:
        return {"status": "ready"}
    credential = os.environ.get("SUB2API_APP_CREDENTIAL", "").strip()
    subject = str(getattr(identity, "subject", "") or "").strip()
    if credential and subject and SUB2API_RELAY_BASE_URL:
        return {"status": "ready"}
    return {"status": "temporarily_unavailable"}


@router.post("/logout")
async def logout_route(request: Request):
    from fastapi import Response

    require_csrf(request)
    response = Response(status_code=204)
    logout(request, response)
    return response


@router.post("/sso/logout")
async def sso_logout(request: SSOLogoutRequest):
    payload = verify_ticket(request.ticket, expected_audience="screen2code:logout")
    consume_ticket(payload)
    revoked = revoke_sessions_for_subject(payload["sub"])
    return {"revoked": revoked}
