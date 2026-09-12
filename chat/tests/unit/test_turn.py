"""The turn route: the ordering decisions that cost the most when got wrong."""

import logging
from datetime import UTC, datetime
from typing import Any

import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient

import urara_chat.api.answering as answering
from urara_chat.agent.pipeline import AgentAnswer
from urara_chat.api.chat_routes import router as chat_router
from urara_chat.api.errors import PROVIDER_FAILED, install_error_handlers
from urara_chat.api.middleware import RequestIDMiddleware
from urara_chat.backend.errors import BackendError, BackendNotFound
from urara_chat.backend.models import Conversation, Message
from urara_chat.config import Settings

CREATED = datetime(2026, 9, 11, 12, 0, tzinfo=UTC)
SNAPSHOT = "real-snapshot-id"

# What a provider error says. It must never reach a caller: the text quotes the request back, so
# it can carry prompt fragments and credentials.
PROVIDER_TEXT = "429 quota exceeded for project, prompt was: AIza-shaped-thing"

FAKE_KEY = "test-key-shaped-value-0123456789abcdef"


def settings(**over: Any) -> Settings:
    base: dict[str, Any] = {"google_api_key": FAKE_KEY, "max_question_chars": 4000}
    return Settings(**(base | over))  # type: ignore[arg-type]


def agent_answer(**over: Any) -> AgentAnswer:
    base: dict[str, Any] = {
        "text": "fact_orders is one row per order.",
        "citations": ["ordering/fact_orders"],
        "tool_calls": [{"name": "get_tables", "args": {"ids": ["ordering/fact_orders"]}}],
        "iterations": 2,
        "truncated": False,
        "model": "gemini-2.5-flash",
        "latency_ms": 3412,
        "usage": {"input_tokens": 900, "output_tokens": 120},
    }
    return AgentAnswer(**(base | over))


class FakeClient:
    """A backend that remembers the transcript it was told to store."""

    def __init__(
        self,
        history: list[Message] | None = None,
        raises: Exception | None = None,
    ) -> None:
        self.history = history if history is not None else []
        self.raises = raises
        self.appended: list[dict[str, Any]] = []

    async def get_conversation(self, cid: str) -> Conversation:
        if self.raises:
            raise self.raises
        return Conversation(
            id=cid,
            snapshot_id=SNAPSHOT,
            created_at=CREATED,
            updated_at=CREATED,
            messages=list(self.history),
        )

    async def append_message(
        self,
        cid: str,
        role: str,
        content: str,
        citations: list[str] | None = None,
        meta: dict[str, Any] | None = None,
    ) -> Message:
        self.appended.append(
            {"cid": cid, "role": role, "content": content, "citations": citations, "meta": meta}
        )
        return Message(
            ordinal=len(self.history) + len(self.appended) - 1,
            role=role,
            content=content,
            citations=citations or [],
            meta=meta or {},
            created_at=CREATED,
        )


class FakePipeline:
    """Stands in for the agent, recording exactly what the route handed it."""

    def __init__(self, result: AgentAnswer | None = None, raises: Exception | None = None) -> None:
        self.result = result if result is not None else agent_answer()
        self.raises = raises
        self.calls: list[dict[str, Any]] = []

    async def __call__(
        self, question: str, snapshot_id: str, history: Any, language: str = "EN"
    ) -> AgentAnswer:
        self.calls.append(
            {
                "question": question,
                "snapshot_id": snapshot_id,
                "history": list(history),
                "language": language,
            }
        )
        if self.raises:
            raise self.raises
        return self.result


def turn(
    monkeypatch: pytest.MonkeyPatch,
    fake: FakeClient | None = None,
    pipeline: FakePipeline | None = None,
    config: Settings | None = None,
    cid: str = "conv-1",
    **body: Any,
) -> Any:
    """POST one turn against an app wired the way it is actually served."""
    monkeypatch.setattr(answering, "answer", pipeline or FakePipeline())

    app = FastAPI()
    app.add_middleware(RequestIDMiddleware)
    install_error_handlers(app)
    app.include_router(chat_router)
    app.state.client = fake or FakeClient()
    app.state.settings = config or settings()

    client = TestClient(app, raise_server_exceptions=False)
    return client.post(f"/api/chat/conversations/{cid}/turn", json=body)


class TestTheSnapshotComesFromTheConversation:
    def test_the_pipeline_is_given_the_conversations_snapshot(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        pipeline = FakePipeline()
        response = turn(monkeypatch, pipeline=pipeline, question="the grain of fact_orders?")

        assert response.status_code == 200
        assert pipeline.calls[0]["snapshot_id"] == SNAPSHOT

    def test_a_body_carrying_a_snapshot_id_is_refused(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """The schema has no such field."""
        pipeline = FakePipeline()
        response = turn(
            monkeypatch, pipeline=pipeline, question="q", snapshotId="some-other-snapshot"
        )

        assert response.status_code == 400
        assert pipeline.calls == []


class TestTheQuestionIsStoredFirst:
    def test_a_provider_failure_leaves_the_question_stored(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """The reason the ordering exists."""
        fake = FakeClient()
        response = turn(
            monkeypatch,
            fake,
            FakePipeline(raises=RuntimeError(PROVIDER_TEXT)),
            question="the grain of fact_orders?",
        )

        assert response.status_code == 502
        assert [m["role"] for m in fake.appended] == ["user"]
        assert fake.appended[0]["content"] == "the grain of fact_orders?"

    def test_the_provider_text_never_reaches_the_caller(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        response = turn(
            monkeypatch, pipeline=FakePipeline(raises=RuntimeError(PROVIDER_TEXT)), question="q"
        )

        assert response.json()["error"] == PROVIDER_FAILED
        assert "quota" not in response.text
        assert "AIza" not in response.text

    def test_a_backend_failure_is_not_reported_as_the_model(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """A tool's backend call failing is the store, not the provider, and"""
        response = turn(
            monkeypatch,
            pipeline=FakePipeline(raises=BackendError(500, "neo4j is down")),
            question="q",
        )

        assert response.status_code == 502
        assert response.json()["error"] != PROVIDER_FAILED


class TestALostAnswerIsRecoverable:
    """The turn must still fail -- telling the caller it worked would leave the"""

    def failing_append(self, monkeypatch: pytest.MonkeyPatch) -> tuple[Any, FakeClient]:
        class LosesTheAnswer(FakeClient):
            async def append_message(
                self,
                cid: str,
                role: str,
                content: str,
                citations: list[str] | None = None,
                meta: dict[str, Any] | None = None,
            ) -> Message:
                stored = await super().append_message(cid, role, content, citations, meta)
                if role == "assistant":
                    raise BackendError(500, "messages table is on fire")
                return stored

        fake = LosesTheAnswer()
        return turn(monkeypatch, fake, question="what is the grain?"), fake

    def test_the_turn_fails(self, monkeypatch: pytest.MonkeyPatch) -> None:
        response, _ = self.failing_append(monkeypatch)
        assert response.status_code == 502

    def test_the_question_is_still_stored(self, monkeypatch: pytest.MonkeyPatch) -> None:
        _, fake = self.failing_append(monkeypatch)
        assert fake.appended[0]["role"] == "user"
        assert fake.appended[0]["content"] == "what is the grain?"

    def test_the_answer_reaches_the_log(
        self, monkeypatch: pytest.MonkeyPatch, caplog: pytest.LogCaptureFixture
    ) -> None:
        with caplog.at_level(logging.ERROR, logger="urara_chat.api.chat_routes"):
            self.failing_append(monkeypatch)

        record = next(r for r in caplog.records if "could not be stored" in str(r.msg))
        assert record.answer == "fact_orders is one row per order."  # type: ignore[attr-defined]
        assert record.conversation_id == "conv-1"  # type: ignore[attr-defined]
        assert record.usage == {"input_tokens": 900, "output_tokens": 120}  # type: ignore[attr-defined]


class TestHistory:
    def test_stored_history_reaches_the_pipeline_in_order(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        history = [
            Message(ordinal=0, role="user", content="what is fact_orders?"),
            Message(ordinal=1, role="assistant", content="a fact table."),
        ]
        pipeline = FakePipeline()
        turn(monkeypatch, FakeClient(history), pipeline, question="and what joins to it?")

        sent = pipeline.calls[0]["history"]
        assert [(m.ordinal, m.role) for m in sent] == [(0, "user"), (1, "assistant")]

    def test_the_new_question_is_not_in_the_history(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """The conversation is read before the question is written, so the"""
        pipeline = FakePipeline()
        turn(monkeypatch, pipeline=pipeline, question="and what joins to it?")

        assert pipeline.calls[0]["question"] == "and what joins to it?"
        assert all(m.content != "and what joins to it?" for m in pipeline.calls[0]["history"])


class TestWhatIsStoredAndReturned:
    def test_the_assistant_message_carries_citations_and_meta(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        fake = FakeClient()
        turn(monkeypatch, fake, question="q")

        assert [m["role"] for m in fake.appended] == ["user", "assistant"]
        stored = fake.appended[1]
        assert stored["citations"] == ["ordering/fact_orders"]
        # Phase 08 bills from this; a turn that cannot say what it spent cannot be costed after
        # the fact.
        assert stored["meta"]["usage"] == {"input_tokens": 900, "output_tokens": 120}
        assert stored["meta"]["model"] == "gemini-2.5-flash"
        assert stored["meta"]["iterations"] == 2
        assert stored["meta"]["latencyMs"] == 3412
        assert stored["meta"]["toolCalls"][0]["name"] == "get_tables"

    def test_the_response_carries_both_stored_messages(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """Ordinals are the database's to assign, so what comes back is what was"""
        response = turn(monkeypatch, question="q")
        body = response.json()

        assert body["conversationId"] == "conv-1"
        assert body["userMessage"]["role"] == "user"
        assert body["assistantMessage"]["role"] == "assistant"
        assert body["assistantMessage"]["citations"] == ["ordering/fact_orders"]
        assert body["userMessage"]["ordinal"] == 0
        assert body["assistantMessage"]["ordinal"] == 1
        assert body["toolCalls"][0]["name"] == "get_tables"
        assert body["truncated"] is False
        assert body["latencyMs"] == 3412
        assert body["model"] == "gemini-2.5-flash"


class TestValidation:
    def test_an_empty_question_is_400_naming_the_field(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        fake = FakeClient()
        response = turn(monkeypatch, fake, question="   ")

        assert response.status_code == 400
        assert "question" in response.json()["detail"]
        # Nothing was stored: a rejected question must not leave the thread looking touched.
        assert fake.appended == []

    def test_an_over_long_question_is_400_naming_the_limit(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        fake = FakeClient()
        response = turn(
            monkeypatch, fake, config=settings(max_question_chars=100), question="x" * 101
        )

        assert response.status_code == 400
        detail = response.json()["detail"]
        assert "100" in detail and "101" in detail
        assert fake.appended == []

    def test_an_unknown_conversation_is_404(self, monkeypatch: pytest.MonkeyPatch) -> None:
        fake = FakeClient(raises=BackendNotFound(404, "conversation not found"))
        response = turn(monkeypatch, fake, cid="nope", question="hi")

        assert response.status_code == 404
        assert fake.appended == []

    def test_an_unknown_language_falls_back_rather_than_failing(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """A question is worth answering in the wrong language, and is not worth"""
        pipeline = FakePipeline()
        response = turn(monkeypatch, pipeline=pipeline, question="q", language="KL")

        assert response.status_code == 200
        assert pipeline.calls[0]["language"] == "EN"
