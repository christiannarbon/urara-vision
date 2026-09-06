"""Fixtures for the tests that meet a real backend.

Two conventions are borrowed wholesale from the Go suites, because a reader who
knows one should recognise the other.

**Skip unless told where to connect.** The suite is inert without
CHAT_TEST_BACKEND_URL, so `uv run pytest` stays fast and needs nothing running.
CI sets it, and CI treats a skip as a failure: a suite that quietly stops
testing anything is worse than one that goes red.

**Isolation is by snapshot.** The suite ingests its own copy of the demo set and
deletes it afterwards, rather than reading `latest`. Asserting against `latest`
would mean a developer's own ingest, or another suite's, silently changes what
is being tested.
"""

import os
from collections.abc import AsyncIterator, Iterator
from pathlib import Path

import httpx
import pytest
from pydantic import SecretStr

from urara_chat.backend.client import BackendClient
from urara_chat.config import Settings

# The demo set these tests assert against. Located relative to this file rather
# than the working directory, and by the same arithmetic in both places it runs:
# from <root>/chat/tests/integration it is <root>/docs/demo/..., and from
# /src/tests/integration inside the test container it is /docs/demo/..., which
# is where the Makefile mounts it.
DEMO_SET = Path(__file__).resolve().parents[3] / "docs" / "demo" / "jaffle-shop-ddd"


@pytest.fixture(scope="session")
def backend_url() -> str:
    url = os.getenv("CHAT_TEST_BACKEND_URL")
    if not url:
        pytest.skip("set CHAT_TEST_BACKEND_URL to run this test (see: make test-chat-integration)")
    return url.rstrip("/")


@pytest.fixture(scope="session")
def api_token() -> str:
    """Empty is valid: it is the backend's own unauthenticated mode."""
    return os.getenv("CHAT_TEST_API_TOKEN", "")


@pytest.fixture(scope="session")
def auth_headers(api_token: str) -> dict[str, str]:
    return {"Authorization": f"Bearer {api_token}"} if api_token else {}


@pytest.fixture(scope="session")
def snapshot_id(backend_url: str, auth_headers: dict[str, str]) -> Iterator[str]:
    """Ingest the demo set, yield its snapshot ID, and delete it afterwards.

    Synchronous on purpose: it runs once per session, and a sync fixture avoids
    having to pin an event loop scope for something that is not under test.
    """
    if not DEMO_SET.is_dir():
        raise RuntimeError(
            f"demo set not found at {DEMO_SET}. Inside the test container the "
            "repository's docs/ must be mounted at /docs."
        )

    files = [
        {"path": str(p.relative_to(DEMO_SET)), "content": p.read_text()}
        for p in sorted(DEMO_SET.rglob("*"))
        if p.suffix in {".md", ".toml"}
    ]
    body = {"name": "chat-integration", "sourceLabel": "phase02", "files": files}

    with httpx.Client(base_url=backend_url, headers=auth_headers, timeout=60.0) as http:
        response = http.post("/api/v1/ingest", json=body)
        response.raise_for_status()
        sid: str = response.json()["snapshot"]["id"]

        try:
            yield sid
        finally:
            # Deleted whatever happened above, so a failing test does not leave
            # a snapshot behind for the next run to trip over -- and the outcome
            # is checked, because a silent failure here is exactly the leak the
            # teardown exists to prevent.
            deleted = http.delete(f"/api/v1/snapshots/{sid}")
            assert deleted.status_code in (204, 404), (
                f"failed to delete test snapshot {sid}: {deleted.status_code} {deleted.text[:200]}"
            )


@pytest.fixture(scope="session")
def llm_settings() -> Settings:
    """Settings for a real Studio call, or a skip.

    Separate from the backend fixtures on purpose: these tests spend money, so
    they are gated on their own credential and never run because a backend
    happened to be reachable.

    The output cap is small but not tiny, and 512 is not arbitrary. Gemini 2.5
    spends output tokens on reasoning before it emits anything, so at 64 the
    reply comes back with finish_reason=MAX_TOKENS, 55 reasoning tokens and no
    tool call at all -- test_binds_tools fails for a reason that looks nothing
    like a token limit. Cost is dominated by the ~1k input tokens the tool
    schemas take anyway, so the cap buys little and costs a confusing failure.
    """
    key = os.getenv("GOOGLE_API_KEY")
    if not key:
        pytest.skip(
            "set GOOGLE_API_KEY to run this test; it calls a real model and "
            "costs money (see chat/README.md)"
        )
    return Settings(
        llm_provider="gemini-studio",
        google_api_key=SecretStr(key),
        llm_max_output_tokens=512,
        llm_timeout_seconds=30.0,
    )


@pytest.fixture
async def client(backend_url: str, api_token: str) -> AsyncIterator[BackendClient]:
    settings = Settings(
        backend_base_url=backend_url,
        backend_api_token=api_token,
        # Required since 03.2; this suite never reaches an LLM.
        google_api_key="test-key-not-real",  # type: ignore[arg-type]
        backend_timeout_seconds=30.0,
        log_level="info",
        app_addr=":8090",
    )
    backend = BackendClient(settings)
    yield backend
    await backend.aclose()
