"""Scripted stand-ins, so the unit suite never needs a key or a network.

`FakeChatModel` is the most reused fixture in Phase 04. It replays a list of
`AIMessage`s, one per call, so a test reads as the conversation it is describing:

    FakeChatModel([
        AIMessage(content="", tool_calls=[{"name": "get_tables", "args": {}, "id": "1"}]),
        AIMessage(content="fact_orders is one row per order."),
    ])
"""

from __future__ import annotations

from collections.abc import Sequence
from typing import Any

from langchain_core.callbacks import AsyncCallbackManagerForLLMRun
from langchain_core.language_models import BaseChatModel
from langchain_core.messages import AIMessage, BaseMessage
from langchain_core.outputs import ChatGeneration, ChatResult

from urara_chat.backend.models import SnapshotContext


class FakeChatModel(BaseChatModel):
    """Replays a scripted sequence of replies.

    The script is consumed one reply per call. When it runs out the last reply
    repeats, so a test for the tool-budget cap can be written as "always asks
    for a tool" without listing the reply once per iteration.
    """

    replies: list[AIMessage]
    calls: list[list[BaseMessage]] = []
    bound_tools: list[Any] = []

    def __init__(self, replies: Sequence[AIMessage], **kwargs: Any) -> None:
        super().__init__(replies=list(replies), calls=[], bound_tools=[], **kwargs)

    @property
    def _llm_type(self) -> str:
        return "fake"

    def _generate(self, *args: Any, **kwargs: Any) -> ChatResult:
        raise NotImplementedError("the pipeline is async; use ainvoke")

    async def _agenerate(
        self,
        messages: list[BaseMessage],
        stop: list[str] | None = None,
        run_manager: AsyncCallbackManagerForLLMRun | None = None,
        **kwargs: Any,
    ) -> ChatResult:
        self.calls.append(list(messages))
        index = min(len(self.calls) - 1, len(self.replies) - 1)

        # A fresh id per call. add_messages merges by id, so replaying the same
        # object twice would *replace* the earlier reply rather than appending
        # -- the loop would then read a ToolMessage as the model's last word and
        # finish a turn early. A fake that quietly changes the graph's control
        # flow is worse than no fake.
        reply = self.replies[index].model_copy(update={"id": f"fake-reply-{len(self.calls)}"})
        return ChatResult(generations=[ChatGeneration(message=reply)])

    def bind_tools(self, tools: Sequence[Any], **kwargs: Any) -> FakeChatModel:
        """Records what it was bound to, and stays itself.

        Returning the same object rather than a Runnable wrapper keeps `calls`
        reachable from the test after the graph has bound it.
        """
        self.bound_tools = list(tools)
        return self

    @property
    def call_count(self) -> int:
        return len(self.calls)


class CountingContextClient:
    """A backend client for the context card only, counting fetches."""

    def __init__(self, context: SnapshotContext) -> None:
        self.context = context
        self.calls = 0

    async def get_context(self, snapshot_id: str) -> SnapshotContext:
        self.calls += 1
        return self.context
