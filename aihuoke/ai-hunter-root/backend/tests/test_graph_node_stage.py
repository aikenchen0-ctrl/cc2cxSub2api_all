"""Tests for graph/node_stage.py."""

import pytest

from graph.node_stage import with_stage, wrap_graph_node


def _fallback(_state):
    return {}


def test_with_stage_fills_missing_current_stage():
    assert with_stage({}, "insight")["current_stage"] == "insight"
    assert with_stage(None, "search")["current_stage"] == "search"


def test_with_stage_keeps_explicit_stage():
    assert with_stage({"current_stage": "evaluate", "x": 1}, "insight")["current_stage"] == "evaluate"


def test_wrap_sync_node_calls_on_start_before_work():
    order: list[str] = []

    def inner(_state):
        order.append("work")
        return {"ok": True}

    def on_start(name: str):
        order.append(f"start:{name}")

    wrapped = wrap_graph_node("insight", inner, on_start=on_start, fallback=_fallback)
    result = wrapped({})
    assert order == ["start:insight", "work"]
    assert result["ok"] is True
    assert result["current_stage"] == "insight"


@pytest.mark.asyncio
async def test_wrap_async_node_calls_on_start_before_work():
    order: list[str] = []

    async def inner(_state):
        order.append("work")
        return {"ok": True}

    def on_start(name: str):
        order.append(f"start:{name}")

    wrapped = wrap_graph_node("insight", inner, on_start=on_start, fallback=_fallback)
    result = await wrapped({})
    assert order == ["start:insight", "work"]
    assert result["current_stage"] == "insight"


@pytest.mark.asyncio
async def test_wrap_async_node_marks_stage_even_when_inner_raises():
    started: list[str] = []

    async def inner(_state):
        raise RuntimeError("InsightAgent failed")

    wrapped = wrap_graph_node(
        "insight",
        inner,
        on_start=started.append,
        fallback=_fallback,
    )
    with pytest.raises(RuntimeError, match="InsightAgent failed"):
        await wrapped({})
    assert started == ["insight"]
