"""One place that turns an exception into a response."""

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

# What a caller is told. Each says which side failed and nothing else: that is the one piece of
# information that changes what they should do next.
NOT_FOUND = "not found"
BACKEND_UNAVAILABLE = "the model store is unavailable"
PROVIDER_FAILED = "the language model did not answer"
INTERNAL = "internal error"


class ProviderError(Exception):
    """A call to the language model failed."""


def _body(request: Request, status: int, **fields: Any) -> JSONResponse:
    """An error response carrying the request ID, in the body and the header."""
    request_id = request_id_of(request)
    return JSONResponse(
        status_code=status,
        content={**fields, "requestId": request_id},
        headers={REQUEST_ID_HEADER: request_id},
    )


async def backend_not_found(request: Request, exc: Exception) -> Response:
    return _body(request, 404, error=NOT_FOUND)


async def backend_failed(request: Request, exc: Exception) -> Response:
    """502 -- the fault is upstream, in the Go API or below it."""
    detail = exc.message if isinstance(exc, BackendError) else str(exc)
    log.error(
        "backend call failed",
        extra={"request_id": request_id_of(request), "backend_error": detail},
    )
    return _body(request, 502, error=BACKEND_UNAVAILABLE)


async def backend_rejected(request: Request, exc: Exception) -> Response:
    """500 -- the backend refused a request this service built."""
    detail = exc.message if isinstance(exc, BackendError) else str(exc)
    log.error(
        "the backend refused a request built here",
        extra={"request_id": request_id_of(request), "backend_error": detail},
    )
    return _body(request, 500, error=INTERNAL)


async def turns_busy(request: Request, exc: Exception) -> Response:
    """429 -- every slot is taken."""
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
    """502 -- the model did not answer, or did not answer in time."""
    log.error(
        "language model call failed", extra={"request_id": request_id_of(request)}, exc_info=exc
    )
    return _body(request, 502, error=PROVIDER_FAILED)


# Where FastAPI says a bad field came from: the first element of an error's
# `loc` for anything parsed out of a request.
_LOCATIONS = frozenset({"body", "query", "path", "header", "cookie"})


def _fields(exc: ValidationError | RequestValidationError) -> list[dict[str, str]]:
    """Which field was wrong, where it lives, and why."""
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
    """400 -- the caller's request does not fit the schema."""
    fields = _fields(exc) if isinstance(exc, ValidationError | RequestValidationError) else []
    return _body(request, 400, error="the request is invalid", fields=fields)


async def http_exception(request: Request, exc: Exception) -> Response:
    """FastAPI's own HTTPException, with the request ID added."""
    status = exc.status_code if isinstance(exc, StarletteHTTPException) else 500
    detail = exc.detail if isinstance(exc, StarletteHTTPException) else INTERNAL
    response = _body(request, status, detail=detail)
    if isinstance(exc, StarletteHTTPException) and exc.headers:
        # WWW-Authenticate and friends: dropping them would break the meaning of the status the
        # route chose.
        for key, value in exc.headers.items():
            response.headers[key] = value
    return response


async def unhandled(request: Request, exc: Exception) -> Response:
    """500 -- a bug here."""
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
    """Map every exception this service can raise onto a response."""
    app.add_exception_handler(BackendError, backend_failed)
    app.add_exception_handler(BackendNotFound, backend_not_found)
    app.add_exception_handler(BackendRejected, backend_rejected)
    app.add_exception_handler(TurnsBusy, turns_busy)
    app.add_exception_handler(ProviderError, provider_failed)
    # asyncio.TimeoutError is this class in 3.11 and later, so the deadline on a turn and a
    # provider that never answers arrive at the same place.
    app.add_exception_handler(TimeoutError, provider_failed)
    # RequestValidationError only. A bare pydantic ValidationError is the backend
    # client rejecting the Go API's response, not a caller error -- it falls to
    # `unhandled`.
    app.add_exception_handler(RequestValidationError, validation_failed)
    app.add_exception_handler(StarletteHTTPException, http_exception)
    app.add_exception_handler(Exception, unhandled)
