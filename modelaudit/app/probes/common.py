from __future__ import annotations

from dataclasses import asdict, dataclass, field
import hashlib
import hmac
import json
import math
from typing import Any, Literal
import uuid

from ..config import Settings
from ..sub2api import Sub2APIClient


ProbeStatus = Literal["ok", "alert", "unknown", "inconclusive", "unpinned", "error", "not_configured", "skipped"]
Severity = Literal["normal", "yellow", "red", "none"]


def stable_account_session_id(secret: str, account_id: int) -> str:
    """Derive one opaque, repeatable gateway session ID for each upstream account."""
    message = f"modelaudit:account-session:v1:{account_id}".encode("ascii")
    digest = bytearray(hmac.new(secret.encode("utf-8"), message, hashlib.sha256).digest()[:16])
    return str(uuid.UUID(bytes=bytes(digest), version=5))


def account_metadata_user_id(secret: str, account_id: int, session_id: str) -> str:
    """Build Sub2API's existing metadata.user_id session anchor without exposing IDs."""
    message = f"modelaudit:metadata-device:v1:{account_id}".encode("ascii")
    device_id = hmac.new(secret.encode("utf-8"), message, hashlib.sha256).hexdigest()
    return json.dumps(
        {"device_id": device_id, "account_uuid": "", "session_id": session_id},
        separators=(",", ":"),
    )


def uses_anthropic_messages(context: ProbeContext) -> bool:
    return context.account.platform.lower() == "anthropic"


def probe_protocol(context: ProbeContext) -> str:
    return "anthropic" if uses_anthropic_messages(context) else context.profile.cache_protocol


def probe_user_agent(context: ProbeContext) -> str | None:
    if (
        uses_anthropic_messages(context)
        and context.account.account_type.lower() in {"oauth", "setup-token"}
    ):
        # Sub2API otherwise injects Claude Code system blocks into generic OAuth traffic.
        return "claude-cli/2.1.161 (modelaudit probe)"
    return None


@dataclass(frozen=True)
class AccountSnapshot:
    account_id: int
    name: str
    platform: str
    account_type: str
    status: str
    schedulable: bool
    concurrency: int
    current_concurrency: int
    rate_multiplier: float | None
    group_ids: tuple[int, ...]


@dataclass(frozen=True)
class GroupProfile:
    group_id: int
    model: str
    expected_model: str
    cache_min_prefix_tokens: int | None
    cache_protocol: str
    pricing: dict[str, float]
    gateway_cost_multiplier: float | None
    expected_provider_cost_multiplier: float | None
    declared_cache_ratio: float | None
    declared_completion_ratio: float | None
    pricing_model: str = ""
    pricing_source: str = "unavailable"
    price_table_revision: str = ""
    price_table_status: str = "not_loaded"


@dataclass(frozen=True)
class ProbeContext:
    account: AccountSnapshot
    profile: GroupProfile
    group_id: int
    session_id: str
    api_key: str
    run_id: str
    settings: Settings
    sub2api: Sub2APIClient
    exclusive_group: bool = False
    calls: list["ProbeCall"] = field(default_factory=list, compare=False)


@dataclass(frozen=True)
class ProbeResult:
    probe: str
    status: ProbeStatus
    severity: Severity
    summary: str
    model: str = ""
    expected_model: str = ""
    actual_account_id: int | None = None
    request_ids: tuple[str, ...] = ()
    metrics: dict[str, Any] = field(default_factory=dict)
    evidence: dict[str, Any] = field(default_factory=dict)

    def as_dict(self) -> dict[str, Any]:
        return asdict(self)


def get_account_id(raw: dict[str, Any]) -> int | None:
    value = raw.get("id", raw.get("account_id"))
    if isinstance(value, bool):
        return None
    if isinstance(value, int):
        parsed = value
    elif isinstance(value, str) and value.isdecimal():
        parsed = int(value)
    else:
        return None
    return parsed if parsed > 0 else None


def parse_group_ids(raw: dict[str, Any]) -> tuple[int, ...]:
    candidates = raw.get("group_ids")
    if not isinstance(candidates, list):
        candidates = raw.get("groups")
    if not isinstance(candidates, list):
        candidates = raw.get("account_groups")
    ids: set[int] = set()
    if isinstance(candidates, list):
        for candidate in candidates:
            value = candidate.get("id", candidate.get("group_id")) if isinstance(candidate, dict) else candidate
            try:
                group_id = int(value)
            except (TypeError, ValueError):
                continue
            if group_id > 0:
                ids.add(group_id)
    return tuple(sorted(ids))


def account_snapshot(raw: dict[str, Any]) -> AccountSnapshot | None:
    account_id = get_account_id(raw)
    if account_id is None:
        return None
    multiplier = raw.get("rate_multiplier")
    if (
        isinstance(multiplier, bool)
        or not isinstance(multiplier, (int, float))
        or not math.isfinite(float(multiplier))
        or multiplier < 0
    ):
        # Sub2API treats a missing/null account multiplier as 1.0.
        multiplier = 1.0 if multiplier is None else None
    concurrency = raw.get("concurrency", 1)
    current = raw.get("current_concurrency", 0)
    try:
        if isinstance(concurrency, bool):
            raise ValueError("invalid concurrency")
        concurrency = int(concurrency)
        if concurrency < 0:
            concurrency = 1
    except (TypeError, ValueError):
        concurrency = 1
    try:
        current = max(int(current), 0)
    except (TypeError, ValueError):
        current = 0
    return AccountSnapshot(
        account_id=account_id,
        name=str(raw.get("name") or f"account-{account_id}")[:160],
        platform=str(raw.get("platform") or "unknown")[:64],
        account_type=str(raw.get("type") or "unknown")[:64],
        status=str(raw.get("status") or "unknown")[:32],
        schedulable=bool(raw.get("schedulable", True)),
        concurrency=concurrency,
        current_concurrency=current,
        rate_multiplier=float(multiplier) if multiplier is not None else None,
        group_ids=parse_group_ids(raw),
        )


@dataclass(frozen=True)
class ProbeCall:
    request_id: str
    probe: str
    http_status: int
    error_code: str
    actual_account_id: int | None
    session_matches: bool
    pinned: bool
    input_tokens: int | None
    output_tokens: int | None
    cache_creation_tokens: int | None
    cache_read_tokens: int | None
    pin_reason: str


@dataclass(frozen=True)
class GatewayCall:
    request_id: str
    http_status: int
    payload: dict[str, Any] | None
    error_code: str
    pinned: bool
    actual_account_id: int | None
    pin_reason: str


def _response_usage(payload: dict[str, Any] | None, protocol: str) -> dict[str, int | None]:
    """Extract numeric usage only, preserving cache token semantics for diagnostics."""
    empty: dict[str, int | None] = {
        "input_tokens": None,
        "output_tokens": None,
        "cache_creation_tokens": None,
        "cache_read_tokens": None,
    }
    usage = payload.get("usage") if isinstance(payload, dict) else None
    if not isinstance(usage, dict):
        return empty

    def token(value: Any) -> int | None:
        if isinstance(value, bool) or not isinstance(value, int) or value < 0:
            return None
        return value

    anthropic_fields = ("input_tokens", "cache_creation_input_tokens", "cache_read_input_tokens")
    if protocol == "anthropic" or any(field in usage for field in anthropic_fields):
        return {
            "input_tokens": token(usage.get("input_tokens")),
            "output_tokens": token(usage.get("output_tokens")),
            "cache_creation_tokens": token(usage.get("cache_creation_input_tokens", 0)),
            "cache_read_tokens": token(usage.get("cache_read_input_tokens", 0)),
        }

    total_input = token(usage.get("prompt_tokens", usage.get("input_tokens")))
    output = token(usage.get("completion_tokens", usage.get("output_tokens")))
    details = usage.get("prompt_tokens_details") or usage.get("input_tokens_details")
    cached = token(details.get("cached_tokens", 0)) if isinstance(details, dict) else 0
    if total_input is None or cached is None or cached > total_input:
        input_tokens = None
        cache_read = None
    else:
        input_tokens = total_input - cached
        cache_read = cached
    return {
        "input_tokens": input_tokens,
        "output_tokens": output,
        "cache_creation_tokens": 0,
        "cache_read_tokens": cache_read,
    }


async def correlated_chat(
    context: ProbeContext,
    *,
    probe: str,
    messages: list[dict[str, Any]],
    max_tokens: int,
    extra_body: dict[str, Any] | None = None,
    serialized_body: bytes | None = None,
) -> GatewayCall:
    request_id = str(uuid.uuid4())
    metadata_user_id = account_metadata_user_id(
        context.settings.session_secret,
        context.account.account_id,
        context.session_id,
    )
    request = context.sub2api.gateway_chat
    if uses_anthropic_messages(context):
        # The Chat Completions bridge always applies Claude OAuth mimicry. Native
        # Messages preserves the probe body and lets the stable CLI UA + metadata
        # identify this as Claude Code traffic without adding a system prompt.
        request = context.sub2api.gateway_messages
    status_code, payload, error_code = await request(
        key=context.api_key,
        model=context.profile.model,
        session_id=context.session_id,
        request_id=request_id,
        metadata_user_id=metadata_user_id,
        user_agent=probe_user_agent(context),
        messages=messages,
        max_tokens=max_tokens,
        extra_body=extra_body,
        serialized_body=serialized_body,
    )
    pinned = context.exclusive_group and 200 <= status_code < 300
    actual_account_id = context.account.account_id if pinned else None
    # All probes in this context send the same derived session ID. The unique
    # schedulable account in the group supplies the initial route pin.
    session_matches = pinned
    if not context.exclusive_group:
        pin_reason = "exclusive_group_not_confirmed"
    elif not 200 <= status_code < 300:
        pin_reason = "gateway_response_not_successful"
    else:
        pin_reason = "exclusive_group_single_schedulable_account"

    # ModelTrace is an attribution probe only. Keep its call/routing diagnostics,
    # but never inspect or persist its response usage for token or cost accounting.
    response_usage = (
        {
            "input_tokens": None,
            "output_tokens": None,
            "cache_creation_tokens": None,
            "cache_read_tokens": None,
        }
        if probe == "modeltrace"
        else _response_usage(payload, probe_protocol(context))
    )
    cache_creation_tokens = response_usage["cache_creation_tokens"]
    cache_read_tokens = response_usage["cache_read_tokens"]
    input_tokens = response_usage["input_tokens"]

    context.calls.append(
        ProbeCall(
            request_id=request_id,
            probe=probe,
            http_status=status_code,
            error_code=error_code,
            actual_account_id=actual_account_id,
            session_matches=session_matches,
            pinned=pinned,
            input_tokens=input_tokens,
            output_tokens=response_usage["output_tokens"],
            cache_creation_tokens=cache_creation_tokens,
            cache_read_tokens=cache_read_tokens,
            pin_reason=pin_reason,
        )
    )
    return GatewayCall(
        request_id=request_id,
        http_status=status_code,
        payload=payload,
        error_code=error_code,
        pinned=pinned,
        actual_account_id=actual_account_id,
        pin_reason=pin_reason,
    )


def result(
    probe: str,
    status: ProbeStatus,
    summary: str,
    *,
    severity: Severity = "none",
    model: str = "",
    expected_model: str = "",
    actual_account_id: int | None = None,
    request_ids: tuple[str, ...] = (),
    metrics: dict[str, Any] | None = None,
    evidence: dict[str, Any] | None = None,
) -> ProbeResult:
    return ProbeResult(
        probe=probe,
        status=status,
        severity=severity,
        summary=summary,
        model=model,
        expected_model=expected_model,
        actual_account_id=actual_account_id,
        request_ids=request_ids,
        metrics=metrics or {},
        evidence=evidence or {},
    )
