"""Titles derived from a conversation's first question.

A list of four rows reading "Untitled" is useless, and the first question is the
best title available for nothing: no second provider call, no summarisation of a
string nobody reads closely.

Two rules carry the weight. Truncation is by rune, because a Japanese question
cut at sixty *bytes* lands mid-character and renders as mojibake in exactly the
place a reader looks to tell two threads apart. And a title that cannot be set
must not take the turn down with it -- by then the answer has been computed,
paid for and stored.
"""

from typing import Any

import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient

import urara_chat.api.answering as answering
from urara_chat.agent.pipeline import AgentAnswer
from urara_chat.api.chat_routes import router as chat_router
from urara_chat.api.errors import install_error_handlers
from urara_chat.api.middleware import RequestIDMiddleware
from urara_chat.api.titles import MAX_TITLE_RUNES, title_from_question
from urara_chat.backend.errors import BackendError
from urara_chat.backend.models import Conversation, Message
from urara_chat.config import Settings

FAKE_KEY = "test-key-shaped-value-0123456789abcdef"
ELLIPSIS = "…"

# Questions that survive clean_question -- they are not empty before stripping
# -- and derive nothing at all.
QUOTES_ONLY_ASCII = chr(34) * 3
QUOTES_ONLY_CJK = chr(0x300C) + chr(0x300D)

# Sixty runes of Japanese and then some: none of it ASCII, and every character
# three bytes in UTF-8, so a byte-wise cut cannot help but land mid-character.
JAPANESE = (
    "fact_orders の粒度と、顧客テーブルとの関係について教えてください。"
    + "詳しく説明してほしいです。" * 6
)


class TestDerivation:
    """The pure part: question in, title out."""

    def test_a_short_question_is_used_whole(self) -> None:
        assert title_from_question("What is fact_orders?") == "What is fact_orders?"

    def test_a_short_question_gets_no_ellipsis(self) -> None:
        assert ELLIPSIS not in title_from_question("What is fact_orders?")

    def test_whitespace_is_collapsed(self) -> None:
        """A question pasted across three lines would otherwise carry its
        newlines into a list row and break the layout."""
        assert title_from_question("  What\n  is\tthe   grain?  ") == "What is the grain?"

    @pytest.mark.parametrize("quote", ['"', "'", "“", "「"])
    def test_surrounding_quotes_are_stripped(self, quote: str) -> None:
        """A title that opens with a mark and never closes it reads as broken."""
        assert title_from_question(f"{quote}What is fact_orders?{quote}").startswith("What")

    def test_a_long_question_trims_at_a_word_boundary(self) -> None:
        title = title_from_question(
            "What is the grain of fact_orders and how does it relate to customers?"
        )

        assert title.endswith(ELLIPSIS)
        assert len(title) <= MAX_TITLE_RUNES + 1
        # Cut between words, not through one.
        assert not title.removesuffix(ELLIPSIS).endswith(" ")
        assert title.removesuffix(ELLIPSIS).split()[-1] == "to"

    def test_no_boundary_means_a_hard_cut(self) -> None:
        """A language without spaces has no boundary to find, and hunting
        further back for one would throw half the title away."""
        title = title_from_question("a" * 100)

        assert len(title) == MAX_TITLE_RUNES + 1
        assert title == "a" * MAX_TITLE_RUNES + ELLIPSIS

    def test_a_distant_boundary_is_not_used(self) -> None:
        """Only a boundary near the end is worth honouring. One at rune 20 of 60
        would cost the title two thirds of its content."""
        question = "word " + "x" * 100
        assert title_from_question(question).startswith("word x")


class TestJapanese:
    """The case a byte-wise implementation fails, and the reason for runes."""

    def test_it_truncates_to_sixty_runes(self) -> None:
        title = title_from_question(JAPANESE)

        # Codepoints, not bytes. len() on a str is the count that matters here.
        assert len(title) == MAX_TITLE_RUNES + 1
        assert len(title.encode()) > MAX_TITLE_RUNES, "the test text is not multi-byte"

    def test_it_is_still_valid_text(self) -> None:
        """The failure a byte cut produces: a trailing partial character that
        cannot be encoded and renders as a replacement glyph."""
        title = title_from_question(JAPANESE)

        assert title.encode().decode() == title
        assert "�" not in title
        assert title.startswith("fact_orders の粒度")

    def test_a_short_japanese_question_is_untouched(self) -> None:
        question = "粒度を教えてください"
        assert title_from_question(question) == question


class FakeClient:
    """A backend that records the order it was called in.

    The order is the assertion: a title must never exist for a turn that
    produced nothing, so the PATCH has to come after the assistant message.
    """

    def __init__(self, title: str = "", patch_raises: Exception | None = None) -> None:
        self.title = title
        self.patch_raises = patch_raises
        self.calls: list[str] = []
        self.titles: list[str] = []

    async def get_conversation(self, cid: str) -> Conversation:
        self.calls.append("get")
        return Conversation(id=cid, snapshot_id="real-snapshot-id", title=self.title)

    async def append_message(
        self,
        cid: str,
        role: str,
        content: str,
        citations: list[str] | None = None,
        meta: dict[str, Any] | None = None,
    ) -> Message:
        self.calls.append(f"append:{role}")
        return Message(ordinal=len(self.calls), role=role, content=content)

    async def set_conversation_title(self, cid: str, title: str) -> Conversation:
        self.calls.append("patch")
        self.titles.append(title)
        if self.patch_raises:
            raise self.patch_raises
        self.title = title
        return Conversation(id=cid, title=title)


class FakePipeline:
    async def __call__(
        self, question: str, snapshot_id: str, history: Any, language: str = "EN"
    ) -> AgentAnswer:
        return AgentAnswer(
            text="fact_orders is one row per order.",
            citations=[],
            tool_calls=[],
            iterations=1,
            truncated=False,
            model="gemini-2.5-flash",
            latency_ms=1,
        )


def turn(monkeypatch: pytest.MonkeyPatch, fake: FakeClient, question: str) -> Any:
    monkeypatch.setattr(answering, "answer", FakePipeline())

    app = FastAPI()
    app.add_middleware(RequestIDMiddleware)
    install_error_handlers(app)
    app.include_router(chat_router)
    app.state.client = fake
    app.state.settings = Settings(google_api_key=FAKE_KEY)  # type: ignore[arg-type]

    client = TestClient(app, raise_server_exceptions=False)
    return client.post("/api/chat/conversations/conv-1/turn", json={"question": question})


class TestSettingIt:
    def test_an_untitled_thread_is_titled_from_its_question(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        fake = FakeClient()
        response = turn(monkeypatch, fake, "What is the grain of fact_orders?")

        assert response.status_code == 200
        assert fake.titles == ["What is the grain of fact_orders?"]

    def test_the_title_is_set_after_the_answer_is_stored(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """So a title never exists for a turn that produced nothing."""
        fake = FakeClient()
        turn(monkeypatch, fake, "What is fact_orders?")

        assert fake.calls == ["get", "append:user", "append:assistant", "patch"]

    def test_a_titled_thread_is_left_alone(self, monkeypatch: pytest.MonkeyPatch) -> None:
        fake = FakeClient(title="Something the reader chose")
        turn(monkeypatch, fake, "Something completely different about payments")

        assert fake.titles == []
        assert "patch" not in fake.calls

    def test_a_whitespace_title_counts_as_absent(self, monkeypatch: pytest.MonkeyPatch) -> None:
        fake = FakeClient(title="   ")
        turn(monkeypatch, fake, "What is fact_orders?")

        assert fake.titles == ["What is fact_orders?"]


class TestAnEmptyTitleIsNotWritten:
    """`clean_question` lets a question of nothing but quotes through -- it is
    not empty before stripping -- and the derivation then returns "". Writing
    that leaves the thread untitled anyway, so the check for an existing title
    never becomes true and every later turn tries again."""

    @pytest.mark.parametrize("question", [QUOTES_ONLY_ASCII, QUOTES_ONLY_CJK])
    def test_nothing_is_written(self, question: str, monkeypatch: pytest.MonkeyPatch) -> None:
        assert title_from_question(question) == "", "the test question derives a title"
        fake = FakeClient()
        response = turn(monkeypatch, fake, question)

        assert response.status_code == 200
        assert fake.titles == []
        assert "patch" not in fake.calls

    def test_the_turn_still_answers(self, monkeypatch: pytest.MonkeyPatch) -> None:
        fake = FakeClient()
        response = turn(monkeypatch, fake, QUOTES_ONLY_ASCII)

        assert response.json()["assistantMessage"]["content"]
        assert fake.calls == ["get", "append:user", "append:assistant"]


class TestAFailedTitleDoesNotFailTheTurn:
    """By this point the answer has been computed, paid for and stored. Losing
    all of it because a cosmetic PATCH came back 500 would be absurd."""

    def test_the_turn_still_answers(self, monkeypatch: pytest.MonkeyPatch) -> None:
        fake = FakeClient(patch_raises=BackendError(500, "titles table is on fire"))
        response = turn(monkeypatch, fake, "What is fact_orders?")

        assert response.status_code == 200
        assert response.json()["assistantMessage"]["content"] == "fact_orders is one row per order."

    def test_the_failure_is_logged_with_the_request_id(
        self, monkeypatch: pytest.MonkeyPatch, caplog: pytest.LogCaptureFixture
    ) -> None:
        """Swallowed, not hidden: a title that silently never appears is a bug
        report nobody can act on."""
        fake = FakeClient(patch_raises=BackendError(500, "titles table is on fire"))
        with caplog.at_level("WARNING", logger="urara_chat.api.chat_routes"):
            turn(monkeypatch, fake, "What is fact_orders?")

        warnings = [r for r in caplog.records if "title" in str(r.msg)]
        assert len(warnings) == 1
        assert warnings[0].request_id  # type: ignore[attr-defined]
        assert warnings[0].conversation_id == "conv-1"  # type: ignore[attr-defined]

    def test_an_unexpected_error_is_swallowed_too(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """Any failure, not only a tidy backend one: the turn is what matters."""
        fake = FakeClient(patch_raises=RuntimeError("boom"))
        assert turn(monkeypatch, fake, "What is fact_orders?").status_code == 200
