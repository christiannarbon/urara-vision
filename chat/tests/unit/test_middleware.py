"""Request IDs and error mapping: what every response carries, and what it does"""

import logging
from typing import Any

import httpx
import pytest
import respx
from fastapi import FastAPI
from fastapi.testclient import TestClient
from pydantic import BaseModel, ValidationError
from starlette.requests import Request
from starlette.responses import JSONResponse, Response

from urara_chat.api.errors import (
    BACKEND_UNAVAILABLE,
    PROVIDER_FAILED,
    ProviderError,
    install_error_handlers,
)
from urara_chat.api.middleware import (
    MAX_REQUEST_ID_LENGTH,
    RequestIDMiddleware,
    current_request_id,
    sanitise_request_id,
)
from urara_chat.backend.client import BackendClient
from urara_chat.backend.errors import BackendError, BackendNotFound, BackendRejected
from urara_chat.config import Settings

BASE = "http://backend:8080"

# The text a caller must never see, used as the message on every exception the handlers below are
# asked to render.
SECRET = "prompt fragment and AIza-shaped-credential"


def settings() -> Settings:
    return Settings(
        backend_base_url=BASE,
        google_api_key="test-key-not-real",  # type: ignore[arg-type]
        log_level="info",
        app_addr=":8090",
    )


def build_app(raises: Exception | None = None) -> FastAPI:
    """An app with the middleware and the handlers, and routes that misbehave."""
    app = FastAPI()
    app.add_middleware(RequestIDMiddleware)
    install_error_handlers(app)

    class Body(BaseModel):
        question: str

    @app.get("/ok")
    async def ok() -> dict[str, str]:
        # Read from inside the handler: this is what every log line and every outbound call
        # depends on.
        return {"requestId": current_request_id()}

    @app.get("/boom")
    async def boom() -> dict[str, str]:
        if raises is not None:
            raise raises
        return {}

    @app.post("/echo")
    async def echo(body: Body) -> dict[str, str]:
        return {"question": body.question}

    @app.get("/backend")
    async def backend(request: Request) -> Any:
        client: BackendClient = request.app.state.client
        return await client.list_domains("snap-1")

    return app


class TestTheHeader:
    def test_a_generated_id_comes_back_when_none_was_sent(self) -> None:
        response = TestClient(build_app()).get("/ok")

        assert response.status_code == 200
        assert response.headers["x-request-id"]
        # The same value the handler saw: a header that disagrees with the log lines is worse than
        # no header.
        assert response.json()["requestId"] == response.headers["x-request-id"]

    def test_an_inbound_id_is_echoed_unchanged(self) -> None:
        response = TestClient(build_app()).get("/ok", headers={"X-Request-ID": "my-trace-1"})

        assert response.headers["x-request-id"] == "my-trace-1"
        assert response.json()["requestId"] == "my-trace-1"

    def test_a_long_inbound_id_is_truncated(self) -> None:
        response = TestClient(build_app()).get("/ok", headers={"X-Request-ID": "a" * 300})

        assert len(response.headers["x-request-id"]) == MAX_REQUEST_ID_LENGTH

    def test_the_id_is_sent_once(self) -> None:
        """The middleware sets the header and so does every error handler."""
        app = build_app(raises=BackendNotFound(404, SECRET))
        response = TestClient(app).get("/boom", headers={"X-Request-ID": "trace-dup"})

        assert response.headers.get_list("x-request-id") == ["trace-dup"]


class TestSanitising:
    """An inbound ID is attacker-controlled and ends up in log lines."""

    def test_a_newline_is_stripped(self) -> None:
        """The one that matters: a newline lets a caller forge a log entry that"""
        cleaned = sanitise_request_id('abc\n{"level":"error","msg":"forged"}')

        assert "\n" not in cleaned
        assert cleaned.startswith("abc")
        # Nothing that could be read as the start of a JSON log record survives.
        assert "{" not in cleaned and '"' not in cleaned

    def test_control_characters_and_spaces_go(self) -> None:
        assert sanitise_request_id("a\r\n\tb c\x00d") == "abcd"

    def test_a_long_id_is_cut(self) -> None:
        assert len(sanitise_request_id("b" * 300)) == MAX_REQUEST_ID_LENGTH

    def test_an_id_that_survives_nothing_is_replaced(self) -> None:
        """Empty is not an option: the request still needs an ID, and a caller"""
        assert sanitise_request_id("\n\n\n")
        assert sanitise_request_id("")
        assert sanitise_request_id(None)

    def test_two_requests_do_not_share_an_id(self) -> None:
        client = TestClient(build_app())
        first = client.get("/ok").headers["x-request-id"]
        second = client.get("/ok").headers["x-request-id"]

        assert first != second


class TestTheIdReachesTheBackend:
    @respx.mock
    def test_the_outbound_call_carries_the_header(self) -> None:
        """One ID across both services, which is the whole point of the header."""
        route = respx.get(f"{BASE}/api/v1/snapshots/snap-1/domains").mock(
            return_value=httpx.Response(200, json={"domains": []})
        )
        app = build_app()
        app.state.client = BackendClient(settings())

        with TestClient(app) as client:
            response = client.get("/backend", headers={"X-Request-ID": "trace-out"})

        assert response.status_code == 200
        assert route.calls.last.request.headers["x-request-id"] == "trace-out"

    @respx.mock
    def test_the_readiness_check_carries_it_too(self) -> None:
        """/readyz is the call you most want to trace: when it says the backend"""
        route = respx.get(f"{BASE}/readyz").mock(return_value=httpx.Response(200))
        app = build_app()
        app.state.client = BackendClient(settings())

        @app.get("/probe")
        async def probe() -> dict[str, bool]:
            client: BackendClient = app.state.client
            return {"ready": await client.health()}

        with TestClient(app) as client:
            response = client.get("/probe", headers={"X-Request-ID": "trace-probe"})

        assert response.json() == {"ready": True}
        assert route.calls.last.request.headers["x-request-id"] == "trace-probe"

    @respx.mock
    async def test_no_header_is_sent_outside_a_request(self) -> None:
        """A tool driven from a script or a test is not in a request, and an"""
        route = respx.get(f"{BASE}/api/v1/snapshots/snap-1/domains").mock(
            return_value=httpx.Response(200, json={"domains": []})
        )
        client = BackendClient(settings())
        await client.list_domains("snap-1")
        await client.aclose()

        assert "x-request-id" not in route.calls.last.request.headers


class TestErrorMapping:
    def response_for(self, exc: Exception) -> httpx.Response:
        client = TestClient(build_app(raises=exc), raise_server_exceptions=False)
        return client.get("/boom", headers={"X-Request-ID": "trace-err"})

    def test_backend_not_found_is_404(self) -> None:
        response = self.response_for(BackendNotFound(404, SECRET))

        assert response.status_code == 404
        assert response.json() == {"error": "not found", "requestId": "trace-err"}

    def test_a_backend_failure_is_502_and_generic(self) -> None:
        response = self.response_for(BackendError(500, SECRET))

        assert response.status_code == 502
        body = response.json()
        assert body["error"] == BACKEND_UNAVAILABLE
        assert SECRET not in response.text

    def test_a_refused_request_is_500_not_502(self) -> None:
        """A 4xx from the backend is this service having built a bad request."""
        response = self.response_for(BackendRejected(400, SECRET))

        assert response.status_code == 500
        assert response.json()["error"] == "internal error"
        assert SECRET not in response.text

    def test_a_broken_backend_response_is_500_not_the_callers_fault(self) -> None:
        """A bare ValidationError is what the backend client raises when the Go"""

        class Conversation(BaseModel):
            id: str

        try:
            Conversation.model_validate({"title": "no id here"})
        except ValidationError as exc:
            broken = exc

        response = self.response_for(broken)

        assert response.status_code == 500
        assert response.json() == {"error": "internal error", "requestId": "trace-err"}
        # The caller is told nothing about a field they did not send.
        assert "fields" not in response.json()
        assert "id" not in response.json()["error"]

    def test_the_broken_response_fields_reach_the_log(
        self, caplog: pytest.LogCaptureFixture
    ) -> None:
        """Swallowed from the response, not from the diagnosis: the field list"""

        class Conversation(BaseModel):
            id: str

        try:
            Conversation.model_validate({"title": "no id here"})
        except ValidationError as exc:
            broken = exc

        with caplog.at_level(logging.ERROR, logger="urara_chat.api.errors"):
            self.response_for(broken)

        record = next(
            r for r in caplog.records if "did not match this service's models" in str(r.msg)
        )
        assert record.fields == [{"field": "id", "reason": "Field required"}]  # type: ignore[attr-defined]
        assert record.request_id == "trace-err"  # type: ignore[attr-defined]

    def test_a_provider_failure_is_502_and_says_nothing(self) -> None:
        """The reason this mapping exists: a provider error quotes the request"""
        response = self.response_for(ProviderError(SECRET))

        assert response.status_code == 502
        assert response.json()["error"] == PROVIDER_FAILED
        assert SECRET not in response.text

    def test_a_timeout_is_a_provider_failure(self) -> None:
        response = self.response_for(TimeoutError())

        assert response.status_code == 502
        assert response.json()["error"] == PROVIDER_FAILED

    def test_an_unhandled_exception_is_500_without_a_traceback(self) -> None:
        response = self.response_for(RuntimeError(SECRET))

        assert response.status_code == 500
        assert response.json() == {"error": "internal error", "requestId": "trace-err"}
        assert SECRET not in response.text
        assert "Traceback" not in response.text
        # The 500 is written above this middleware, so the header has to come from the handler
        # itself.
        assert response.headers["x-request-id"] == "trace-err"

    def test_a_validation_error_is_400_naming_the_field(self) -> None:
        response = TestClient(build_app()).post("/echo", json={"quesiton": "typo"})

        assert response.status_code == 400
        body = response.json()
        assert body["requestId"]
        assert body["fields"][0]["field"] == "question"
        assert body["fields"][0]["location"] == "body"
        assert body["fields"][0]["reason"]

    def test_every_error_body_carries_the_id(self) -> None:
        for exc in (
            BackendNotFound(404, SECRET),
            BackendError(502, SECRET),
            ProviderError(SECRET),
            RuntimeError(SECRET),
        ):
            assert self.response_for(exc).json()["requestId"] == "trace-err"


class TestProbesAreQuiet:
    """At the periods the cluster sets, the probes are nine lines a minute per"""

    def lines(self, caplog: pytest.LogCaptureFixture, level: int) -> list[str]:
        return [r.path for r in caplog.records if r.msg == "request" and r.levelno == level]  # type: ignore[attr-defined]

    def test_a_probe_does_not_log_at_info(self, caplog: pytest.LogCaptureFixture) -> None:
        app = build_app()

        @app.get("/healthz")
        async def healthz() -> dict[str, str]:
            return {"status": "ok"}

        with caplog.at_level(logging.DEBUG, logger="urara_chat.api.middleware"):
            TestClient(app).get("/healthz")

        assert self.lines(caplog, logging.INFO) == []
        assert self.lines(caplog, logging.DEBUG) == ["/healthz"]

    def test_an_ordinary_request_still_logs_at_info(self, caplog: pytest.LogCaptureFixture) -> None:
        with caplog.at_level(logging.DEBUG, logger="urara_chat.api.middleware"):
            TestClient(build_app()).get("/ok")

        assert self.lines(caplog, logging.INFO) == ["/ok"]

    def test_a_failing_probe_is_loud(self, caplog: pytest.LogCaptureFixture) -> None:
        """The one probe line anybody wants: /readyz starting to answer 503."""
        app = build_app()

        @app.get("/readyz")
        async def readyz() -> Response:
            return JSONResponse({"status": "unready"}, status_code=503)

        with caplog.at_level(logging.DEBUG, logger="urara_chat.api.middleware"):
            TestClient(app).get("/readyz")

        assert self.lines(caplog, logging.INFO) == ["/readyz"]

    def test_a_probe_still_carries_the_request_id(self) -> None:
        app = build_app()

        @app.get("/healthz")
        async def healthz() -> dict[str, str]:
            return {"status": "ok"}

        assert TestClient(app).get("/healthz").headers["x-request-id"]


class TestTheLogLine:
    def test_one_line_per_request_carrying_the_id(self, caplog: pytest.LogCaptureFixture) -> None:
        with caplog.at_level(logging.INFO, logger="urara_chat.api.middleware"):
            TestClient(build_app()).get("/ok", headers={"X-Request-ID": "trace-log"})

        lines = [r for r in caplog.records if r.msg == "request"]
        assert len(lines) == 1
        record = lines[0]
        assert record.request_id == "trace-log"  # type: ignore[attr-defined]
        assert record.method == "GET"  # type: ignore[attr-defined]
        assert record.path == "/ok"  # type: ignore[attr-defined]
        assert record.status == 200  # type: ignore[attr-defined]
        assert record.duration_ms >= 0  # type: ignore[attr-defined]

    def test_the_line_is_written_even_when_the_request_fails(
        self, caplog: pytest.LogCaptureFixture
    ) -> None:
        app = build_app(raises=RuntimeError("boom"))
        with caplog.at_level(logging.INFO, logger="urara_chat.api.middleware"):
            TestClient(app, raise_server_exceptions=False).get("/boom")

        lines = [r for r in caplog.records if r.msg == "request"]
        assert len(lines) == 1
        assert lines[0].status == 500  # type: ignore[attr-defined]
