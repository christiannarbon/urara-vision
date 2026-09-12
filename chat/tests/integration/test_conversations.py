"""Conversations end to end: the chat service, the Go API and Postgres."""

import os
import subprocess
import time
from collections.abc import Callable
from pathlib import Path
from typing import Any

import httpx
import pytest

from urara_chat.backend.client import BackendClient
from urara_chat.config import Settings

FACT_ORDERS = "ordering/fact_orders"

# How long the service is given to come back after a restart.
RESTART_TIMEOUT_SECONDS = 60.0


async def new_conversation(chat: httpx.AsyncClient, snapshot: str) -> str:
    response = await chat.post("/api/chat/conversations", json={"snapshotId": snapshot})
    response.raise_for_status()
    cid: str = response.json()["id"]
    return cid


async def take_turn(chat: httpx.AsyncClient, cid: str, question: str) -> dict[str, Any]:
    response = await chat.post(f"/api/chat/conversations/{cid}/turn", json={"question": question})
    response.raise_for_status()
    body: dict[str, Any] = response.json()
    return body


async def transcript(chat: httpx.AsyncClient, cid: str) -> list[dict[str, Any]]:
    response = await chat.get(f"/api/chat/conversations/{cid}")
    response.raise_for_status()
    messages: list[dict[str, Any]] = response.json()["messages"]
    return messages


def wait_for_health(url: str, timeout: float = RESTART_TIMEOUT_SECONDS) -> float:
    """Poll /healthz until it answers."""
    deadline = time.monotonic() + timeout
    started = time.monotonic()
    while time.monotonic() < deadline:
        try:
            if httpx.get(f"{url}/healthz", timeout=2.0).status_code == 200:
                return time.monotonic() - started
        except httpx.HTTPError:
            pass
        time.sleep(0.5)
    raise AssertionError(f"the chat service did not come back within {timeout}s")


@pytest.mark.integration
async def test_conversation_crud(chat: httpx.AsyncClient, snapshot_id: str) -> None:
    """Create, list, get, delete -- no model involved."""
    created = await chat.post("/api/chat/conversations", json={"snapshotId": "latest"})
    assert created.status_code == 201
    conversation = created.json()
    cid = conversation["id"]

    try:
        assert conversation["snapshotId"] != "latest"
        assert len(conversation["snapshotId"]) == 36, "a snapshot ID is a UUID"

        listed = await chat.get("/api/chat/conversations", params={"snapshot": "latest"})
        assert listed.status_code == 200
        conversations = listed.json()["conversations"]
        assert conversations[0]["id"] == cid, "listing is not newest-first"
        assert conversations[0]["messages"] == [], "a listing carries transcripts"

        fetched = await chat.get(f"/api/chat/conversations/{cid}")
        assert fetched.status_code == 200
        assert fetched.json()["id"] == cid
    finally:
        deleted = await chat.delete(f"/api/chat/conversations/{cid}")
        assert deleted.status_code == 204

    assert (await chat.get(f"/api/chat/conversations/{cid}")).status_code == 404


@pytest.mark.integration
async def test_cascade_from_snapshot(
    chat: httpx.AsyncClient,
    client: BackendClient,
    ingest: Callable[[Path, str], str],
    other_demo_set: Path,
    backend_url: str,
    auth_headers: dict[str, str],
) -> None:
    """Deleting a snapshot takes its conversations with it, across both services."""
    sid = ingest(other_demo_set, "chat-cascade")
    cid = await new_conversation(chat, sid)
    # A message, so the cascade has something to cascade through.
    await client.append_message(cid, "user", "does this survive?")

    with httpx.Client(base_url=backend_url, headers=auth_headers, timeout=60.0) as http:
        assert http.delete(f"/api/v1/snapshots/{sid}").status_code == 204

    assert (await chat.get(f"/api/chat/conversations/{cid}")).status_code == 404


@pytest.mark.llm
async def test_three_turn_conversation(chat: httpx.AsyncClient, snapshot_id: str) -> None:
    """Three turns, six messages, ordinals assigned by the database."""
    cid = await new_conversation(chat, snapshot_id)
    try:
        for question in (
            "What is the grain of fact_orders?",
            "Which domains exist in this model?",
            "Which tables are conformed?",
        ):
            await take_turn(chat, cid, question)

        messages = await transcript(chat, cid)

        assert [m["ordinal"] for m in messages] == [0, 1, 2, 3, 4, 5]
        assert [m["role"] for m in messages] == ["user", "assistant"] * 3
        assert any(m["citations"] for m in messages if m["role"] == "assistant"), (
            "no answer cited anything; retrieval is not reaching the model"
        )
    finally:
        await chat.delete(f"/api/chat/conversations/{cid}")


@pytest.mark.llm
async def test_history_reaches_model(chat: httpx.AsyncClient, snapshot_id: str) -> None:
    """A pronoun in the second question must resolve against the first turn."""
    cid = await new_conversation(chat, snapshot_id)
    try:
        await take_turn(chat, cid, "What is the grain of fact_orders?")
        second = await take_turn(chat, cid, "And what joins to it?")

        assert FACT_ORDERS in second["assistantMessage"]["citations"], (
            "the follow-up did not resolve 'it'; history is not reaching the model"
        )
    finally:
        await chat.delete(f"/api/chat/conversations/{cid}")


@pytest.mark.llm
async def test_snapshot_pinned_to_conversation(
    chat: httpx.AsyncClient,
    snapshot_id: str,
    ingest: Callable[[Path, str], str],
    other_demo_set: Path,
) -> None:
    """A re-ingest must not move a conversation onto the new snapshot."""
    cid = await new_conversation(chat, snapshot_id)
    try:
        newer = ingest(other_demo_set, "chat-pinning-newer")
        assert newer != snapshot_id

        answer = await take_turn(chat, cid, f"Which columns does {FACT_ORDERS} have?")

        assert answer["toolCalls"], "the answer needed no lookup, so it proves nothing"
        # fact_orders lives only in the Jaffle Shop set.
        assert FACT_ORDERS in answer["assistantMessage"]["citations"], (
            "the conversation followed 'latest' onto a newer snapshot"
        )

        pinned = await chat.get(f"/api/chat/conversations/{cid}")
        assert pinned.json()["snapshotId"] == snapshot_id
    finally:
        await chat.delete(f"/api/chat/conversations/{cid}")


@pytest.mark.llm
async def test_survives_restart(chat: httpx.AsyncClient, chat_url: str, snapshot_id: str) -> None:
    """The test this phase chose Postgres for."""
    cid = await new_conversation(chat, snapshot_id)
    try:
        for question in (
            "What is the grain of fact_orders?",
            "Which domains exist in this model?",
            "Which tables are conformed?",
        ):
            await take_turn(chat, cid, question)

        before = await transcript(chat, cid)
        assert len(before) == 6

        subprocess.run(["docker", "compose", "restart", "chat"], check=True)
        took = wait_for_health(chat_url)
        print(f"\nchat service came back in {took:.1f}s")

        after = await transcript(chat, cid)

        assert len(after) == 6, "messages were lost across a restart"
        assert [(m["ordinal"], m["role"]) for m in after] == [
            (m["ordinal"], m["role"]) for m in before
        ]
        assert [m["content"] for m in after] == [m["content"] for m in before]
        assert [m["citations"] for m in after] == [m["citations"] for m in before]
    finally:
        await chat.delete(f"/api/chat/conversations/{cid}")


@pytest.mark.llm
async def test_user_message_survives_provider_failure(
    chat: httpx.AsyncClient, chat_url: str, snapshot_id: str, llm_settings: Settings
) -> None:
    """A failed turn leaves the question in the transcript."""
    cid = await new_conversation(chat, snapshot_id)
    restart = ["docker", "compose", "up", "-d", "chat"]
    try:
        subprocess.run(restart, check=True, env=_env(llm_settings, LLM_MODEL="not-a-real-model"))
        wait_for_health(chat_url)

        failed = await chat.post(
            f"/api/chat/conversations/{cid}/turn", json={"question": "does this survive?"}
        )
        assert failed.status_code == 502
        assert "not-a-real-model" not in failed.text, "the provider's error reached the caller"
    finally:
        subprocess.run(restart, check=True, env=_env(llm_settings))
        wait_for_health(chat_url)

    try:
        messages = await transcript(chat, cid)
        assert [m["role"] for m in messages] == ["user"], "the question was lost"
        assert messages[0]["content"] == "does this survive?"
    finally:
        await chat.delete(f"/api/chat/conversations/{cid}")


def _env(settings: Settings, **over: str) -> dict[str, str]:
    return {
        **os.environ,
        "LLM_PROVIDER": "vertex",
        "VERTEX_PROJECT": settings.vertex_project,
        "VERTEX_LOCATION": settings.vertex_location,
        **over,
    }
