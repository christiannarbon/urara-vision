"""Answering one question, with nothing written down."""

from __future__ import annotations

import asyncio
import logging
import time
from dataclasses import asdict
from typing import Any

from fastapi import HTTPException

from urara_chat.agent.pipeline import AgentAnswer, answer
from urara_chat.api.errors import ProviderError, redact_reason
from urara_chat.api.middleware import current_request_id
from urara_chat.api.schemas import AnswerResponse
from urara_chat.backend.client import BackendClient
from urara_chat.backend.errors import BackendError
from urara_chat.backend.models import Message
from urara_chat.config import Settings

log = logging.getLogger(__name__)


def clean_question(question: str, max_chars: int) -> str:
    """The question as the model will read it, or a 400 saying why not."""
    cleaned = question.strip()
    if not cleaned:
        raise HTTPException(status_code=400, detail='"question" must not be empty')
    if len(cleaned) > max_chars:
        # The limit is in the message: a caller that hit it is usually pasting a document, and
        # needs to know how much to cut rather than that it was too much.
        raise HTTPException(
            status_code=400,
            detail=f'"question" is {len(cleaned)} characters, over the {max_chars} character limit',
        )
    return cleaned


async def run_pipeline(
    question: str,
    snapshot_id: str,
    history: list[Message],
    language: str,
    settings: Settings,
    conversation_id: str | None = None,
) -> AgentAnswer:
    """Run one turn, classifying what went wrong rather than mapping it."""
    started = time.perf_counter()
    try:
        return await asyncio.wait_for(
            answer(question, snapshot_id, history=history, language=language),
            timeout=settings.answer_timeout_seconds,
        )
    except Exception as exc:
        log_failed_turn(snapshot_id, conversation_id, exc, started)
        if isinstance(exc, BackendError):
            raise
        raise ProviderError(
            f"the turn failed for snapshot {snapshot_id}", reason=redact_reason(str(exc), question)
        ) from exc


async def answer_question(
    client: BackendClient,
    settings: Settings,
    snapshot_ref: str,
    question: str,
    language: str,
) -> AgentAnswer:
    """One question, one answer, nothing persisted."""
    cleaned = clean_question(question, settings.max_question_chars)
    # 404 for an unknown snapshot, through the shared handler.
    snapshot_id = await client.resolve_snapshot(snapshot_ref)
    result = await run_pipeline(cleaned, snapshot_id, [], language, settings)
    log_turn(turn_record(result, snapshot_id, conversation_id=None))
    return result


def turn_record(
    result: AgentAnswer, snapshot_id: str, conversation_id: str | None
) -> dict[str, Any]:
    """Logged per turn and stored as the assistant message's meta."""
    return {
        "outcome": "answered",
        "requestId": current_request_id(),
        "conversationId": conversation_id,
        "snapshotId": snapshot_id,
        "model": result.model,
        "promptTokens": result.prompt_tokens,
        "completionTokens": result.completion_tokens,
        "tokensEstimated": result.tokens_estimated,
        "toolCalls": len(result.tool_calls),
        "tools": [call["name"] for call in result.tool_calls],
        "iterations": result.iterations,
        "citations": len(result.citations),
        "latencyMs": result.latency_ms,
        "truncated": result.truncated,
    }


def log_turn(record: dict[str, Any]) -> None:
    log.info("turn answered", extra={"event": "turn", **record})


def to_response(result: AgentAnswer) -> AnswerResponse:
    return AnswerResponse(**asdict(result))


def log_failed_turn(
    snapshot_id: str, conversation_id: str | None, exc: Exception, started: float
) -> None:
    # The exception type only: provider messages can quote the prompt back.
    log.warning(
        "turn failed",
        extra={
            "event": "turn",
            "outcome": "failed",
            "requestId": current_request_id(),
            "conversationId": conversation_id,
            "snapshotId": snapshot_id,
            "error": type(exc).__name__,
            "latencyMs": round((time.perf_counter() - started) * 1000),
        },
    )
