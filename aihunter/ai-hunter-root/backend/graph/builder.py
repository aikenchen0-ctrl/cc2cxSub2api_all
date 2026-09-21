"""LangGraph StateGraph builder — assembles the hunt pipeline with loop."""

from typing import Callable

from langgraph.graph import END, StateGraph

from graph.node_stage import wrap_graph_node
from graph.state import HuntState


def _noop_node(state: HuntState) -> dict:
    """Placeholder node used when real agent is not yet wired."""
    return {}


def build_graph(
    *,
    parse_description_node: Callable | None = None,
    insight_node: Callable | None = None,
    keyword_gen_node: Callable | None = None,
    search_node: Callable | None = None,
    lead_extract_node: Callable | None = None,
    evaluate_node: Callable | None = None,
    should_continue_fn: Callable | None = None,
    email_craft_node: Callable | None = None,
    checkpointer=None,
    on_node_start: Callable[[str], None] | None = None,
) -> StateGraph:
    """Build and compile the AI Hunter StateGraph.

    All node callables default to no-op placeholders so the graph structure
    can be tested independently of agent implementations.

    Args:
        insight_node: InsightAgent node function.
        keyword_gen_node: KeywordGenAgent node function.
        search_node: SearchAgent node function.
        lead_extract_node: LeadExtractAgent node function.
        evaluate_node: evaluate_progress function.
        should_continue_fn: Conditional edge function returning "continue" | "finish".
        email_craft_node: EmailCraftAgent node function.
        checkpointer: LangGraph checkpointer for persistence.
        on_node_start: Optional callback fired with the node name before work.

    Returns:
        Compiled StateGraph ready to invoke.
    """
    builder = StateGraph(HuntState)

    def _node(name: str, fn: Callable | None) -> Callable:
        return wrap_graph_node(name, fn, on_start=on_node_start, fallback=_noop_node)

    # ── Register nodes ──────────────────────────────────────────────────
    builder.add_node("parse_description", _node("parse_description", parse_description_node))
    builder.add_node("insight", _node("insight", insight_node))
    builder.add_node("keyword_gen", _node("keyword_gen", keyword_gen_node))
    builder.add_node("search", _node("search", search_node))
    builder.add_node("lead_extract", _node("lead_extract", lead_extract_node))
    builder.add_node("evaluate", _node("evaluate", evaluate_node))
    builder.add_node("email_craft", _node("email_craft", email_craft_node))

    # ── Edges ───────────────────────────────────────────────────────────
    # parse_description always runs first (no-op when description is empty)
    builder.set_entry_point("parse_description")
    builder.add_edge("parse_description", "insight")
    builder.add_edge("insight", "keyword_gen")

    # Hunting loop: KeywordGen → Search → LeadExtract → Evaluate
    builder.add_edge("keyword_gen", "search")
    builder.add_edge("search", "lead_extract")
    builder.add_edge("lead_extract", "evaluate")

    # Conditional edge: evaluate decides continue, email_craft, or done
    _base_should_continue = should_continue_fn or (lambda _: "finish")

    def _route_after_evaluate(state: HuntState) -> str:
        decision = _base_should_continue(state)
        if decision == "finish":
            if state.get("enable_email_craft", False):
                return "email_craft"
            return "done"
        return "continue"

    builder.add_conditional_edges(
        "evaluate",
        _route_after_evaluate,
        {"continue": "keyword_gen", "email_craft": "email_craft", "done": END},
    )

    builder.add_edge("email_craft", END)

    # ── Compile ─────────────────────────────────────────────────────────
    return builder.compile(checkpointer=checkpointer)
