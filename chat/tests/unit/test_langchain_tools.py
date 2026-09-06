"""Wrapping the retrieval tools for LangChain.

The conversion itself is thin. What matters is the error mapping: a model that
guessed an ID wrong should be told which one and given the tool that fixes it,
because a wrong guess mid-reasoning is normal operation. An unexpected error
must still propagate -- turning a bug into a sentence the model apologises about
means the answer still arrives, just quietly wrong.
"""

import asyncio
import json
import logging
from typing import Any

import pytest
from langchain_core.tools import BaseTool, StructuredTool
from pydantic import BaseModel

from urara_chat.backend.errors import BackendError, BackendNotFound, BackendUnavailable
from urara_chat.tools.langchain import to_langchain_tools
from urara_chat.tools.registry import TOOL_NAMES, ToolSpec, build_tools


class FakeClient:
    """Enough of the client for build_tools; every method is unused unless a
    test drives it."""

    async def list_domains(self, sid: str) -> list[Any]:
        return []

    async def list_tables(self, sid: str, domain: str | None = None) -> list[Any]:
        return []

    async def get_tables(self, sid: str, ids: Any) -> Any:
        from urara_chat.backend.models import TablesDetailResponse

        return TablesDetailResponse()

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


def real_tools() -> list[BaseTool]:
    return to_langchain_tools(build_tools(FakeClient(), "snap-1"))  # type: ignore[arg-type]


class OneArg(BaseModel):
    table_id: str


def one_tool(raises: Exception | None = None, result: Any = "ok") -> BaseTool:
    """A single wrapped tool whose callable does exactly what a test needs."""

    async def fn(table_id: str) -> Any:
        if raises is not None:
            raise raises
        return result

    spec = ToolSpec(name="probe", description="a probe", args_schema=OneArg, fn=fn)
    return to_langchain_tools([spec])[0]


class TestTheConversion:
    def test_nine_tools_convert(self) -> None:
        tools = real_tools()
        assert len(tools) == 9
        assert tuple(t.name for t in tools) == TOOL_NAMES
        assert all(isinstance(t, StructuredTool) for t in tools)

    def test_descriptions_survive_unchanged(self) -> None:
        """The description is the only thing the model reads before choosing, so
        a wrapper that reworded it would change which tool gets called."""
        specs = build_tools(FakeClient(), "snap-1")  # type: ignore[arg-type]
        wrapped = {t.name: t for t in to_langchain_tools(specs)}

        for spec in specs:
            assert wrapped[spec.name].description == spec.description

    def test_the_args_schema_is_the_same_object(self) -> None:
        specs = build_tools(FakeClient(), "snap-1")  # type: ignore[arg-type]
        wrapped = {t.name: t for t in to_langchain_tools(specs)}

        for spec in specs:
            assert wrapped[spec.name].args_schema is spec.args_schema

    def test_no_wrapped_tool_offers_a_snapshot(self) -> None:
        """The binding must not reintroduce what the registry withheld."""
        for tool in real_tools():
            assert "snapshot" not in json.dumps(tool.args).lower()

    async def test_a_successful_call_returns_the_real_result(self) -> None:
        tool = one_tool(result={"items": [1], "truncated": False, "total": 1})
        assert await tool.ainvoke({"table_id": "d/t"}) == {
            "items": [1],
            "truncated": False,
            "total": 1,
        }

    async def test_the_callable_is_actually_awaited(self) -> None:
        awaited = False

        async def fn(table_id: str) -> str:
            nonlocal awaited
            await asyncio.sleep(0)
            awaited = True
            return "done"

        tool = to_langchain_tools(
            [ToolSpec(name="probe", description="d", args_schema=OneArg, fn=fn)]
        )[0]

        assert await tool.ainvoke({"table_id": "d/t"}) == "done"
        assert awaited, "the coroutine was never awaited"

    def test_a_wrapped_tool_cannot_be_called_synchronously(self) -> None:
        """Built with coroutine= and no func=, so a sync caller fails loudly
        rather than silently doing nothing."""
        with pytest.raises(NotImplementedError):
            one_tool().invoke({"table_id": "d/t"})


class TestRecoverableFailuresBecomeAdvice:
    """Returned, not raised. A wrong guess mid-reasoning is normal operation;
    a 500 to the reader because of one is not."""

    async def test_not_found_names_the_id_and_the_tool_that_fixes_it(self) -> None:
        tool = one_tool(raises=BackendNotFound(404, "not found"))

        message = await tool.ainvoke({"table_id": "ordering/nope"})

        assert isinstance(message, str)
        assert "ordering/nope" in message, "the model must know which argument was wrong"
        assert "search_model" in message

    async def test_not_found_names_every_id_from_a_batch(self) -> None:
        class Ids(BaseModel):
            ids: list[str]

        async def fn(ids: list[str]) -> Any:
            raise BackendNotFound(404, "not found")

        tool = to_langchain_tools(
            [ToolSpec(name="probe", description="d", args_schema=Ids, fn=fn)]
        )[0]

        message = await tool.ainvoke({"ids": ["a/one", "b/two"]})

        assert "a/one" in message and "b/two" in message

    async def test_a_backend_error_carries_the_reason(self) -> None:
        tool = one_tool(raises=BackendError(500, "the index is rebuilding"))

        message = await tool.ainvoke({"table_id": "d/t"})

        assert "could not answer" in message
        assert "the index is rebuilding" in message

    async def test_an_unreachable_backend_is_recoverable_too(self) -> None:
        """BackendUnavailable subclasses BackendError, and neither is fixed by
        ending the turn."""
        tool = one_tool(raises=BackendUnavailable(502, "backend unreachable: refused"))

        message = await tool.ainvoke({"table_id": "d/t"})

        assert "could not answer" in message
        assert "unreachable" in message

    async def test_a_timeout_suggests_narrowing(self) -> None:
        tool = one_tool(raises=TimeoutError("too slow"))

        message = await tool.ainvoke({"table_id": "d/t"})

        assert "timed out" in message
        assert "narrower" in message

    async def test_asyncio_timeout_is_the_same_exception(self) -> None:
        """From 3.11 asyncio.TimeoutError is an alias of the builtin, so one
        except clause covers both."""
        assert asyncio.TimeoutError is TimeoutError

        tool = one_tool(raises=TimeoutError())
        assert "timed out" in await tool.ainvoke({"table_id": "d/t"})


class TestUnexpectedErrorsPropagate:
    """An unexpected error is a bug. Turning it into a sentence the model
    apologises about means the answer still arrives, just quietly wrong."""

    async def test_a_value_error_is_not_swallowed(self) -> None:
        tool = one_tool(raises=ValueError("a real bug"))

        with pytest.raises(ValueError, match="a real bug"):
            await tool.ainvoke({"table_id": "d/t"})

    async def test_a_key_error_is_not_swallowed(self) -> None:
        tool = one_tool(raises=KeyError("envelope"))

        with pytest.raises(KeyError):
            await tool.ainvoke({"table_id": "d/t"})


class TestLogging:
    async def test_an_invocation_is_logged_at_debug(self, caplog: pytest.LogCaptureFixture) -> None:
        """Phase 08 explains a bad answer from these lines."""
        with caplog.at_level(logging.DEBUG, logger="urara_chat.tools.langchain"):
            await one_tool().ainvoke({"table_id": "d/t"})

        record = next(r for r in caplog.records if r.message == "tool call")
        assert record.tool == "probe"  # type: ignore[attr-defined]
        assert record.tool_args == {"table_id": "d/t"}  # type: ignore[attr-defined]
        assert record.duration_ms >= 0  # type: ignore[attr-defined]
        assert record.returned_error is False  # type: ignore[attr-defined]

    async def test_a_returned_error_is_marked_in_the_log(
        self, caplog: pytest.LogCaptureFixture
    ) -> None:
        """A tool that answered with advice looks like a success from the
        outside, so the log is the only place the difference survives."""
        tool = one_tool(raises=BackendNotFound(404, "nope"))

        with caplog.at_level(logging.DEBUG, logger="urara_chat.tools.langchain"):
            await tool.ainvoke({"table_id": "d/t"})

        record = next(r for r in caplog.records if r.message == "tool call")
        assert record.returned_error is True  # type: ignore[attr-defined]
        assert "search_model" in record.error  # type: ignore[attr-defined]
