"""The agent's control flow.

Every test scripts the model, so nothing here needs a key or a network. What is
worth asserting is the shape of the loop: that it runs the tools the model asks
for, that it stops, and that stopping produces a partial answer rather than an
exception — the reader has already waited, and an answer naming its own gaps
beats an error.
"""

import json
from pathlib import Path
from typing import Any

import pytest
from fakes import CountingContextClient, FakeChatModel
from langchain_core.messages import AIMessage, HumanMessage, SystemMessage
from langchain_core.tools import StructuredTool
from pydantic import BaseModel

from urara_chat.agent.citations import extract_citations
from urara_chat.agent.context_card import ContextCardCache
from urara_chat.agent.graph import build_graph, initial_state
from urara_chat.agent.prompts import TOOL_BUDGET_SPENT
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
        """And still answers. A partial answer that names its own gaps is more
        useful than an exception, and the reader has already waited."""
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
        """The post-budget pass asks for tools too, in this script. It must
        answer with what it holds rather than going round again."""
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
        """A wrapped tool answers with advice when a lookup fails. It parses as
        nothing, contributes no citation, and that is correct: nothing was
        retrieved."""
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
