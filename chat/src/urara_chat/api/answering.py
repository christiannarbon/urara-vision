"""Answering one question, with nothing written down.

Two routes reach this: `/api/chat/answer`, which Phase 08's eval runner drives
thousands of times, and `/debug/answer`, which is how a bad answer is explained.
They share an implementation on purpose. Two copies would drift, and the one
that drifted would be the debugging path -- so the tool for explaining a wrong
answer would stop describing the route that produced it.

**Nothing here writes.** No conversation, no message, no title. An eval run that
left a thousand transcripts behind is an eval run nobody does twice, and a
debugging call that changes what it is debugging is worse than useless.
"""

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
    """The question as the model will read it, or a 400 saying why not.

    Trimmed before it is measured, so a body of spaces is empty rather than
    short, and so the limit counts characters the model will actually be sent.
    """
    cleaned = question.strip()
    if not cleaned:
        raise HTTPException(status_code=400, detail='"question" must not be empty')
    if len(cleaned) > max_chars:
        # The limit is in the message: a caller that hit it is usually pasting a
        # document, and needs to know how much to cut rather than that it was
        # too much.
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
    """Run one turn, classifying what went wrong rather than mapping it.

    The wrap is what lets `api.errors` keep one handler per kind of failure: a
    provider raises its own SDK's exception types, several per provider, and
    listing them there would mean importing every SDK and keeping the list
    current. A backend failure is re-raised untouched -- it reached here from a
    tool or a context card, and telling a reader the model did not answer when
    the store was down sends them to the wrong service.

    The deadline bounds the whole turn rather than one call. Each provider call
    is bounded already, but a turn is up to seven of them plus their tools, and
    the route holds the connection for all of it -- and the eval runner drives
    this thousands of times, where one hung turn hangs the run.
    """
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
    """One question, one answer, nothing persisted.

    The snapshot reference is resolved here rather than inside the pipeline,
    which refuses the alias outright: reading the wrong snapshot produces a
    confidently wrong answer, so what reaches it is always concrete.

    History is empty by definition. This path has no conversation to read one
    from, which is exactly what makes it cheap enough to run in bulk.
    """
    cleaned = clean_question(question, settings.max_question_chars)
    # 404 for an unknown snapshot, through the shared handler.
    snapshot_id = await client.resolve_snapshot(snapshot_ref)
    return await run_pipeline(cleaned, snapshot_id, [], language, settings)


def to_response(result: AgentAnswer) -> AnswerResponse:
    """One turn's answer on the wire.

    Here rather than in each route: 05.7 moved the answering itself into this
    module so the two paths could not drift, and left the last step copied into
    both. A field added to AgentAnswer that needs handling on the way out should
    have one site to find.

    asdict() gives the dataclass's own field names; the schema carries the
    camelCase wire names and populate_by_name lets it be built from either.
    """
    return AnswerResponse(**asdict(result))
