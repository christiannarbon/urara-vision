"""Answering one question, with nothing written down."""

from __future__ import annotations

import asyncio
from dataclasses import asdict

from fastapi import HTTPException

from urara_chat.agent.pipeline import AgentAnswer, answer
from urara_chat.api.errors import ProviderError
from urara_chat.api.schemas import AnswerResponse
from urara_chat.backend.client import BackendClient
from urara_chat.backend.errors import BackendError
from urara_chat.backend.models import Message
from urara_chat.config import Settings


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
) -> AgentAnswer:
    """Run one turn, classifying what went wrong rather than mapping it."""
    try:
        return await asyncio.wait_for(
            answer(question, snapshot_id, history=history, language=language),
            timeout=settings.answer_timeout_seconds,
        )
    except BackendError:
        raise
    except Exception as exc:
        raise ProviderError(f"the turn failed for snapshot {snapshot_id}") from exc


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
    return await run_pipeline(cleaned, snapshot_id, [], language, settings)


def to_response(result: AgentAnswer) -> AnswerResponse:
    return AnswerResponse(**asdict(result))
