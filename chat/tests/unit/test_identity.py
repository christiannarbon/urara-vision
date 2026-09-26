"""The identity nginx passes in, and the X-Acting-User it becomes."""

import asyncio
from typing import Any

import httpx
import respx
from fakes import TEST_USER_ID
from fastapi import FastAPI
from fastapi.testclient import TestClient

from urara_chat.api.chat_routes import router as chat_router
from urara_chat.api.errors import install_error_handlers
from urara_chat.api.features import FeatureGate
from urara_chat.api.middleware import RequestIDMiddleware
from urara_chat.api.routes import router as probe_router
from urara_chat.backend.client import BackendClient
from urara_chat.backend.models import ChatFeature, Features
from urara_chat.config import Settings

BASE = "http://backend:8080"


def settings() -> Settings:
    return Settings(
        backend_base_url=BASE,
        backend_api_token="service-token-0123456789abcdef",
        google_api_key="test-key-not-real",  # type: ignore[arg-type]
        backend_timeout_seconds=5.0,
        log_level="info",
        app_addr=":8090",
    )


def app_with(client: Any, gate: FeatureGate | None = None) -> FastAPI:
    app = FastAPI()
    app.add_middleware(RequestIDMiddleware)
    install_error_handlers(app)
    app.include_router(chat_router)
    app.include_router(probe_router)
    app.state.client = client
    if gate is not None:
        app.state.feature_gate = gate
    return app


def conversations_json() -> dict[str, Any]:
    return {
        "conversations": [
            {
                "id": "conv-1",
                "snapshotId": "s1",
                "title": "",
                "createdAt": "2026-01-01T00:00:00Z",
                "updatedAt": "2026-01-01T00:00:00Z",
            }
        ]
    }


def test_missing_identity_is_401_with_request_id() -> None:
    client = TestClient(app_with(object()), raise_server_exceptions=False)
    client.headers.pop("X-User-Id")
    res = client.get("/api/chat/conversations", params={"snapshot": "latest"})
    assert res.status_code == 401
    body = res.json()
    assert body["error"] == "not signed in"
    assert body["requestId"] == res.headers["x-request-id"]


@respx.mock
async def test_identity_becomes_acting_user() -> None:
    route = respx.get(f"{BASE}/api/v1/conversations").mock(
        return_value=httpx.Response(200, json=conversations_json())
    )
    backend = BackendClient(settings())
    try:
        client = TestClient(app_with(backend), raise_server_exceptions=False)
        res = client.get("/api/chat/conversations", params={"snapshot": "s1"})
        assert res.status_code == 200
        sent = route.calls.last.request.headers
        assert sent["X-Acting-User"] == TEST_USER_ID
        assert sent["Authorization"].startswith("Bearer ")
    finally:
        await backend.aclose()


def test_probes_need_no_identity() -> None:
    client = TestClient(app_with(object()), raise_server_exceptions=False)
    client.headers.pop("X-User-Id")
    assert client.get("/healthz").status_code == 200


class OffClient:
    feature_calls = 0

    async def features(self) -> Features:
        self.feature_calls += 1
        return Features(chat=ChatFeature(available=True, enabled=False))


def test_identity_is_checked_before_the_feature_gate() -> None:
    off = OffClient()
    client = TestClient(app_with(off, FeatureGate(off, 60)), raise_server_exceptions=False)  # type: ignore[arg-type]
    client.headers.pop("X-User-Id")
    res = client.get("/api/chat/conversations", params={"snapshot": "latest"})
    assert res.status_code == 401
    assert off.feature_calls == 0

    client.headers["X-User-Id"] = TEST_USER_ID
    assert client.get("/api/chat/conversations", params={"snapshot": "latest"}).status_code == 503


@respx.mock
async def test_feature_check_runs_as_the_service() -> None:
    features = respx.get(f"{BASE}/api/v1/features").mock(
        return_value=httpx.Response(200, json={"chat": {"available": True, "enabled": True}})
    )
    respx.get(f"{BASE}/api/v1/conversations").mock(
        return_value=httpx.Response(200, json=conversations_json())
    )
    backend = BackendClient(settings())
    try:
        client = TestClient(
            app_with(backend, FeatureGate(backend, 60)), raise_server_exceptions=False
        )
        assert client.get("/api/chat/conversations", params={"snapshot": "s1"}).status_code == 200
        assert "X-Acting-User" not in features.calls.last.request.headers
    finally:
        await backend.aclose()


@respx.mock
async def test_identity_does_not_leak_between_concurrent_requests() -> None:
    seen: list[tuple[str, str]] = []

    async def slow(request: httpx.Request) -> httpx.Response:
        # Both requests are in flight together when this yields.
        await asyncio.sleep(0.05)
        seen.append((request.url.params["snapshot"], request.headers.get("X-Acting-User", "")))
        return httpx.Response(200, json=conversations_json())

    respx.get(f"{BASE}/api/v1/conversations").mock(side_effect=slow)
    backend = BackendClient(settings())
    try:
        async with httpx.AsyncClient(
            transport=httpx.ASGITransport(app=app_with(backend)), base_url="http://test"
        ) as http:

            async def as_user(user: str) -> int:
                res = await http.get(
                    "/api/chat/conversations",
                    params={"snapshot": f"snap-{user}"},
                    headers={"X-User-Id": user},
                )
                return res.status_code

            assert await asyncio.gather(as_user("alice"), as_user("bob")) == [200, 200]
            # A later request without identity must not inherit the last one's.
            http.headers.pop("X-User-Id")
            no_user = await http.get("/api/chat/conversations", params={"snapshot": "s"})
            assert no_user.status_code == 401
    finally:
        await backend.aclose()

    assert sorted(seen) == [("snap-alice", "alice"), ("snap-bob", "bob")]
