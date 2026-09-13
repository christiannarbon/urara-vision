"""The conversation CRUD the frontend talks to."""

from __future__ import annotations

import asyncio
import logging
import math
from collections import Counter
from collections.abc import Sequence

from fastapi import APIRouter, HTTPException, Query, Request

from urara_chat.api.answering import (
    answer_question,
    clean_question,
    log_turn,
    run_pipeline,
    to_response,
    turn_record,
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
    StatsResponse,
    TurnRequest,
    TurnResponse,
)
from urara_chat.api.titles import title_from_question
from urara_chat.backend.client import BackendClient
from urara_chat.backend.errors import BackendError, BackendNotFound
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


# Fetches every conversation for the snapshot: fine at this scale, not at ten thousand. The fix
# then is a GET /api/v1/conversations/stats aggregate in the Go backend.
STATS_CONVERSATION_LIMIT = 200  # the backend's own list cap
STATS_FETCH_CONCURRENCY = 8
# Nearest-rank p95 over fewer samples than this is not a percentile.
MIN_P95_SAMPLES = 20


@router.get("/stats", response_model=StatsResponse)
async def stats(
    request: Request,
    snapshot: str = Query(min_length=1, description="Snapshot to report on. 'latest' is accepted."),
) -> StatsResponse:
    client = get_client(request)
    snapshot_id = await client.resolve_snapshot(snapshot)
    listed = await client.list_conversations(snapshot_id, STATS_CONVERSATION_LIMIT)

    gate = asyncio.Semaphore(STATS_FETCH_CONCURRENCY)

    async def fetch(cid: str) -> Conversation | None:
        async with gate:
            try:
                return await client.get_conversation(cid)
            except BackendNotFound:
                # Deleted after it was listed.
                return None

    fetched = await asyncio.gather(*(fetch(c.id) for c in listed))
    conversations = [c for c in fetched if c is not None]
    return aggregate_stats(
        snapshot_id, conversations, capped=len(listed) >= STATS_CONVERSATION_LIMIT
    )


def aggregate_stats(
    snapshot_id: str, conversations: Sequence[Conversation], *, capped: bool = False
) -> StatsResponse:
    turns = 0
    prompt: list[int] = []
    completion: list[int] = []
    latencies: list[int] = []
    estimated = truncated = recorded_estimated = recorded_truncated = 0
    by_model: Counter[str] = Counter()

    for conversation in conversations:
        for message in conversation.messages:
            if message.role != "assistant":
                continue
            turns += 1
            meta: dict[str, object] = message.meta if isinstance(message.meta, dict) else {}
            raw_usage = meta.get("usage")
            usage: dict[str, object] = raw_usage if isinstance(raw_usage, dict) else {}

            # Pre-08.5 messages carry only the provider's usage block.
            if (tokens := _count(meta.get("promptTokens"), usage.get("input_tokens"))) is not None:
                prompt.append(tokens)
            tokens = _count(meta.get("completionTokens"), usage.get("output_tokens"))
            if tokens is not None:
                completion.append(tokens)
            if (latency := _count(meta.get("latencyMs"))) is not None:
                latencies.append(latency)
            if isinstance(model := meta.get("model"), str) and model:
                by_model[model] += 1
            if isinstance(was_estimated := meta.get("tokensEstimated"), bool):
                recorded_estimated += 1
                estimated += was_estimated
            if isinstance(was_truncated := meta.get("truncated"), bool):
                recorded_truncated += 1
                truncated += was_truncated

    return StatsResponse(
        snapshot_id=snapshot_id,
        conversations=len(conversations),
        conversations_capped=capped,
        turns=turns,
        prompt_tokens=sum(prompt) if prompt else None,
        completion_tokens=sum(completion) if completion else None,
        estimated_token_turns=estimated if recorded_estimated else None,
        mean_latency_ms=round(sum(latencies) / len(latencies)) if latencies else None,
        p95_latency_ms=_p95(latencies),
        truncated_turns=truncated if recorded_truncated else None,
        by_model=dict(by_model),
    )


def _count(*candidates: object) -> int | None:
    """The first non-negative integer, ignoring bools and malformed values."""
    for value in candidates:
        if isinstance(value, int) and not isinstance(value, bool) and value >= 0:
            return value
    return None


def _p95(values: list[int]) -> int | None:
    if len(values) < MIN_P95_SAMPLES:
        return None
    ordered = sorted(values)
    return ordered[math.ceil(0.95 * len(ordered)) - 1]


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

    # 409, not 400: the request is fine, the conversation's length is what refuses it.
    turns = sum(1 for m in history if m.role == "assistant")
    if turns >= settings.max_conversation_turns:
        raise HTTPException(
            status_code=409,
            detail=(
                f"this conversation has reached its limit of {settings.max_conversation_turns} "
                "turns; start a new conversation to keep asking"
            ),
        )

    # The question is stored before the model is called, so a provider failure leaves it in the
    # transcript and the reader can retry without retyping.
    user_message = await client.append_message(cid, "user", question)

    result = await run_pipeline(
        question, conversation.snapshot_id, history, language, settings, conversation_id=cid
    )
    record = turn_record(result, conversation.snapshot_id, conversation_id=cid)
    log_turn(record)

    try:
        assistant_message = await client.append_message(
            cid,
            "assistant",
            result.text,
            citations=result.citations,
            # Stores the calls themselves; the log line carries only their count.
            meta={**record, "toolCalls": result.tool_calls, "usage": result.usage},
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
