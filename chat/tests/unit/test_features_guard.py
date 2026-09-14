"""The chat feature gate, with the backend client faked and a fake clock."""

import asyncio
from typing import Any

import pytest
from fastapi import FastAPI
from fastapi.routing import APIRoute
from fastapi.testclient import TestClient
from pydantic import ValidationError

from urara_chat.api.chat_routes import router as chat_router
from urara_chat.api.errors import CHAT_TURNED_OFF, install_error_handlers
from urara_chat.api.features import FeatureGate
from urara_chat.api.middleware import RequestIDMiddleware
from urara_chat.api.routes import router as probe_router
from urara_chat.backend.errors import BackendError, BackendUnavailable
from urara_chat.backend.models import ChatFeature, Conversation, Features
from urara_chat.config import Settings

TTL = 15.0


class Clock:
    def __init__(self) -> None:
        self.now = 1000.0

    def __call__(self) -> float:
        return self.now


class FakeClient:
    def __init__(
        self, enabled: bool = True, raises: BackendError | None = None, malformed: bool = False
    ) -> None:
        self.enabled = enabled
        self.raises = raises
        self.malformed = malformed
        self.feature_calls = 0
        self.listed = 0

    async def features(self) -> Features:
        self.feature_calls += 1
        # Yields, so concurrent callers overlap as they would against a real backend.
        await asyncio.sleep(0)
        if self.raises:
            raise self.raises
        if self.malformed:
            return Features.model_validate({})
        return Features(chat=ChatFeature(available=True, enabled=self.enabled))

    async def list_conversations(self, snapshot_id: str, limit: int | None = None) -> list[Any]:
        self.listed += 1
        return [Conversation(id="conv-1", snapshot_id=snapshot_id)]

    async def aclose(self) -> None:
        pass


def client_for(fake: FakeClient, clock: Clock | None = None) -> TestClient:
    app = FastAPI()
    app.add_middleware(RequestIDMiddleware)
    install_error_handlers(app)
    app.include_router(chat_router)
    app.include_router(probe_router)
    app.state.client = fake
    app.state.feature_gate = FeatureGate(fake, TTL, clock or Clock())  # type: ignore[arg-type]
    return TestClient(app, raise_server_exceptions=False)


def list_conversations(client: TestClient) -> Any:
    return client.get("/api/chat/conversations", params={"snapshot": "latest"})


def test_enabled_runs_the_route() -> None:
    fake = FakeClient(enabled=True)
    response = list_conversations(client_for(fake))
    assert response.status_code == 200
    assert fake.listed == 1


def test_disabled_is_503_and_the_route_never_runs() -> None:
    fake = FakeClient(enabled=False)
    response = list_conversations(client_for(fake))
    assert response.status_code == 503
    body = response.json()
    assert body["error"] == CHAT_TURNED_OFF
    assert body["requestId"]
    assert fake.listed == 0


def test_answer_is_gated() -> None:
    fake = FakeClient(enabled=False)
    response = client_for(fake).post(
        "/api/chat/answer", json={"snapshotId": "latest", "question": "what is fact_orders"}
    )
    assert response.status_code == 503


def test_answer_is_cached_for_the_ttl() -> None:
    fake = FakeClient()
    clock = Clock()
    client = client_for(fake, clock)

    list_conversations(client)
    clock.now += TTL - 1
    list_conversations(client)
    assert fake.feature_calls == 1

    clock.now += 1
    list_conversations(client)
    assert fake.feature_calls == 2


def test_a_change_is_seen_after_the_ttl() -> None:
    fake = FakeClient(enabled=True)
    clock = Clock()
    client = client_for(fake, clock)

    assert list_conversations(client).status_code == 200
    fake.enabled = False
    assert list_conversations(client).status_code == 200
    clock.now += TTL
    assert list_conversations(client).status_code == 503


def test_unreachable_backend_allows_the_request() -> None:
    fake = FakeClient(raises=BackendUnavailable(502, "backend unreachable"))
    response = list_conversations(client_for(fake))
    assert response.status_code == 200
    assert fake.listed == 1


def test_backend_error_response_allows_the_request() -> None:
    fake = FakeClient(raises=BackendError(500, "internal error"))
    assert list_conversations(client_for(fake)).status_code == 200


def test_a_known_off_survives_a_backend_error() -> None:
    fake = FakeClient(enabled=False)
    clock = Clock()
    client = client_for(fake, clock)
    assert list_conversations(client).status_code == 503

    fake.raises = BackendUnavailable(502, "backend unreachable")
    clock.now += TTL
    assert list_conversations(client).status_code == 503


def test_a_failure_is_cached_for_the_ttl() -> None:
    fake = FakeClient(raises=BackendUnavailable(502, "backend unreachable"))
    client = client_for(fake)
    list_conversations(client)
    list_conversations(client)
    assert fake.feature_calls == 1


def test_malformed_features_allow_the_request() -> None:
    fake = FakeClient(malformed=True)
    assert list_conversations(client_for(fake)).status_code == 200


async def test_concurrent_requests_refresh_once() -> None:
    fake = FakeClient()
    gate = FeatureGate(fake, TTL, Clock())  # type: ignore[arg-type]
    await asyncio.gather(*(gate.require_chat() for _ in range(10)))
    assert fake.feature_calls == 1


def test_cache_seconds_below_one_is_refused() -> None:
    with pytest.raises(ValidationError):
        Settings(google_api_key="k", features_cache_seconds=0.5)  # type: ignore[arg-type]


def test_healthz_is_not_gated() -> None:
    fake = FakeClient(enabled=False)
    response = client_for(fake).get("/healthz")
    assert response.status_code == 200
    assert fake.feature_calls == 0


def test_the_lifespan_gates_the_real_app(monkeypatch: pytest.MonkeyPatch) -> None:
    import urara_chat.main as main
    from urara_chat.config import get_settings

    monkeypatch.setenv("GOOGLE_API_KEY", "test-key-not-real")
    get_settings.cache_clear()
    monkeypatch.setattr(main, "BackendClient", lambda settings: FakeClient(enabled=False))
    monkeypatch.setattr(main, "build_chat_model", lambda settings: object())

    routes = [r for r in chat_router.routes if isinstance(r, APIRoute)]
    assert routes
    try:
        with TestClient(main.app) as c:
            # The gate runs before params and body are validated, so no route needs real input.
            for route in routes:
                path = route.path.replace("{cid}", "conv-1")
                for method in route.methods:
                    assert c.request(method, path).status_code == 503, f"{method} {path}"
            assert c.get("/healthz").status_code == 200
    finally:
        get_settings.cache_clear()
