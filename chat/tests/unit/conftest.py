"""Keeps the unit suite hermetic."""

from typing import Any

import httpx
import pytest
from fakes import USER_HEADERS
from fastapi.testclient import TestClient

_LLM_VARS = (
    "LLM_PROVIDER",
    "LLM_MODEL",
    "LLM_TEMPERATURE",
    "LLM_MAX_OUTPUT_TOKENS",
    "LLM_TIMEOUT_SECONDS",
    "GOOGLE_API_KEY",
    "VERTEX_PROJECT",
    "VERTEX_LOCATION",
)


@pytest.fixture(autouse=True)
def _isolate_llm_environment(monkeypatch: pytest.MonkeyPatch) -> None:
    for var in _LLM_VARS:
        monkeypatch.delenv(var, raising=False)


@pytest.fixture(autouse=True)
def _signed_in_by_default(monkeypatch: pytest.MonkeyPatch) -> None:
    """Clients of the app send X-User-Id, as nginx would. Pop it to test its absence."""
    test_init = TestClient.__init__
    async_init = httpx.AsyncClient.__init__

    def with_user(init: Any) -> Any:
        def wrapped(self: Any, *args: Any, **kwargs: Any) -> None:
            init(self, *args, **kwargs)
            if isinstance(self, TestClient) or isinstance(
                kwargs.get("transport"), httpx.ASGITransport
            ):
                self.headers.update(USER_HEADERS)

        return wrapped

    monkeypatch.setattr(TestClient, "__init__", with_user(test_init))
    monkeypatch.setattr(httpx.AsyncClient, "__init__", with_user(async_init))
