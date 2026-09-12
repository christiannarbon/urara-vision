"""Conversations end to end: the chat service, the Go API and Postgres.

Everything up to here proved the pieces behave. These say whether a transcript
survives contact with all three, and one of them -- `test_survives_restart` --
is the reason this phase chose Postgres over in-memory sessions. Without it that
decision is a comment.

**Assertions are on structure, never on prose.** Ordinals, roles, citations and
status codes are the contract; a model's wording drifts between versions, and a
test pinning it fails for no reason, gets deleted, and takes its real assertion
with it.

Two markers are in play. `integration` needs a backend and a chat service and
spends nothing. `llm` spends money and never runs in CI -- and here it carries a
second meaning: those tests drive `docker compose` or need a working provider,
so they run from a shell on the host rather than from inside the test container.
"""

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

# How long the service is given to come back after a restart. Polled rather than
# slept through: a fixed sleep is either slower than it needs to be or shorter
# than the day the image takes longer to start.
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
    """Poll /healthz until it answers. Returns how long that took.

    A fixed sleep is the alternative, and it is wrong in both directions: too
    short on the day the image is slow, and needlessly slow every other day.
    """
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
    """Create, list, get, delete -- no model involved.

    The assertion that matters is the stored snapshot: it must be the concrete
    UUID the alias resolved to, because a thread holding the literal "latest"
    would change subject on the next ingest and its earlier answers would then
    cite tables from a different model.
    """
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
    """Deleting a snapshot takes its conversations with it, across both services.

    Its own snapshot rather than the session's: this test destroys what it is
    given, and the shared fixture is still wanted by everything after it.
    """
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
    """A pronoun in the second question must resolve against the first turn.

    Asserted on citations rather than prose: a model without history cannot tell
    what "it" refers to, and the answer that comes back will be about nothing in
    particular -- or will ask. Either way it will not cite fact_orders.
    """
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
    """A re-ingest must not move a conversation onto the new snapshot.

    The whole reason `latest` is resolved at creation. A thread that followed
    the alias would, after any ingest, answer confidently about a model nobody
    asked it about -- while still looking like the same conversation.

    The second set is ingested *here*, after the conversation exists, so it is
    provably the newer one and "latest" would mean it.

    The question asks for column names on purpose. Grain and table names are in
    the context card, so a question about those is answered without a lookup and
    cites nothing -- which proves only that the model read a card, not which
    snapshot the tools are bound to. Columns are not in the card, so answering
    requires a real lookup in the pinned snapshot.
    """
    cid = await new_conversation(chat, snapshot_id)
    try:
        newer = ingest(other_demo_set, "chat-pinning-newer")
        assert newer != snapshot_id

        answer = await take_turn(chat, cid, f"Which columns does {FACT_ORDERS} have?")

        assert answer["toolCalls"], "the answer needed no lookup, so it proves nothing"
        # fact_orders lives only in the Jaffle Shop set. A successful lookup is
        # proof the turn read the pinned snapshot: against the newer one the
        # tool would have come back not-found and cited nothing.
        assert FACT_ORDERS in answer["assistantMessage"]["citations"], (
            "the conversation followed 'latest' onto a newer snapshot"
        )

        pinned = await chat.get(f"/api/chat/conversations/{cid}")
        assert pinned.json()["snapshotId"] == snapshot_id
    finally:
        await chat.delete(f"/api/chat/conversations/{cid}")


@pytest.mark.llm
async def test_survives_restart(chat: httpx.AsyncClient, chat_url: str, snapshot_id: str) -> None:
    """The test this phase chose Postgres for.

    Three turns, then the process holding them is destroyed. Everything must
    still be there -- content, ordinals and citations -- because none of it ever
    lived in that process. An in-memory session store passes every other test in
    this file and fails this one.

    Drives `docker compose` directly, which is why it is marked `llm` alongside
    the billed tests: both run from a shell on the host, never from inside the
    test container, where there is no docker socket.
    """
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
    """A failed turn leaves the question in the transcript.

    The ordering from 05.4, proven where it actually matters. The reader can
    retry without retyping; the tidier implementation -- store both messages at
    the end -- loses the question at exactly the moment losing it hurts.

    The provider is broken by pointing the service at a model that does not
    exist, which costs nothing: the call fails before any tokens are billed.
    Marked `llm` because it drives compose, not because it spends.
    """
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
    """The environment a `compose up` needs, with overrides applied.

    The whole provider configuration is named explicitly rather than inherited.
    `compose up` re-reads docker-compose.yml and substitutes from whatever
    environment it is handed, so a shell that exported VERTEX_PROJECT but not
    LLM_PROVIDER brings the service back on the *default* provider -- which has
    no credential, crash-loops, and fails every test after this one with a
    connection error that says nothing about why.

    Compose substitutes only the variables the file names, which is also why
    LLM_MODEL can be steered from here at all: every setting the service reads
    is declared there in ${VAR:-default} form.
    """
    return {
        **os.environ,
        "LLM_PROVIDER": "vertex",
        "VERTEX_PROJECT": settings.vertex_project,
        "VERTEX_LOCATION": settings.vertex_location,
        **over,
    }
