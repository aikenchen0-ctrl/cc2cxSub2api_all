import math
import os
from urllib.parse import urlsplit, urlunsplit

import httpx
from fastapi import APIRouter, Depends, HTTPException
from fastapi.responses import JSONResponse

from api.v1.auth.users import User, get_current_user
from utils.sub2api_satellite import openai_base_url, satellite_headers

ROUTER = APIRouter(prefix="/api/v1/sub2api", tags=["Sub2API"])


def _purchase_url() -> str:
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


@ROUTER.get("/balance")
async def get_balance(_user: User = Depends(get_current_user)):
    base_url = openai_base_url().rstrip("/")
    if not base_url:
        raise HTTPException(status_code=503, detail="Sub2API relay is unavailable")
    try:
        headers = satellite_headers(required=True)
    except RuntimeError as exc:
        raise HTTPException(status_code=503, detail="Sub2API satellite credentials are unavailable") from exc

    try:
        async with httpx.AsyncClient(timeout=httpx.Timeout(12.0)) as client:
            upstream = await client.get(f"{base_url}/sub2api/balance", headers=headers)
        if upstream.status_code != 200:
            status = upstream.status_code if upstream.status_code in {401, 503} else 502
            raise HTTPException(status_code=status, detail="Sub2API balance is unavailable")
        value = upstream.json().get("balance")
        balance = float(value)
        if not math.isfinite(balance):
            raise ValueError("balance is not finite")
    except HTTPException:
        raise
    except (httpx.HTTPError, ValueError, TypeError, AttributeError) as exc:
        raise HTTPException(status_code=502, detail="Sub2API balance is unavailable") from exc

    return JSONResponse(
        content={"balance": balance, "recharge_url": _purchase_url()},
        headers={"Cache-Control": "no-store"},
    )
