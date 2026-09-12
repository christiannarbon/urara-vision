"""The conversation CRUD the frontend talks to."""

from __future__ import annotations

import logging

from fastapi import APIRouter, Query, Request

from urara_chat.api.answering import (
    answer_question,
    clean_question,
    run_pipeline,
    to_response,
)
from urara_chat.api.locks import ConversationLocks, TurnLimiter
from urara_chat.api.middleware import current_request_id
from urara_chat.api.routes import get_client, get_settings_for
from urara_chat.api.schemas import (
    AnswerRequest,
    AnswerResponse,
    ConversationListResponse,
    ConversationResponse,
    CreateConversationRequest,
    MessageResponse,
    TurnRequest,
    TurnResponse,
)
from urara_chat.api.titles import title_from_question
from urara_chat.backend.client import BackendClient
from urara_chat.backend.errors import BackendError
from urara_chat.backend.models import Conversation, Message
from urara_chat.config import Settings

router = APIRouter(prefix="/api/chat", tags=["chat"])
log = logging.getLogger(__name__)


def get_locks(request: Request) -> ConversationLocks:
    """The process's conversation locks, made on first use."""
    state = request.app.state
    if not hasattr(state, "conversation_locks"):
        state.conversation_locks = ConversationLocks()
    locks: ConversationLocks = state.conversation_locks
    return locks


def get_limiter(request: Request) -> TurnLimiter:
    """The process's turn limiter, made on first use from the configured cap."""
    state = request.app.state
    if not hasattr(state, "turn_limiter"):
        settings = get_settings_for(request)
        state.turn_limiter = TurnLimiter(
            settings.max_concurrent_turns, settings.turn_admission_wait_seconds
        )
    limiter: TurnLimiter = state.turn_limiter
    return limiter


@router.post("/conversations", status_code=201, response_model=ConversationResponse)
async def create_conversation(request: Request, body: CreateConversationRequest) -> Conversation:
    return await get_client(request).create_conversation(body.snapshot_id, body.title)


@router.get("/conversations", response_model=ConversationListResponse)
async def list_conversations(
    request: Request,
    snapshot: str = Query(
        min_length=1,
        description="Snapshot to list threads for. 'latest' is accepted.",
    ),
    limit: int | None = Query(
        default=None,
        ge=1,
        description="How many threads to return. The backend defaults and caps it.",
    ),
) -> dict[str, list[Conversation]]:
    return {"conversations": await get_client(request).list_conversations(snapshot, limit)}


@router.get("/conversations/{cid}", response_model=ConversationResponse)
async def get_conversation(request: Request, cid: str) -> Conversation:
    return await get_client(request).get_conversation(cid)


@router.delete("/conversations/{cid}", status_code=204)
async def delete_conversation(request: Request, cid: str) -> None:
    await get_client(request).delete_conversation(cid)


@router.post("/answer", response_model=AnswerResponse)
async def answer_once(request: Request, body: AnswerRequest) -> AnswerResponse:
    """One question, one answer."""
    async with get_limiter(request).hold():
        result = await answer_question(
            get_client(request),
            get_settings_for(request),
            body.snapshot_id,
            body.question,
            body.language,
        )
    return to_response(result)


@router.post("/conversations/{cid}/turn", response_model=TurnResponse)
async def take_turn(request: Request, cid: str, body: TurnRequest) -> TurnResponse:
    """Ask one question of an existing conversation, and store both messages."""
    client = get_client(request)
    settings = get_settings_for(request)

    # Checked before anything is held, fetched or stored: it costs no round trip, and a rejected
    # question should not occupy a slot, block a thread or leave a conversation looking touched.
    question = clean_question(body.question, settings.max_question_chars)

    # A slot first, then the conversation. The reverse lets a turn hold the thread
    # while it queues, blocking a second turn behind one that is not even running.
    async with get_limiter(request).hold(), get_locks(request).hold(cid):
        return await _run_turn(client, settings, cid, question, body.language)


async def _run_turn(
    client: BackendClient,
    settings: Settings,
    cid: str,
    question: str,
    language: str,
) -> TurnResponse:
    """One turn, with the conversation already held."""
    # 404 for an unknown conversation, through the shared handler.
    conversation = await client.get_conversation(cid)
    # Read before the new message is written, so the history handed to the pipeline cannot contain
    # the question it is being asked.
    history = list(conversation.messages)

    # The question is stored before the model is called, so a provider failure leaves it in the
    # transcript and the reader can retry without retyping.
    user_message = await client.append_message(cid, "user", question)

    result = await run_pipeline(question, conversation.snapshot_id, history, language, settings)

    try:
        assistant_message = await client.append_message(
            cid,
            "assistant",
            result.text,
            citations=result.citations,
            # What the turn cost and what it did. Phase 08 bills from these, and a stored turn
            # that cannot say what it spent cannot be costed later.
            meta={
                "model": result.model,
                "usage": result.usage,
                "toolCalls": result.tool_calls,
                "iterations": result.iterations,
                "latencyMs": result.latency_ms,
                "truncated": result.truncated,
            },
        )
    except BackendError:
        # The turn still fails -- telling the caller it worked would leave the next fetch of this
        # conversation disagreeing with what they were told.
        log.error(
            "the answer could not be stored",
            extra={
                "request_id": current_request_id(),
                "conversation_id": cid,
                "answer": result.text,
                "usage": result.usage,
            },
        )
        raise

    # After the assistant message is stored, so a title never exists for a turn that produced
    # nothing.
    await _set_title_once(client, conversation, question)

    return TurnResponse(
        conversation_id=cid,
        user_message=_message(user_message),
        assistant_message=_message(assistant_message),
        tool_calls=result.tool_calls,
        truncated=result.truncated,
        latency_ms=result.latency_ms,
        model=result.model,
    )


def _message(stored: Message) -> MessageResponse:
    return MessageResponse.model_validate(stored, from_attributes=True)


async def _set_title_once(client: BackendClient, conversation: Conversation, question: str) -> None:
    """Title a thread from its first question, if it has none."""
    if conversation.title.strip():
        return

    title = title_from_question(question)
    if not title:
        # Nothing to title it with -- a question of nothing but quotes derives an empty string.
        return

    try:
        await client.set_conversation_title(conversation.id, title)
    except Exception as exc:
        log.warning(
            "could not set the conversation title",
            extra={"request_id": current_request_id(), "conversation_id": conversation.id},
            exc_info=exc,
        )
