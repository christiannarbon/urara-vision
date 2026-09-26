"""Fixtures for the tests that meet a real backend."""

import os
import re
import uuid
from collections.abc import AsyncIterator, Callable, Iterator
from pathlib import Path

import httpx
import pytest

from urara_chat.backend.client import BackendClient
from urara_chat.config import Settings

# The demo set these tests assert against.
DEMO_DIR = Path(__file__).resolve().parents[3] / "docs" / "demo"
DEMO_SET = DEMO_DIR / "jaffle-shop-ddd"

# A second, unrelated set, so one test can prove the snapshot binding holds:
# asking Jaffle Shop about a table that lives only here must come back empty.
OTHER_DEMO_SET = DEMO_DIR / "eshop-ddd"


@pytest.fixture(scope="session")
def backend_url() -> str:
    url = os.getenv("CHAT_TEST_BACKEND_URL")
    if not url:
        pytest.skip("set CHAT_TEST_BACKEND_URL to run this test (see: make test-chat-integration)")
    return url.rstrip("/")


@pytest.fixture(scope="session")
def api_token() -> str:
    return os.getenv("CHAT_TEST_API_TOKEN", "")


@pytest.fixture(scope="session")
def auth_headers(api_token: str) -> dict[str, str]:
    return {"Authorization": f"Bearer {api_token}"} if api_token else {}


def _demo_files(demo_set: Path) -> list[dict[str, str]]:
    """The set's documents, with a fresh version so a used database cannot 409."""
    version = f'version = "chat-it-{uuid.uuid4().hex[:12]}"'
    files = []
    for p in sorted(demo_set.rglob("*")):
        if p.suffix not in {".md", ".toml"}:
            continue
        content = p.read_text()
        if p.name == "projectmeta.toml":
            content = re.sub(r'^version\s*=\s*".*"$', version, content, count=1, flags=re.M)
        files.append({"path": str(p.relative_to(demo_set)), "content": content})
    return files


def _ingest(
    demo_set: Path, name: str, backend_url: str, auth_headers: dict[str, str]
) -> Iterator[str]:
    """Ingest one demo set, yield its snapshot ID, and delete it afterwards."""
    if not demo_set.is_dir():
        raise RuntimeError(
            f"demo set not found at {demo_set}. Inside the test container the "
            "repository's docs/ must be mounted at /docs."
        )

    body = {"name": name, "sourceLabel": "phase02", "files": _demo_files(demo_set)}

    with httpx.Client(base_url=backend_url, headers=auth_headers, timeout=60.0) as http:
        response = http.post("/api/v1/ingest", json=body)
        response.raise_for_status()
        sid: str = response.json()["snapshot"]["id"]

        try:
            yield sid
        finally:
            # Deleted whatever happened above, so a failing test does not leave a snapshot behind
            # for the next run to trip over -- and the outcome is checked, because a silent
            # failure here is exactly the leak the teardown exists to prevent.
            deleted = http.delete(f"/api/v1/snapshots/{sid}")
            assert deleted.status_code in (204, 404), (
                f"failed to delete test snapshot {sid}: {deleted.status_code} {deleted.text[:200]}"
            )


@pytest.fixture(scope="session")
def snapshot_id(backend_url: str, auth_headers: dict[str, str]) -> Iterator[str]:
    yield from _ingest(DEMO_SET, "chat-integration", backend_url, auth_headers)


@pytest.fixture(scope="session")
def other_snapshot_id(backend_url: str, auth_headers: dict[str, str]) -> Iterator[str]:
    yield from _ingest(OTHER_DEMO_SET, "chat-integration-other", backend_url, auth_headers)


@pytest.fixture(scope="session")
def llm_settings() -> Settings:
    """Settings for a real model call, or a skip."""
    project = os.getenv("VERTEX_PROJECT")
    if not project:
        pytest.skip(
            "set VERTEX_PROJECT (and run `gcloud auth application-default login`) "
            "to run this test; it calls a real model and costs money "
            "(see chat/README.md)"
        )
    return Settings(
        llm_provider="vertex",
        vertex_project=project,
        vertex_location=os.getenv("VERTEX_LOCATION", "us-central1"),
        llm_max_output_tokens=512,
        llm_timeout_seconds=30.0,
    )


@pytest.fixture(scope="session")
def other_demo_set() -> Path:
    return OTHER_DEMO_SET


@pytest.fixture(scope="session")
def chat_url() -> str:
    """Where the chat service is, for the tests that drive its HTTP surface."""
    url = os.getenv("CHAT_TEST_CHAT_URL")
    if not url:
        pytest.skip("set CHAT_TEST_CHAT_URL to run this test (see: make test-chat-integration)")
    return url.rstrip("/")


@pytest.fixture(scope="session")
def user_id(backend_url: str) -> str:
    """The bootstrap admin's ID, logged in over HTTP as a browser would."""
    username = os.getenv("CHAT_TEST_ADMIN_USERNAME", "admin")
    password = os.getenv("CHAT_TEST_ADMIN_PASSWORD", "relviz-dev-admin-password")
    with httpx.Client(base_url=backend_url, timeout=30.0) as http:
        login = http.post(
            "/api/v1/auth/login",
            json={"username": username, "password": password},
            headers={"X-Requested-With": "urara"},
        )
        login.raise_for_status()
        me = http.get("/api/v1/auth/me")
        me.raise_for_status()
        uid: str = me.json()["user"]["id"]
    return uid


@pytest.fixture
async def chat(chat_url: str, user_id: str) -> AsyncIterator[httpx.AsyncClient]:
    """An HTTP client for the chat service, carrying the identity nginx would set."""
    async with httpx.AsyncClient(
        base_url=chat_url, timeout=180.0, headers={"X-User-Id": user_id}
    ) as client:
        yield client


@pytest.fixture
def ingest(backend_url: str, auth_headers: dict[str, str]) -> Iterator[Callable[[Path, str], str]]:
    """Ingest a demo set mid-test and clean it up afterwards."""
    created: list[str] = []

    def go(demo_set: Path, name: str) -> str:
        files = _demo_files(demo_set)
        with httpx.Client(base_url=backend_url, headers=auth_headers, timeout=60.0) as http:
            response = http.post(
                "/api/v1/ingest",
                json={"name": name, "sourceLabel": "phase05", "files": files},
            )
            response.raise_for_status()
            sid: str = response.json()["snapshot"]["id"]
        created.append(sid)
        return sid

    yield go

    with httpx.Client(base_url=backend_url, headers=auth_headers, timeout=60.0) as http:
        for sid in created:
            http.delete(f"/api/v1/snapshots/{sid}")


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
