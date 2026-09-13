"""GET /api/chat/stats."""

from typing import Any

from fastapi import FastAPI
from fastapi.testclient import TestClient

from urara_chat.api.chat_routes import MIN_P95_SAMPLES, STATS_CONVERSATION_LIMIT
from urara_chat.api.chat_routes import router as chat_router
from urara_chat.api.errors import install_error_handlers
from urara_chat.api.middleware import RequestIDMiddleware
from urara_chat.backend.errors import BackendNotFound
from urara_chat.backend.models import Conversation, Message

SNAPSHOT = "real-snapshot-id"


def turn(meta: Any = None) -> list[Message]:
    reply = Message(role="assistant", content="a")
    if meta is not None:
        # Bypasses validation so malformed rows can be represented.
        reply = reply.model_copy(update={"meta": meta})
    return [Message(role="user", content="q"), reply]


def meta(**over: Any) -> dict[str, Any]:
    base: dict[str, Any] = {
        "model": "gemini-2.5-flash",
        "promptTokens": 1000,
        "completionTokens": 100,
        "tokensEstimated": False,
        "latencyMs": 2000,
        "truncated": False,
    }
    return base | over


class FakeClient:
    def __init__(self, conversations: dict[str, list[Message]] | None = None) -> None:
        self.conversations = conversations or {}
        self.resolved: list[str] = []
        self.limits: list[int | None] = []
        self.deleted: set[str] = set()

    async def resolve_snapshot(self, sid: str) -> str:
        self.resolved.append(sid)
        if sid not in ("latest", SNAPSHOT):
            raise BackendNotFound(404, "snapshot not found")
        return SNAPSHOT

    async def list_conversations(
        self, snapshot_id: str, limit: int | None = None
    ) -> list[Conversation]:
        self.limits.append(limit)
        return [Conversation(id=cid, snapshot_id=snapshot_id) for cid in self.conversations]

    async def get_conversation(self, cid: str) -> Conversation:
        if cid in self.deleted:
            raise BackendNotFound(404, "conversation not found")
        return Conversation(id=cid, snapshot_id=SNAPSHOT, messages=self.conversations[cid])


def get_stats(fake: FakeClient, snapshot: str | None = SNAPSHOT) -> Any:
    app = FastAPI()
    app.add_middleware(RequestIDMiddleware)
    install_error_handlers(app)
    app.include_router(chat_router)
    app.state.client = fake
    client = TestClient(app, raise_server_exceptions=False)
    params = {"snapshot": snapshot} if snapshot is not None else {}
    return client.get("/api/chat/stats", params=params)


class TestAggregation:
    def test_it_aggregates_across_conversations(self) -> None:
        fake = FakeClient(
            {
                "c1": turn(meta()) + turn(meta(latencyMs=4000, truncated=True)),
                "c2": turn(meta(promptTokens=500, completionTokens=50, tokensEstimated=True)),
            }
        )
        body = get_stats(fake).json()

        assert body["snapshotId"] == SNAPSHOT
        assert body["conversations"] == 2
        assert body["turns"] == 3
        assert body["promptTokens"] == 2500
        assert body["completionTokens"] == 250
        assert body["estimatedTokenTurns"] == 1
        assert body["meanLatencyMs"] == 2667
        assert body["truncatedTurns"] == 1
        assert body["conversationsCapped"] is False
        assert fake.limits == [STATS_CONVERSATION_LIMIT]

    def test_by_model_counts_two_models(self) -> None:
        fake = FakeClient(
            {
                "c1": turn(meta()) + turn(meta(model="gemini-2.5-pro")),
                "c2": turn(meta()),
            }
        )
        assert get_stats(fake).json()["byModel"] == {"gemini-2.5-flash": 2, "gemini-2.5-pro": 1}


class TestOldAndMalformedMeta:
    def test_a_message_with_no_meta_is_counted_but_adds_nothing(self) -> None:
        fake = FakeClient({"c1": turn({}) + turn(meta())})
        body = get_stats(fake).json()

        assert body["turns"] == 2
        assert body["promptTokens"] == 1000
        assert body["meanLatencyMs"] == 2000

    def test_a_partial_meta_contributes_what_it_has(self) -> None:
        fake = FakeClient({"c1": turn({"latencyMs": 3000}) + turn(meta(latencyMs=1000))})
        body = get_stats(fake).json()

        assert body["meanLatencyMs"] == 2000
        assert body["promptTokens"] == 1000
        assert body["byModel"] == {"gemini-2.5-flash": 1}

    def test_a_pre_cost_record_meta_falls_back_to_usage(self) -> None:
        old = {"model": "gemini-2.5-flash", "usage": {"input_tokens": 900, "output_tokens": 90}}
        body = get_stats(FakeClient({"c1": turn(old)})).json()

        assert (body["promptTokens"], body["completionTokens"]) == (900, 90)
        assert body["estimatedTokenTurns"] is None

    def test_malformed_values_are_ignored(self) -> None:
        bad = {"promptTokens": "lots", "latencyMs": True, "model": 7, "usage": "x", "truncated": 1}
        body = get_stats(FakeClient({"c1": turn(bad) + turn(meta())})).json()

        assert body["promptTokens"] == 1000
        assert body["meanLatencyMs"] == 2000
        assert body["byModel"] == {"gemini-2.5-flash": 1}
        assert body["truncatedTurns"] == 0


class TestP95:
    def test_it_is_null_below_the_sample_threshold(self) -> None:
        turns = [m for i in range(MIN_P95_SAMPLES - 1) for m in turn(meta(latencyMs=i))]
        assert get_stats(FakeClient({"c1": turns})).json()["p95LatencyMs"] is None

    def test_it_is_nearest_rank_at_the_threshold(self) -> None:
        latencies = range(1, MIN_P95_SAMPLES + 1)
        turns = [m for ms in latencies for m in turn(meta(latencyMs=ms * 100))]
        assert get_stats(FakeClient({"c1": turns})).json()["p95LatencyMs"] == 1900


class TestEmptyAndErrors:
    def test_an_empty_snapshot_returns_zeros_and_nulls(self) -> None:
        response = get_stats(FakeClient())

        assert response.status_code == 200
        assert response.json() == {
            "snapshotId": SNAPSHOT,
            "conversations": 0,
            "conversationsCapped": False,
            "turns": 0,
            "promptTokens": None,
            "completionTokens": None,
            "estimatedTokenTurns": None,
            "meanLatencyMs": None,
            "p95LatencyMs": None,
            "truncatedTurns": None,
            "byModel": {},
        }

    def test_a_missing_snapshot_is_a_400(self) -> None:
        assert get_stats(FakeClient(), snapshot=None).status_code == 400

    def test_an_unknown_snapshot_is_a_404(self) -> None:
        assert get_stats(FakeClient(), snapshot="nope").status_code == 404

    def test_latest_resolves(self) -> None:
        fake = FakeClient({"c1": turn(meta())})
        body = get_stats(fake, snapshot="latest").json()

        assert fake.resolved == ["latest"]
        assert body["snapshotId"] == SNAPSHOT
        assert body["turns"] == 1

    def test_a_full_listing_is_marked_capped(self) -> None:
        fake = FakeClient({f"c{i}": [] for i in range(STATS_CONVERSATION_LIMIT)})
        assert get_stats(fake).json()["conversationsCapped"] is True

    def test_a_conversation_deleted_mid_read_is_skipped(self) -> None:
        fake = FakeClient({"c1": turn(meta()), "gone": turn(meta())})
        fake.deleted = {"gone"}
        response = get_stats(fake)

        assert response.status_code == 200
        assert response.json()["conversations"] == 1
        assert response.json()["turns"] == 1
