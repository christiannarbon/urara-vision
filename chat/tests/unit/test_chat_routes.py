"""The conversation routes, with the backend client faked.

These wrappers have almost no logic of their own, so what is worth asserting is
the little they do decide: that `latest` reaches the backend unchanged, that a
missing `?snapshot=` is refused here rather than upstream, and that every
failure becomes the documented status through the shared exception handlers
rather than through a try/except in a route.
"""

import asyncio
from datetime import UTC, datetime
from typing import Any

import httpx
import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient

import urara_chat.api.answering as answering
from urara_chat.agent.pipeline import AgentAnswer
from urara_chat.api.chat_routes import router as chat_router
from urara_chat.api.errors import BACKEND_UNAVAILABLE, PROVIDER_FAILED, install_error_handlers
from urara_chat.api.locks import ConversationLocks
from urara_chat.api.middleware import RequestIDMiddleware
from urara_chat.api.routes import router as debug_router
from urara_chat.backend.errors import BackendError, BackendNotFound
from urara_chat.backend.models import Conversation, Message
from urara_chat.config import Settings

CREATED = datetime(2026, 9, 11, 12, 0, tzinfo=UTC)

# What the backend would say. Never seen by a caller: an upstream message can
# name internal hosts, so the handlers answer with their own words.
UPSTREAM = "conversations table is on fire at db-internal-7"


def conversation(**over: Any) -> Conversation:
    base: dict[str, Any] = {
        "id": "conv-1",
        "snapshot_id": "real-snapshot-id",
        "title": "why is fact_orders one row per order",
        "created_at": CREATED,
        "updated_at": CREATED,
    }
    return Conversation(**(base | over))


class FakeClient:
    """Stands in for BackendClient, recording what the routes handed it."""

    def __init__(self, raises: Exception | None = None) -> None:
        self.raises = raises
        self.created: list[tuple[str, str]] = []
        self.listed: list[str] = []
        self.fetched: list[str] = []
        self.deleted: list[str] = []

    async def create_conversation(self, snapshot_id: str, title: str = "") -> Conversation:
        self.created.append((snapshot_id, title))
        if self.raises:
            raise self.raises
        return conversation(snapshot_id="real-snapshot-id", title=title)

    async def list_conversations(self, snapshot_id: str) -> list[Conversation]:
        self.listed.append(snapshot_id)
        if self.raises:
            raise self.raises
        return [conversation()]

    async def get_conversation(self, cid: str) -> Conversation:
        self.fetched.append(cid)
        if self.raises:
            raise self.raises
        return conversation(
            id=cid,
            messages=[
                Message(role="user", content="hi"),
                Message(role="assistant", content="hello"),
            ],
        )

    async def delete_conversation(self, cid: str) -> None:
        self.deleted.append(cid)
        if self.raises:
            raise self.raises


def client_for(fake: FakeClient) -> TestClient:
    """The routes as they are actually served: middleware, handlers and all.

    The error mapping is part of what these routes promise, and it lives in the
    handlers rather than in them -- an app without those would test a contract
    nobody ships.
    """
    app = FastAPI()
    app.add_middleware(RequestIDMiddleware)
    install_error_handlers(app)
    app.include_router(chat_router)
    app.state.client = fake
    return TestClient(app, raise_server_exceptions=False)


class TestCreate:
    def test_latest_reaches_the_backend_untouched(self) -> None:
        """The backend resolves the alias and stores what it resolved to. A
        second opinion here is how a thread ends up pinned to two snapshots."""
        fake = FakeClient()
        response = client_for(fake).post(
            "/api/chat/conversations", json={"snapshotId": "latest", "title": "why"}
        )

        assert response.status_code == 201
        assert fake.created == [("latest", "why")]

    def test_it_returns_the_backend_conversation(self) -> None:
        response = client_for(FakeClient()).post(
            "/api/chat/conversations", json={"snapshotId": "latest"}
        )

        assert response.status_code == 201
        body = response.json()
        assert body["id"] == "conv-1"
        # camelCase on the wire, and the concrete ID rather than the alias.
        assert body["snapshotId"] == "real-snapshot-id"

    def test_a_missing_snapshot_id_is_400(self) -> None:
        fake = FakeClient()
        response = client_for(fake).post("/api/chat/conversations", json={"title": "orphan"})

        assert response.status_code == 400
        assert response.json()["fields"][0] == {
            "field": "snapshotId",
            "location": "body",
            "reason": "Field required",
        }
        assert fake.created == []

    def test_an_unknown_snapshot_is_404(self) -> None:
        response = client_for(FakeClient(raises=BackendNotFound(404, UPSTREAM))).post(
            "/api/chat/conversations", json={"snapshotId": "nope"}
        )

        assert response.status_code == 404
        assert UPSTREAM not in response.text


class TestList:
    def test_it_lists_for_the_snapshot_it_was_given(self) -> None:
        fake = FakeClient()
        response = client_for(fake).get("/api/chat/conversations?snapshot=latest")

        assert response.status_code == 200
        assert fake.listed == ["latest"]
        assert len(response.json()["conversations"]) == 1

    def test_a_listing_carries_no_transcripts(self) -> None:
        response = client_for(FakeClient()).get("/api/chat/conversations?snapshot=latest")

        assert response.json()["conversations"][0]["messages"] == []

    def test_a_missing_snapshot_is_400(self) -> None:
        fake = FakeClient()
        response = client_for(fake).get("/api/chat/conversations")

        assert response.status_code == 400
        assert fake.listed == []
        # Where to go and fix it. The body carries a "snapshotId" and the query
        # string a "snapshot"; the name alone does not say which one is meant.
        assert response.json()["fields"][0] == {
            "field": "snapshot",
            "location": "query",
            "reason": "Field required",
        }

    def test_an_empty_snapshot_is_400_here_not_502_upstream(self) -> None:
        """Forwarded, it would come back as a 502 blaming the backend for the
        caller's own missing parameter."""
        fake = FakeClient()
        response = client_for(fake).get("/api/chat/conversations?snapshot=")

        assert response.status_code == 400
        assert fake.listed == []


class TestGetAndDelete:
    def test_get_returns_the_transcript(self) -> None:
        response = client_for(FakeClient()).get("/api/chat/conversations/conv-9")

        assert response.status_code == 200
        body = response.json()
        assert body["id"] == "conv-9"
        assert [m["role"] for m in body["messages"]] == ["user", "assistant"]

    def test_get_unknown_is_404(self) -> None:
        response = client_for(FakeClient(raises=BackendNotFound(404, UPSTREAM))).get(
            "/api/chat/conversations/nope"
        )

        assert response.status_code == 404
        assert response.json()["error"] == "not found"

    def test_delete_is_204(self) -> None:
        fake = FakeClient()
        response = client_for(fake).delete("/api/chat/conversations/conv-1")

        assert response.status_code == 204
        assert response.content == b""
        assert fake.deleted == ["conv-1"]

    def test_delete_unknown_is_404(self) -> None:
        response = client_for(FakeClient(raises=BackendNotFound(404, UPSTREAM))).delete(
            "/api/chat/conversations/nope"
        )

        assert response.status_code == 404


class TestUpstreamFailures:
    """A backend failure is 502 and says nothing about why."""

    def test_a_backend_error_is_502_and_generic(self) -> None:
        response = client_for(FakeClient(raises=BackendError(500, UPSTREAM))).get(
            "/api/chat/conversations?snapshot=latest"
        )

        assert response.status_code == 502
        assert response.json()["error"] == BACKEND_UNAVAILABLE
        assert UPSTREAM not in response.text

    def test_every_response_carries_a_request_id(self) -> None:
        """Including the failures, which are the ones somebody will quote."""
        ok = client_for(FakeClient()).get("/api/chat/conversations?snapshot=latest")
        failed = client_for(FakeClient(raises=BackendError(500, UPSTREAM))).get(
            "/api/chat/conversations?snapshot=latest"
        )

        assert ok.headers["x-request-id"]
        assert failed.headers["x-request-id"]
        assert failed.json()["requestId"] == failed.headers["x-request-id"]


# --- the stateless answer route --------------------------------------------
#
# Phase 08's eval runner drives this thousands of times, which is what makes
# "writes nothing" the assertion that matters: a run leaving a thousand
# transcripts behind is a run nobody repeats.

FAKE_KEY = "test-key-shaped-value-0123456789abcdef"

# What the provider would say. Never seen by a caller: the text quotes the
# request back, so it can carry prompt fragments and credentials.
PROVIDER_TEXT = "429 quota exceeded, prompt was: AIza-shaped-thing"


def agent_answer(**over: Any) -> AgentAnswer:
    base: dict[str, Any] = {
        "text": "dim_customers and dim_date are conformed.",
        "citations": ["shared_kernel/dim_date"],
        "tool_calls": [{"name": "list_tables", "args": {}}],
        "iterations": 2,
        "truncated": False,
        "model": "gemini-2.5-flash",
        "latency_ms": 1234,
        "usage": {"input_tokens": 900, "output_tokens": 120},
    }
    return AgentAnswer(**(base | over))


class WritelessClient:
    """A backend that fails loudly if anything tries to write through it.

    The route's whole promise is that it does not, so the fake does not merely
    record writes -- it refuses them. A test asserting on a counter afterwards
    would still have let the write happen.
    """

    def __init__(self, raises: Exception | None = None) -> None:
        self.raises = raises
        self.resolved: list[str] = []

    async def resolve_snapshot(self, sid: str) -> str:
        self.resolved.append(sid)
        if self.raises:
            raise self.raises
        return "real-snapshot-id" if sid == "latest" else sid

    async def create_conversation(self, *args: Any, **kwargs: Any) -> Any:
        raise AssertionError("the answer route created a conversation")

    async def append_message(self, *args: Any, **kwargs: Any) -> Any:
        raise AssertionError("the answer route stored a message")

    async def set_conversation_title(self, *args: Any, **kwargs: Any) -> Any:
        raise AssertionError("the answer route set a title")


class FakePipeline:
    def __init__(self, result: AgentAnswer | None = None, raises: Exception | None = None) -> None:
        self.result = result if result is not None else agent_answer()
        self.raises = raises
        self.calls: list[dict[str, Any]] = []

    async def __call__(
        self, question: str, snapshot_id: str, history: Any, language: str = "EN"
    ) -> AgentAnswer:
        self.calls.append(
            {"question": question, "snapshot_id": snapshot_id, "history": list(history)}
        )
        if self.raises:
            raise self.raises
        return self.result


def answer_app(
    fake: WritelessClient, pipeline: FakePipeline, monkeypatch: pytest.MonkeyPatch
) -> TestClient:
    """Both answer paths on one app, so their shapes can be compared."""
    monkeypatch.setattr(answering, "answer", pipeline)

    app = FastAPI()
    app.add_middleware(RequestIDMiddleware)
    install_error_handlers(app)
    app.include_router(chat_router)
    app.include_router(debug_router)
    app.state.client = fake
    app.state.settings = Settings(google_api_key=FAKE_KEY)  # type: ignore[arg-type]
    return TestClient(app, raise_server_exceptions=False)


def ask(client: TestClient, **body: Any) -> Any:
    return client.post("/api/chat/answer", json=body)


class TestAnswerWritesNothing:
    def test_no_conversation_or_message_is_created(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """The fake raises on every write, so this passes only if none was
        attempted."""
        client = answer_app(WritelessClient(), FakePipeline(), monkeypatch)
        response = ask(client, snapshotId="latest", question="Which tables are conformed?")

        assert response.status_code == 200

    def test_the_pipeline_is_given_no_history(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """There is no conversation to read one from, which is what makes this
        path cheap enough to run in bulk."""
        pipeline = FakePipeline()
        ask(answer_app(WritelessClient(), pipeline, monkeypatch), snapshotId="s1", question="q")

        assert pipeline.calls[0]["history"] == []


class TestAnswerShape:
    def test_it_returns_every_documented_field(self, monkeypatch: pytest.MonkeyPatch) -> None:
        client = answer_app(WritelessClient(), FakePipeline(), monkeypatch)
        response = ask(client, snapshotId="latest", question="Which tables are conformed?")

        assert response.json() == {
            "text": "dim_customers and dim_date are conformed.",
            "citations": ["shared_kernel/dim_date"],
            "toolCalls": [{"name": "list_tables", "args": {}}],
            "iterations": 2,
            "truncated": False,
            "model": "gemini-2.5-flash",
            "latencyMs": 1234,
            "usage": {"input_tokens": 900, "output_tokens": 120},
        }

    def test_both_paths_answer_in_the_same_shape(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """They share an implementation. If these ever differ, the route used to
        explain a bad answer has stopped describing the one that produced it."""
        client = answer_app(WritelessClient(), FakePipeline(), monkeypatch)
        body = {"snapshotId": "latest", "question": "Which tables are conformed?"}

        promoted = client.post("/api/chat/answer", json=body)
        debug = client.post("/debug/answer", json=body)

        assert promoted.status_code == debug.status_code == 200
        assert promoted.json() == debug.json()


class TestAnswerValidation:
    def test_latest_is_resolved_before_the_pipeline_sees_it(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """answer() refuses the alias, and rightly: reading the wrong snapshot
        produces a confidently wrong answer."""
        fake, pipeline = WritelessClient(), FakePipeline()
        ask(answer_app(fake, pipeline, monkeypatch), snapshotId="latest", question="q")

        assert fake.resolved == ["latest"]
        assert pipeline.calls[0]["snapshot_id"] == "real-snapshot-id"

    @pytest.mark.parametrize("question", ["", "   ", "\n\t "])
    def test_an_empty_question_is_400(self, question: str, monkeypatch: pytest.MonkeyPatch) -> None:
        pipeline = FakePipeline()
        response = ask(
            answer_app(WritelessClient(), pipeline, monkeypatch), snapshotId="s1", question=question
        )

        assert response.status_code == 400
        assert "question" in response.json()["detail"]
        assert pipeline.calls == []

    def test_an_over_long_question_is_400(self, monkeypatch: pytest.MonkeyPatch) -> None:
        client = answer_app(WritelessClient(), FakePipeline(), monkeypatch)
        response = ask(client, snapshotId="s1", question="x" * 4001)

        assert response.status_code == 400
        assert "4000" in response.json()["detail"]

    def test_an_unknown_snapshot_is_404(self, monkeypatch: pytest.MonkeyPatch) -> None:
        fake = WritelessClient(raises=BackendNotFound(404, "snapshot not found"))
        pipeline = FakePipeline()
        response = ask(answer_app(fake, pipeline, monkeypatch), snapshotId="nope", question="q")

        assert response.status_code == 404
        assert pipeline.calls == []

    def test_a_provider_failure_is_502_without_its_text(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        pipeline = FakePipeline(raises=RuntimeError(PROVIDER_TEXT))
        response = ask(
            answer_app(WritelessClient(), pipeline, monkeypatch), snapshotId="s1", question="q"
        )

        assert response.status_code == 502
        assert response.json()["error"] == PROVIDER_FAILED
        assert "quota" not in response.text
        assert "AIza" not in response.text


class TestAnswerTakesASlotButNoLock:
    """It costs a provider call like any other, and the eval runner is exactly
    the client that fires many at once. There is no conversation to serialise,
    so there is nothing for a lock to protect -- and holding one would only slow
    the bulk case down."""

    def build(
        self, monkeypatch: pytest.MonkeyPatch, seconds: float
    ) -> tuple[Any, ConversationLocks]:
        class Slow(FakePipeline):
            async def __call__(self, *args: Any, **kwargs: Any) -> AgentAnswer:
                await asyncio.sleep(seconds)
                return agent_answer()

        monkeypatch.setattr(answering, "answer", Slow())

        app = FastAPI()
        app.add_middleware(RequestIDMiddleware)
        install_error_handlers(app)
        app.include_router(chat_router)
        app.state.client = WritelessClient()
        app.state.settings = Settings(  # type: ignore[call-arg]
            google_api_key=FAKE_KEY, max_concurrent_turns=1
        )
        locks = ConversationLocks()
        app.state.conversation_locks = locks
        return app, locks

    async def test_over_the_cap_is_429(self, monkeypatch: pytest.MonkeyPatch) -> None:
        app, _ = self.build(monkeypatch, seconds=1.0)

        async with httpx.AsyncClient(
            transport=httpx.ASGITransport(app=app), base_url="http://test"
        ) as client:
            first, second = await asyncio.gather(
                client.post("/api/chat/answer", json={"snapshotId": "s1", "question": "one"}),
                client.post("/api/chat/answer", json={"snapshotId": "s1", "question": "two"}),
            )

        assert {first.status_code, second.status_code} == {200, 429}

    async def test_no_conversation_lock_is_taken(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """Sampled while the request is in flight: a lock taken and released
        would be invisible afterwards."""
        app, locks = self.build(monkeypatch, seconds=0.3)

        async with httpx.AsyncClient(
            transport=httpx.ASGITransport(app=app), base_url="http://test"
        ) as client:
            running = asyncio.create_task(
                client.post("/api/chat/answer", json={"snapshotId": "s1", "question": "q"})
            )
            await asyncio.sleep(0.1)
            tracked_while_running = locks.tracked
            assert (await running).status_code == 200

        assert tracked_while_running == set(), "the answer route took a conversation lock"
