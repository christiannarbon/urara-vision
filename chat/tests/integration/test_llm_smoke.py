"""The first tests that spend money."""

from typing import Any

import pytest

from urara_chat.backend.errors import BackendNotFound
from urara_chat.backend.models import TablesDetailResponse
from urara_chat.config import Settings
from urara_chat.llm.factory import build_chat_model
from urara_chat.tools.langchain import to_langchain_tools
from urara_chat.tools.registry import TOOL_NAMES, build_tools

pytestmark = pytest.mark.llm


class StubClient:
    """A backend that answers instantly."""

    async def list_domains(self, sid: str) -> list[Any]:
        return []

    async def list_tables(self, sid: str, domain: str | None = None) -> list[Any]:
        return []

    async def get_tables(self, sid: str, ids: Any) -> TablesDetailResponse:
        raise BackendNotFound(404, "not found")

    async def search(self, sid: str, query: str, limit: int = 20) -> list[Any]:
        return []

    async def neighbourhood(
        self, sid: str, table_id: str, depth: int = 1, sources: bool = False
    ) -> Any:
        from urara_chat.backend.models import Graph

        return Graph()

    async def join_paths(
        self, sid: str, frm: str, to: str, max_depth: int = 4, limit: int = 10
    ) -> list[Any]:
        return []

    async def lineage(self, sid: str, table_id: str, direction: str = "upstream") -> list[Any]:
        return []

    async def diagnostics(self, sid: str, severity: str | None = None) -> list[Any]:
        return []

    async def list_sources(self, sid: str) -> list[Any]:
        return []


def bound_tools() -> list[Any]:
    return to_langchain_tools(build_tools(StubClient(), "smoke-snapshot"))  # type: ignore[arg-type]


async def test_studio_answers(llm_settings: Settings) -> None:
    """One turn, one short reply."""
    model = build_chat_model(llm_settings)

    reply = await model.ainvoke("Reply with exactly: pong")

    text = reply.content if isinstance(reply.content, str) else str(reply.content)
    assert text.strip(), "the model returned an empty reply"
    assert "pong" in text.lower()


async def test_binds_tools(llm_settings: Settings) -> None:
    """Asking about domains must produce a list_domains call."""
    model = build_chat_model(llm_settings).bind_tools(bound_tools())

    reply = await model.ainvoke("List the domains in this data model.")

    assert reply.tool_calls, "the model answered without calling a tool"
    assert reply.tool_calls[0]["name"] == "list_domains"


async def test_recovers_from_bad_id() -> None:
    """A wrong guess comes back as guidance, not an exception."""
    get_tables = next(t for t in bound_tools() if t.name == "get_tables")

    answer = await get_tables.ainvoke({"ids": ["nonsense/not_a_table"]})

    assert isinstance(answer, str), "a failed lookup must not raise out of the tool"
    assert "nonsense/not_a_table" in answer
    assert "search_model" in answer


async def test_unknown_tool_not_invented(llm_settings: Settings) -> None:
    """A question no tool covers must not produce an invented tool."""
    model = build_chat_model(llm_settings).bind_tools(bound_tools())

    reply = await model.ainvoke("What is the weather?")

    for call in reply.tool_calls:
        assert call["name"] in TOOL_NAMES, f"invented a tool: {call['name']}"
