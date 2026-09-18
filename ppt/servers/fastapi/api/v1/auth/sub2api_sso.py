import base64
import hashlib
import hmac
import json
import os
import secrets
import time
import uuid

from fastapi import APIRouter, Depends, Request
from sqlalchemy import delete, select
from sqlalchemy.exc import IntegrityError
from sqlalchemy.ext.asyncio import AsyncSession
from starlette.responses import RedirectResponse

from api.v1.auth.config import SESSION_COOKIE_NAME, SESSION_TTL_SECONDS
from api.v1.auth.users import PASSWORD_HELPER, get_jwt_strategy
from models.sql.key_value import KeyValueSqlModel
from models.sql.user import User
from services.database import get_async_session

SUB2API_SSO_ROUTER = APIRouter()
_NAMESPACE = uuid.UUID("ae47cb68-42e0-44cd-9572-e905c6684da9")
_HEADERS = {"Cache-Control": "no-store", "Referrer-Policy": "no-referrer"}


def verify_ticket(raw: str, secret: str, now: int | None = None) -> dict:
    if len(secret) < 32 or len(raw) > 8192:
        raise ValueError("SSO unavailable")
    encoded, signature = raw.split(".")
    supplied = base64.b64decode(signature + "=" * (-len(signature) % 4), altchars=b"-_", validate=True)
    expected = hmac.digest(secret.encode(), encoded.encode(), "sha256")
    if not hmac.compare_digest(supplied, expected):
        raise ValueError("Invalid signature")
    payload = json.loads(base64.b64decode(encoded + "=" * (-len(encoded) % 4), altchars=b"-_", validate=True))
    if not isinstance(payload, dict) or payload.get("aud") != "presenton":
        raise ValueError("Invalid audience")
    for name in ("sub", "jti"):
        value = payload.get(name)
        if not isinstance(value, str) or not value.strip() or len(value) > 160 or any(ord(c) < 32 for c in value):
            raise ValueError("Invalid identity")
    issued, expires = payload.get("iat"), payload.get("exp")
    current = int(time.time()) if now is None else now
    if type(issued) is not int or type(expires) is not int or issued <= 0 or expires <= current or not 0 < expires-issued <= 120 or issued > current+30:
        raise ValueError("Expired ticket")
    return payload


def safe_next(value: object) -> str:
    if not isinstance(value, str) or not value.startswith("/") or value.startswith("//") or "\\" in value or len(value) > 2048 or any(ord(c) < 32 for c in value):
        return "/upload"
    return value


@SUB2API_SSO_ROUTER.get("/sso/callback")
async def sub2api_sso_callback(request: Request, session: AsyncSession = Depends(get_async_session)):
    try:
        payload = verify_ticket(request.query_params.get("ticket", ""), os.environ.get("SUB2API_SSO_SECRET", "").strip())
    except (ValueError, TypeError, UnicodeError):
        return RedirectResponse("/login?error=SSO_failed", status_code=303, headers=_HEADERS)
    # Primary-key uniqueness consumes each ticket atomically across processes/restarts.
    nonce_id = uuid.uuid5(_NAMESPACE, "nonce:" + hashlib.sha256(payload["jti"].encode()).hexdigest())
    identity_id = uuid.uuid5(_NAMESPACE, "identity:" + payload["sub"])
    try:
        # Do not let an SSO user consume the first-account administrator setup.
        admin = await session.scalar(select(User.id).where(User.is_superuser.is_(True)).limit(1))
        if admin is None:
            raise ValueError("Administrator setup required")
        await session.execute(delete(KeyValueSqlModel).where(
            KeyValueSqlModel.key == "sub2api_sso_nonce",
            KeyValueSqlModel.value["exp"].as_integer() < int(time.time()),
        ))
        session.add(KeyValueSqlModel(id=nonce_id, key="sub2api_sso_nonce", value={"exp": payload["exp"]}))
        await session.flush()
        identity = await session.get(KeyValueSqlModel, identity_id)
        if identity is None:
            user = User(id=uuid.uuid4(), username="sub2api-" + hashlib.sha256(payload["sub"].encode()).hexdigest(), hashed_password=PASSWORD_HELPER.hash(secrets.token_urlsafe(32)), is_active=True, is_verified=True, is_superuser=False)
            session.add(user)
            session.add(KeyValueSqlModel(id=identity_id, key="sub2api_sso_identity", value={"user_id": str(user.id), "sub": payload["sub"]}))
        else:
            if identity.key != "sub2api_sso_identity" or identity.value.get("sub") != payload["sub"]:
                raise ValueError("Invalid identity mapping")
            user = await session.get(User, uuid.UUID(identity.value["user_id"]))
        if user is None or not user.is_active or user.is_superuser:
            raise ValueError("SSO account unavailable")
        await session.commit()
    except (IntegrityError, ValueError, KeyError):
        await session.rollback()
        return RedirectResponse("/login?error=SSO_failed", status_code=303, headers=_HEADERS)
    token = await get_jwt_strategy().write_token(user)
    response = RedirectResponse(safe_next(payload.get("next")), status_code=303, headers=_HEADERS)
    response.set_cookie(SESSION_COOKIE_NAME, token, max_age=SESSION_TTL_SECONDS, httponly=True, secure=request.url.scheme == "https" or request.headers.get("x-forwarded-proto", "").lower() == "https", samesite="lax", path="/")
    return response
