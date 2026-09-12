"""The service's HTTP surface: probes, and the debugging routes."""

from __future__ import annotations

import asyncio
import logging
import time
from typing import Any

from fastapi import APIRouter, HTTPException, Request
from fastapi.responses import JSONResponse, Response
from langchain_core.language_models import BaseChatModel
from pydantic import ValidationError

from urara_chat.api.answering import answer_question, to_response
from urara_chat.api.schemas import AnswerRequest, AnswerResponse, ToolInvokeRequest
from urara_chat.backend.client import BackendClient
from urara_chat.backend.errors import BackendError, BackendNotFound
from urara_chat.config import Settings
from urara_chat.llm.content import flatten_content
from urara_chat.llm.factory import describe_model
from urara_chat.tools.registry import ToolSpec, build_tools

router = APIRouter()
log = logging.getLogger(__name__)

# The probe's own timeout, deliberately shorter than a turn's and independent of it: /debug/llm is
# what you reach for when the provider is misbehaving, and a probe that hangs as long as the thing
# it is diagnosing is no use.
PROBE_TIMEOUT_SECONDS = 15.0

# Fixed, and short enough to cost nothing. The point is whether credentials work and the provider
# answers, not what it says.
PROBE_PROMPT = "Reply with exactly: pong"


def get_client(request: Request) -> BackendClient:
    """The one client, built in the lifespan."""
    client: BackendClient = request.app.state.client
    return client


def get_settings_for(request: Request) -> Settings:
    settings: Settings = request.app.state.settings
    return settings


def get_chat_model(request: Request) -> BaseChatModel:
    """The one model, built in the lifespan."""
    model: BaseChatModel = request.app.state.chat_model
    return model


@router.get("/healthz")
async def healthz() -> dict[str, str]:
    return {"status": "ok"}


@router.get("/readyz")
async def readyz(request: Request) -> Response:
    """Readiness."""
    client = get_client(request)
    # describe_model reads configuration only.
    llm = describe_model(get_settings_for(request))

    if await client.health():
        return JSONResponse({"status": "ok", "backend": "ok", "llm": llm})
    # A JSONResponse rather than an HTTPException: raising would nest the body under "detail", and
    # the shape a probe and an operator read should be the one the route documents.
    return JSONResponse(
        status_code=503,
        content={
            "status": "unready",
            "backend": "unreachable",
            "llm": llm,
            "reason": "backend is not reachable or not ready",
        },
    )


@router.get("/debug/tools")
async def list_tools(request: Request) -> list[dict[str, Any]]:
    """Every tool, with the JSON schema a model would be shown."""
    client = get_client(request)
    return [
        {
            "name": spec.name,
            "description": spec.description,
            "schema": spec.args_schema.model_json_schema(),
        }
        for spec in build_tools(client, "unused-for-schemas")
    ]


@router.post("/debug/tool")
async def invoke_tool(request: Request, body: ToolInvokeRequest) -> Any:
    """Run one tool and return exactly what it returned."""
    client = get_client(request)

    # Resolved first, so "latest" works here as it does everywhere else and so an unknown snapshot
    # is a 404 rather than a puzzling empty result.
    try:
        snapshot_id = await client.resolve_snapshot(body.snapshot_id)
    except BackendNotFound as exc:
        raise HTTPException(status_code=404, detail=exc.message) from exc
    except BackendError as exc:
        raise HTTPException(status_code=502, detail=exc.message) from exc

    specs = {spec.name: spec for spec in build_tools(client, snapshot_id)}
    spec = specs.get(body.tool)
    if spec is None:
        # The valid names are in the message: whoever is debugging has usually mistyped one, and a
        # bare "unknown tool" makes them go and look.
        raise HTTPException(
            status_code=400,
            detail=f"unknown tool {body.tool!r}; expected one of {sorted(specs)}",
        )

    try:
        args = spec.args_schema.model_validate(body.args)
    except ValidationError as exc:
        # Pydantic's own detail, which names the field and the rule it broke.
        raise HTTPException(status_code=400, detail=exc.errors(include_url=False)) from exc

    return await _run(spec, args.model_dump())


async def _run(spec: ToolSpec, args: dict[str, Any]) -> Any:
    """Await a tool, translating a backend failure into a status."""
    try:
        return await spec.fn(**args)
    except BackendNotFound as exc:
        raise HTTPException(status_code=404, detail=exc.message) from exc
    except BackendError as exc:
        raise HTTPException(status_code=502, detail=exc.message) from exc


@router.post("/debug/answer", response_model=AnswerResponse)
async def debug_answer(request: Request, body: AnswerRequest) -> AnswerResponse:
    """One turn through the whole pipeline."""
    result = await answer_question(
        get_client(request),
        get_settings_for(request),
        body.snapshot_id,
        body.question,
        body.language,
    )
    return to_response(result)


@router.get("/debug/llm")
async def debug_llm(request: Request) -> dict[str, Any]:
    """One fixed prompt to the provider: the cheapest check that credentials work."""
    settings = get_settings_for(request)
    model = get_chat_model(request)

    started = time.perf_counter()
    try:
        reply = await asyncio.wait_for(model.ainvoke(PROBE_PROMPT), timeout=PROBE_TIMEOUT_SECONDS)
    except Exception as exc:
        # Broad on purpose: every provider raises its own types.
        log.error(
            "llm probe failed",
            extra={"provider": settings.llm_provider, "model": settings.llm_model},
            exc_info=exc,
        )
        raise HTTPException(
            status_code=502,
            detail="the language model provider did not answer; see the service logs",
        ) from exc

    return {
        "text": flatten_content(reply.content),
        "latencyMs": round((time.perf_counter() - started) * 1000, 2),
        **describe_model(settings),
    }
