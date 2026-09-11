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

from fastapi import APIRouter, Query, Request

from urara_chat.api.routes import get_client
from urara_chat.api.schemas import (
    ConversationListResponse,
    ConversationResponse,
    CreateConversationRequest,
)
from urara_chat.backend.models import Conversation

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
