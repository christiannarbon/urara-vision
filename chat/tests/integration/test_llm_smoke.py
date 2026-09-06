"""The first tests that spend money.

Marked `llm` so they never run by accident: `make test-chat`, CI and a plain
`uv run pytest` all leave them alone, and they skip with a message when no key
is present.

**The assertions are on the contract, never on prose.** A model's wording drifts
between versions, and a test that pins it fails for no reason, gets deleted, and
takes its real assertion with it. What is worth asserting is that a reply came
back at all, and that asking about domains produces a `list_domains` tool call --
the tool call is the interface between the model and the retrieval layer.

**Vertex has no automated test here.** It needs a real GCP project and
Application Default Credentials, which cannot be depended on in CI or on a
contributor's machine, and a test that skips everywhere is worse than none.
The Vertex path is verified by hand instead, following the steps in
`chat/README.md` -- which is how it was checked in 03.6.
"""

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
    """A backend that answers instantly.

    These tests are about the model and the binding, not about retrieval, and a
    real backend would only add a dependency and a delay. `get_tables` raises
    not-found because that is what the recovery test needs.
    """

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
    """One turn, one short reply. Containment rather than equality: models add
    punctuation, and pinning the exact string is how a test earns deletion."""
    model = build_chat_model(llm_settings)

    reply = await model.ainvoke("Reply with exactly: pong")

    text = reply.content if isinstance(reply.content, str) else str(reply.content)
    assert text.strip(), "the model returned an empty reply"
    assert "pong" in text.lower()


async def test_binds_tools(llm_settings: Settings) -> None:
    """Asking about domains must produce a list_domains call.

    The tool call is the contract between the model and the retrieval layer. The
    prose around it is judgement and will drift, so it is not asserted on.
    """
    model = build_chat_model(llm_settings).bind_tools(bound_tools())

    reply = await model.ainvoke("List the domains in this data model.")

    assert reply.tool_calls, "the model answered without calling a tool"
    assert reply.tool_calls[0]["name"] == "list_domains"


async def test_recovers_from_bad_id() -> None:
    """A wrong guess comes back as guidance, not an exception.

    No model is involved -- this is the wrapper from 03.4 -- but it belongs
    beside the others because it is the behaviour they depend on: a model that
    guesses an ID wrong must be able to carry on.
    """
    get_tables = next(t for t in bound_tools() if t.name == "get_tables")

    answer = await get_tables.ainvoke({"ids": ["nonsense/not_a_table"]})

    assert isinstance(answer, str), "a failed lookup must not raise out of the tool"
    assert "nonsense/not_a_table" in answer
    assert "search_model" in answer


async def test_unknown_tool_not_invented(llm_settings: Settings) -> None:
    """A question no tool covers must not produce an invented tool.

    The refusal's wording is not asserted -- only that nothing was called that
    does not exist, and that nothing raised.
    """
    model = build_chat_model(llm_settings).bind_tools(bound_tools())

    reply = await model.ainvoke("What is the weather?")

    for call in reply.tool_calls:
        assert call["name"] in TOOL_NAMES, f"invented a tool: {call['name']}"
