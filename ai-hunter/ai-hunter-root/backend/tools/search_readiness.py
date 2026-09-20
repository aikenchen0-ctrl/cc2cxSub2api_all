"""Search readiness for operator-facing warnings.

Does not probe the network. It only reports which discovery backend the
current settings will use, so the new-hunt page can warn before a job
burns Insight tokens.
"""

from __future__ import annotations

from typing import Any

from tools.search_limits import uses_paid_maps


def search_readiness(settings: Any) -> dict[str, Any]:
    """Return a JSON-safe snapshot of discovery search readiness."""
    paid = uses_paid_maps(settings)
    backend = "maps" if paid else "free_web"
    if paid:
        warning = ""
        new_hunt_note = ""
        resume_note = ""
    else:
        proxy = str(getattr(settings, "search_http_proxy", "") or "").strip()
        if proxy:
            warning = (
                "未配置可用的 Serper Key。发现阶段走 DuckDuckGo，"
                f"出口走 {proxy}（跳过沙箱 60376）。"
                "代理通不等于 DuckDuckGo 通。百科不当线索。"
            )
        else:
            warning = (
                "未配置可用的 Serper Key。发现阶段走 DuckDuckGo，本机被墙时会明确失败，"
                "不会把百科当线索。"
            )
        new_hunt_note = "提交任务仍会先跑 Insight。"
        resume_note = "续挖会跳过 Insight，但仍会搜索；被墙且本轮 0 新增会立刻停。"
    return {
        "discovery_backend": backend,
        "paid_maps": paid,
        "warning": warning,
        "new_hunt_note": new_hunt_note,
        "resume_note": resume_note,
    }
