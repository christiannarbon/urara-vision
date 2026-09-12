"""Scripted stand-ins, so the unit suite never needs a key or a network."""

from __future__ import annotations

from collections.abc import Sequence
from typing import Any

from langchain_core.callbacks import AsyncCallbackManagerForLLMRun
from langchain_core.language_models import BaseChatModel
from langchain_core.messages import AIMessage, BaseMessage
from langchain_core.outputs import ChatGeneration, ChatResult

from urara_chat.backend.models import SnapshotContext


class FakeChatModel(BaseChatModel):
    """Replays a scripted sequence of replies."""

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

        # A fresh id per call. add_messages merges by id, so replaying one would replace
        # the earlier reply and let the loop finish a turn early.
        reply = self.replies[index].model_copy(update={"id": f"fake-reply-{len(self.calls)}"})
        return ChatResult(generations=[ChatGeneration(message=reply)])

    def bind_tools(self, tools: Sequence[Any], **kwargs: Any) -> FakeChatModel:
        """Records what it was bound to, and stays itself."""
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
