"""The service's HTTP surface: probes, and the debugging routes.

`/debug/tool` runs the retrieval layer with no model in the way, which is the
only way to tell a bad answer caused by bad retrieval from one caused by bad
reasoning -- a model papers over a thin tool result with fluent prose, and the
prose is convincing. `/debug/answer` runs the whole pipeline for one turn and
persists nothing, and returns every diagnostic field rather than only the text:
without the tool calls and the iteration count a wrong answer is unexplainable,
which is the whole reason the route exists. Both stay permanently -- Phase 05
promotes `/debug/answer` to `/api/chat/answer` rather than replacing it, and
Phase 08's eval runner drives it thousands of times.

The probe split is deliberate and mirrors the backend's. `/healthz` answers
while the process is alive and never touches the backend: a liveness probe that
fails during a backend outage restarts every chat pod, which fixes nothing and
loses whatever they were doing. `/readyz` does check, so a pod that cannot
answer leaves the load balancer without being killed.
"""

from __future__ import annotations

import asyncio
import logging
import time
from dataclasses import asdict
from typing import Any

from fastapi import APIRouter, HTTPException, Request
from fastapi.responses import JSONResponse, Response
from langchain_core.language_models import BaseChatModel
from pydantic import ValidationError

from urara_chat.agent.pipeline import answer
from urara_chat.api.schemas import AnswerRequest, AnswerResponse, ToolInvokeRequest
from urara_chat.backend.client import BackendClient
from urara_chat.backend.errors import BackendError, BackendNotFound
from urara_chat.config import Settings
from urara_chat.llm.content import flatten_content
from urara_chat.llm.factory import describe_model
from urara_chat.tools.registry import ToolSpec, build_tools

router = APIRouter()
log = logging.getLogger(__name__)

# The probe's own timeout, deliberately shorter than a turn's and independent of
# it: /debug/llm is what you reach for when the provider is misbehaving, and a
# probe that hangs as long as the thing it is diagnosing is no use.
PROBE_TIMEOUT_SECONDS = 15.0

# Fixed, and short enough to cost nothing. The point is whether credentials work
# and the provider answers, not what it says.
PROBE_PROMPT = "Reply with exactly: pong"

# What a caller is told when a turn fails upstream. Deliberately says nothing
# about why: a provider error quotes the request back, so it can carry prompt
# fragments and occasionally credentials. The real reason is logged. This
# mirrors Server.fail in backend/internal/api/respond.go.
UPSTREAM_FAILURE = "the answer could not be produced; see the service logs"


def get_client(request: Request) -> BackendClient:
    """The one client, built in the lifespan.

    A dependency rather than a module global so a test can override it, and so
    nothing is tempted to construct a second one per request.
    """
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
    """Liveness. Deliberately answers without reaching the backend.

    Kubelet has to be able to tell "this pod is broken" from "its dependency
    is". Only the first is fixed by a restart.
    """
    return {"status": "ok"}


@router.get("/readyz")
async def readyz(request: Request) -> Response:
    """Readiness. This one does check the backend.

    The service can answer nothing useful without it, so a pod that cannot
    reach it should leave the load balancer -- but stay running, because the
    outage is not its fault and a restart will not mend it.
    """
    client = get_client(request)
    # describe_model reads configuration only. Readiness runs every ten seconds
    # per pod, and a provider round trip on each would be a standing bill for
    # information this endpoint is not being asked for -- /debug/llm is what
    # tests whether the model actually answers.
    llm = describe_model(get_settings_for(request))

    if await client.health():
        return JSONResponse({"status": "ok", "backend": "ok", "llm": llm})
    # A JSONResponse rather than an HTTPException: raising would nest the body
    # under "detail", and the shape a probe and an operator read should be the
    # one the route documents.
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
    """Every tool, with the JSON schema a model would be shown.

    The snapshot here is a placeholder: the schemas do not depend on it, and no
    tool is called.
    """
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
    """Run one tool and return exactly what it returned.

    Nothing is reshaped on the way out: the value here is seeing what the model
    would see.
    """
    client = get_client(request)

    # Resolved first, so "latest" works here as it does everywhere else and so
    # an unknown snapshot is a 404 rather than a puzzling empty result.
    try:
        snapshot_id = await client.resolve_snapshot(body.snapshot_id)
    except BackendNotFound as exc:
        raise HTTPException(status_code=404, detail=exc.message) from exc
    except BackendError as exc:
        raise HTTPException(status_code=502, detail=exc.message) from exc

    specs = {spec.name: spec for spec in build_tools(client, snapshot_id)}
    spec = specs.get(body.tool)
    if spec is None:
        # The valid names are in the message: whoever is debugging has usually
        # mistyped one, and a bare "unknown tool" makes them go and look.
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
    """Await a tool, translating a backend failure into a status.

    A 502 rather than a 500 for a backend error: the fault is upstream, and the
    distinction is what tells you which service to go and look at.
    """
    try:
        return await spec.fn(**args)
    except BackendNotFound as exc:
        raise HTTPException(status_code=404, detail=exc.message) from exc
    except BackendError as exc:
        raise HTTPException(status_code=502, detail=exc.message) from exc


@router.post("/debug/answer", response_model=AnswerResponse)
async def debug_answer(request: Request, body: AnswerRequest) -> AnswerResponse:
    """One turn through the whole pipeline. Nothing is persisted.

    This is how the pipeline is checked without polluting a transcript store,
    and it returns the full result rather than the text alone: `toolCalls` and
    `iterations` are what turn a wrong answer from a mystery into a bug you can
    point at.
    """
    settings = get_settings_for(request)
    client = get_client(request)

    # Trimmed before it is measured, so a body of spaces is empty rather than
    # short, and so the limit counts characters the model will actually read.
    question = body.question.strip()
    if not question:
        raise HTTPException(status_code=400, detail="question must not be empty")
    if len(question) > settings.max_question_chars:
        # The limit is in the message: a caller that hit it is usually pasting a
        # document, and needs to know how much to cut rather than that it was
        # too much.
        raise HTTPException(
            status_code=400,
            detail=(
                f"question is {len(question)} characters, over the "
                f"{settings.max_question_chars} character limit"
            ),
        )

    # Resolved here rather than in the pipeline, so "latest" works as it does
    # everywhere else and the concrete ID is what reaches answer() -- which
    # refuses the alias, because reading the wrong snapshot produces a
    # confidently wrong answer.
    try:
        snapshot_id = await client.resolve_snapshot(body.snapshot_id)
    except BackendNotFound as exc:
        raise HTTPException(status_code=404, detail=exc.message) from exc
    except BackendError as exc:
        log.error("snapshot could not be resolved", exc_info=exc)
        raise HTTPException(status_code=502, detail=UPSTREAM_FAILURE) from exc

    try:
        # A deadline on the whole turn. Every call inside it is bounded already,
        # but a turn is up to seven model calls plus their tools, and this route
        # holds the connection for all of them -- Phase 08's eval runner drives
        # it thousands of times, where one hung turn hangs the run.
        result = await asyncio.wait_for(
            answer(question, snapshot_id, history=[], language=body.language),
            timeout=settings.answer_timeout_seconds,
        )
    except BackendNotFound as exc:
        # The snapshot resolved a moment ago, so this is one deleted mid-turn.
        # Still the caller's answer to have: the ID they asked about is gone.
        raise HTTPException(status_code=404, detail=exc.message) from exc
    except Exception as exc:
        # Broad on purpose, and the same posture as /debug/llm: every provider
        # raises its own exception types, a spent tool budget is already a 200
        # with truncated set, and what is left is "upstream did not answer".
        # The detail is logged and not returned.
        log.error(
            "answer failed",
            extra={"snapshot_id": snapshot_id, "language": body.language},
            exc_info=exc,
        )
        raise HTTPException(status_code=502, detail=UPSTREAM_FAILURE) from exc

    # asdict() gives the dataclass's own field names; the schema carries the
    # camelCase wire names and populate_by_name lets it be built from either.
    return AnswerResponse(**asdict(result))


@router.get("/debug/llm")
async def debug_llm(request: Request) -> dict[str, Any]:
    """One fixed prompt to the provider: the cheapest check that credentials work.

    This is the first thing to reach for when the service misbehaves in a
    cluster, before reading a log line, so it answers quickly or not at all.
    """
    settings = get_settings_for(request)
    model = get_chat_model(request)

    started = time.perf_counter()
    try:
        reply = await asyncio.wait_for(model.ainvoke(PROBE_PROMPT), timeout=PROBE_TIMEOUT_SECONDS)
    except Exception as exc:
        # Broad on purpose: every provider raises its own exception types, and
        # this endpoint exists to report that the provider did not answer rather
        # than to distinguish why.
        #
        # The detail is logged and *not* returned. Provider errors quote the
        # request back, so they can carry prompt fragments and occasionally
        # credentials -- the same reason Server.fail in the Go backend logs the
        # error and answers with a generic one.
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
