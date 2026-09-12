"""The agent's control flow."""

import json
from pathlib import Path
from typing import Any

import pytest
from fakes import CountingContextClient, FakeChatModel
from langchain_core.messages import AIMessage, HumanMessage, SystemMessage, ToolMessage
from langchain_core.tools import StructuredTool
from pydantic import BaseModel

from urara_chat.agent.citations import extract_citations
from urara_chat.agent.context_card import ContextCardCache
from urara_chat.agent.graph import build_graph, initial_state
from urara_chat.agent.prompts import (
    NO_ANSWER_PRODUCED,
    TOOL_BUDGET_SPENT,
    TOOL_BUDGET_SPENT_RESULT,
)
from urara_chat.backend.models import SnapshotContext

FIXTURES = Path(__file__).parent / "fixtures"
FACT_ORDERS = "ordering/fact_orders"


def jaffle() -> SnapshotContext:
    return SnapshotContext.model_validate(json.loads((FIXTURES / "context.json").read_text()))


class Args(BaseModel):
    ids: list[str] = []


def make_tool(name: str = "get_tables", result: Any = None, raises: Exception | None = None):
    """One scripted tool, returning a result in the shape the real ones do."""
    payload = (
        result
        if result is not None
        else {
            "items": [{"table": {"id": FACT_ORDERS, "name": "fact_orders"}}],
            "truncated": False,
            "total": 1,
            "missing": [],
        }
    )

    async def fn(**kwargs: Any) -> Any:
        if raises is not None:
            raise raises
        return payload

    return StructuredTool.from_function(
        coroutine=fn, name=name, description="a scripted tool", args_schema=Args
    )


def call_tool(name: str = "get_tables", call_id: str = "1") -> AIMessage:
    return AIMessage(
        content="", tool_calls=[{"name": name, "args": {"ids": [FACT_ORDERS]}, "id": call_id}]
    )


async def run(
    replies: list[AIMessage],
    tools: list[Any] | None = None,
    *,
    question: str = "what is in this model?",
    max_tool_iterations: int = 6,
) -> tuple[dict[str, Any], FakeChatModel, CountingContextClient]:
    model = FakeChatModel(replies)
    client = CountingContextClient(jaffle())
    cache = ContextCardCache(ttl_seconds=300.0)
    graph = build_graph(
        model, tools or [make_tool()], cache, client, max_tool_iterations=max_tool_iterations
    )

    final = await graph.ainvoke(initial_state("snap-1", "EN", [HumanMessage(content=question)]))
    return final, model, client


class TestTheHappyPaths:
    async def test_no_tool_calls_goes_straight_to_finalise(self) -> None:
        final, model, _ = await run([AIMessage(content="fact_orders is one row per order.")])

        assert final["iterations"] == 1
        assert final["answer"] == "fact_orders is one row per order."
        assert final["truncated"] is False
        assert model.call_count == 1

    async def test_one_round_of_tools_then_an_answer(self) -> None:
        final, model, _ = await run(
            [call_tool(), AIMessage(content="fact_orders holds one row per order.")]
        )

        assert model.call_count == 2
        assert len(final["tool_results"]) == 1
        assert final["answer"].startswith("fact_orders")

    async def test_two_rounds_run_both(self) -> None:
        final, model, _ = await run(
            [
                call_tool(call_id="1"),
                call_tool(call_id="2"),
                AIMessage(content="fact_orders, twice looked at."),
            ]
        )

        assert model.call_count == 3
        assert len(final["tool_results"]) == 2

    async def test_the_model_is_bound_to_the_tools(self) -> None:
        _, model, _ = await run([AIMessage(content="done")])
        assert [t.name for t in model.bound_tools] == ["get_tables"]


class TestTheSystemPrompt:
    async def test_it_is_first_and_appears_once(self) -> None:
        """The model must read its instructions before the question."""
        _, model, _ = await run([AIMessage(content="done")])

        sent = model.calls[0]
        assert isinstance(sent[0], SystemMessage)
        assert sum(isinstance(m, SystemMessage) for m in sent) == 1

    async def test_the_question_survives_the_prepend(self) -> None:
        _, model, _ = await run([AIMessage(content="done")], question="where are refunds?")

        sent = model.calls[0]
        assert isinstance(sent[1], HumanMessage)
        assert sent[1].content == "where are refunds?"

    async def test_the_card_is_in_the_prompt_and_the_state(self) -> None:
        final, model, _ = await run([AIMessage(content="done")])

        assert FACT_ORDERS in final["context_card"]
        assert FACT_ORDERS in str(model.calls[0][0].content)


class TestTheBoundedLoop:
    async def test_a_model_that_always_asks_for_tools_stops(self) -> None:
        """And still answers."""
        final, model, _ = await run([call_tool()], max_tool_iterations=3)

        assert final["truncated"] is True
        assert final["answer"] != "" or final["citations"] == []
        assert model.call_count <= 5, "the loop did not stop"

    async def test_it_does_not_raise_on_the_budget(self) -> None:
        final, _, _ = await run([call_tool()], max_tool_iterations=2)
        assert "truncated" in final

    async def test_the_budget_notice_is_sent_exactly_once(self) -> None:
        _, model, _ = await run([call_tool()], max_tool_iterations=3)

        last_sent = model.calls[-1]
        notices = [m for m in last_sent if str(m.content) == TOOL_BUDGET_SPENT]
        assert len(notices) == 1

    async def test_the_final_pass_cannot_loop_again(self) -> None:
        """The post-budget pass asks for tools too, in this script."""
        final, model, _ = await run([call_tool()], max_tool_iterations=2)

        # load + (agent, tools) x2 + budget + one final agent pass.
        assert model.call_count == 3
        assert final["budget_notice_sent"] is True

    async def test_a_partial_answer_after_the_budget_is_returned(self) -> None:
        replies = [call_tool(), call_tool(), AIMessage(content="Only fact_orders confirmed.")]
        final, _, _ = await run(replies, max_tool_iterations=2)

        assert final["truncated"] is True
        assert final["answer"] == "Only fact_orders confirmed."
        assert final["citations"] == [FACT_ORDERS]


class TestTheBudgetNoticeReachesTheModel:
    """Where the notice lands, not just that it was appended."""

    async def test_the_notice_is_the_last_thing_the_model_reads(self) -> None:
        _, model, _ = await run([call_tool()], max_tool_iterations=2)

        last_sent = model.calls[-1]
        assert str(last_sent[-1].content) == TOOL_BUDGET_SPENT

    async def test_the_notice_is_not_a_system_message(self) -> None:
        """A system-shaped notice is merged into the system instruction and"""
        _, model, _ = await run([call_tool()], max_tool_iterations=2)

        notice = next(m for m in model.calls[-1] if str(m.content) == TOOL_BUDGET_SPENT)
        assert isinstance(notice, HumanMessage)

    async def test_exactly_one_system_message_still_reaches_the_model(self) -> None:
        _, model, _ = await run([call_tool()], max_tool_iterations=2)

        systems = [m for m in model.calls[-1] if isinstance(m, SystemMessage)]
        assert len(systems) == 1
        assert "SNAPSHOT INVENTORY" in str(systems[0].content)

    async def test_no_tool_call_is_left_unanswered(self) -> None:
        """A turn whose last model message holds a call with no result is one"""
        _, model, _ = await run([call_tool()], max_tool_iterations=3)

        sent = model.calls[-1]
        asked = {
            call["id"]
            for message in sent
            if isinstance(message, AIMessage)
            for call in message.tool_calls
        }
        answered = {m.tool_call_id for m in sent if isinstance(m, ToolMessage)}
        assert asked <= answered, f"unanswered tool calls: {sorted(asked - answered)}"

    async def test_the_unrun_calls_say_so(self) -> None:
        _, model, _ = await run([call_tool()], max_tool_iterations=2)

        bodies = [str(m.content) for m in model.calls[-1] if isinstance(m, ToolMessage)]
        assert TOOL_BUDGET_SPENT_RESULT in bodies


class TestTheAnswerIsNeverEmpty:
    async def test_a_script_of_bare_tool_calls_still_answers(self) -> None:
        """A model asking for a tool emits empty content."""
        final, _, _ = await run([call_tool()], max_tool_iterations=3)

        assert final["truncated"] is True
        assert final["answer"].strip() != ""
        assert final["answer"] == NO_ANSWER_PRODUCED

    async def test_the_last_text_the_model_wrote_wins(self) -> None:
        """Not the fallback, and not an earlier answer either."""
        replies = [
            AIMessage(content="a first thought", tool_calls=[]),
            call_tool(),
            call_tool(),
        ]
        final, _, _ = await run(replies, max_tool_iterations=2)

        assert final["answer"] == "a first thought"

    async def test_an_answer_after_the_budget_still_wins_over_the_fallback(self) -> None:
        replies = [call_tool(), call_tool(), AIMessage(content="Only fact_orders confirmed.")]
        final, _, _ = await run(replies, max_tool_iterations=2)

        assert final["answer"] == "Only fact_orders confirmed."

    async def test_whitespace_is_not_an_answer(self) -> None:
        final, _, _ = await run([AIMessage(content="   \n  ")])

        assert final["answer"] == NO_ANSWER_PRODUCED


class TestToolResultsAndCitations:
    async def test_results_accumulate_in_order(self) -> None:
        tools = [
            make_tool("get_tables"),
            make_tool("list_domains", result={"items": [{"id": "d/x"}]}),
        ]
        replies = [
            call_tool("get_tables", "1"),
            call_tool("list_domains", "2"),
            AIMessage(content="done"),
        ]
        final, _, _ = await run(replies, tools)

        assert len(final["tool_results"]) == 2
        assert final["tool_results"][0]["items"][0]["table"]["id"] == FACT_ORDERS
        assert final["tool_results"][1]["items"][0]["id"] == "d/x"

    async def test_results_are_parsed_objects_not_strings(self) -> None:
        """Citation extraction walks objects; a serialised copy is no use."""
        final, _, _ = await run([call_tool(), AIMessage(content="done")])
        assert isinstance(final["tool_results"][0], dict)

    async def test_a_guidance_string_is_kept_as_text(self) -> None:
        """A wrapped tool answers with advice when a lookup fails."""
        guidance = "No table with id 'nope/missing' in this model. Call search_model."
        tools = [make_tool(result=guidance)]
        final, _, _ = await run(
            [call_tool(), AIMessage(content="nope/missing is not documented.")], tools
        )

        assert final["tool_results"] == [guidance]
        assert final["citations"] == []

    async def test_citations_match_calling_the_extractor_directly(self) -> None:
        answer = "fact_orders holds one row per order."
        final, _, _ = await run([call_tool(), AIMessage(content=answer)])

        assert final["citations"] == extract_citations(final["tool_results"], answer)
        assert final["citations"] == [FACT_ORDERS]

    async def test_an_unmentioned_table_is_not_cited(self) -> None:
        final, _, _ = await run([call_tool(), AIMessage(content="Nothing relevant here.")])
        assert final["citations"] == []


class TestTheContextCache:
    async def test_two_turns_on_one_snapshot_fetch_once(self) -> None:
        model = FakeChatModel([AIMessage(content="done")])
        client = CountingContextClient(jaffle())
        cache = ContextCardCache(ttl_seconds=300.0)
        graph = build_graph(model, [make_tool()], cache, client)  # type: ignore[arg-type]

        for _ in range(2):
            await graph.ainvoke(initial_state("snap-1", "EN", [HumanMessage(content="q")]))

        assert client.calls == 1


class TestErrors:
    async def test_an_unexpected_tool_error_propagates(self) -> None:
        """A bug must not become a sentence the model apologises about."""
        tools = [make_tool(raises=ValueError("a real bug"))]

        with pytest.raises(ValueError, match="a real bug"):
            await run([call_tool(), AIMessage(content="done")], tools)


class TestCompilation:
    async def test_the_graph_is_compiled_once_and_reused(self) -> None:
        model = FakeChatModel([AIMessage(content="done")])
        client = CountingContextClient(jaffle())
        cache = ContextCardCache(ttl_seconds=300.0)

        graph = build_graph(model, [make_tool()], cache, client)  # type: ignore[arg-type]
        first = await graph.ainvoke(initial_state("snap-1", "EN", [HumanMessage(content="a")]))
        second = await graph.ainvoke(initial_state("snap-1", "EN", [HumanMessage(content="b")]))

        assert first["answer"] == second["answer"] == "done"
        assert model.call_count == 2, "one compiled graph served both turns"
