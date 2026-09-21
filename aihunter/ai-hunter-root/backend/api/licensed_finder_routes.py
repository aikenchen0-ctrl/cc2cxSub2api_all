"""Licensed-source finder bypass — not part of the LangGraph hunt.

POST /api/v1/licensed-finder/search  — BetterContact Lead Finder
POST /api/v1/licensed-finder/enrich  — waterfall enrich already-known people
GET  /api/v1/licensed-finder/status  — never spends

Default off. Email enrichment is opt-in. OpenOutreach was a thought source
only; this module is original.
"""

from __future__ import annotations

from typing import Any

from fastapi import APIRouter, Depends, HTTPException, Query
from fastapi.responses import Response
from pydantic import BaseModel, Field

from api.security import require_api_access
from config.settings import get_settings
from tools.lead_contract import leads_to_csv
from tools.licensed_finder import (
    LicensedFinderError,
    build_search_filters,
    enrich_people,
    readiness,
    search_people,
)

router = APIRouter(prefix="/api/v1/licensed-finder", tags=["licensed-finder"])


class LicensedSearchRequest(BaseModel):
    company_domains: list[str] = Field(default_factory=list)
    industries: list[str] = Field(default_factory=list)
    job_titles: list[str] = Field(default_factory=list)
    seniorities: list[str] = Field(default_factory=list)
    countries: list[str] = Field(default_factory=list)
    technologies: list[str] = Field(default_factory=list)
    filters: dict[str, Any] = Field(default_factory=dict)
    max_leads: int = Field(default=10, ge=1, le=200)
    enrich_emails: bool = False
    enrich_phones: bool = False
    reason: str = ""
    format: str = Field(default="json", description="json | csv")


class LicensedEnrichRequest(BaseModel):
    leads: list[dict[str, Any]] = Field(default_factory=list)
    enrich_emails: bool = True
    enrich_phones: bool = False
    verify_catch_all: bool = False
    format: str = Field(default="json")


def _http_status(exc: LicensedFinderError) -> int:
    mapping = {
        "disabled": 409,
        "no_credential": 409,
        "bad_config": 400,
        "bad_response": 502,
        "bad_request_id": 404,
        "not_found": 404,
        "provider_auth": 401,
        "provider_out_of_credits": 402,
        "provider_rate_limited": 429,
        "provider_unavailable": 502,
        "timeout": 504,
    }
    return mapping.get(exc.code, 502)


def _raise(exc: LicensedFinderError) -> None:
    raise HTTPException(status_code=_http_status(exc), detail={"type": exc.code, "message": exc.message})


def _format_result(payload: dict[str, Any], fmt: str) -> Any:
    if fmt == "csv":
        body = leads_to_csv(payload.get("leads") or [])
        return Response(content=body, media_type="text/csv; charset=utf-8")
    return payload


@router.get("/status", dependencies=[Depends(require_api_access)])
async def licensed_finder_status():
    return readiness(get_settings())


@router.post("/search", dependencies=[Depends(require_api_access)])
async def licensed_finder_search(request: LicensedSearchRequest, format: str | None = Query(default=None)):
    filters = dict(request.filters or {})
    composed = build_search_filters(
        company_domains=request.company_domains,
        industries=request.industries,
        job_titles=request.job_titles,
        seniorities=request.seniorities,
        countries=request.countries,
        technologies=request.technologies,
        extra=filters,
    )
    try:
        result = await search_people(
            composed,
            settings=get_settings(),
            max_leads=request.max_leads,
            enrich_emails=request.enrich_emails,
            enrich_phones=request.enrich_phones,
            reason=request.reason,
        )
    except LicensedFinderError as exc:
        _raise(exc)
        raise
    fmt = (format or request.format or "json").lower()
    return _format_result(result, fmt)


@router.post("/enrich", dependencies=[Depends(require_api_access)])
async def licensed_finder_enrich(request: LicensedEnrichRequest, format: str | None = Query(default=None)):
    try:
        result = await enrich_people(
            request.leads,
            settings=get_settings(),
            enrich_emails=request.enrich_emails,
            enrich_phones=request.enrich_phones,
            verify_catch_all=request.verify_catch_all,
        )
    except LicensedFinderError as exc:
        _raise(exc)
        raise
    fmt = (format or request.format or "json").lower()
    return _format_result(result, fmt)
