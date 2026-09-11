"""One request ID per request, shared with every service the turn touches.

Three services sit between a question and an answer -- nginx, this one, and the
Go backend -- and without an ID that spans them, tracing a bad answer back is
guesswork. The Go side already does this with chi's `middleware.RequestID`, and
logs it in `Server.fail`; this wears the same header so the two join up.

The ID lives in a ContextVar rather than being threaded through every
signature. The places that want it -- the backend client building outbound
headers, the pipeline's per-turn cost line, the error handlers -- are nowhere
near the request object, and passing it down to them would mean a parameter on
functions that otherwise have no idea HTTP is involved.
"""

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

# Long enough for a UUID, or for one of chi's "hostname/random-1" IDs arriving
# from the Go side, and short enough that a header cannot pad out every log line
# the request writes.
MAX_REQUEST_ID_LENGTH = 64

# What survives sanitising. The newline is the one that matters: an inbound ID
# reaches the log line below, and a value carrying "\n{"level":"error",..." puts
# a forged entry in the log stream that a reader cannot tell from a real one.
_ALLOWED_CHARS = frozenset(string.ascii_letters + string.digits + "-_.:/")

_request_id: ContextVar[str] = ContextVar("request_id", default="")


def current_request_id() -> str:
    """The ID of the request being served, or "" outside one.

    Empty rather than raising: a tool called from a test or a script is not in
    a request, and that is not an error worth failing a log line over.
    """
    return _request_id.get()


def sanitise_request_id(raw: str | None) -> str:
    """An inbound ID made safe to log, or a fresh one when there is none.

    The value is attacker-controlled -- it arrives in a header and ends up in
    log lines and in response bodies -- so it is cut to length and stripped to a
    known character set rather than trusted. An ID that survives nothing, all
    newlines say, is replaced outright: the request still needs an ID, and a
    caller does not get to make it empty.
    """
    if not raw:
        return uuid.uuid4().hex
    cleaned = "".join(c for c in raw[:MAX_REQUEST_ID_LENGTH] if c in _ALLOWED_CHARS)
    return cleaned or uuid.uuid4().hex


def request_id_of(request: Request) -> str:
    """The request's ID, read from the request itself first.

    The ContextVar is the usual source, but the handler for an unhandled
    exception runs in Starlette's ServerErrorMiddleware, which sits *outside*
    this middleware -- by the time it is called the ContextVar has been reset.
    The ID is stashed on the scope for exactly that case.
    """
    stashed = request.scope.get("state", {}).get("request_id", "")
    return str(stashed) if stashed else current_request_id()


class RequestIDMiddleware:
    """Assigns a request ID, echoes it, and logs one line per request.

    A raw ASGI middleware rather than BaseHTTPMiddleware: the latter runs the
    rest of the application in a task of its own, and whether a ContextVar set
    before `call_next` is visible inside the handler has moved between Starlette
    versions. This sets it in the same context the handler runs in, which is not
    a detail worth rediscovering later from a log line with no ID in it.
    """

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
        # The status the line reports if nothing is ever sent, which is what
        # happens when the application raises: the 500 is written above this
        # middleware, so it is never seen here.
        status = 500

        async def send_with_id(message: Message) -> None:
            nonlocal status
            if message["type"] == "http.response.start":
                status = message["status"]
                # setdefault, not append: an error handler sets the header on
                # its own response, and appending here would send it twice.
                MutableHeaders(scope=message).setdefault(REQUEST_ID_HEADER, request_id)
            await send(message)

        try:
            await self.app(scope, receive, send_with_id)
        finally:
            _request_id.reset(token)
            log.info(
                "request",
                extra={
                    "method": scope.get("method", ""),
                    "path": scope.get("path", ""),
                    "status": status,
                    "duration_ms": round((time.perf_counter() - started) * 1000, 2),
                    "request_id": request_id,
                },
            )
