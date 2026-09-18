from __future__ import annotations

import copy
import json
import time
from collections import deque
from concurrent.futures import Future, ThreadPoolExecutor
from dataclasses import dataclass, field
from datetime import datetime, timezone
from pathlib import Path
from threading import Event, RLock
from typing import Any, Callable

from service.scope import safe_user_id, user_data_path


CoreFactory = Callable[[str, dict[str, Any], Path, Callable[[str], None]], Any]
_SENSITIVE_KEYS = {
    "api_key",
    "password",
    "access_key_secret",
    "token",
    "secret",
    "service_token",
    "wechat_token",
}


def _default_config() -> dict[str, Any]:
    """返回每个用户的最小可运行配置。"""
    return {
        "keywords": [],
        "exclude_keywords": [],
        "must_contain_keywords": [],
        "notify_method": "none",
        "email": "",
        "phone": "",
        "interval_minutes": 30,
        "crawler": {"enabled_sites": [], "use_selenium": False},
        "custom_sites": [],
    }


def _redact(value: Any) -> Any:
    """递归删除不能落盘的敏感配置字段。"""
    if isinstance(value, dict):
        return {
            str(key): _redact(item)
            for key, item in value.items()
            if str(key).lower() not in _SENSITIVE_KEYS
        }
    if isinstance(value, list):
        return [_redact(item) for item in value]
    return value


@dataclass
class _UserState:
    user_id: str
    data_dir: Path
    config: dict[str, Any]
    logs: deque[str] = field(default_factory=lambda: deque(maxlen=300))
    is_running: bool = False
    current_task_running: bool = False
    last_run_time: str | None = None
    last_result: dict[str, Any] | None = None
    last_error: str | None = None
    stop_event: Event = field(default_factory=Event)
    future: Future[Any] | None = None
    core: Any | None = None


def _default_core_factory(
    user_id: str,
    config: dict[str, Any],
    data_dir: Path,
    log_callback: Callable[[str], None],
) -> Any:
    """用来源项目的核心类创建一个用户作用域实例。"""
    from src.monitor_core import MonitorCore

    return MonitorCore(
        keywords=list(config.get("keywords") or []),
        exclude_keywords=list(config.get("exclude_keywords") or []),
        must_contain_keywords=list(config.get("must_contain_keywords") or []),
        notify_method=str(config.get("notify_method") or "none"),
        email=str(config.get("email") or ""),
        phone=str(config.get("phone") or ""),
        email_config=config.get("email_config"),
        sms_config=config.get("sms_config"),
        log_callback=log_callback,
        ai_config=config.get("ai_config"),
        storage_dir=str(data_dir),
        config=config,
    )


class MonitorManager:
    """按用户管理 BidMonitor 核心实例和后台任务。"""

    def __init__(
        self,
        data_root: str | Path,
        core_factory: CoreFactory | None = None,
        max_workers: int = 4,
    ) -> None:
        self.data_root = Path(data_root).expanduser().resolve()
        self.data_root.mkdir(parents=True, exist_ok=True)
        self.core_factory = core_factory or _default_core_factory
        self._executor = ThreadPoolExecutor(max_workers=max_workers, thread_name_prefix="bid-monitor")
        self._states: dict[str, _UserState] = {}
        self._lock = RLock()

    def _state(self, user_id: int | str) -> _UserState:
        normalized = safe_user_id(user_id)
        with self._lock:
            existing = self._states.get(normalized)
            if existing is not None:
                return existing
            data_dir = user_data_path(self.data_root, normalized)
            state = _UserState(normalized, data_dir, self._load_config(data_dir))
            self._states[normalized] = state
            return state

    def _load_config(self, data_dir: Path) -> dict[str, Any]:
        path = data_dir / "monitor.json"
        config = _default_config()
        if path.exists():
            try:
                saved = json.loads(path.read_text(encoding="utf-8"))
                if isinstance(saved, dict):
                    config.update(_redact(saved))
            except (OSError, ValueError):
                pass
        return config

    def _save_config(self, state: _UserState) -> None:
        path = state.data_dir / "monitor.json"
        path.write_text(
            json.dumps(_redact(state.config), ensure_ascii=True, indent=2),
            encoding="utf-8",
        )

    def update_config(self, user_id: int | str, config: dict[str, Any]) -> dict[str, Any]:
        """更新当前用户配置并返回已脱敏配置。"""
        if not isinstance(config, dict):
            raise ValueError("config must be an object")
        state = self._state(user_id)
        with self._lock:
            state.config.update(copy.deepcopy(_redact(config)))
            self._save_config(state)
            return copy.deepcopy(state.config)

    def _append_log(self, state: _UserState, message: str) -> None:
        with self._lock:
            state.logs.append(str(message))

    def _build_core(self, state: _UserState) -> Any:
        config = copy.deepcopy(state.config)
        core = self.core_factory(state.user_id, config, state.data_dir, lambda msg: self._append_log(state, msg))
        state.core = core
        return core

    def _execute_once(self, state: _UserState) -> None:
        try:
            core = self._build_core(state)
            result = core.run_once(stop_event=state.stop_event)
            with self._lock:
                state.last_result = result if isinstance(result, dict) else {"value": result}
                state.last_error = None
                state.last_run_time = datetime.now(timezone.utc).isoformat()
        except Exception as exc:
            with self._lock:
                state.last_error = str(exc)
            self._append_log(state, f"run failed: {exc}")

    def _run_once_task(self, state: _UserState) -> None:
        try:
            self._execute_once(state)
        finally:
            with self._lock:
                state.current_task_running = False
                state.future = None

    def _run_loop(self, state: _UserState) -> None:
        try:
            while not state.stop_event.is_set():
                state.current_task_running = True
                self._execute_once(state)
                state.current_task_running = False
                interval = max(1, int(state.config.get("interval_minutes") or 30)) * 60
                if state.stop_event.wait(interval):
                    break
        finally:
            with self._lock:
                state.current_task_running = False
                state.is_running = False
                state.future = None

    def start(self, user_id: int | str) -> bool:
        """启动当前用户的立即运行加周期任务。"""
        state = self._state(user_id)
        with self._lock:
            if state.is_running or state.current_task_running:
                return False
            state.stop_event = Event()
            state.is_running = True
            state.future = self._executor.submit(self._run_loop, state)
            return True

    def stop(self, user_id: int | str) -> bool:
        """发送停止信号并立即更新外部状态。"""
        state = self._state(user_id)
        with self._lock:
            was_active = state.is_running or state.current_task_running
            state.stop_event.set()
            state.is_running = False
            return was_active

    def run_once(self, user_id: int | str) -> bool:
        """提交一次后台运行，避免与周期任务并发。"""
        state = self._state(user_id)
        with self._lock:
            if state.is_running or state.current_task_running:
                return False
            state.stop_event = Event()
            state.current_task_running = True
            state.future = self._executor.submit(self._run_once_task, state)
            return True

    def wait_for_idle(self, user_id: int | str, timeout: float = 10) -> bool:
        """等待当前任务退出，主要用于测试和受控关闭。"""
        state = self._state(user_id)
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            with self._lock:
                if not state.current_task_running:
                    return True
            time.sleep(0.01)
        return not state.current_task_running

    def status(self, user_id: int | str) -> dict[str, Any]:
        state = self._state(user_id)
        with self._lock:
            return {
                "user_id": state.user_id,
                "data_dir": str(state.data_dir),
                "is_running": state.is_running,
                "current_task_running": state.current_task_running,
                "last_run_time": state.last_run_time,
                "last_result": copy.deepcopy(state.last_result),
                "last_error": state.last_error,
                "config": copy.deepcopy(state.config),
            }

    def logs(self, user_id: int | str, limit: int = 100) -> list[str]:
        state = self._state(user_id)
        with self._lock:
            bounded_limit = max(1, min(int(limit), 300))
            return list(state.logs)[-bounded_limit:]

    def results(self, user_id: int | str, limit: int = 50, offset: int = 0) -> dict[str, Any]:
        state = self._state(user_id)
        core = state.core or self._build_core(state)
        all_bids = core.storage.get_all()
        bounded_limit = max(1, min(int(limit), 200))
        bounded_offset = max(0, int(offset))
        items = all_bids[bounded_offset : bounded_offset + bounded_limit]
        return {
            "total": len(all_bids),
            "offset": bounded_offset,
            "limit": bounded_limit,
            "items": [
                {
                    "title": bid.title,
                    "url": bid.url,
                    "source": bid.source,
                    "publish_date": bid.publish_date or None,
                    "purchaser": bid.purchaser or None,
                }
                for bid in items
            ],
        }

    def clear_history(self, user_id: int | str) -> None:
        state = self._state(user_id)
        core = state.core or self._build_core(state)
        core.storage.clear_all()
        self._append_log(state, "history cleared")

    def close(self) -> None:
        with self._lock:
            states = list(self._states.values())
            for state in states:
                state.stop_event.set()
                state.is_running = False
        self._executor.shutdown(wait=True, cancel_futures=True)
