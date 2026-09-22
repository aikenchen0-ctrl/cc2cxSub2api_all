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
from api.hunt_store import load_all_hunts
from config.settings import get_settings
from emailing.store import EmailStore
from auth_sso import current_subject, known_subjects, managed as satellite_managed, scoped_email_db_path
from emailing.suppression import (
    apply_unsubscribe,
    is_valid_email,
    normalize_email,
    verify_unsubscribe_token,
)

router = APIRouter(prefix="/api/v1", tags=["unsubscribe"])


def _store() -> EmailStore:
    subject = current_subject().strip() if satellite_managed() else ""
    store = EmailStore(scoped_email_db_path(get_settings().email_db_path, subject))
    store.init_db()
    return store


def _public_stores() -> list[EmailStore]:
    """Find managed stores for a public unsubscribe token.

    The public link intentionally carries only the recipient and a signed
    token, not a user id.  Try each known subject store and apply the token
    only where it verifies; local mode retains the historical single store.
    """
    if not satellite_managed():
        return [_store()]
    subjects = set(known_subjects())
    subjects.update(
        str(hunt.get("owner_user_id") or "").strip()
        for hunt in load_all_hunts(mark_interrupted=False).values()
        if str(hunt.get("owner_user_id") or "").strip()
    )
    stores: list[EmailStore] = []
    for subject in sorted(subjects):
        store = EmailStore(scoped_email_db_path(get_settings().email_db_path, subject))
        store.init_db()
        stores.append(store)
    return stores


def _now_iso() -> str:
    return datetime.now(timezone.utc).isoformat()


def _secret(store: EmailStore) -> str:
    if satellite_managed():
        return store.unsubscribe_secret()
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
    stores = _public_stores()
    matches = [store for store in stores if verify_unsubscribe_token(normalize_email(email), token, secret=_secret(store))]
    if not matches:
        raise HTTPException(status_code=status.HTTP_403_FORBIDDEN, detail="invalid unsubscribe token")
    results = [_confirm(store, email, token, source="one_click") for store in matches]
    return {
        "ok": True,
        "email": normalize_email(email),
        "suppressed": True,
        "sequences_stopped": sum(int(result.get("sequences_stopped", 0) or 0) for result in results),
        "message": "You have been unsubscribed. We will not email this address again.",
    }


@router.post("/unsubscribe")
async def unsubscribe_post(request: Request):
    """RFC 8058 one-click (form) plus JSON body for our own UI."""
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
    stores = _public_stores()
    matches = [store for store in stores if verify_unsubscribe_token(normalize_email(email), token, secret=_secret(store))]
    if not matches:
        raise HTTPException(status_code=status.HTTP_403_FORBIDDEN, detail="invalid unsubscribe token")
    results = [_confirm(store, email, token, source="one_click") for store in matches]
    return {
        "ok": True,
        "email": normalize_email(email),
        "suppressed": True,
        "sequences_stopped": sum(int(result.get("sequences_stopped", 0) or 0) for result in results),
        "message": "You have been unsubscribed. We will not email this address again.",
    }


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
