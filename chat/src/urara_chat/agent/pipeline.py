"""The public face of the agent: one function Phase 05 calls.

It does **no persistence**. The caller supplies the history and stores the
result, which keeps this testable without a backend and leaves the transcript's
home to whoever owns it.

The dependencies -- model, tool factory, context cache, backend client -- are
installed once by the lifespan rather than passed on every call, so a graph is
compiled once per snapshot and the process keeps one client. `configure_pipeline`
is how they get here; calling `answer` before that is a programming error and
says so.
"""

from __future__ import annotations

import logging
import time
from collections import OrderedDict
from collections.abc import Callable, Sequence
from dataclasses import dataclass, field
from typing import Any

from langchain_core.language_models import BaseChatModel
from langchain_core.messages import AIMessage, BaseMessage, HumanMessage
from langchain_core.tools import BaseTool

from urara_chat.agent.context_card import MAX_CACHED_CARDS, ContextCardCache
from urara_chat.agent.graph import build_graph, initial_state
from urara_chat.backend.client import BackendClient
from urara_chat.backend.models import Message

log = logging.getLogger(__name__)

# One graph per snapshot, bounded like the cards beside them. The same
# ceiling: a pod that is holding a card for a snapshot is the pod that will
# be asked about it again.
MAX_CACHED_GRAPHS = MAX_CACHED_CARDS


@dataclass
class AgentAnswer:
    """One turn's answer, and everything needed to explain it later."""

    text: str
    citations: list[str]
    # Name and args for every call made, so a wrong answer can be traced to what
    # it actually looked at rather than to what it says it looked at.
    tool_calls: list[dict[str, Any]]
    iterations: int
    truncated: bool
    model: str
    latency_ms: int
    # Token counts where the provider reports them, and empty where it does not.
    # Never guessed: a fabricated number in a cost report is worse than a gap.
    usage: dict[str, int] = field(default_factory=dict)


def truncate_history(history: Sequence[Message], limit: int) -> list[Message]:
    """Keep the most recent turns.

    Dropped from the front: an unbounded transcript is an unbounded bill, and
    the prompt grows on every turn. Recent turns are what a follow-up question
    refers to.
    """
    kept = list(history[-limit:]) if limit > 0 else []

    # An assistant message with no question above it reads as the model talking
    # to itself, so a pair split by the boundary loses its orphaned half.
    while kept and kept[0].role != "user":
        kept.pop(0)
    return kept


def to_langchain_messages(history: Sequence[Message]) -> list[BaseMessage]:
    """Stored turns as model input.

    Stored `system` messages are dropped. The system prompt is rebuilt each turn
    from the current card and language, and one saved three turns ago would
    fight it -- with the stale copy winning, because it comes first.
    """
    converted: list[BaseMessage] = []
    for message in history:
        if message.role == "user":
            converted.append(HumanMessage(content=message.content))
        elif message.role == "assistant":
            converted.append(AIMessage(content=message.content))
    return converted


class Pipeline:
    """The agent, wired. One compiled graph per snapshot.

    Not one graph for the process, which is what this was until 04.R. A tool
    closes over the snapshot it reads (`tools/registry.py`), so a graph built
    from one snapshot's tools can only ever answer about that snapshot -- while
    the HTTP layer takes a snapshot per request, and at start-up there is no
    snapshot to bind. A process-wide graph would have answered every question
    from whichever snapshot happened to be bound first.

    So the tools arrive as a factory and the graph is built on first use for
    each snapshot. 04.4's "compiled once" still holds, per snapshot rather than
    per process: a turn never compiles a graph a previous turn on the same
    snapshot already built.
    """

    def __init__(
        self,
        model: BaseChatModel,
        tools_for: Callable[[str], Sequence[BaseTool]],
        cache: ContextCardCache,
        client: BackendClient,
        *,
        model_name: str,
        max_history_messages: int = 20,
        max_tool_iterations: int = 6,
        max_graphs: int = MAX_CACHED_GRAPHS,
    ) -> None:
        self._model = model
        self._tools_for = tools_for
        self._cache = cache
        self._client = client
        self._model_name = model_name
        self._max_history = max_history_messages
        self._max_tool_iterations = max_tool_iterations
        self._max_graphs = max_graphs
        self._graphs: OrderedDict[str, Any] = OrderedDict()

    def _graph_for(self, snapshot_id: str) -> Any:
        """This snapshot's compiled graph, building it once.

        Bounded like the context card cache beside it, and for the same reason:
        a long-running pod asked about many snapshots would otherwise hold a
        compiled graph for every one it had ever seen.
        """
        graph = self._graphs.get(snapshot_id)
        if graph is not None:
            self._graphs.move_to_end(snapshot_id)
            return graph

        graph = build_graph(
            self._model,
            self._tools_for(snapshot_id),
            self._cache,
            self._client,
            max_tool_iterations=self._max_tool_iterations,
        )
        self._graphs[snapshot_id] = graph
        while len(self._graphs) > self._max_graphs:
            self._graphs.popitem(last=False)
        return graph

    async def answer(
        self,
        question: str,
        snapshot_id: str,
        history: Sequence[Message],
        language: str = "EN",
    ) -> AgentAnswer:
        """Answer one question about one snapshot.

        No persistence: the caller supplies the history and stores the result, so
        this stays testable without a backend and the transcript's home stays the
        caller's decision.
        """
        # An assertion rather than a comment: reading the wrong snapshot produces
        # a confidently wrong answer, which is the failure worth stopping here.
        if snapshot_id == "latest":
            raise ValueError(
                "answer() requires a concrete snapshot ID, not 'latest'; resolve it first"
            )

        messages = to_langchain_messages(truncate_history(history, self._max_history))
        messages.append(HumanMessage(content=question))

        started = time.perf_counter()
        graph = self._graph_for(snapshot_id)
        final = await graph.ainvoke(initial_state(snapshot_id, language, messages))
        latency_ms = int((time.perf_counter() - started) * 1000)

        produced: list[BaseMessage] = final["messages"]
        answer = AgentAnswer(
            text=final["answer"],
            citations=final["citations"],
            tool_calls=_tool_calls(produced),
            iterations=final["iterations"],
            truncated=final["truncated"],
            model=self._model_name,
            latency_ms=latency_ms,
            usage=_usage(produced),
        )

        # One line per turn. Phase 08 costs the feature from these, so it carries
        # what was spent as well as what was done.
        log.info(
            "turn answered",
            extra={
                "snapshot_id": snapshot_id,
                "language": language,
                "iterations": answer.iterations,
                "tools": [call["name"] for call in answer.tool_calls],
                "citations": len(answer.citations),
                "latency_ms": answer.latency_ms,
                "usage": answer.usage,
                "truncated": answer.truncated,
            },
        )
        return answer


def _tool_calls(messages: Sequence[BaseMessage]) -> list[dict[str, Any]]:
    """Every tool call the turn made, in order."""
    calls: list[dict[str, Any]] = []
    for message in messages:
        if isinstance(message, AIMessage):
            calls.extend({"name": c["name"], "args": c["args"]} for c in message.tool_calls)
    return calls


def _usage(messages: Sequence[BaseMessage]) -> dict[str, int]:
    """Token counts summed over every model call in the turn.

    Empty when the provider reported nothing. A turn costs the sum of its calls,
    not the last one, and a tool loop makes several.
    """
    totals: dict[str, int] = {}
    for message in messages:
        if not isinstance(message, AIMessage):
            continue
        usage = message.usage_metadata
        if not usage:
            continue
        for key, value in usage.items():
            if isinstance(value, int):
                totals[key] = totals.get(key, 0) + value
    return totals


_pipeline: Pipeline | None = None


def configure_pipeline(pipeline: Pipeline) -> None:
    """Install the process's pipeline. Called by the lifespan."""
    global _pipeline
    _pipeline = pipeline


def get_pipeline() -> Pipeline:
    if _pipeline is None:
        raise RuntimeError("the agent pipeline has not been configured; call configure_pipeline")
    return _pipeline


async def answer(
    question: str,
    snapshot_id: str,
    history: Sequence[Message],
    language: str = "EN",
) -> AgentAnswer:
    """Answer one question about one snapshot.

    The module-level entry point, delegating to the pipeline the lifespan
    installed. Kept as a free function because that is what the HTTP layer
    calls, and it has no business knowing how the agent is assembled.
    """
    return await get_pipeline().answer(question, snapshot_id, history, language)
