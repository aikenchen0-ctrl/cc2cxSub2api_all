"""Optional licensed-source finder (BetterContact).

Bypass of the public-web hunt: describe filters / a person, get rows from a
licensed waterfall. Original adapter — OpenOutreach / openoutfind are GPL-3.0
thought sources only; none of their Python was copied.

Not wired into the LangGraph hunt. Default off. Failures never abort a hunt.
Email enrichment credits are opt-in (`enrich_emails=False` by default).
"""

from __future__ import annotations

import logging
import time
from typing import Any

from tools.lead_contract import domain_from_website, finalize_lead_contract

logger = logging.getLogger(__name__)

DEFAULT_BASE_URL = "https://app.bettercontact.rocks/api/v2"
ACCEPTABLE_EMAIL_STATUSES = {"deliverable", "catch_all_safe", "valid"}
SAFE_EMAIL_STATUSES = ACCEPTABLE_EMAIL_STATUSES | {"catch_all"}
TERMINAL_OK = "terminated"
HOLD_STATUS = "on_hold"
POLL_STATUSES = {"not_started", "processing"}

class LicensedFinderError(RuntimeError):
    """Raised when the licensed source cannot complete a request."""

    def __init__(self, code: str, message: str):
        super().__init__(message)
        self.code = code
        self.message = message


def _strip_slash(url: str) -> str:
    return str(url or "").rstrip("/")


def _api_key(settings: Any | None) -> str:
    if settings is None:
        return ""
    return str(getattr(settings, "bettercontact_api_key", "") or "").strip()


def _base_url(settings: Any | None) -> str:
    if settings is None:
        return DEFAULT_BASE_URL
    return _strip_slash(getattr(settings, "licensed_finder_base_url", "") or DEFAULT_BASE_URL)


def _enabled(settings: Any | None) -> bool:
    return bool(getattr(settings, "licensed_finder_enabled", False)) if settings else False


def _headers(api_key: str) -> dict[str, str]:
    return {
        "X-API-Key": api_key,
        "Accept": "application/json",
        "Content-Type": "application/json",
    }


def include_exclude(include: list[str] | tuple[str, ...] | str | None = None, exclude: list[str] | tuple[str, ...] | str | None = None) -> dict[str, list[str]]:
    """Build a BetterContact include/exclude filter object."""

    def _as_list(value: Any) -> list[str]:
        if value is None:
            return []
        if isinstance(value, str):
            parts = [p.strip() for p in value.split(",") if p.strip()]
            return parts
        return [str(item).strip() for item in value if str(item).strip()]

    out: dict[str, list[str]] = {}
    inc = _as_list(include)
    exc = _as_list(exclude)
    if inc:
        out["include"] = inc
    if exc:
        out["exclude"] = exc
    return out


def build_search_filters(
    *,
    company_domains: list[str] | str | None = None,
    industries: list[str] | str | None = None,
    job_titles: list[str] | str | None = None,
    seniorities: list[str] | str | None = None,
    countries: list[str] | str | None = None,
    technologies: list[str] | str | None = None,
    extra: dict[str, Any] | None = None,
) -> dict[str, Any]:
    """Compose a Lead Finder `filters` object from simple lists.

    List filters become `{include: [...]}`. Extra keys pass through so an
    operator who already has BetterContact taxonomy values can send them as-is.
    """
    filters: dict[str, Any] = {}
    mapping = {
        "company_domain": company_domains,
        "company_industry": industries,
        "contact_job_title": job_titles,
        "contact_seniority": seniorities,
        "contact_location_country": countries,
        "company_technologies": technologies,
    }
    for key, value in mapping.items():
        packed = include_exclude(value)
        if packed:
            filters[key] = packed
    if extra:
        for key, value in extra.items():
            if value in (None, "", [], {}):
                continue
            filters[key] = value
    return filters


def filters_are_empty(filters: dict[str, Any] | None) -> bool:
    if not filters:
        return True
    for value in filters.values():
        if isinstance(value, dict):
            if value.get("include") or value.get("exclude") or any(
                v not in (None, "", [], {}) for k, v in value.items() if k not in {"include", "exclude"}
            ):
                return False
        elif value not in (None, "", [], {}):
            return False
    return True


def website_from_domain(domain: str) -> str:
    host = str(domain or "").strip().lower()
    if not host:
        return ""
    if host.startswith("http://") or host.startswith("https://"):
        return host
    return f"https://{host}"


def email_status_is_usable(status: str, *, accept_catch_all: bool = False) -> bool:
    key = str(status or "").strip().lower()
    if not key:
        return False
    if key in ACCEPTABLE_EMAIL_STATUSES:
        return True
    if accept_catch_all and key in SAFE_EMAIL_STATUSES:
        return True
    return False


def _person_from_row(row: dict[str, Any]) -> dict[str, str]:
    first = str(row.get("contact_first_name") or row.get("first_name") or "").strip()
    last = str(row.get("contact_last_name") or row.get("last_name") or "").strip()
    full = str(row.get("contact_full_name") or row.get("full_name") or "").strip()
    if not first and full:
        bits = full.split()
        first = bits[0] if bits else ""
        last = bits[-1] if len(bits) > 1 else last
    title = str(row.get("contact_job_title") or row.get("job_title") or row.get("title") or "").strip()
    linkedin = str(
        row.get("contact_linkedin_profile_url")
        or row.get("linkedin_url")
        or row.get("contact_linkedin_url")
        or ""
    ).strip()
    return {"first_name": first, "last_name": last, "title": title, "linkedin_url": linkedin, "full_name": full}


def licensed_row_to_lead(
    row: dict[str, Any],
    *,
    reason: str = "",
    source: str = "bettercontact",
    accept_catch_all: bool = False,
) -> dict[str, Any]:
    """Map a BetterContact person/company row onto our lead contract."""
    person = _person_from_row(row)
    company = str(row.get("company_name") or row.get("company") or "").strip()
    domain = str(row.get("company_domain") or row.get("company_website") or "").strip()
    if domain and "://" in domain:
        domain = domain_from_website(domain)
    website = str(row.get("website") or "").strip() or website_from_domain(domain)
    email = str(row.get("contact_email_address") or row.get("email") or "").strip().lower()
    status = str(row.get("contact_email_address_status") or row.get("email_status") or "").strip().lower()
    emails: list[str] = []
    if email and "@" in email and (not status or email_status_is_usable(status, accept_catch_all=accept_catch_all) or status in SAFE_EMAIL_STATUSES):
        emails.append(email)
    phones: list[str] = []
    phone = str(row.get("contact_phone_number") or row.get("phone") or "").strip()
    if phone:
        phones.append(phone)
    country = str(
        row.get("contact_location_country")
        or row.get("company_head_quarters_country")
        or row.get("country")
        or ""
    ).strip()
    industry = str(row.get("company_industry") or row.get("contact_industry") or row.get("industry") or "").strip()
    social: dict[str, str] = {}
    if person["linkedin_url"]:
        social["linkedin"] = person["linkedin_url"]
    company_li = str(row.get("company_linkedin_url") or "").strip()
    if company_li and "linkedin" not in social:
        social["company_linkedin"] = company_li
    composed_reason = (reason or "").strip()
    if not composed_reason:
        bits = []
        if person["full_name"] or person["first_name"]:
            who = person["full_name"] or f"{person['first_name']} {person['last_name']}".strip()
            bits.append(who)
        if person["title"]:
            bits.append(person["title"])
        if company:
            bits.append(f"at {company}")
        if industry:
            bits.append(f"({industry})")
        bits.append("from a licensed contact source.")
        composed_reason = " ".join(bits)
    lead: dict[str, Any] = {
        "company_name": company or domain or "Unknown company",
        "website": website,
        "industry": industry,
        "emails": emails,
        "phone_numbers": phones,
        "social_media": social,
        "contact_person": person["full_name"] or f"{person['first_name']} {person['last_name']}".strip() or None,
        "country_code": country,
        "first_name": person["first_name"],
        "last_name": person["last_name"],
        "title": person["title"],
        "linkedin_url": person["linkedin_url"],
        "reason": composed_reason,
        "fit_reasons": [composed_reason] if composed_reason else [],
        "source_keyword": source,
        "licensed_source": source,
        "email_status": status,
        "evidence_urls": [person["linkedin_url"]] if person["linkedin_url"] else [],
    }
    if row.get("contact_id") is not None:
        lead["licensed_contact_id"] = row.get("contact_id")
    return finalize_lead_contract(lead)


def enrichment_payload_from_lead(lead: dict[str, Any]) -> dict[str, Any] | None:
    """Build one BetterContact enrichment row. Returns None if identity is too thin."""
    first = str(lead.get("first_name") or "").strip()
    last = str(lead.get("last_name") or "").strip()
    if not first or not last:
        contact = str(lead.get("contact_person") or "").strip()
        bits = [p for p in contact.split() if p]
        if len(bits) >= 2:
            first, last = bits[0], bits[-1]
    company = str(lead.get("company_name") or lead.get("company") or "").strip()
    domain = str(lead.get("company_domain") or "").strip() or domain_from_website(str(lead.get("website") or ""))
    linkedin = str(lead.get("linkedin_url") or "").strip()
    social = lead.get("social_media") or {}
    if not linkedin and isinstance(social, dict):
        linkedin = str(social.get("linkedin") or "").strip()
    if not ((first and last and (company or domain)) or linkedin):
        return None
    row: dict[str, Any] = {}
    if first:
        row["first_name"] = first
    if last:
        row["last_name"] = last
    if company:
        row["company"] = company
    if domain:
        row["company_domain"] = domain
    if linkedin:
        row["linkedin_url"] = linkedin
    custom = {}
    lead_id = str(lead.get("lead_id") or "").strip()
    if lead_id:
        custom["lead_id"] = lead_id
    if custom:
        row["custom_fields"] = custom
    return row


def parse_submit_id(payload: Any, *, kind: str = "enrich") -> str:
    if not isinstance(payload, dict):
        raise LicensedFinderError("bad_response", "Licensed source returned a non-object submit body.")
    if kind == "search":
        rid = str(payload.get("request_id") or payload.get("id") or "").strip()
    else:
        rid = str(payload.get("id") or payload.get("request_id") or "").strip()
    if not rid:
        raise LicensedFinderError("bad_response", "Licensed source did not return a request id.")
    return rid


def parse_poll_status(payload: Any) -> str:
    if not isinstance(payload, dict):
        return ""
    return str(payload.get("status") or "").strip().lower()


def rows_from_search_result(payload: Any) -> list[dict[str, Any]]:
    if not isinstance(payload, dict):
        return []
    leads = payload.get("leads")
    if isinstance(leads, list):
        return [row for row in leads if isinstance(row, dict)]
    data = payload.get("data")
    if isinstance(data, list):
        return [row for row in data if isinstance(row, dict)]
    return []


def rows_from_enrich_result(payload: Any) -> list[dict[str, Any]]:
    if not isinstance(payload, dict):
        return []
    data = payload.get("data")
    if isinstance(data, list):
        return [row for row in data if isinstance(row, dict)]
    return []


def _httpx():
    import httpx

    return httpx


def _classify_http_error(status_code: int, body: Any) -> LicensedFinderError:
    message = ""
    if isinstance(body, dict):
        message = str(body.get("error") or body.get("message") or "")
    if status_code == 401:
        return LicensedFinderError("provider_auth", message or "Licensed source rejected the API key.")
    if status_code == 402:
        return LicensedFinderError("provider_out_of_credits", message or "Licensed source is out of credits.")
    if status_code == 406:
        return LicensedFinderError("bad_request_id", message or "Unknown request id.")
    if status_code == 404:
        return LicensedFinderError("not_found", message or "Licensed source returned 404.")
    if status_code == 429:
        return LicensedFinderError("provider_rate_limited", message or "Licensed source rate-limited the request.")
    if status_code in {400, 422}:
        return LicensedFinderError("bad_config", message or "Licensed source rejected the payload.")
    return LicensedFinderError("provider_unavailable", message or f"Licensed source HTTP {status_code}.")


async def _request(
    method: str,
    url: str,
    *,
    api_key: str,
    json_body: dict[str, Any] | None = None,
    client: Any | None = None,
    timeout: float = 30.0,
) -> tuple[int, Any]:
    httpx = _httpx()
    own = client is None
    http = client or httpx.AsyncClient(timeout=timeout)
    try:
        resp = await http.request(
            method,
            url,
            headers=_headers(api_key),
            json=json_body,
        )
        body: Any
        if resp.status_code == 202 and not (resp.content or b"").strip():
            body = {"status": "processing"}
        else:
            try:
                body = resp.json()
            except Exception:
                body = {"raw": resp.text}
        return resp.status_code, body
    except LicensedFinderError:
        raise
    except Exception as exc:
        raise LicensedFinderError("provider_unavailable", f"Licensed source unreachable: {exc}") from exc
    finally:
        if own:
            await http.aclose()


async def _poll(
    url: str,
    *,
    api_key: str,
    timeout_seconds: float,
    poll_seconds: float,
    client: Any | None = None,
    sleep=None,
) -> dict[str, Any]:
    sleeper = sleep or _async_sleep
    deadline = time.monotonic() + max(1.0, float(timeout_seconds))
    last: dict[str, Any] = {}
    while True:
        status_code, body = await _request("GET", url, api_key=api_key, client=client)
        if status_code in {401, 402, 406, 404, 429, 400, 422}:
            raise _classify_http_error(status_code, body)
        payload = body if isinstance(body, dict) else {}
        last = payload
        status = parse_poll_status(payload)
        if status == TERMINAL_OK:
            return payload
        if status == HOLD_STATUS:
            raise LicensedFinderError("provider_out_of_credits", "Licensed source paused the request (out of credits).")
        if time.monotonic() >= deadline:
            raise LicensedFinderError("timeout", "Timed out waiting for the licensed source.")
        if status and status not in POLL_STATUSES and status_code not in {200, 202}:
            raise _classify_http_error(status_code, body)
        await sleeper(max(0.2, float(poll_seconds)))


async def _async_sleep(seconds: float) -> None:
    import asyncio

    await asyncio.sleep(seconds)


def _require_ready(settings: Any | None) -> tuple[str, str]:
    if not _enabled(settings):
        raise LicensedFinderError("disabled", "Licensed finder is off. Set LICENSED_FINDER_ENABLED to use it.")
    key = _api_key(settings)
    if not key:
        raise LicensedFinderError("no_credential", "BETTERCONTACT_API_KEY is missing.")
    return key, _base_url(settings)


async def search_people(
    filters: dict[str, Any],
    *,
    settings: Any | None = None,
    max_leads: int = 10,
    enrich_emails: bool = False,
    enrich_phones: bool = False,
    reason: str = "",
    client: Any | None = None,
    sleep=None,
) -> dict[str, Any]:
    """Lead Finder search. Does not spend enrichment credits unless enrich_emails/phones."""
    if filters_are_empty(filters):
        raise LicensedFinderError("bad_config", "At least one Lead Finder filter is required.")
    key, base = _require_ready(settings)
    n = max(1, min(int(max_leads or 10), 200))
    body: dict[str, Any] = {
        "filters": filters,
        "max_leads": n,
        "enrich_email_address": bool(enrich_emails),
        "enrich_phone_number": bool(enrich_phones),
    }
    timeout = float(getattr(settings, "licensed_finder_timeout_seconds", 300) or 300) if settings else 300.0
    poll = float(getattr(settings, "licensed_finder_poll_seconds", 5.0) or 5.0) if settings else 5.0
    status_code, submitted = await _request(
        "POST",
        f"{base}/lead_finder/async",
        api_key=key,
        json_body=body,
        client=client,
    )
    if status_code not in {200, 201, 202}:
        raise _classify_http_error(status_code, submitted)
    request_id = parse_submit_id(submitted, kind="search")
    result = await _poll(
        f"{base}/lead_finder/async/{request_id}",
        api_key=key,
        timeout_seconds=timeout,
        poll_seconds=poll,
        client=client,
        sleep=sleep,
    )
    rows = rows_from_search_result(result)
    leads = [
        licensed_row_to_lead(row, reason=reason, accept_catch_all=False)
        for row in rows
    ]
    summary = result.get("summary") if isinstance(result.get("summary"), dict) else {}
    return {
        "request_id": request_id,
        "credits_consumed": int(result.get("credits_consumed") or 0),
        "credits_left": result.get("credits_left"),
        "leads_found": int(summary.get("leads_found") or len(leads)),
        "leads": leads,
        "raw_status": parse_poll_status(result),
    }


async def enrich_people(
    leads: list[dict[str, Any]],
    *,
    settings: Any | None = None,
    enrich_emails: bool = True,
    enrich_phones: bool = False,
    verify_catch_all: bool = False,
    client: Any | None = None,
    sleep=None,
) -> dict[str, Any]:
    """Waterfall-enrich up to 100 already-identified people. Opt-in paid step."""
    key, base = _require_ready(settings)
    batch: list[dict[str, Any]] = []
    for lead in leads:
        if not isinstance(lead, dict):
            continue
        row = enrichment_payload_from_lead(lead)
        if row:
            batch.append(row)
        if len(batch) >= 100:
            break
    if not batch:
        raise LicensedFinderError("bad_config", "No enrichable people (need name+company/domain or a LinkedIn URL).")
    body: dict[str, Any] = {
        "data": batch,
        "enrich_email_address": bool(enrich_emails),
        "enrich_phone_number": bool(enrich_phones),
        "verify_catch_all": bool(verify_catch_all),
    }
    timeout = float(getattr(settings, "licensed_finder_timeout_seconds", 300) or 300) if settings else 300.0
    poll = float(getattr(settings, "licensed_finder_poll_seconds", 5.0) or 5.0) if settings else 5.0
    status_code, submitted = await _request(
        "POST",
        f"{base}/async",
        api_key=key,
        json_body=body,
        client=client,
    )
    if status_code not in {200, 201, 202}:
        raise _classify_http_error(status_code, submitted)
    request_id = parse_submit_id(submitted, kind="enrich")
    result = await _poll(
        f"{base}/async/{request_id}",
        api_key=key,
        timeout_seconds=timeout,
        poll_seconds=poll,
        client=client,
        sleep=sleep,
    )
    rows = rows_from_enrich_result(result)
    mapped = [licensed_row_to_lead(row, source="bettercontact_enrich") for row in rows]
    return {
        "request_id": request_id,
        "credits_consumed": int(result.get("credits_consumed") or 0),
        "credits_left": result.get("credits_left"),
        "leads": mapped,
        "raw_status": parse_poll_status(result),
    }


def readiness(settings: Any | None) -> dict[str, Any]:
    """Cheap status document. Never spends."""
    enabled = _enabled(settings)
    has_key = bool(_api_key(settings))
    if not enabled:
        next_action = "set LICENSED_FINDER_ENABLED=true if you want the licensed bypass"
        blocked = "disabled"
    elif not has_key:
        next_action = "set BETTERCONTACT_API_KEY"
        blocked = "no_credential"
    else:
        next_action = "POST /api/v1/licensed-finder/search"
        blocked = ""
    return {
        "enabled": enabled,
        "has_api_key": has_key,
        "base_url": _base_url(settings),
        "in_graph": False,
        "blocked": blocked,
        "next_action": next_action,
    }
