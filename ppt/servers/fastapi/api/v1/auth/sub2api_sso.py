import base64
import hashlib
import hmac
import json
import os
import secrets
import time
import uuid
from urllib.parse import urlsplit

from fastapi import APIRouter, Depends, Request
from sqlalchemy import delete, select
from sqlalchemy.exc import IntegrityError
from sqlalchemy.ext.asyncio import AsyncSession
from starlette.responses import JSONResponse, RedirectResponse

from api.v1.auth.config import SESSION_COOKIE_NAME, SESSION_TTL_SECONDS
from api.v1.auth.users import PASSWORD_HELPER, get_jwt_strategy
from models.sql.key_value import KeyValueSqlModel
from models.sql.user import User
from services.database import get_async_session
from utils.sub2api_satellite import user_map_id as _user_map_id

SUB2API_SSO_ROUTER = APIRouter()
_NAMESPACE = uuid.UUID("ae47cb68-42e0-44cd-9572-e905c6684da9")
_HEADERS = {"Cache-Control": "no-store", "Referrer-Policy": "no-referrer"}
HANDOFF_COOKIE_NAME = "presenton_sso_handoff"
HANDOFF_KEY = "sub2api_sso_handoff"
HANDOFF_TTL_SECONDS = 60


def _sso_username(payload: dict, subject: str) -> str:
    display = payload.get("displayName") or payload.get("username")
    if not isinstance(display, str):
        display = ""
    display = " ".join(display.split())
    display = "".join(char for char in display if char.isprintable())[:100].strip()
    suffix = hashlib.sha256(subject.encode()).hexdigest()[:10]
    return f"{display or 'sub2api'}-{suffix}"


def verify_ticket(raw: str, secret: str, now: int | None = None) -> dict:
    if len(secret) < 32 or len(raw) > 8192:
        raise ValueError("SSO unavailable")
    encoded, signature = raw.split(".")
    supplied = base64.b64decode(signature + "=" * (-len(signature) % 4), altchars=b"-_", validate=True)
    expected = hmac.digest(secret.encode(), encoded.encode(), "sha256")
    if not hmac.compare_digest(supplied, expected):
        raise ValueError("Invalid signature")
    payload = json.loads(base64.b64decode(encoded + "=" * (-len(encoded) % 4), altchars=b"-_", validate=True))
    if (
        not isinstance(payload, dict)
        or payload.get("iss") != "sub2api"
        or payload.get("aud") != "presenton"
    ):
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


def _secure_request(request: Request) -> bool:
    return request.url.scheme == "https" or request.headers.get("x-forwarded-proto", "").lower() == "https"


def _clear_handoff_cookie(response: JSONResponse, request: Request) -> None:
    response.delete_cookie(
        HANDOFF_COOKIE_NAME,
        path="/api/v1/auth/sso/exchange",
        secure=_secure_request(request),
        httponly=True,
        samesite="strict",
    )


def _same_origin(request: Request) -> bool:
    origin = request.headers.get("origin", "").strip()
    if not origin or origin == "null":
        return True
    parsed = urlsplit(origin)
    if parsed.scheme not in {"http", "https"} or not parsed.netloc or parsed.path not in {"", "/"}:
        return False
    forwarded_scheme = request.headers.get("x-forwarded-proto", "").split(",", 1)[0].strip().lower()
    expected_scheme = forwarded_scheme or request.url.scheme
    forwarded_host = request.headers.get("x-forwarded-host", "").split(",", 1)[0].strip()
    expected_host = forwarded_host or request.headers.get("host", "") or request.url.netloc
    return parsed.scheme == expected_scheme and parsed.netloc.lower() == expected_host.lower()


@SUB2API_SSO_ROUTER.get("/sso/callback")
async def sub2api_sso_callback(request: Request, session: AsyncSession = Depends(get_async_session)):
    try:
        payload = verify_ticket(request.query_params.get("ticket", ""), os.environ.get("SUB2API_SSO_SECRET", "").strip())
    except (ValueError, TypeError, UnicodeError):
        return RedirectResponse("/?error=SSO_failed", status_code=303, headers=_HEADERS)
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
            user = User(id=uuid.uuid4(), username=_sso_username(payload, payload["sub"]), hashed_password=PASSWORD_HELPER.hash(secrets.token_urlsafe(32)), is_active=True, is_verified=True, is_superuser=False)
            session.add(user)
            session.add(KeyValueSqlModel(id=identity_id, key="sub2api_sso_identity", value={"user_id": str(user.id), "sub": payload["sub"]}))
            session.add(KeyValueSqlModel(id=_user_map_id(user.id), key="sub2api_sso_user", value={"sub": payload["sub"]}))
        else:
            if identity.key != "sub2api_sso_identity" or identity.value.get("sub") != payload["sub"]:
                raise ValueError("Invalid identity mapping")
            user = await session.get(User, uuid.UUID(identity.value["user_id"]))
            if user is not None:
                user.username = _sso_username(payload, payload["sub"])
                map_row = await session.get(KeyValueSqlModel, _user_map_id(user.id))
                if map_row is None:
                    session.add(KeyValueSqlModel(id=_user_map_id(user.id), key="sub2api_sso_user", value={"sub": payload["sub"]}))
                else:
                    map_row.value = {"sub": payload["sub"]}
        if user is None or not user.is_active or user.is_superuser:
            raise ValueError("SSO account unavailable")
        await session.commit()
    except (IntegrityError, ValueError, KeyError):
        await session.rollback()
        return RedirectResponse("/?error=SSO_failed", status_code=303, headers=_HEADERS)
    handoff_id = uuid.uuid4()
    session.add(KeyValueSqlModel(
        id=handoff_id,
        key=HANDOFF_KEY,
        value={"user_id": str(user.id), "next": safe_next(payload.get("next")), "exp": int(time.time()) + HANDOFF_TTL_SECONDS},
    ))
    try:
        await session.commit()
    except IntegrityError:
        await session.rollback()
        return RedirectResponse("/?error=SSO_failed", status_code=303, headers=_HEADERS)
    response = RedirectResponse("/?sso=1", status_code=303, headers=_HEADERS)
    response.set_cookie(HANDOFF_COOKIE_NAME, str(handoff_id), max_age=HANDOFF_TTL_SECONDS, httponly=True, secure=_secure_request(request), samesite="strict", path="/api/v1/auth/sso/exchange")
    return response


@SUB2API_SSO_ROUTER.post("/sso/exchange")
async def sub2api_sso_exchange(request: Request, session: AsyncSession = Depends(get_async_session)):
    response_headers = dict(_HEADERS)
    if request.headers.get("x-presenton-sso") != "1" or request.headers.get("sec-fetch-site", "").lower() == "cross-site" or not _same_origin(request):
        response = JSONResponse({"detail": "SSO exchange rejected"}, status_code=403, headers=response_headers)
        _clear_handoff_cookie(response, request)
        return response
    response = JSONResponse({"detail": "SSO handoff expired"}, status_code=401, headers=response_headers)
    _clear_handoff_cookie(response, request)
    raw_id = request.cookies.get(HANDOFF_COOKIE_NAME, "")
    try:
        handoff_id = uuid.UUID(raw_id)
    except (ValueError, AttributeError):
        return response
    result = await session.execute(
        delete(KeyValueSqlModel)
        .where(KeyValueSqlModel.id == handoff_id, KeyValueSqlModel.key == HANDOFF_KEY)
        .returning(KeyValueSqlModel.value)
    )
    value = result.scalar_one_or_none()
    await session.commit()
    if not isinstance(value, dict) or not isinstance(value.get("exp"), int) or value["exp"] < int(time.time()):
        return response
    try:
        user = await session.get(User, uuid.UUID(str(value["user_id"])))
    except (ValueError, KeyError, TypeError):
        user = None
    if user is None or not user.is_active or user.is_superuser:
        return response
    token = await get_jwt_strategy().write_token(user)
    response = JSONResponse({"authenticated": True, **{"id": str(user.id), "username": user.username, "role": "user", "created_at": user.created_at.isoformat() if user.created_at else None}, "redirect": safe_next(value.get("next"))}, headers=response_headers)
    _clear_handoff_cookie(response, request)
    response.set_cookie(SESSION_COOKIE_NAME, token, max_age=SESSION_TTL_SECONDS, httponly=True, secure=_secure_request(request), samesite="lax", path="/")
    return response
