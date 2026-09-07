"""The agent: four nodes, one conditional edge, and a bounded tool loop.

```
START -> load_context -> agent <-> tools -> finalise -> END
```

Deliberately shallow. There is no planner, no router and no reflection step: a
linear pipeline whose failures you understand beats a clever one whose failures
you do not, and every node here is one you can point at when an answer is wrong.

The loop is bounded and **never raises on the bound**. A partial answer that
names its own gaps is more useful than an error, and by the time the cap is hit
the reader has already waited.
"""

from __future__ import annotations

import json
import logging
from collections.abc import Sequence
from typing import Annotated, Any, TypedDict

from langchain_core.language_models import BaseChatModel
from langchain_core.messages import (
    AIMessage,
    BaseMessage,
    HumanMessage,
    RemoveMessage,
    SystemMessage,
    ToolMessage,
)
from langchain_core.tools import BaseTool
from langgraph.graph import END, START, StateGraph
from langgraph.graph.message import REMOVE_ALL_MESSAGES, add_messages
from langgraph.prebuilt import ToolNode

from urara_chat.agent.citations import extract_citations
from urara_chat.agent.context_card import ContextCardCache
from urara_chat.agent.prompts import (
    NO_ANSWER_PRODUCED,
    TOOL_BUDGET_SPENT,
    TOOL_BUDGET_SPENT_RESULT,
    build_system_prompt,
)
from urara_chat.backend.client import BackendClient
from urara_chat.llm.content import flatten_content

log = logging.getLogger(__name__)


def _accumulate(existing: list[Any], new: list[Any]) -> list[Any]:
    """Reducer for tool results: later rounds add to earlier ones.

    Without this a node's return value *replaces* the key, so a second round of
    tools would silently discard the first round's results and the answer would
    be cited from only its last retrieval.
    """
    return [*existing, *new]


class AgentState(TypedDict):
    """What one turn carries.

    `tool_results` holds every raw result the turn produced. It is what
    citations are computed from, which is why a stock ToolNode is not enough:
    the messages carry a serialised copy, and the objects are what matters.
    """

    snapshot_id: str  # concrete, resolved before the graph is entered
    language: str
    messages: Annotated[list[BaseMessage], add_messages]
    context_card: str
    tool_results: Annotated[list[Any], _accumulate]
    iterations: int
    truncated: bool
    # Tracked explicitly rather than inferred from the iteration count: the
    # post-budget pass must run exactly once, and a count can be reasoned about
    # wrongly by anyone who later changes when it is incremented.
    budget_notice_sent: bool
    citations: list[str]
    answer: str


def build_graph(
    model: BaseChatModel,
    tools: Sequence[BaseTool],
    cache: ContextCardCache,
    client: BackendClient,
    max_tool_iterations: int = 6,
) -> Any:
    """Compile the agent. Called once, not per turn."""
    bound = model.bind_tools(list(tools))

    async def load_context(state: AgentState) -> dict[str, Any]:
        """Fetch the inventory and put the system prompt in front of the turn."""
        card = await cache.get(client, state["snapshot_id"])
        system = SystemMessage(content=build_system_prompt(card, state["language"]))

        # add_messages appends, so prepending means replacing the list: the
        # system prompt has to be first or the model reads the question before
        # its instructions.
        return {
            "context_card": card,
            "messages": [RemoveMessage(id=REMOVE_ALL_MESSAGES), system, *state["messages"]],
        }

    async def agent(state: AgentState) -> dict[str, Any]:
        reply = await bound.ainvoke(state["messages"])
        return {"messages": [reply], "iterations": state["iterations"] + 1}

    tool_node = ToolNode(list(tools))

    async def run_tools(state: AgentState) -> dict[str, Any]:
        """Run the requested tools, keeping the results as well as the messages.

        ToolNode serialises each result into a ToolMessage, and a serialised
        copy is no use to citation extraction, which walks objects. The parsed
        result is kept where it parses; where it does not -- a wrapped tool
        answering with guidance after a failed lookup -- the string is kept as
        it is, and contributes no citation, which is correct: nothing was
        retrieved.
        """
        produced = await tool_node.ainvoke(state)
        messages: list[BaseMessage] = produced["messages"]

        results: list[Any] = []
        for message in messages:
            if isinstance(message, ToolMessage):
                results.append(_parsed(message.content))

        return {"messages": messages, "tool_results": results}

    async def finalise(state: AgentState) -> dict[str, Any]:
        """Assemble the answer and work out what it rested on. No model call."""
        answer = _last_answer(state["messages"])
        citations = extract_citations(state["tool_results"], answer)

        log.debug(
            "turn finalised",
            extra={
                "snapshot_id": state["snapshot_id"],
                "iterations": state["iterations"],
                "tool_results": len(state["tool_results"]),
                "citations": len(citations),
                "truncated": state["truncated"],
            },
        )
        return {"answer": answer, "citations": citations}

    async def spend_budget(state: AgentState) -> dict[str, Any]:
        """Tell the model its budget is gone and let it answer from what it has.

        Two things have to be true of what this appends, and neither was before
        04.R.

        Every tool call the model just asked for is answered, with a result
        saying it was not run. A turn whose last model message holds a call with
        no result is one most providers refuse to continue from -- and the whole
        point of this node is that there *is* a continuation.

        The notice itself goes in as a human turn, not a system one. The Google
        adapter collects every SystemMessage into the system instruction wherever
        it sits in the list, so a system-shaped notice arrives at the *front* of
        the context: an instruction to stop retrieving and answer, read before
        the retrieval it is talking about.
        """
        last = state["messages"][-1]
        refusals: list[BaseMessage] = [
            ToolMessage(content=TOOL_BUDGET_SPENT_RESULT, tool_call_id=call["id"] or "")
            for call in (last.tool_calls if isinstance(last, AIMessage) else [])
        ]
        return {
            "truncated": True,
            "budget_notice_sent": True,
            "messages": [*refusals, HumanMessage(content=TOOL_BUDGET_SPENT)],
        }

    def route(state: AgentState) -> str:
        """The only branching in the graph."""
        last = state["messages"][-1]
        wants_tools = isinstance(last, AIMessage) and bool(last.tool_calls)

        if not wants_tools:
            return "finalise"
        if state["budget_notice_sent"]:
            # The post-budget pass asked for tools anyway. It has had its one
            # extra turn; answer with what is held rather than looping.
            return "finalise"
        if state["iterations"] >= max_tool_iterations:
            return "spend_budget"
        return "tools"

    graph = StateGraph(AgentState)
    graph.add_node("load_context", load_context)
    graph.add_node("agent", agent)
    graph.add_node("tools", run_tools)
    graph.add_node("spend_budget", spend_budget)
    graph.add_node("finalise", finalise)

    graph.add_edge(START, "load_context")
    graph.add_edge("load_context", "agent")
    graph.add_conditional_edges(
        "agent", route, {"tools": "tools", "spend_budget": "spend_budget", "finalise": "finalise"}
    )
    graph.add_edge("tools", "agent")
    # Unconditionally back to the agent for one final pass, then out: the flag
    # set above is what stops that pass from routing into the loop again.
    graph.add_edge("spend_budget", "agent")
    graph.add_edge("finalise", END)

    return graph.compile()


def initial_state(snapshot_id: str, language: str, messages: list[BaseMessage]) -> AgentState:
    """A turn's starting state, with every field set.

    Written out rather than left to defaults: a missing key in a TypedDict is a
    KeyError deep inside a node, which reads as a graph bug rather than as a
    caller that forgot something.
    """
    return AgentState(
        snapshot_id=snapshot_id,
        language=language,
        messages=messages,
        context_card="",
        tool_results=[],
        iterations=0,
        truncated=False,
        budget_notice_sent=False,
        citations=[],
        answer="",
    )


def _last_answer(messages: Sequence[BaseMessage]) -> str:
    """The text the model actually wrote, latest first.

    Not simply the final message. A turn that spends its whole budget can end on
    an AIMessage that is *only* a tool call, whose content is the empty string --
    which is what a model emits when it wants a tool, and what this returned
    until 04.R. The loop is supposed to degrade to a partial answer; an empty
    body is not one, and the reader has already waited for it.
    """
    for message in reversed(messages):
        if isinstance(message, AIMessage):
            text = flatten_content(message.content).strip()
            if text:
                return text
    return NO_ANSWER_PRODUCED


def _parsed(content: Any) -> Any:
    """A tool result as an object where it is one, and as text where it is not."""
    if isinstance(content, str):
        try:
            return json.loads(content)
        except ValueError:
            return content
    return content
