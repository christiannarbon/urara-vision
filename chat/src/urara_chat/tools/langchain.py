"""The only module in the service that knows LangChain exists."""

from __future__ import annotations

import logging
import time
from collections.abc import Awaitable, Callable, Sequence
from typing import Any

from langchain_core.tools import BaseTool, StructuredTool

from urara_chat.backend.errors import BackendError, BackendNotFound
from urara_chat.tools.registry import ToolSpec

log = logging.getLogger(__name__)

# The arguments that carry a table or source ID, in the order a message should mention them.
_ID_ARGUMENTS = ("table_id", "ids", "from_table", "to_table")


def _offending_ids(kwargs: dict[str, Any]) -> list[str]:
    """The IDs a failing call was given, for the not-found message."""
    found: list[str] = []
    for key in _ID_ARGUMENTS:
        value = kwargs.get(key)
        if not value:
            continue
        if isinstance(value, list):
            found.extend(str(v) for v in value)
        else:
            found.append(str(value))
    return found


def _not_found_message(kwargs: dict[str, Any]) -> str:
    ids = _offending_ids(kwargs)
    named = ", ".join(f"'{i}'" for i in ids)
    subject = f"No table with id {named}" if ids else "That was not found"
    return f"{subject} in this model. Call search_model to find the right ID."


def _guard(spec: ToolSpec) -> Callable[..., Awaitable[Any]]:
    """Wrap a tool so a recoverable failure becomes advice rather than a crash."""

    async def run(**kwargs: Any) -> Any:
        started = time.perf_counter()
        failed_with: str | None = None
        try:
            return await spec.fn(**kwargs)
        except BackendNotFound:
            failed_with = _not_found_message(kwargs)
            return failed_with
        except BackendError as exc:
            # Covers BackendUnavailable too, which subclasses it: an unreachable backend is as
            # recoverable from the model's point of view as a rejected query -- neither is fixed
            # by ending the turn.
            failed_with = f"The model store could not answer that: {exc.message}."
            return failed_with
        except TimeoutError:
            # asyncio.TimeoutError is an alias of the builtin from 3.11, so this catches both.
            failed_with = "That lookup timed out. Try a narrower query."
            return failed_with
        finally:
            # Phase 08 explains a bad answer from these lines, so they carry what was asked as
            # well as what came back.
            log.debug(
                "tool call",
                extra={
                    "tool": spec.name,
                    # "args" is reserved on LogRecord and passing it raises, so every tool call
                    # would fail the moment LOG_LEVEL=debug.
                    "tool_args": kwargs,
                    "duration_ms": round((time.perf_counter() - started) * 1000, 2),
                    "returned_error": failed_with is not None,
                    "error": failed_with,
                },
            )

    return run


def to_langchain_tools(specs: Sequence[ToolSpec]) -> list[BaseTool]:
    return [
        StructuredTool.from_function(
            # coroutine= rather than func=: these are async, and a tool built this way raises
            # NotImplementedError if anything calls it synchronously, which is the failure mode
            # worth having.
            coroutine=_guard(spec),
            name=spec.name,
            description=spec.description,
            args_schema=spec.args_schema,
        )
        for spec in specs
    ]
