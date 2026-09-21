"""Mark hunt stage when a graph node starts, not only when it finishes.

LangGraph astream yields after a node returns. If Insight raises mid-call,
the last completed stage stays on disk (usually parse_description) and the
queue page looks like description parsing failed.
"""

from __future__ import annotations

import asyncio
from collections.abc import Callable
from typing import Any

OnNodeStart = Callable[[str], None]


def with_stage(result: Any, node_name: str) -> dict:
    """Ensure node output records the stage that just ran."""
    payload = dict(result or {}) if isinstance(result, dict) else {}
    if not str(payload.get("current_stage") or "").strip():
        payload["current_stage"] = node_name
    return payload


def wrap_graph_node(
    node_name: str,
    fn: Callable[..., Any] | None,
    *,
    on_start: OnNodeStart | None = None,
    fallback: Callable[..., Any],
) -> Callable[..., Any]:
    """Wrap a graph node so on_start fires before the real work."""
    inner = fn or fallback
    if asyncio.iscoroutinefunction(inner):
        async def async_wrapped(state):
            if on_start is not None:
                on_start(node_name)
            return with_stage(await inner(state), node_name)

        return async_wrapped

    def sync_wrapped(state):
        if on_start is not None:
            on_start(node_name)
        return with_stage(inner(state), node_name)

    return sync_wrapped
