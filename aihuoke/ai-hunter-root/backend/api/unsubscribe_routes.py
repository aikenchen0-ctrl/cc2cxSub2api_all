"""Public unsubscribe + operator suppression list.

GET/POST /api/v1/unsubscribe is public (tokenised). Operator list/add
requires API access. Original to this tree — OpenOutreach is thought only.
"""

from __future__ import annotations

from datetime import datetime, timezone
from typing import Any

from fastapi import APIRouter, Depends, HTTPException, Query, Request, status
from pydantic import BaseModel

from api.security import require_api_access
from config.settings import get_settings
from emailing.store import EmailStore
from emailing.suppression import (
    apply_unsubscribe,
    is_valid_email,
    normalize_email,
    verify_unsubscribe_token,
)

router = APIRouter(prefix="/api/v1", tags=["unsubscribe"])


def _store() -> EmailStore:
    store = EmailStore(get_settings().email_db_path)
    store.init_db()
    return store


def _now_iso() -> str:
    return datetime.now(timezone.utc).isoformat()


def _secret(store: EmailStore) -> str:
    configured = str(getattr(get_settings(), "email_unsubscribe_secret", "") or "").strip()
    return configured or store.unsubscribe_secret()


class OperatorSuppressBody(BaseModel):
    email: str
    reason: str = "operator"


def _confirm(
    store: EmailStore,
    email: str,
    token: str,
    *,
    source: str,
) -> dict[str, Any]:
    normalised = normalize_email(email)
    if not is_valid_email(normalised):
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail="invalid email")
    if not verify_unsubscribe_token(normalised, token, secret=_secret(store)):
        raise HTTPException(status_code=status.HTTP_403_FORBIDDEN, detail="invalid unsubscribe token")
    record = apply_unsubscribe(
        store,
        normalised,
        reason="unsubscribed",
        source=source,
        created_at=_now_iso(),
    )
    return {
        "ok": True,
        "email": normalised,
        "suppressed": True,
        "sequences_stopped": int(record.get("sequences_stopped", 0) or 0),
        "message": "You have been unsubscribed. We will not email this address again.",
    }


@router.get("/unsubscribe")
async def unsubscribe_get(
    email: str = Query(default=""),
    token: str = Query(default=""),
):
    store = _store()
    return _confirm(store, email, token, source="one_click")


@router.post("/unsubscribe")
async def unsubscribe_post(request: Request):
    """RFC 8058 one-click (form) plus JSON body for our own UI."""
    store = _store()
    email = str(request.query_params.get("email") or "")
    token = str(request.query_params.get("token") or "")
    content_type = (request.headers.get("content-type") or "").lower()
    if "application/json" in content_type:
        try:
            payload = await request.json()
        except Exception:
            payload = {}
        if isinstance(payload, dict):
            email = str(payload.get("email") or email)
            token = str(payload.get("token") or token)
    elif "application/x-www-form-urlencoded" in content_type or "multipart/form-data" in content_type:
        form = await request.form()
        email = str(form.get("email") or email)
        token = str(form.get("token") or token)
    return _confirm(store, email, token, source="one_click")


@router.get("/suppressions", dependencies=[Depends(require_api_access)])
async def list_suppressions(limit: int = Query(default=200, ge=1, le=1000)):
    store = _store()
    return {"items": store.list_suppressions(limit=limit)}


@router.post("/suppressions", dependencies=[Depends(require_api_access)])
async def add_operator_suppression(payload: OperatorSuppressBody):
    store = _store()
    email = normalize_email(payload.email)
    if not is_valid_email(email):
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail="invalid email")
    record = apply_unsubscribe(
        store,
        email,
        reason=payload.reason or "operator",
        source="operator",
        created_at=_now_iso(),
    )
    return {
        "ok": True,
        "email": email,
        "suppressed": True,
        "sequences_stopped": int(record.get("sequences_stopped", 0) or 0),
        "item": store.get_suppression(email),
    }
