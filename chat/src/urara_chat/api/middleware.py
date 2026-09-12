"""One request ID per request, shared with every service the turn touches."""

from __future__ import annotations

import logging
import string
import time
import uuid
from contextvars import ContextVar

from starlette.datastructures import Headers, MutableHeaders
from starlette.requests import Request
from starlette.types import ASGIApp, Message, Receive, Scope, Send

log = logging.getLogger(__name__)

REQUEST_ID_HEADER = "x-request-id"

# Long enough for a UUID, or for one of chi's "hostname/random-1" IDs arriving from the Go side,
# and short enough that a header cannot pad out every log line the request writes.
MAX_REQUEST_ID_LENGTH = 64

# What survives sanitising. The newline is the one that matters: an inbound ID
# reaches the log line below, and one carrying a fake record forges an entry.
_ALLOWED_CHARS = frozenset(string.ascii_letters + string.digits + "-_.:/")

# Paths a probe hits on a timer.
QUIET_PATHS = frozenset({"/healthz", "/readyz"})

_request_id: ContextVar[str] = ContextVar("request_id", default="")


def current_request_id() -> str:
    return _request_id.get()


def sanitise_request_id(raw: str | None) -> str:
    """An inbound ID made safe to log, or a fresh one when there is none."""
    if not raw:
        return uuid.uuid4().hex
    cleaned = "".join(c for c in raw[:MAX_REQUEST_ID_LENGTH] if c in _ALLOWED_CHARS)
    return cleaned or uuid.uuid4().hex


def request_id_of(request: Request) -> str:
    """The request's ID, read from the request itself first."""
    stashed = request.scope.get("state", {}).get("request_id", "")
    return str(stashed) if stashed else current_request_id()


class RequestIDMiddleware:
    """Assigns a request ID, echoes it, and logs one line per request."""

    def __init__(self, app: ASGIApp) -> None:
        self.app = app

    async def __call__(self, scope: Scope, receive: Receive, send: Send) -> None:
        if scope["type"] != "http":
            await self.app(scope, receive, send)
            return

        request_id = sanitise_request_id(Headers(scope=scope).get(REQUEST_ID_HEADER))
        scope.setdefault("state", {})["request_id"] = request_id
        token = _request_id.set(request_id)

        started = time.perf_counter()
        # The status the line reports if nothing is ever sent, which is what happens when the
        # application raises: the 500 is written above this middleware, so it is never seen here.
        status = 500

        async def send_with_id(message: Message) -> None:
            nonlocal status
            if message["type"] == "http.response.start":
                status = message["status"]
                # setdefault, not append: an error handler sets the header on its own response,
                # and appending here would send it twice.
                MutableHeaders(scope=message).setdefault(REQUEST_ID_HEADER, request_id)
            await send(message)

        try:
            await self.app(scope, receive, send_with_id)
        finally:
            _request_id.reset(token)
            log.log(
                _level_for(str(scope.get("path", "")), status),
                "request",
                extra={
                    "method": scope.get("method", ""),
                    "path": scope.get("path", ""),
                    "status": status,
                    "duration_ms": round((time.perf_counter() - started) * 1000, 2),
                    "request_id": request_id,
                },
            )


def _level_for(path: str, status: int) -> int:
    """How loudly to report one request."""
    if status >= 400 or path not in QUIET_PATHS:
        return logging.INFO
    return logging.DEBUG
