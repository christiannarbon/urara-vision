"""The service's own routes, with the backend client faked.

The probe split carries the most weight here: /healthz must answer while the
backend is on fire, because a liveness probe that fails during an outage
restarts every chat pod and mends nothing.
"""

import asyncio
import logging
from typing import Any

import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient
from langchain_core.messages import AIMessage

from urara_chat.api.routes import router
from urara_chat.backend.errors import BackendError, BackendNotFound, BackendUnavailable
from urara_chat.backend.models import Domain, SearchHit
from urara_chat.config import Settings
from urara_chat.tools.registry import TOOL_NAMES

# Deliberately not shaped like a real Google key. A fixture with an
# AIza prefix trips every secret scanner in the repository, and a leak
# detector that always cries wolf is one nobody reads.
FAKE_KEY = "test-key-shaped-value-0123456789abcdef"


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


class FakeModel:
    """Stands in for the chat model. Records whether it was ever called, which
    is how /readyz is held to not calling it."""

    def __init__(self, reply: Any = "pong", raises: Exception | None = None) -> None:
        self.reply = reply
        self.raises = raises
        self.calls = 0

    async def ainvoke(self, prompt: Any, *args: Any, **kwargs: Any) -> Any:
        self.calls += 1
        if self.raises:
            raise self.raises
        return AIMessage(content=self.reply)


def fake_settings(**over: Any) -> Settings:
    base: dict[str, Any] = {
        "llm_provider": "gemini-studio",
        "llm_model": "gemini-2.5-flash",
        "google_api_key": FAKE_KEY,
    }
    return Settings(**(base | over))


def app_with(
    client: FakeClient,
    model: FakeModel | None = None,
    settings: Settings | None = None,
) -> FastAPI:
    """An app whose state carries the fakes, skipping the real lifespan."""
    app = FastAPI()
    app.include_router(router)
    app.state.client = client
    app.state.chat_model = model or FakeModel()
    app.state.settings = settings or fake_settings()
    return app


def client_for(
    fake: FakeClient,
    model: FakeModel | None = None,
    settings: Settings | None = None,
) -> TestClient:
    return TestClient(app_with(fake, model, settings))


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
        body = response.json()
        assert body["status"] == "ok"
        assert body["backend"] == "ok"
        assert body["llm"] == {"provider": "gemini-studio", "model": "gemini-2.5-flash"}

    def test_readyz_is_503_and_says_why(self) -> None:
        """A pod that cannot answer leaves the load balancer, but is not killed:
        the outage is upstream and a restart will not mend it."""
        response = client_for(FakeClient(healthy=False)).get("/readyz")

        assert response.status_code == 503
        # The documented shape, not nested under "detail": a probe and an
        # operator both read this, and neither should have to unwrap it.
        body = response.json()
        assert body["status"] == "unready"
        assert body["reason"]


class TestReadyzReportsTheModelWithoutCallingIt:
    def test_the_model_is_never_invoked(self) -> None:
        """Readiness runs every ten seconds per pod. A provider round trip on
        each would be a standing bill for information nobody asked for."""
        model = FakeModel()
        response = client_for(FakeClient(healthy=True), model).get("/readyz")

        assert response.status_code == 200
        assert model.calls == 0, "/readyz called the provider"

    def test_it_reports_the_configured_model_when_unready_too(self) -> None:
        """Knowing which model a failing pod is configured for is exactly what
        you want while it is failing."""
        model = FakeModel()
        response = client_for(FakeClient(healthy=False), model).get("/readyz")

        assert response.status_code == 503
        assert response.json()["llm"]["model"] == "gemini-2.5-flash"
        assert model.calls == 0

    def test_the_body_carries_no_part_of_the_key(self) -> None:
        rendered = client_for(FakeClient(healthy=True)).get("/readyz").text
        assert FAKE_KEY not in rendered
        assert FAKE_KEY[:12] not in rendered

    def test_vertex_reports_its_region(self) -> None:
        settings = Settings(
            llm_provider="vertex",
            llm_model="gemini-2.5-pro",
            vertex_project="p",
            vertex_location="europe-west2",
        )
        body = client_for(FakeClient(), None, settings).get("/readyz").json()

        assert body["llm"] == {
            "provider": "vertex",
            "model": "gemini-2.5-pro",
            "location": "europe-west2",
        }


class TestDebugLLM:
    def test_returns_the_reply_and_a_latency(self) -> None:
        model = FakeModel(reply="pong")
        response = client_for(FakeClient(), model).get("/debug/llm")

        assert response.status_code == 200
        body = response.json()
        assert body["text"] == "pong"
        assert body["latencyMs"] >= 0
        assert body["provider"] == "gemini-studio"
        assert body["model"] == "gemini-2.5-flash"
        assert model.calls == 1

    def test_a_list_shaped_reply_is_flattened(self) -> None:
        """A probe that reports [{'type': 'text', ...}] has failed at its one
        job, and providers do answer in parts."""
        model = FakeModel(reply=[{"type": "text", "text": "po"}, {"type": "text", "text": "ng"}])
        body = client_for(FakeClient(), model).get("/debug/llm").json()

        assert body["text"] == "pong"

    def test_a_provider_failure_is_502(self) -> None:
        model = FakeModel(raises=RuntimeError("boom"))
        response = client_for(FakeClient(), model).get("/debug/llm")
        assert response.status_code == 502

    def test_the_provider_error_text_is_not_echoed(self) -> None:
        """Provider errors quote the request back, so they can carry prompt
        fragments and occasionally credentials."""
        secret = f"quota exceeded for key {FAKE_KEY}"
        model = FakeModel(raises=RuntimeError(secret))

        response = client_for(FakeClient(), model).get("/debug/llm")

        assert response.status_code == 502
        assert FAKE_KEY not in response.text
        assert "quota exceeded" not in response.text
        assert FAKE_KEY[:12] not in response.text

    def test_the_real_reason_is_logged(self, caplog: pytest.LogCaptureFixture) -> None:
        model = FakeModel(raises=RuntimeError("the real reason"))

        with caplog.at_level(logging.ERROR, logger="urara_chat.api.routes"):
            client_for(FakeClient(), model).get("/debug/llm")

        record = next(r for r in caplog.records if r.message == "llm probe failed")
        assert record.provider == "gemini-studio"  # type: ignore[attr-defined]
        assert "the real reason" in str(record.exc_info[1])  # type: ignore[index]

    def test_a_hung_provider_times_out_rather_than_hanging(self) -> None:
        """The probe you reach for while the provider is wedged must not wedge
        with it."""
        import urara_chat.api.routes as routes

        class Hangs(FakeModel):
            async def ainvoke(self, prompt: Any, *args: Any, **kwargs: Any) -> Any:
                await asyncio.sleep(10)
                return AIMessage(content="never")

        original = routes.PROBE_TIMEOUT_SECONDS
        routes.PROBE_TIMEOUT_SECONDS = 0.05
        try:
            response = client_for(FakeClient(), Hangs()).get("/debug/llm")
        finally:
            routes.PROBE_TIMEOUT_SECONDS = original

        assert response.status_code == 502


class TestHealthzIsUnaffectedByTheModel:
    def test_a_model_that_raises_does_not_touch_liveness(self) -> None:
        """A liveness probe that failed while a provider rate-limits would
        restart every pod in the deployment at the worst possible moment."""
        model = FakeModel(raises=RuntimeError("provider is down"))
        response = client_for(FakeClient(), model).get("/healthz")

        assert response.status_code == 200
        assert response.json() == {"status": "ok"}
        assert model.calls == 0


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

    def test_an_unreachable_backend_is_502_not_500(self) -> None:
        """A transport failure is still the backend's fault. Answering 500 would
        point at this service for an outage in the one it depends on.

        The wrapping itself lives in BackendClient, so this checks the mapping;
        test_client.py checks that a refused connection produces the exception.
        """
        fake = FakeClient(raises=BackendUnavailable(502, "backend unreachable: refused"))
        response = self.invoke(fake, snapshotId="snap-1", tool="list_domains", args={})
        assert response.status_code == 502

    def test_an_unknown_body_field_is_rejected(self) -> None:
        """A misspelled key should be reported, not silently dropped."""
        response = client_for(FakeClient()).post(
            "/debug/tool",
            json={"snapshotId": "snap-1", "tool": "list_domains", "args": {}, "toolz": "x"},
        )
        assert response.status_code == 422

    def test_a_misspelled_argument_is_rejected(self) -> None:
        """`limt` would otherwise take the default and the caller would reason
        about a result it did not ask for."""
        response = self.invoke(
            FakeClient(), snapshotId="snap-1", tool="search_model", args={"query": "x", "limt": 5}
        )
        assert response.status_code == 400

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
        from urara_chat.config import get_settings

        # The lifespan reads real settings, and since 03.2 those refuse to build
        # without the credential the default provider needs. This test is about
        # the client's lifecycle, not about configuration.
        monkeypatch.setenv("GOOGLE_API_KEY", "test-key-not-real")
        get_settings.cache_clear()

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
        get_settings.cache_clear()

    def test_a_bad_model_setting_does_not_leak_the_key(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """The SDK validates its own constructor, and its ValidationError embeds
        the kwargs it was given -- including the key. A crash loop would put the
        key's tail in the container log on every restart."""
        import urara_chat.main as main
        from urara_chat.config import ConfigurationError, get_settings

        monkeypatch.setenv("GOOGLE_API_KEY", FAKE_KEY)
        # In range for Settings, out of range for the SDK, so the failure
        # happens in build_chat_model rather than in get_settings.
        monkeypatch.setenv("LLM_TEMPERATURE", "5.0")
        get_settings.cache_clear()

        with pytest.raises(ConfigurationError) as caught:  # noqa: SIM117
            with TestClient(main.app):
                pass

        assert "temperature" in str(caught.value)
        assert FAKE_KEY[-12:] not in str(caught.value)
        assert FAKE_KEY not in str(caught.value)
        get_settings.cache_clear()

    def test_the_model_is_built_once_in_the_lifespan(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """A model per request adds latency to every turn and, on Vertex, a
        credential refresh with it."""
        import urara_chat.main as main
        from urara_chat.config import get_settings

        monkeypatch.setenv("GOOGLE_API_KEY", "test-key-not-real")
        get_settings.cache_clear()

        built: list[FakeModel] = []

        def build(settings: Any) -> FakeModel:
            model = FakeModel()
            built.append(model)
            return model

        monkeypatch.setattr(main, "BackendClient", lambda settings: FakeClient())
        monkeypatch.setattr(main, "build_chat_model", build)

        with TestClient(main.app) as c:
            for _ in range(3):
                assert c.get("/debug/llm").status_code == 200

        assert len(built) == 1, "the model should be built once in the lifespan"
        assert built[0].calls == 3, "all three requests used the same model"
        get_settings.cache_clear()
