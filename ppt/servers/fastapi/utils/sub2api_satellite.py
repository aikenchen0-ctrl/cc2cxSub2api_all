import os
from contextvars import ContextVar, Token
from urllib.parse import urlsplit
from uuid import UUID, uuid5

import httpx
from sqlalchemy.ext.asyncio import AsyncSession

from models.sql.key_value import KeyValueSqlModel

_NAMESPACE = UUID("ae47cb68-42e0-44cd-9572-e905c6684da9")
_CURRENT_SUBJECT: ContextVar[str] = ContextVar("presenton_sub2api_subject", default="")
_HOOK_INSTALLED = False


def set_current_sub2api_subject(subject: str) -> Token:
    return _CURRENT_SUBJECT.set(str(subject or "").strip())


def get_current_sub2api_subject() -> str:
    return _CURRENT_SUBJECT.get()


def reset_current_sub2api_subject(token: Token) -> None:
    _CURRENT_SUBJECT.reset(token)


def user_map_id(user_id: UUID) -> UUID:
    return uuid5(_NAMESPACE, "user:" + str(user_id))


async def load_subject_for_user(session: AsyncSession, user_id: UUID) -> str:
    row = await session.get(KeyValueSqlModel, user_map_id(user_id))
    if row is None or not isinstance(row.value, dict):
        return ""
    return str(row.value.get("sub") or "").strip()


def satellite_configured() -> bool:
    return bool(
        (os.getenv("SUB2API_APP_CREDENTIAL", "").strip()
         or os.getenv("SUB2API_SSO_SECRET", "").strip())
        and openai_base_url()
    )


def satellite_managed() -> bool:
    return bool(
        os.getenv("SUB2API_APP_CREDENTIAL", "").strip()
        or os.getenv("SUB2API_SSO_SECRET", "").strip()
    )


def openai_base_url() -> str:
    # A managed satellite must always terminate at Sub2API.  Do not let a
    # persisted CUSTOM_LLM_URL redirect the app credential to an arbitrary
    # external endpoint; CUSTOM_LLM_URL is only a local/unmanaged override.
    if satellite_managed():
        raw = (
            os.getenv("SUB2API_RELAY_BASE_URL")
            or os.getenv("LINK")
            or os.getenv("SUB2API_BASE_URL")
            or ""
        ).strip()
    else:
        raw = (
            os.getenv("CUSTOM_LLM_URL")
            or os.getenv("LINK")
            or os.getenv("SUB2API_BASE_URL")
            or os.getenv("SUB2API_RELAY_BASE_URL")
            or ""
        ).strip()
    if raw and "://" not in raw:
        raw = "http://" + raw
    raw = raw.rstrip("/")
    if not raw:
        return ""
    if raw.endswith("/v1"):
        return raw
    return raw + "/v1"


def satellite_headers(*, required: bool = False) -> dict[str, str]:
    subject = get_current_sub2api_subject()
    credential = os.getenv("SUB2API_APP_CREDENTIAL", "").strip()
    if not subject or not credential:
        if required or satellite_managed():
            if required:
                raise RuntimeError("Sub2API satellite credential and session subject are required")
        return {}
    return {
        "Authorization": f"Bearer {credential}",
        "X-Sub2API-On-Behalf-Of": subject,
        "X-Sub2API-Satellite": "ppt",
    }


def _is_sub2api_request(url: httpx.URL) -> bool:
    raw = openai_base_url()
    if not raw:
        return False
    parsed = urlsplit(raw)
    return bool(parsed.hostname) and url.host == parsed.hostname


def _apply_satellite_headers(request: httpx.Request) -> None:
    if not _is_sub2api_request(request.url):
        return
    headers = satellite_headers(required=satellite_configured())
    if not headers:
        return
    for key, value in headers.items():
        request.headers[key] = value


def install_satellite_httpx_headers() -> None:
    global _HOOK_INSTALLED
    if _HOOK_INSTALLED:
        return
    _HOOK_INSTALLED = True
    original_async = httpx.AsyncClient.send
    original_sync = httpx.Client.send

    async def send_async(self, request, *args, **kwargs):
        _apply_satellite_headers(request)
        return await original_async(self, request, *args, **kwargs)

    def send_sync(self, request, *args, **kwargs):
        _apply_satellite_headers(request)
        return original_sync(self, request, *args, **kwargs)

    httpx.AsyncClient.send = send_async  # type: ignore[method-assign]
    httpx.Client.send = send_sync  # type: ignore[method-assign]
