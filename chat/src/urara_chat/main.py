"""The FastAPI application."""

from __future__ import annotations

import logging
import sys
from collections.abc import AsyncIterator, Awaitable, Callable
from contextlib import asynccontextmanager

from fastapi import FastAPI, Request, Response
from fastapi.responses import JSONResponse
from pydantic import ValidationError

from urara_chat.agent.context_card import ContextCardCache
from urara_chat.agent.pipeline import Pipeline, configure_pipeline
from urara_chat.api.chat_routes import router as chat_router
from urara_chat.api.errors import install_error_handlers
from urara_chat.api.middleware import REQUEST_ID_HEADER, RequestIDMiddleware, request_id_of
from urara_chat.api.routes import router
from urara_chat.backend.client import BackendClient
from urara_chat.config import ConfigurationError, configure_logging, get_settings
from urara_chat.llm.factory import build_chat_model, describe_model
from urara_chat.tools.langchain import to_langchain_tools
from urara_chat.tools.registry import build_tools


def _reasons(exc: ValidationError) -> list[str]:
    return [str(err.get("msg", "")).removeprefix("Value error, ") for err in exc.errors()]


@asynccontextmanager
async def lifespan(app: FastAPI) -> AsyncIterator[None]:
    try:
        settings = get_settings()
    except ValidationError as exc:
        # Logged as one line, then re-raised as a ConfigurationError carrying only the reasons.
        reasons = "; ".join(_reasons(exc))
        logging.basicConfig(stream=sys.stdout, level=logging.ERROR, format="%(message)s")
        logging.getLogger("urara_chat").error(
            "configuration is invalid, refusing to start: %s", reasons
        )
        raise ConfigurationError(reasons) from None

    configure_logging(settings)

    log = logging.getLogger("urara_chat")
    log.info("starting, backend at %s", settings.backend_base_url)

    app.state.settings = settings
    app.state.client = BackendClient(settings)
    # Built once, here. A model per request adds latency to every turn and, on Vertex, a
    # credential refresh with it.
    try:
        app.state.chat_model = build_chat_model(settings)
    except ValidationError as exc:
        # Same sanitising as the settings above, and for the same reason: the SDK validates its
        # own constructor, and its ValidationError embeds the kwargs it was given -- which include
        # the API key.
        reasons = "; ".join(_reasons(exc))
        log.error("language model configuration is invalid, refusing to start: %s", reasons)
        raise ConfigurationError(reasons) from None
    log.info("language model configured: %s", describe_model(settings))

    # The agent, assembled here and nowhere else. The tools are a factory rather than a list: each
    # closes over the snapshot it reads, which is per request.
    app.state.card_cache = ContextCardCache(settings.context_cache_ttl_seconds)
    configure_pipeline(
        Pipeline(
            app.state.chat_model,
            lambda sid: to_langchain_tools(build_tools(app.state.client, sid)),
            app.state.card_cache,
            app.state.client,
            model_name=settings.llm_model,
            max_history_messages=settings.max_history_messages,
            max_tool_iterations=settings.max_tool_iterations,
        )
    )
    log.info("agent pipeline configured")

    # Said out loud, once, where kubectl logs will show it.
    log.info(
        "conversation turns are serialised per process, not across replicas; "
        "run one replica or expect interleaved transcripts",
        extra={"max_concurrent_turns": settings.max_concurrent_turns},
    )

    try:
        yield
    finally:
        await app.state.client.aclose()
        log.info("stopped")


app = FastAPI(
    title="urara-vision chat",
    description="Answers questions about a documented data model.",
    lifespan=lifespan,
)


@app.middleware("http")
async def limit_request_size(
    request: Request, call_next: Callable[[Request], Awaitable[Response]]
) -> Response:
    """Refuse an oversized body on Content-Length, before it is read."""
    declared = request.headers.get("content-length")
    if declared is not None and declared.isdigit():
        limit = request.app.state.settings.max_request_bytes
        if int(declared) > limit:
            # The request ID goes in the body as well as the header, as it does for every error
            # the exception handlers render.
            request_id = request_id_of(request)
            return JSONResponse(
                status_code=413,
                content={
                    "detail": f"request body is larger than the {limit} byte limit",
                    "requestId": request_id,
                },
                headers={REQUEST_ID_HEADER: request_id},
            )
    return await call_next(request)


# Added last, so it wraps everything else: Starlette applies middleware outermost-last, and the ID
# has to be assigned before any other middleware can answer -- otherwise the 413 above leaves
# without one.
app.add_middleware(RequestIDMiddleware)

# One place decides what an exception becomes, so no route needs its own try/except for a backend,
# provider or validation failure.
install_error_handlers(app)

app.include_router(chat_router)
# The debug routes stay mounted alongside them: they are how Phase 08 explains a bad answer, and
# /debug/answer is promoted rather than replaced in 05.7.
app.include_router(router)
