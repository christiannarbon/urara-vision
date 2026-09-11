"""The conversation CRUD the frontend talks to.

Thin wrappers over the Go backend's own conversation endpoints, and thin on
purpose: the backend owns the conversation schema, and a second copy of its
validation here would drift from it without anyone noticing until the two
disagreed about a stored thread.

Everything sits under `/api/chat`, so Phase 06's nginx routes it with one
`location` block and no path rewriting.

**`latest` is not resolved here.** The backend resolves it when a conversation
is created and stores the concrete ID it resolved to, which is what stops a
transcript changing subject under a later ingest. Resolving it here as well
would put a second opinion in the system about which snapshot a thread is
pinned to, and the two would eventually differ by one ingest.

No route has a try/except: every failure below is raised by the backend client
and mapped by the handlers installed in `api.errors`.
"""

from __future__ import annotations

import asyncio

from fastapi import APIRouter, HTTPException, Query, Request

from urara_chat.agent.pipeline import AgentAnswer, answer
from urara_chat.api.errors import ProviderError
from urara_chat.api.routes import get_client, get_settings_for
from urara_chat.api.schemas import (
    ConversationListResponse,
    ConversationResponse,
    CreateConversationRequest,
    MessageResponse,
    TurnRequest,
    TurnResponse,
)
from urara_chat.backend.errors import BackendError
from urara_chat.backend.models import Conversation, Message
from urara_chat.config import Settings

router = APIRouter(prefix="/api/chat", tags=["chat"])


@router.post("/conversations", status_code=201, response_model=ConversationResponse)
async def create_conversation(request: Request, body: CreateConversationRequest) -> Conversation:
    """Start a thread about one snapshot.

    The snapshot reference goes through untouched, alias and all.
    """
    return await get_client(request).create_conversation(body.snapshot_id, body.title)


@router.get("/conversations", response_model=ConversationListResponse)
async def list_conversations(
    request: Request,
    snapshot: str = Query(
        min_length=1,
        description="Snapshot to list threads for. 'latest' is accepted.",
    ),
) -> dict[str, list[Conversation]]:
    """Threads about one snapshot, newest first and without their transcripts.

    `min_length=1` is requiredness, not a copy of the backend's validation: an
    empty `?snapshot=` would otherwise be forwarded, refused there, and reach
    the caller as a 502 blaming the backend for their own missing parameter.
    """
    return {"conversations": await get_client(request).list_conversations(snapshot)}


@router.get("/conversations/{cid}", response_model=ConversationResponse)
async def get_conversation(request: Request, cid: str) -> Conversation:
    """One thread with its full transcript."""
    return await get_client(request).get_conversation(cid)


@router.delete("/conversations/{cid}", status_code=204)
async def delete_conversation(request: Request, cid: str) -> None:
    """Remove a thread and its messages."""
    await get_client(request).delete_conversation(cid)


@router.post("/conversations/{cid}/turn", response_model=TurnResponse)
async def take_turn(request: Request, cid: str, body: TurnRequest) -> TurnResponse:
    """Ask one question of an existing conversation, and store both messages.

    The snapshot comes from the conversation and from nowhere else. It was
    pinned when the thread was created, and a turn that could choose its own
    would let a re-ingest change which model is being discussed halfway through
    a transcript -- with every earlier answer left citing tables from a
    different one.
    """
    client = get_client(request)
    settings = get_settings_for(request)

    # Checked before anything is fetched or stored: these cost no round trip,
    # and a rejected question should not leave a conversation looking touched.
    #
    # Trimmed before it is measured, so a body of spaces is empty rather than
    # short, and so the limit counts characters the model will actually read.
    question = body.question.strip()
    if not question:
        raise HTTPException(status_code=400, detail='"question" must not be empty')
    if len(question) > settings.max_question_chars:
        # The limit is in the message: a caller that hit it is usually pasting a
        # document, and needs to know how much to cut rather than that it was
        # too much.
        raise HTTPException(
            status_code=400,
            detail=(
                f'"question" is {len(question)} characters, over the '
                f"{settings.max_question_chars} character limit"
            ),
        )

    # 404 for an unknown conversation, through the shared handler.
    conversation = await client.get_conversation(cid)
    # Read before the new message is written, so the history handed to the
    # pipeline cannot contain the question it is being asked.
    history = list(conversation.messages)

    # The question is stored before the model is called, so a provider failure
    # leaves it in the transcript and the reader can retry without retyping.
    # The tidier version -- store both at the end -- loses the question on every
    # failure, which is exactly when losing it hurts most.
    user_message = await client.append_message(cid, "user", question)

    result = await _answer(question, conversation.snapshot_id, history, body.language, settings)

    assistant_message = await client.append_message(
        cid,
        "assistant",
        result.text,
        citations=result.citations,
        # What the turn cost and what it did. Phase 08 bills from these, and a
        # stored turn that cannot say what it spent cannot be costed later.
        meta={
            "model": result.model,
            "usage": result.usage,
            "toolCalls": result.tool_calls,
            "iterations": result.iterations,
            "latencyMs": result.latency_ms,
            "truncated": result.truncated,
        },
    )

    return TurnResponse(
        conversation_id=cid,
        user_message=_message(user_message),
        assistant_message=_message(assistant_message),
        tool_calls=result.tool_calls,
        truncated=result.truncated,
        latency_ms=result.latency_ms,
        model=result.model,
    )


async def _answer(
    question: str,
    snapshot_id: str,
    history: list[Message],
    language: str,
    settings: Settings,
) -> AgentAnswer:
    """Run the turn, classifying what went wrong rather than mapping it.

    The wrap is what lets `api.errors` keep one handler per kind of failure: a
    provider raises its own SDK's exception types, several per provider, and
    listing them there would mean importing every SDK and keeping the list
    current. A backend failure is re-raised untouched -- it reached here from a
    tool, and telling a reader the model did not answer when the store was down
    sends them to the wrong service.

    The deadline is the one `/debug/answer` already carries, for the same
    reason: a turn is up to seven model calls plus their tools, and this route
    holds the connection for all of them.
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


def _message(stored: Message) -> MessageResponse:
    """A stored turn as this service returns it."""
    return MessageResponse.model_validate(stored, from_attributes=True)
