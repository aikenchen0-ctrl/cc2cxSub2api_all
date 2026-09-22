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
        "ai_enabled": False,
        "ai_prompt": "",
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
    runtime_config: dict[str, Any] = field(default_factory=dict)
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
        voice_phone=str(config.get("voice_phone") or ""),
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

    def _set_runtime_config(self, state: _UserState, runtime_config: dict[str, Any] | None) -> None:
        """只在进程内保存 Fastify 注入的运行时凭据，禁止进入用户配置文件。"""
        if not isinstance(runtime_config, dict):
            state.runtime_config = {}
            return
        allowed = {"notify_method", "email", "phone", "voice_phone", "ai_config", "email_config", "sms_config", "wechat_config", "voice_config", "contacts"}
        state.runtime_config = {
            key: copy.deepcopy(value)
            for key, value in runtime_config.items()
            if key in allowed and isinstance(value, (dict, list, str))
        }

    def update_config(self, user_id: int | str, config: dict[str, Any]) -> dict[str, Any]:
        """更新当前用户配置并返回已脱敏配置。"""
        if not isinstance(config, dict):
            raise ValueError("config must be an object")
        state = self._state(user_id)
        with self._lock:
            state.config.update(copy.deepcopy(_redact(config)))
            self._save_config(state)
            return copy.deepcopy(state.config)

    def sites(self, user_id: int | str) -> dict[str, Any]:
        """返回内置站点和当前用户的自定义站点配置。"""
        state = self._state(user_id)
        enabled = set((state.config.get("crawler") or {}).get("enabled_sites") or [])
        from src.monitor_core import get_default_sites

        builtins = [
            {
                "key": key,
                "name": info.get("name", key),
                "url": info.get("url", ""),
                "enabled": key in enabled,
            }
            for key, info in get_default_sites().items()
        ]
        custom = copy.deepcopy(state.config.get("custom_sites") or [])
        return {"sites": builtins, "custom_sites": custom}

    def update_sites(
        self,
        user_id: int | str,
        enabled_sites: list[str],
        custom_sites: list[dict[str, Any]],
    ) -> dict[str, Any]:
        """保存当前用户的站点选择，并限制自定义站点字段规模。"""
        state = self._state(user_id)
        normalized_enabled = [str(item).strip() for item in enabled_sites if str(item).strip()][:200]
        normalized_custom = []
        for item in custom_sites[:50]:
            if not isinstance(item, dict):
                continue
            name = str(item.get("name") or "").strip()[:200]
            url = str(item.get("url") or "").strip()[:2000]
            if name and url:
                normalized_custom.append({"name": name, "url": url})
        with self._lock:
            crawler = state.config.setdefault("crawler", {})
            crawler["enabled_sites"] = normalized_enabled
            state.config["custom_sites"] = normalized_custom
            self._save_config(state)
            return self.sites(user_id)

    def test_notification(
        self,
        user_id: int | str,
        channel: str,
        target: str,
        runtime_config: dict[str, Any] | None = None,
    ) -> dict[str, Any]:
        """使用服务端运行时凭据发送一条测试通知。"""
        state = self._state(user_id)
        config = copy.deepcopy(state.config)
        if isinstance(runtime_config, dict):
            config.update(copy.deepcopy(runtime_config))
        target = str(target or "").strip()
        channel = str(channel or "").strip().lower()
        if channel not in {"email", "sms", "wechat", "voice"}:
            raise ValueError("unsupported notification channel")
        if channel != "wechat" and not target:
            raise ValueError("notification target is required")
        from src.database.storage import BidInfo

        test_bid = BidInfo(
            title="Bid monitor notification test",
            url="https://example.com/bid-monitor-test",
            publish_date=datetime.now(timezone.utc).date().isoformat(),
            source="Bid Monitor",
            purchaser="System",
        )
        bids = [test_bid]
        if channel == "email":
            from src.notifier.email import EmailNotifier

            email_config = dict(config.get("email_config") or {})
            if not email_config.get("smtp_server") or not email_config.get("sender") or not email_config.get("password"):
                raise ValueError("email notification is not configured")
            email_config["receiver"] = target
            success = EmailNotifier(email_config).send_test()
        elif channel == "sms":
            from src.notifier.sms import SMSNotifier

            if not config.get("sms_config"):
                raise ValueError("sms notification is not configured")
            success = SMSNotifier(config["sms_config"]).send_test(target)
        elif channel == "wechat":
            from src.notifier.wechat import WeChatNotifier

            if not config.get("wechat_config"):
                raise ValueError("wechat notification is not configured")
            success = WeChatNotifier(config["wechat_config"]).send_test()
        else:
            from src.notifier.voice import VoiceNotifier

            if not config.get("voice_config"):
                raise ValueError("voice notification is not configured")
            success = VoiceNotifier(config["voice_config"]).send_test(target)
        return {"success": bool(success), "channel": channel, "target": target}

    def test_ai(self, user_id: int | str, runtime_config: dict[str, Any] | None = None) -> dict[str, Any]:
        """调用一次 AI 过滤器验证当前服务端模型配置。"""
        state = self._state(user_id)
        config = copy.deepcopy(state.config)
        if isinstance(runtime_config, dict):
            config.update(copy.deepcopy(runtime_config))
        ai_config = config.get("ai_config")
        if not isinstance(ai_config, dict) or not ai_config.get("enable"):
            raise ValueError("ai filter is not enabled")
        from src.ai_guard import AIGuard

        relevant, reason = AIGuard(ai_config, log_callback=lambda message: self._append_log(state, message)).check_relevance(
            "Bid monitor configuration test",
            "This request verifies the configured AI filter.",
            raise_on_error=True,
        )
        return {"success": True, "relevant": bool(relevant), "reason": str(reason)}

    def _append_log(self, state: _UserState, message: str) -> None:
        with self._lock:
            state.logs.append(str(message))

    def _build_core(self, state: _UserState) -> Any:
        config = copy.deepcopy(state.config)
        for key, value in state.runtime_config.items():
            config[key] = copy.deepcopy(value)
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
                state.runtime_config = {}

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
                state.runtime_config = {}

    def start(self, user_id: int | str, runtime_config: dict[str, Any] | None = None) -> bool:
        """启动当前用户的立即运行加周期任务。"""
        state = self._state(user_id)
        with self._lock:
            if state.is_running or state.current_task_running:
                return False
            state.stop_event = Event()
            self._set_runtime_config(state, runtime_config)
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

    def run_once(self, user_id: int | str, runtime_config: dict[str, Any] | None = None) -> bool:
        """提交一次后台运行，避免与周期任务并发。"""
        state = self._state(user_id)
        with self._lock:
            if state.is_running or state.current_task_running:
                return False
            state.stop_event = Event()
            self._set_runtime_config(state, runtime_config)
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
