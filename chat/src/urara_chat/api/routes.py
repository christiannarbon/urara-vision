"""The service's HTTP surface: probes, and the tool debugging routes.

`/debug/tool` is the point of the phase. It runs the retrieval layer with no
model in the way, which is the only way to tell a bad answer caused by bad
retrieval from one caused by bad reasoning -- a model papers over a thin tool
result with fluent prose, and the prose is convincing. It stays permanently.

The probe split is deliberate and mirrors the backend's. `/healthz` answers
while the process is alive and never touches the backend: a liveness probe that
fails during a backend outage restarts every chat pod, which fixes nothing and
loses whatever they were doing. `/readyz` does check, so a pod that cannot
answer leaves the load balancer without being killed.
"""

from __future__ import annotations

from typing import Any

from fastapi import APIRouter, HTTPException, Request
from fastapi.responses import JSONResponse, Response
from pydantic import ValidationError

from urara_chat.api.schemas import ToolInvokeRequest
from urara_chat.backend.client import BackendClient
from urara_chat.backend.errors import BackendError, BackendNotFound
from urara_chat.tools.registry import ToolSpec, build_tools

router = APIRouter()


def get_client(request: Request) -> BackendClient:
    """The one client, built in the lifespan.

    A dependency rather than a module global so a test can override it, and so
    nothing is tempted to construct a second one per request.
    """
    client: BackendClient = request.app.state.client
    return client


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
    if await client.health():
        return JSONResponse({"status": "ok"})
    # A JSONResponse rather than an HTTPException: raising would nest the body
    # under "detail", and the shape a probe and an operator read should be the
    # one the route documents.
    return JSONResponse(
        status_code=503,
        content={"status": "unready", "reason": "backend is not reachable or not ready"},
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
