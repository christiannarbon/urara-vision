"""The conversation routes, with the backend client faked.

These wrappers have almost no logic of their own, so what is worth asserting is
the little they do decide: that `latest` reaches the backend unchanged, that a
missing `?snapshot=` is refused here rather than upstream, and that every
failure becomes the documented status through the shared exception handlers
rather than through a try/except in a route.
"""

from datetime import UTC, datetime
from typing import Any

from fastapi import FastAPI
from fastapi.testclient import TestClient

from urara_chat.api.chat_routes import router as chat_router
from urara_chat.api.errors import BACKEND_UNAVAILABLE, install_error_handlers
from urara_chat.api.middleware import RequestIDMiddleware
from urara_chat.backend.errors import BackendError, BackendNotFound
from urara_chat.backend.models import Conversation, Message

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
