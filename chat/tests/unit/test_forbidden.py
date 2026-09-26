"""How backend refusals reach the caller."""

from pathlib import Path
from typing import Any

import httpx
import pytest
import respx
from fakes import CountingContextClient, FakeChatModel
from fastapi import FastAPI
from fastapi.testclient import TestClient
from langchain_core.messages import AIMessage, HumanMessage
from pydantic import BaseModel

import urara_chat.api.answering as answering
from urara_chat.agent.context_card import ContextCardCache
from urara_chat.agent.graph import build_graph, initial_state
from urara_chat.api.chat_routes import router as chat_router
from urara_chat.api.errors import BACKEND_UNAVAILABLE, NOT_ALLOWED, install_error_handlers
from urara_chat.api.middleware import RequestIDMiddleware
from urara_chat.backend.client import BackendClient
from urara_chat.backend.errors import BackendRejected
from urara_chat.backend.models import SnapshotContext
from urara_chat.config import Settings
from urara_chat.tools.langchain import to_langchain_tools
from urara_chat.tools.registry import ToolSpec

BASE = "http://backend:8080"
API = f"{BASE}/api/v1"
SID = "snap-1"
CONTEXT = Path(__file__).parent / "fixtures" / "context.json"
FORBIDDEN_TEXT = "not allowed: the acting user lacks project.view"


def settings() -> Settings:
    return Settings(
        backend_base_url=BASE,
        google_api_key="test-key-not-real",  # type: ignore[arg-type]
        backend_timeout_seconds=5.0,
        log_level="info",
        app_addr=":8090",
    )


def app_client() -> TestClient:
    app = FastAPI()
    app.add_middleware(RequestIDMiddleware)
    install_error_handlers(app)
    app.include_router(chat_router)
    app.state.client = BackendClient(settings())
    app.state.settings = settings()
    return TestClient(app, raise_server_exceptions=False)


def assert_error(response: Any, status: int, error: str) -> None:
    assert response.status_code == status, response.text
    body = response.json()
    assert body["error"] == error
    assert body["requestId"]


@respx.mock
def test_a_403_on_create_is_a_403() -> None:
    respx.post(f"{API}/conversations").mock(
        return_value=httpx.Response(403, json={"error": "not allowed"})
    )

    response = app_client().post("/api/chat/conversations", json={"snapshotId": SID})

    assert_error(response, 403, NOT_ALLOWED)


@respx.mock
def test_a_404_on_get_is_still_a_404() -> None:
    respx.get(f"{API}/conversations/c1").mock(
        return_value=httpx.Response(404, json={"error": "conversation not found"})
    )

    assert_error(app_client().get("/api/chat/conversations/c1"), 404, "not found")


@respx.mock
def test_a_500_is_still_a_502() -> None:
    respx.get(f"{API}/conversations/c1").mock(
        return_value=httpx.Response(500, json={"error": "internal error"})
    )

    assert_error(app_client().get("/api/chat/conversations/c1"), 502, BACKEND_UNAVAILABLE)


class Args(BaseModel):
    ids: list[str] = []


@respx.mock
def test_a_403_from_a_tool_ends_the_turn(monkeypatch: pytest.MonkeyPatch) -> None:
    respx.get(f"{API}/conversations/c1").mock(
        return_value=httpx.Response(
            200, json={"id": "c1", "snapshotId": SID, "title": "t", "messages": []}
        )
    )
    respx.post(f"{API}/conversations/c1/messages").mock(
        return_value=httpx.Response(201, json={"ordinal": 0, "role": "user", "content": "q"})
    )

    async def forbidden(**_: Any) -> Any:
        raise BackendRejected(403, FORBIDDEN_TEXT)

    model = FakeChatModel(
        [
            AIMessage(content="", tool_calls=[{"name": "get_tables", "args": {}, "id": "1"}]),
            AIMessage(content="an answer built on the error"),
        ]
    )

    async def pipeline(question: str, snapshot_id: str, history: Any, language: str = "EN") -> Any:
        tools = to_langchain_tools([ToolSpec("get_tables", "d", Args, forbidden)])
        card = CountingContextClient(SnapshotContext.model_validate_json(CONTEXT.read_text()))
        graph = build_graph(model, tools, ContextCardCache(ttl_seconds=1.0), card)  # type: ignore[arg-type]
        return await graph.ainvoke(
            initial_state(snapshot_id, language, [HumanMessage(content=question)])
        )

    monkeypatch.setattr(answering, "answer", pipeline)

    response = app_client().post("/api/chat/conversations/c1/turn", json={"question": "q"})

    assert_error(response, 403, NOT_ALLOWED)
    assert model.call_count == 1
    seen = " ".join(str(m.content) for call in model.calls for m in call)
    assert FORBIDDEN_TEXT not in seen
