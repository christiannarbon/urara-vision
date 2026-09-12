"""One place that turns an exception into a response.

Registered as exception handlers rather than written into each route. A route
that has to remember its own try/except is a route that will one day forget,
and the failure mode is a 500 carrying whatever the exception happened to say.

**No upstream error's own text reaches a client.** A provider error quotes the
request back to you, so it can carry prompt fragments and occasionally the
credential the call was made with; a backend error can name internal hosts. The
obvious objection -- "just return the real error, it would be so much easier to
debug" -- is answered by the request ID: every body carries one, every log line
carries the same one, and the real message is one `grep` away for anyone who is
allowed to read the logs. That is the whole trade, and it is the same posture as
`Server.fail` in the Go backend.
"""

from __future__ import annotations

import logging
from typing import Any

from fastapi import FastAPI
from fastapi.exceptions import RequestValidationError
from pydantic import ValidationError
from starlette.exceptions import HTTPException as StarletteHTTPException
from starlette.requests import Request
from starlette.responses import JSONResponse, Response

from urara_chat.api.locks import RETRY_AFTER_SECONDS, TurnsBusy
from urara_chat.api.middleware import REQUEST_ID_HEADER, request_id_of
from urara_chat.backend.errors import BackendError, BackendNotFound, BackendRejected

log = logging.getLogger(__name__)

# What a caller is told. Each says which side failed and nothing else: that is
# the one piece of information that changes what they should do next.
NOT_FOUND = "not found"
BACKEND_UNAVAILABLE = "the model store is unavailable"
PROVIDER_FAILED = "the language model did not answer"
INTERNAL = "internal error"


class ProviderError(Exception):
    """A call to the language model failed.

    The provider's own exception types are the SDK's, and there are several of
    them per provider; catching them by class here would mean importing each
    SDK's exception module and keeping the list current. The answer path wraps
    instead, so this layer has one thing to map -- and wrapping is also what
    keeps the original message out of the response, since only the wrapper's
    text is ever rendered.
    """


def _body(request: Request, status: int, **fields: Any) -> JSONResponse:
    """An error response carrying the request ID, in the body and the header.

    In the body because a caller reporting a problem can then quote it, and in
    the header because the 500 handler runs outside the middleware that would
    otherwise add it.
    """
    request_id = request_id_of(request)
    return JSONResponse(
        status_code=status,
        content={**fields, "requestId": request_id},
        headers={REQUEST_ID_HEADER: request_id},
    )


async def backend_not_found(request: Request, exc: Exception) -> Response:
    """404 -- the snapshot, table or conversation does not exist."""
    return _body(request, 404, error=NOT_FOUND)


async def backend_failed(request: Request, exc: Exception) -> Response:
    """502 -- the fault is upstream, in the Go API or below it.

    A 502 rather than a 500 because the distinction tells an operator which
    service to go and look at, which a 500 from here actively misdirects.
    """
    detail = exc.message if isinstance(exc, BackendError) else str(exc)
    log.error(
        "backend call failed",
        extra={"request_id": request_id_of(request), "backend_error": detail},
    )
    return _body(request, 502, error=BACKEND_UNAVAILABLE)


async def backend_rejected(request: Request, exc: Exception) -> Response:
    """500 -- the backend refused a request this service built.

    Not a 502, deliberately. The backend did its job; what failed is the request
    assembled here, which makes this an internal error like any other bug and
    puts the investigation in the right place. The backend's own words go to the
    log, never to the caller.
    """
    detail = exc.message if isinstance(exc, BackendError) else str(exc)
    log.error(
        "the backend refused a request built here",
        extra={"request_id": request_id_of(request), "backend_error": detail},
    )
    return _body(request, 500, error=INTERNAL)


async def turns_busy(request: Request, exc: Exception) -> Response:
    """429 -- every slot is taken.

    Refused rather than queued: a queued request holds a connection while the
    reader watches nothing happen, decides it has hung and reloads, which adds
    another one. Retry-After is what makes the refusal actionable.
    """
    retry_after = exc.retry_after if isinstance(exc, TurnsBusy) else RETRY_AFTER_SECONDS
    limit = exc.limit if isinstance(exc, TurnsBusy) else 0
    response = _body(
        request,
        429,
        detail=(f"too many turns in flight; the limit is {limit}. Retry in {retry_after} seconds."),
    )
    response.headers["Retry-After"] = str(retry_after)
    return response


async def provider_failed(request: Request, exc: Exception) -> Response:
    """502 -- the model did not answer, or did not answer in time.

    exc_info goes to the log and never to the body. See the module docstring:
    this is the one that can carry prompt fragments and credentials.
    """
    log.error(
        "language model call failed", extra={"request_id": request_id_of(request)}, exc_info=exc
    )
    return _body(request, 502, error=PROVIDER_FAILED)


# Where FastAPI says a bad field came from. The first element of an error's
# `loc` is one of these for anything parsed out of a request, and is part of the
# field path for a ValidationError raised anywhere else.
_LOCATIONS = frozenset({"body", "query", "path", "header", "cookie"})


def _fields(exc: ValidationError | RequestValidationError) -> list[dict[str, str]]:
    """Which field was wrong, where it lives, and why.

    The location is its own key rather than a prefix on the field name. A
    caller sent `snapshotId` in a body and `snapshot` in a query string, and
    told only the name they cannot tell which one to go and fix -- while
    folding it into the name would leave "body.args.limit" with no way to say
    where the location stops and the field path starts. It is omitted, rather
    than guessed at, for a ValidationError that did not come from parsing a
    request: that one is a bug here, not a bad request.

    Deliberately not pydantic's full error dicts: those embed the input that
    failed, which for a question is the caller's own prose being echoed back
    into a log line and a response body.
    """
    out: list[dict[str, str]] = []
    for err in exc.errors():
        loc = [str(part) for part in err.get("loc", ())]
        location = loc[0] if loc and loc[0] in _LOCATIONS else ""
        path = loc[1:] if location else loc
        field = {"field": ".".join(path) or location or "the request body"}
        if location:
            field["location"] = location
        field["reason"] = str(err.get("msg", ""))
        out.append(field)
    return out


async def validation_failed(request: Request, exc: Exception) -> Response:
    """400 -- the caller's request does not fit the schema.

    400 rather than FastAPI's default 422: the backend answers a malformed
    request with 400, and one service in a pair using a different status for
    the same mistake is a small permanent confusion.
    """
    fields = _fields(exc) if isinstance(exc, ValidationError | RequestValidationError) else []
    return _body(request, 400, error="the request is invalid", fields=fields)


async def http_exception(request: Request, exc: Exception) -> Response:
    """FastAPI's own HTTPException, with the request ID added.

    The shape stays as it was -- `detail`, which every route already raises and
    the frontend already reads. Only the ID is new.
    """
    status = exc.status_code if isinstance(exc, StarletteHTTPException) else 500
    detail = exc.detail if isinstance(exc, StarletteHTTPException) else INTERNAL
    response = _body(request, status, detail=detail)
    if isinstance(exc, StarletteHTTPException) and exc.headers:
        # WWW-Authenticate and friends: dropping them would break the meaning
        # of the status the route chose.
        for key, value in exc.headers.items():
            response.headers[key] = value
    return response


async def unhandled(request: Request, exc: Exception) -> Response:
    """500 -- a bug here. The traceback goes to the log, not to the caller.

    A traceback names file paths, versions and local variables, and is read by
    whoever asked the question rather than by whoever can fix it.

    A pydantic ValidationError arrives here too, and it is worth naming when it
    does: it means a *backend* response did not match this service's models --
    the models have drifted from the API, or the API has changed under them.
    That is a 500 because it is this service's failure to read a response it is
    responsible for parsing, and the fields go to the log because they are the
    whole diagnosis.
    """
    if isinstance(exc, ValidationError):
        log.error(
            "a backend response did not match this service's models",
            extra={"request_id": request_id_of(request), "fields": _fields(exc)},
            exc_info=exc,
        )
    else:
        log.error("unhandled exception", extra={"request_id": request_id_of(request)}, exc_info=exc)
    return _body(request, 500, error=INTERNAL)


def install_error_handlers(app: FastAPI) -> None:
    """Map every exception this service can raise onto a response.

    Order does not matter: Starlette walks an exception's MRO and takes the most
    specific handler registered, so BackendNotFound and BackendRejected are both
    reachable despite subclassing BackendError. Only three of the four map to an
    upstream fault -- a refused request is this service's own bug.
    """
    app.add_exception_handler(BackendError, backend_failed)
    app.add_exception_handler(BackendNotFound, backend_not_found)
    app.add_exception_handler(BackendRejected, backend_rejected)
    app.add_exception_handler(TurnsBusy, turns_busy)
    app.add_exception_handler(ProviderError, provider_failed)
    # asyncio.TimeoutError is this class in 3.11 and later, so the deadline on a
    # turn and a provider that never answers arrive at the same place.
    app.add_exception_handler(TimeoutError, provider_failed)
    # RequestValidationError only. A bare pydantic ValidationError is *not* a
    # caller error: it is what the backend client raises when the Go API answers
    # with something these models do not accept, and answering 400 tells a
    # caller their perfect request was malformed and names a field they never
    # sent. It falls to `unhandled`, which reports it as the internal failure it
    # is and logs the fields.
    app.add_exception_handler(RequestValidationError, validation_failed)
    app.add_exception_handler(StarletteHTTPException, http_exception)
    app.add_exception_handler(Exception, unhandled)
