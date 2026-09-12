"""Turns under concurrency: what serialises, what runs in parallel, what is"""

import asyncio
from typing import Any

import httpx
import pytest
from fastapi import FastAPI

import urara_chat.api.answering as answering
from urara_chat.agent.pipeline import AgentAnswer
from urara_chat.api.chat_routes import router as chat_router
from urara_chat.api.errors import install_error_handlers
from urara_chat.api.locks import RETRY_AFTER_SECONDS, ConversationLocks
from urara_chat.api.middleware import RequestIDMiddleware
from urara_chat.backend.models import Conversation, Message
from urara_chat.config import Settings

FAKE_KEY = "test-key-shaped-value-0123456789abcdef"
SNAPSHOT = "real-snapshot-id"

# Long enough that a second request definitely arrives while the first is still in the pipeline,
# short enough not to slow the suite down.
TURN_SECONDS = 0.05


def settings(**over: Any) -> Settings:
    base: dict[str, Any] = {"google_api_key": FAKE_KEY}
    return Settings(**(base | over))  # type: ignore[arg-type]


class FakeBackend:
    """A transcript store with the one property that matters: append assigns"""

    def __init__(self) -> None:
        self.threads: dict[str, list[Message]] = {}

    async def get_conversation(self, cid: str) -> Conversation:
        return Conversation(
            id=cid,
            snapshot_id=SNAPSHOT,
            # A copy: the route must not be able to mutate the store by holding what it read.
            messages=list(self.threads.get(cid, [])),
        )

    async def append_message(
        self,
        cid: str,
        role: str,
        content: str,
        citations: list[str] | None = None,
        meta: dict[str, Any] | None = None,
    ) -> Message:
        thread = self.threads.setdefault(cid, [])
        stored = Message(
            ordinal=len(thread),
            role=role,
            content=content,
            citations=citations or [],
            meta=meta or {},
        )
        thread.append(stored)
        return stored

    def roles(self, cid: str) -> list[tuple[int, str]]:
        return [(m.ordinal, m.role) for m in self.threads.get(cid, [])]


class SlowPipeline:
    """A pipeline that takes measurable time and says when it entered and left."""

    def __init__(self, seconds: float = TURN_SECONDS) -> None:
        self.seconds = seconds
        self.events: list[str] = []
        self.calls = 0

    async def __call__(
        self, question: str, snapshot_id: str, history: Any, language: str = "EN"
    ) -> AgentAnswer:
        self.calls += 1
        self.events.append(f"enter:{question}")
        await asyncio.sleep(self.seconds)
        self.events.append(f"exit:{question}")
        return AgentAnswer(
            text=f"answer to {question}",
            citations=[],
            tool_calls=[],
            iterations=1,
            truncated=False,
            model="gemini-2.5-flash",
            latency_ms=1,
        )


def build_app(
    backend: FakeBackend,
    pipeline: SlowPipeline,
    monkeypatch: pytest.MonkeyPatch,
    config: Settings | None = None,
) -> tuple[FastAPI, ConversationLocks]:
    monkeypatch.setattr(answering, "answer", pipeline)

    app = FastAPI()
    app.add_middleware(RequestIDMiddleware)
    install_error_handlers(app)
    app.include_router(chat_router)
    app.state.client = backend
    app.state.settings = config or settings()
    # Held by the test as well as the app, so what the route did to it can be asserted on
    # directly.
    locks = ConversationLocks()
    app.state.conversation_locks = locks
    return app, locks


def http(app: FastAPI) -> httpx.AsyncClient:
    return httpx.AsyncClient(transport=httpx.ASGITransport(app=app), base_url="http://test")


async def ask(client: httpx.AsyncClient, cid: str, question: str) -> httpx.Response:
    return await client.post(f"/api/chat/conversations/{cid}/turn", json={"question": question})


class TestOneConversationSerialises:
    async def test_two_turns_do_not_interleave(self, monkeypatch: pytest.MonkeyPatch) -> None:
        backend, pipeline = FakeBackend(), SlowPipeline()
        app, _ = build_app(backend, pipeline, monkeypatch)

        async with http(app) as client:
            first, second = await asyncio.gather(
                ask(client, "conv-1", "one"), ask(client, "conv-1", "two")
            )

        assert (first.status_code, second.status_code) == (200, 200)
        # One turn runs to completion before the next starts.
        assert pipeline.events in (
            ["enter:one", "exit:one", "enter:two", "exit:two"],
            ["enter:two", "exit:two", "enter:one", "exit:one"],
        )

    async def test_the_transcript_alternates(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """The whole point."""
        backend, pipeline = FakeBackend(), SlowPipeline()
        app, _ = build_app(backend, pipeline, monkeypatch)

        async with http(app) as client:
            await asyncio.gather(ask(client, "conv-1", "one"), ask(client, "conv-1", "two"))

        assert backend.roles("conv-1") == [
            (0, "user"),
            (1, "assistant"),
            (2, "user"),
            (3, "assistant"),
        ]

    async def test_the_second_turn_sees_the_first_in_its_history(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """Serialising is not only about ordering: the turn that waited should"""
        backend, pipeline = FakeBackend(), SlowPipeline()
        app, _ = build_app(backend, pipeline, monkeypatch)

        async with http(app) as client:
            await ask(client, "conv-1", "one")
            await ask(client, "conv-1", "two")

        assert backend.roles("conv-1")[-1] == (3, "assistant")


class TestDifferentConversationsRunInParallel:
    async def test_two_threads_overlap(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """Serialising per conversation, not globally: one reader's slow turn"""
        backend, pipeline = FakeBackend(), SlowPipeline()
        app, _ = build_app(backend, pipeline, monkeypatch)

        async with http(app) as client:
            await asyncio.gather(ask(client, "conv-1", "one"), ask(client, "conv-2", "two"))

        # Both entered before either left.
        assert pipeline.events[0].startswith("enter")
        assert pipeline.events[1].startswith("enter")


class TestTheCap:
    async def test_over_the_cap_is_429_with_retry_after(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        backend, pipeline = FakeBackend(), SlowPipeline(seconds=1.0)
        app, _ = build_app(backend, pipeline, monkeypatch, settings(max_concurrent_turns=1))

        async with http(app) as client:
            first, second = await asyncio.gather(
                ask(client, "conv-1", "one"), ask(client, "conv-2", "two")
            )

        statuses = {first.status_code, second.status_code}
        assert statuses == {200, 429}
        refused = first if first.status_code == 429 else second
        assert refused.headers["retry-after"] == str(RETRY_AFTER_SECONDS)
        # Refused, not queued: the pipeline never ran for it.
        assert pipeline.calls == 1

    async def test_a_refused_turn_stores_nothing(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """A 429 must not leave half a turn behind."""
        backend, pipeline = FakeBackend(), SlowPipeline(seconds=1.0)
        app, _ = build_app(backend, pipeline, monkeypatch, settings(max_concurrent_turns=1))

        async with http(app) as client:
            running = asyncio.create_task(ask(client, "conv-1", "one"))
            await asyncio.sleep(0.05)
            refused = await ask(client, "conv-2", "two")
            assert (await running).status_code == 200

        assert refused.status_code == 429
        assert backend.roles("conv-2") == [], "a refused turn wrote to the transcript"
        assert backend.roles("conv-1") == [(0, "user"), (1, "assistant")]

    async def test_the_configured_wait_reaches_the_limiter(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """A shorter wait refuses sooner."""
        backend, pipeline = FakeBackend(), SlowPipeline(seconds=1.0)
        app, _ = build_app(
            backend,
            pipeline,
            monkeypatch,
            settings(max_concurrent_turns=1, turn_admission_wait_seconds=0.01),
        )

        async with http(app) as client:
            running = asyncio.create_task(ask(client, "conv-1", "one"))
            await asyncio.sleep(0.05)
            started = asyncio.get_running_loop().time()
            refused = await ask(client, "conv-2", "two")
            waited = asyncio.get_running_loop().time() - started
            assert (await running).status_code == 200

        assert refused.status_code == 429
        assert waited < 0.2, f"the configured wait was ignored; held for {waited:.2f}s"

    async def test_a_tiny_wait_still_admits_a_free_slot(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """The footgun the settings validator exists for."""
        backend, pipeline = FakeBackend(), SlowPipeline()
        app, _ = build_app(
            backend,
            pipeline,
            monkeypatch,
            settings(max_concurrent_turns=1, turn_admission_wait_seconds=0.001),
        )

        async with http(app) as client:
            response = await ask(client, "conv-1", "alone")

        assert response.status_code == 200

    async def test_the_slot_is_taken_before_the_conversation_lock(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """The ordering the task insists on, asserted by what it leaves behind."""
        backend, pipeline = FakeBackend(), SlowPipeline(seconds=1.0)
        app, locks = build_app(backend, pipeline, monkeypatch, settings(max_concurrent_turns=1))

        async with http(app) as client:
            running = asyncio.create_task(ask(client, "conv-1", "one"))
            await asyncio.sleep(0.05)
            # Left in flight: it waits for a slot, and is refused when none comes.
            queued = asyncio.create_task(ask(client, "conv-2", "two"))
            await asyncio.sleep(0.1)
            tracked_while_waiting = locks.tracked

            refused = await queued
            assert (await running).status_code == 200

        assert refused.status_code == 429
        assert tracked_while_waiting == {"conv-1"}, (
            "the turn waiting for a slot was already holding its conversation"
        )


class TestLocksAreEvicted:
    async def test_nothing_is_kept_once_a_turn_is_done(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """A pod that keeps one lock per conversation it has ever seen leaks on"""
        backend, pipeline = FakeBackend(), SlowPipeline()
        app, locks = build_app(backend, pipeline, monkeypatch)

        async with http(app) as client:
            for cid in ("conv-1", "conv-2", "conv-3"):
                await ask(client, cid, "q")

        assert locks.tracked == set()

    async def test_a_lock_survives_while_a_second_turn_waits(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """Eviction is by reference count, not by release: dropping the lock the"""
        backend, pipeline = FakeBackend(), SlowPipeline()
        app, locks = build_app(backend, pipeline, monkeypatch)

        async with http(app) as client:
            await asyncio.gather(ask(client, "conv-1", "one"), ask(client, "conv-1", "two"))

        assert backend.roles("conv-1") == [
            (0, "user"),
            (1, "assistant"),
            (2, "user"),
            (3, "assistant"),
        ]
        assert locks.tracked == set()


class TestValidationHappensFirst:
    async def test_an_over_long_question_takes_no_lock(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """Rejected before anything is held, so a bad request cannot occupy a"""
        backend, pipeline = FakeBackend(), SlowPipeline()
        app, locks = build_app(backend, pipeline, monkeypatch, settings(max_question_chars=10))

        async with http(app) as client:
            response = await ask(client, "conv-1", "x" * 11)

        assert response.status_code == 400
        assert locks.tracked == set()
        assert pipeline.calls == 0
        assert backend.roles("conv-1") == []
