from __future__ import annotations

import asyncio
import math
import uuid
from typing import Any

import aiosqlite

from . import db as database
from .config import Settings
from .options import get_options
from .pricing import PRICE_TABLE_COMMIT, resolve_group_pricing
from .probes.cache import run_cache
from .probes.common import (
    AccountSnapshot,
    GroupProfile,
    ProbeCall,
    ProbeContext,
    ProbeResult,
    account_snapshot,
    correlated_chat,
    result,
    stable_account_session_id,
)
from .probes.modeltrace import run_modeltrace_with_confirmation
from .probes.multiplier import run_multiplier
from .probes.token_delta import run_token_delta
from .probes.usage import normalize_usage
from .report import write_report
from .storage import (
    audit_action,
    save_account,
    save_alert,
    save_group_profile,
    save_probe_calls,
    save_probe_result,
    save_usage_snapshot,
    session_hash,
)
from .sub2api import Sub2APIClient, Sub2APIError


class ScanAlreadyRunning(RuntimeError):
    pass


def _optional_number(value: Any) -> float | None:
    if isinstance(value, bool) or not isinstance(value, (int, float)) or not math.isfinite(float(value)):
        return None
    return float(value)


def _profile(settings: Settings, group_id: int) -> GroupProfile | None:
    raw = settings.group_profiles.get(group_id)
    if raw is None:
        return None
    model = raw.get("model")
    if not isinstance(model, str) or not model.strip():
        return None
    expected_model = raw.get("expected_model", "")
    if not isinstance(expected_model, str):
        expected_model = ""
    cache_threshold = raw.get("cache_min_prefix_tokens")
    if isinstance(cache_threshold, bool) or not isinstance(cache_threshold, int):
        cache_threshold = None
    cache_protocol = raw.get("cache_protocol", "openai")
    if cache_protocol not in {"openai", "anthropic"}:
        cache_protocol = "openai"
    pricing_raw = raw.get("pricing", {})
    if not isinstance(pricing_raw, dict):
        pricing_raw = {}
    pricing_model = raw.get("pricing_model", "")
    if not isinstance(pricing_model, str):
        pricing_model = ""
    pricing, pricing_source, resolved_pricing_model, price_table_status = resolve_group_pricing(
        model.strip(), pricing_model.strip(), pricing_raw
    )
    gateway_multiplier = _optional_number(raw.get("gateway_cost_multiplier"))
    expected_provider_multiplier = _optional_number(raw.get("expected_provider_cost_multiplier"))
    declared_cache_ratio = _optional_number(raw.get("declared_cache_ratio"))
    declared_completion_ratio = _optional_number(raw.get("declared_completion_ratio"))
    return GroupProfile(
        group_id=group_id,
        model=model.strip()[:160],
        expected_model=expected_model.strip()[:160],
        cache_min_prefix_tokens=cache_threshold,
        cache_protocol=cache_protocol,
        pricing=pricing,
        gateway_cost_multiplier=gateway_multiplier,
        expected_provider_cost_multiplier=expected_provider_multiplier,
        declared_cache_ratio=declared_cache_ratio,
        declared_completion_ratio=declared_completion_ratio,
        pricing_model=resolved_pricing_model,
        pricing_source=pricing_source,
        price_table_revision=PRICE_TABLE_COMMIT,
        price_table_status=price_table_status,
    )


def _active_schedulable_group_counts(accounts: list[dict[str, Any]]) -> dict[int, int]:
    counts: dict[int, int] = {}
    for raw in accounts:
        account = account_snapshot(raw)
        if account is None or account.status != "active" or not account.schedulable:
            continue
        for group_id in account.group_ids:
            counts[group_id] = counts.get(group_id, 0) + 1
    return counts


def _exclusive_group_ids(account: AccountSnapshot, group_member_counts: dict[int, int]) -> tuple[int, ...]:
    return tuple(group_id for group_id in account.group_ids if group_member_counts.get(group_id) == 1)


class ScanCoordinator:
    def __init__(self, db: aiosqlite.Connection, settings: Settings, sub2api: Sub2APIClient):
        self.db = db
        self.settings = settings
        self.sub2api = sub2api
        self._run_lock = asyncio.Lock()
        self._wake_scheduler = asyncio.Event()
        self._scheduler_task: asyncio.Task[None] | None = None
        self._manual_task: asyncio.Task[None] | None = None

    @property
    def is_running(self) -> bool:
        return self._run_lock.locked()

    async def start_scheduler(self) -> None:
        if self._scheduler_task is None:
            self._scheduler_task = asyncio.create_task(self._scheduler_loop(), name="modelaudit-scheduler")

    async def stop_scheduler(self) -> None:
        if self._scheduler_task:
            self._scheduler_task.cancel()
            try:
                await self._scheduler_task
            except asyncio.CancelledError:
                pass
            self._scheduler_task = None

    def wake_scheduler(self) -> None:
        self._wake_scheduler.set()

    def launch_manual_scan(self) -> None:
        if self.is_running or (self._manual_task is not None and not self._manual_task.done()):
            raise ScanAlreadyRunning("scan_already_running")
        self._manual_task = asyncio.create_task(self.run_once("manual"), name="modelaudit-manual-scan")

    async def _scheduler_loop(self) -> None:
        while True:
            options = await get_options(self.db, self.settings)
            timeout = int(options.get("interval_minutes", self.settings.interval_minutes)) * 60
            if not options.get("scheduler_enabled", self.settings.schedule_enabled):
                timeout = 60
            self._wake_scheduler.clear()
            try:
                await asyncio.wait_for(self._wake_scheduler.wait(), timeout=timeout)
                continue
            except asyncio.TimeoutError:
                pass
            options = await get_options(self.db, self.settings)
            if not options.get("scheduler_enabled", self.settings.schedule_enabled):
                continue
            try:
                await self.run_once("schedule")
            except ScanAlreadyRunning:
                continue
            except Exception:
                # Never include API payloads, request bodies, or credentials in task logs.
                continue

    async def run_once(self, trigger: str = "manual") -> dict[str, Any]:
        if self._run_lock.locked():
            raise ScanAlreadyRunning("scan_already_running")
        await self._run_lock.acquire()
        run_id = str(uuid.uuid4())
        started = database.utc_now()
        account_count = 0
        result_count = 0
        error_count = 0
        try:
            await self.db.execute(
                "INSERT INTO probe_runs(id, trigger, status, started_at) VALUES(?, ?, 'running', ?)",
                (run_id, trigger[:32], started),
            )
            await self.db.commit()
            accounts = await self.sub2api.list_active_accounts()
            options = await get_options(self.db, self.settings)
            target_model = str(options.get("target_model") or "").strip()
            account_count = len(accounts)
            group_member_counts = _active_schedulable_group_counts(accounts)
            semaphore = asyncio.Semaphore(self.settings.max_parallel_accounts)

            async def bounded_scan(raw: dict[str, Any]) -> tuple[int, int]:
                async with semaphore:
                    return await self._scan_account(run_id, raw, group_member_counts, target_model)

            outcomes = await asyncio.gather(*(bounded_scan(account) for account in accounts), return_exceptions=True)
            for outcome in outcomes:
                if isinstance(outcome, BaseException):
                    error_count += 1
                    continue
                result_count += outcome[0]
                error_count += outcome[1]

            status = "completed" if error_count == 0 else "completed_with_errors"
            await self.db.execute(
                "UPDATE probe_runs SET status=?, finished_at=?, account_count=?, result_count=?, error_count=? WHERE id=?",
                (status, database.utc_now(), account_count, result_count, error_count, run_id),
            )
            await self.db.commit()
            try:
                await write_report(self.db, self.settings.db_path, run_id)
            except Exception:
                await self.db.execute("UPDATE probe_runs SET note='report_write_failed' WHERE id=?", (run_id,))
                await self.db.commit()
            return {"id": run_id, "status": status, "account_count": account_count, "result_count": result_count, "error_count": error_count}
        except Sub2APIError as exc:
            await self.db.execute(
                "UPDATE probe_runs SET status='failed', finished_at=?, account_count=?, error_count=1, note=? WHERE id=?",
                (database.utc_now(), account_count, exc.code[:64], run_id),
            )
            await self.db.commit()
            try:
                await write_report(self.db, self.settings.db_path, run_id)
            except Exception:
                pass
            return {"id": run_id, "status": "failed", "account_count": account_count, "error_count": 1, "error_code": exc.code}
        except Exception as exc:
            await self.db.execute(
                "UPDATE probe_runs SET status='failed', finished_at=?, account_count=?, error_count=1, note=? WHERE id=?",
                (database.utc_now(), account_count, type(exc).__name__, run_id),
            )
            await self.db.commit()
            try:
                await write_report(self.db, self.settings.db_path, run_id)
            except Exception:
                pass
            return {"id": run_id, "status": "failed", "account_count": account_count, "error_count": 1, "error_code": type(exc).__name__}
        finally:
            self._run_lock.release()

    async def _fetch_usage(
        self,
        run_id: str,
        account_id: int,
        position: str,
    ) -> dict[str, Any] | None:
        try:
            raw = await self.sub2api.get_account_usage(account_id)
        except Sub2APIError as exc:
            await save_usage_snapshot(
                self.db,
                run_id=run_id,
                account_id=account_id,
                position=position,
                normalized=None,
                error_code=exc.code,
            )
            return None
        normalized = normalize_usage(raw)
        await save_usage_snapshot(
            self.db,
            run_id=run_id,
            account_id=account_id,
            position=position,
            normalized=normalized,
        )
        return normalized

    async def _scan_account(
        self,
        run_id: str,
        raw: dict[str, Any],
        group_member_counts: dict[int, int],
        target_model: str = "",
    ) -> tuple[int, int]:
        raw_id = raw.get("id", raw.get("account_id"))
        try:
            if isinstance(raw_id, bool) or not isinstance(raw_id, (int, str)):
                return 0, 1
            account_id = int(raw_id)
            if account_id <= 0 or (isinstance(raw_id, str) and not raw_id.isdecimal()):
                return 0, 1
        except (TypeError, ValueError):
            return 0, 1

        try:
            detail = await self.sub2api.get_account(account_id)
        except Sub2APIError as exc:
            account = account_snapshot(raw)
            if account is not None:
                await save_account(self.db, account)
            items = [
                result(
                    name,
                    "error",
                    "Account detail could not be read; this account was not probed.",
                    evidence={"error_code": exc.code},
                )
                for name in ("modeltrace", "token_delta", "cache", "multiplier")
            ]
            for item in items:
                await save_probe_result(db=self.db, run_id=run_id, account_id=account_id, profile=None, item=item)
            return len(items), len(items)

        detail.setdefault("id", account_id)
        for key in (
            "name",
            "platform",
            "type",
            "status",
            "schedulable",
            "concurrency",
            "current_concurrency",
            "rate_multiplier",
            "group_ids",
            "groups",
            "account_groups",
        ):
            if key not in detail and key in raw:
                detail[key] = raw[key]

        # The detail response can contain upstream credentials. Only this in-memory
        # sanitized snapshot is retained; never write the raw account response.
        account = account_snapshot(detail)
        if account is None:
            return 0, 1
        await save_account(self.db, account)

        if not account.schedulable:
            skipped = [
                result(name, "skipped", "Account is active but not schedulable.")
                for name in ("modeltrace", "token_delta", "cache", "multiplier")
            ]
            for item in skipped:
                await save_probe_result(db=self.db, run_id=run_id, account_id=account.account_id, profile=None, item=item)
            return len(skipped), 0

        candidates = [
            (group_id, _profile(self.settings, group_id))
            for group_id in account.group_ids
            if group_id in self.settings.gateway_api_keys
        ]
        candidates = [(group_id, profile) for group_id, profile in candidates if profile is not None]
        if target_model:
            candidates = [(group_id, profile) for group_id, profile in candidates if profile.model == target_model]
        if not candidates:
            missing = [
                result(name, "not_configured", "No configured ordinary API key and model profile match this account's groups.")
                for name in ("modeltrace", "token_delta", "cache")
            ]
            missing.append(result("multiplier", "inconclusive", "No configured model profile is available for this account."))
            for item in missing:
                await save_probe_result(db=self.db, run_id=run_id, account_id=account.account_id, profile=None, item=item)
            return len(missing), 0

        exclusive_ids = set(_exclusive_group_ids(account, group_member_counts))
        exclusive_candidates = [
            (group_id, profile)
            for group_id, profile in candidates
            if group_id in exclusive_ids
        ]
        if not exclusive_candidates:
            unpinned = [
                result(
                    name,
                    "unpinned",
                    "No configured group routes exclusively to this account; gateway selection could reach another account.",
                    evidence={
                        "reason": "no_exclusive_account_group",
                        "groups": [
                            {"group_id": group_id, "active_schedulable_accounts": group_member_counts.get(group_id, 0)}
                            for group_id, _ in candidates
                        ],
                    },
                )
                for name in ("modeltrace", "token_delta", "cache", "multiplier")
            ]
            for item in unpinned:
                await save_probe_result(db=self.db, run_id=run_id, account_id=account.account_id, profile=None, item=item)
            return len(unpinned), 0
        candidates = exclusive_candidates

        contexts: list[ProbeContext] = []
        selected: ProbeContext | None = None
        last_pin_reason = "calibration_exhausted"
        for attempt in range(self.settings.max_group_calibrations):
            group_id, profile = candidates[attempt % len(candidates)]
            assert profile is not None
            context = ProbeContext(
                account=account,
                profile=profile,
                group_id=group_id,
                session_id=stable_account_session_id(self.settings.session_secret, account.account_id),
                api_key=self.settings.gateway_api_keys[group_id],
                run_id=run_id,
                settings=self.settings,
                sub2api=self.sub2api,
                exclusive_group=True,
            )
            contexts.append(context)
            call = await correlated_chat(
                context,
                probe="pin_calibration",
                messages=[{"role": "user", "content": "Say hi"}],
                max_tokens=8,
            )
            if call.pinned:
                selected = context
                break
            last_pin_reason = call.pin_reason

        if selected is None:
            for context in contexts:
                await save_group_profile(self.db, context.profile)
            call_rows = [
                (call, context.group_id, context.session_id)
                for context in contexts
                for call in context.calls
            ]
            await save_probe_calls(self.db, run_id=run_id, account_id=account.account_id, call_rows=call_rows)
            request_ids = tuple(call.request_id for context in contexts for call in context.calls)
            calibration_profile = candidates[0][1]
            failed_results = [
                result(
                    name,
                    "unpinned",
                    "Calibration could not verify that requests reached this account; probe evidence was discarded.",
                    model=calibration_profile.model if calibration_profile else "",
                    expected_model=calibration_profile.expected_model if calibration_profile else "",
                    evidence={"reason": last_pin_reason, "calibration_request_ids": list(request_ids)},
                )
                for name in ("modeltrace", "token_delta", "cache")
            ]
            failed_results.append(
                result(
                    "multiplier",
                    "inconclusive",
                    "Supplier cost cannot be attributed without a pinned gateway session.",
                    evidence={
                        "reason": "no_pinned_session",
                        "calibration_request_ids": list(request_ids),
                    },
                )
            )
            for item in failed_results:
                await save_probe_result(
                    db=self.db,
                    run_id=run_id,
                    account_id=account.account_id,
                    profile=calibration_profile,
                    item=item,
                )
            return len(failed_results), 0

        for context in contexts:
            await save_group_profile(self.db, context.profile)
        outcomes: list[ProbeResult] = []
        probes = (
            ("token_delta", run_token_delta),
            ("cache", run_cache),
        )
        # The supplier usage delta covers only probes with configured pricing.
        # Pin calibration and ModelTrace remain in request diagnostics,
        # but cannot affect this multiplier conclusion.
        before = await self._fetch_usage(run_id, account.account_id, "before")
        for name, probe_fn in probes:
            try:
                outcomes.append(await probe_fn(selected))
            except Exception as exc:
                outcomes.append(
                    result(
                        name,
                        "error",
                        f"{name} probe failed.",
                        model=selected.profile.model,
                        expected_model=selected.profile.expected_model,
                        evidence={"error_code": type(exc).__name__},
                    )
                )
        after = await self._fetch_usage(run_id, account.account_id, "after")
        outcomes.append(run_multiplier(selected, before, after))
        try:
            outcomes.append(await run_modeltrace_with_confirmation(selected))
        except Exception as exc:
            outcomes.append(
                result(
                    "modeltrace",
                    "error",
                    "modeltrace probe failed.",
                    model=selected.profile.model,
                    expected_model=selected.profile.expected_model,
                    evidence={"error_code": type(exc).__name__},
                )
            )

        call_rows = [
            (call, context.group_id, context.session_id)
            for context in contexts
            for call in context.calls
        ]
        await save_probe_calls(self.db, run_id=run_id, account_id=account.account_id, call_rows=call_rows)

        for item in outcomes:
            await save_probe_result(
                db=self.db,
                run_id=run_id,
                account_id=account.account_id,
                profile=selected.profile,
                item=item,
                session_id=selected.session_id,
            )
        options = await get_options(self.db, self.settings)
        await self._evaluate_alerts(run_id, account, outcomes, options)
        error_count = sum(1 for item in outcomes if item.status == "error")
        return len(outcomes), error_count

    async def _evaluate_alerts(
        self,
        run_id: str,
        account: AccountSnapshot,
        outcomes: list[ProbeResult],
        options: dict[str, Any],
    ) -> None:
        stop_key = {
            "modeltrace": "modeltrace_red",
        }
        stop_attempted = False
        for item in outcomes:
            if item.status == "alert":
                status = "open"
                severity = item.severity
            elif item.status == "unknown":
                if item.severity == "none":
                    continue
                status = "warning"
                severity = item.severity
            else:
                continue
            rule_id = f"{item.probe}_{item.severity}"
            should_stop = (
                item.status == "alert"
                and item.severity == "red"
                and bool(options.get("auto_stop_enabled"))
                and stop_key.get(item.probe) in options.get("stop_rules", [])
                and (item.probe != "modeltrace" or item.metrics.get("auto_stop_eligible") is True)
            )
            action = ""
            if should_stop and account.schedulable:
                if stop_attempted:
                    action = "auto_stop_already_attempted_for_run"
                else:
                    stop_attempted = True
                    try:
                        await self.sub2api.set_schedulable(account.account_id, False)
                        action = "schedulable_set_false"
                        await self.db.execute(
                            "UPDATE accounts SET schedulable=0, last_seen_at=? WHERE account_id=?",
                            (database.utc_now(), account.account_id),
                        )
                        await self.db.commit()
                        await audit_action(
                            self.db,
                            account_id=account.account_id,
                            actor_sub="scheduler",
                            action="auto_stop",
                            result="success",
                            details={"probe": item.probe, "rule_id": rule_id},
                        )
                    except Sub2APIError as exc:
                        action = f"auto_stop_failed:{exc.code}"
                        await audit_action(
                            self.db,
                            account_id=account.account_id,
                            actor_sub="scheduler",
                            action="auto_stop",
                            result="failed",
                            details={"probe": item.probe, "error_code": exc.code},
                        )
            await save_alert(
                self.db,
                run_id=run_id,
                account_id=account.account_id,
                rule_id=rule_id,
                severity=severity,
                status=status,
                message=item.summary,
                details={"probe": item.probe, "metrics": item.metrics, "evidence": item.evidence},
                action_taken=action,
            )
