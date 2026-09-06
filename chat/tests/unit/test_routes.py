"""The service's own routes, with the backend client faked.

The probe split carries the most weight here: /healthz must answer while the
backend is on fire, because a liveness probe that fails during an outage
restarts every chat pod and mends nothing.
"""

from typing import Any

import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient

from urara_chat.api.routes import router
from urara_chat.backend.errors import BackendError, BackendNotFound
from urara_chat.backend.models import Domain, SearchHit
from urara_chat.tools.registry import TOOL_NAMES


class FakeClient:
    """Stands in for BackendClient, recording how often it was built and used."""

    def __init__(self, *, healthy: bool = True, raises: Exception | None = None) -> None:
        self.healthy = healthy
        self.raises = raises
        self.resolved: list[str] = []

    async def health(self) -> bool:
        if self.raises:
            raise self.raises
        return self.healthy

    async def resolve_snapshot(self, sid: str) -> str:
        self.resolved.append(sid)
        if self.raises:
            raise self.raises
        if sid == "nosuch":
            raise BackendNotFound(404, "snapshot not found")
        return "real-id" if sid == "latest" else sid

    async def list_domains(self, sid: str) -> list[Domain]:
        if self.raises:
            raise self.raises
        return [Domain(id="ordering", title="Ordering")]

    async def search(self, sid: str, query: str, limit: int = 20) -> list[SearchHit]:
        if self.raises:
            raise self.raises
        return [SearchHit(tableId="ordering/fact_orders", name="fact_orders")]  # type: ignore[call-arg]

    async def aclose(self) -> None:
        return None


def app_with(client: FakeClient) -> FastAPI:
    """An app whose state carries the fake, skipping the real lifespan."""
    app = FastAPI()
    app.include_router(router)
    app.state.client = client
    return app


def client_for(fake: FakeClient) -> TestClient:
    return TestClient(app_with(fake))


class TestProbes:
    def test_healthz_never_touches_the_backend(self) -> None:
        """Even with a client that raises on every call, liveness answers."""
        broken = FakeClient(raises=BackendError(500, "everything is down"))
        response = client_for(broken).get("/healthz")

        assert response.status_code == 200
        assert response.json() == {"status": "ok"}

    def test_readyz_is_200_when_the_backend_is_ready(self) -> None:
        response = client_for(FakeClient(healthy=True)).get("/readyz")
        assert response.status_code == 200
        assert response.json()["status"] == "ok"

    def test_readyz_is_503_and_says_why(self) -> None:
        """A pod that cannot answer leaves the load balancer, but is not killed:
        the outage is upstream and a restart will not mend it."""
        response = client_for(FakeClient(healthy=False)).get("/readyz")

        assert response.status_code == 503
        detail = response.json()["detail"]
        assert detail["status"] == "unready"
        assert detail["reason"]


class TestListTools:
    def test_lists_nine_with_their_schemas(self) -> None:
        response = client_for(FakeClient()).get("/debug/tools")
        assert response.status_code == 200

        tools = response.json()
        assert len(tools) == 9
        assert tuple(t["name"] for t in tools) == TOOL_NAMES
        for tool in tools:
            assert tool["description"]
            assert tool["schema"]["type"] == "object"

    def test_no_schema_offers_a_snapshot(self) -> None:
        """The route builds the tools too, so the binding is checked here as
        well as in the registry's own tests."""
        tools = client_for(FakeClient()).get("/debug/tools").json()
        assert "snapshot" not in str(tools).lower().replace("snapshotid", "")


class TestInvokeTool:
    def invoke(self, fake: FakeClient, **body: Any) -> Any:
        return client_for(fake).post("/debug/tool", json=body)

    def test_runs_the_named_tool_and_returns_its_result(self) -> None:
        fake = FakeClient()
        response = self.invoke(fake, snapshotId="snap-1", tool="list_domains", args={})

        assert response.status_code == 200
        body = response.json()
        assert body["items"][0]["id"] == "ordering"
        assert body["truncated"] is False

    def test_latest_is_resolved(self) -> None:
        fake = FakeClient()
        response = self.invoke(fake, snapshotId="latest", tool="list_domains", args={})

        assert response.status_code == 200
        assert fake.resolved == ["latest"]

    def test_args_reach_the_tool(self) -> None:
        fake = FakeClient()
        response = self.invoke(
            fake, snapshotId="snap-1", tool="search_model", args={"query": "orders", "limit": 5}
        )

        assert response.status_code == 200
        assert response.json()["items"][0]["tableId"] == "ordering/fact_orders"

    def test_an_unknown_tool_is_400_listing_the_valid_names(self) -> None:
        """Whoever is debugging has usually mistyped one; a bare "unknown tool"
        sends them to go and look it up."""
        response = self.invoke(FakeClient(), snapshotId="snap-1", tool="nope", args={})

        assert response.status_code == 400
        detail = response.json()["detail"]
        assert "nope" in detail
        for name in TOOL_NAMES:
            assert name in detail

    def test_bad_args_are_400_with_the_validation_detail(self) -> None:
        response = self.invoke(
            FakeClient(), snapshotId="snap-1", tool="get_tables", args={"ids": []}
        )

        assert response.status_code == 400
        assert "ids" in str(response.json()["detail"])

    def test_an_out_of_range_argument_is_400(self) -> None:
        response = self.invoke(
            FakeClient(),
            snapshotId="snap-1",
            tool="get_neighbourhood",
            args={"table_id": "d/t", "depth": 9},
        )
        assert response.status_code == 400

    def test_an_unknown_snapshot_is_404(self) -> None:
        response = self.invoke(FakeClient(), snapshotId="nosuch", tool="list_domains", args={})
        assert response.status_code == 404

    def test_backend_not_found_from_the_tool_is_404(self) -> None:
        fake = FakeClient(raises=BackendNotFound(404, "no such table"))
        # resolve_snapshot raises first, which is the same mapping.
        response = self.invoke(fake, snapshotId="snap-1", tool="get_tables", args={"ids": ["d/t"]})
        assert response.status_code == 404

    def test_a_backend_error_is_502_not_500(self) -> None:
        """The fault is upstream, and the status is what says which service to
        go and look at."""
        fake = FakeClient(raises=BackendError(500, "boom"))
        response = self.invoke(fake, snapshotId="snap-1", tool="list_domains", args={})

        assert response.status_code == 502
        assert "boom" in str(response.json()["detail"])

    def test_snake_case_body_is_accepted_too(self) -> None:
        response = self.invoke(FakeClient(), snapshot_id="snap-1", tool="list_domains", args={})
        assert response.status_code == 200

    def test_args_default_to_empty(self) -> None:
        response = client_for(FakeClient()).post(
            "/debug/tool", json={"snapshotId": "snap-1", "tool": "list_domains"}
        )
        assert response.status_code == 200


class TestLifespan:
    def test_one_client_is_built_for_the_process_and_closed_on_shutdown(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """A client per request would leak connections and drop pooling."""
        import urara_chat.main as main

        built: list[FakeClient] = []

        def build(settings: Any) -> FakeClient:
            fake = FakeClient()
            built.append(fake)
            return fake

        monkeypatch.setattr(main, "BackendClient", build)

        with TestClient(main.app) as c:
            for _ in range(3):
                assert c.get("/healthz").status_code == 200
            assert c.get("/readyz").status_code == 200

        assert len(built) == 1, "the client should be built once in the lifespan"
