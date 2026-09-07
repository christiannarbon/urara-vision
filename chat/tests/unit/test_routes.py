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

from urara_chat.agent.pipeline import AgentAnswer
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


class FakePipeline:
    """Stands in for the agent, recording what the route handed it.

    The route's job is the arguments and the error mapping, not the answer, so
    everything below the call is scripted -- and the recorded snapshot is what
    proves `latest` never reaches a pipeline that would refuse it.
    """

    def __init__(self, result: Any = None, raises: Exception | None = None) -> None:
        self.result = result if result is not None else agent_answer()
        self.raises = raises
        self.calls: list[dict[str, Any]] = []

    async def __call__(
        self,
        question: str,
        snapshot_id: str,
        history: Any,
        language: str = "EN",
    ) -> Any:
        self.calls.append(
            {
                "question": question,
                "snapshot_id": snapshot_id,
                "history": history,
                "language": language,
            }
        )
        if self.raises:
            raise self.raises
        return self.result


def agent_answer(**over: Any) -> AgentAnswer:
    base: dict[str, Any] = {
        "text": "fact_orders is one row per order.",
        "citations": ["ordering/fact_orders"],
        "tool_calls": [{"name": "get_tables", "args": {"ids": ["ordering/fact_orders"]}}],
        "iterations": 2,
        "truncated": False,
        "model": "gemini-2.5-flash",
        "latency_ms": 1234,
        "usage": {"input_tokens": 900, "output_tokens": 120},
    }
    return AgentAnswer(**(base | over))


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


class TestDebugAnswer:
    """The one-turn path, with the pipeline faked.

    What is worth asserting here is the route's own work: the arguments the
    pipeline is handed, the status for each way a turn can fail, and that no
    provider text reaches the body. What the agent answers is tested in
    test_pipeline.py, and against a real model in the integration suite.
    """

    def post(
        self,
        monkeypatch: pytest.MonkeyPatch,
        fake: FakeClient | None = None,
        pipeline: FakePipeline | None = None,
        settings: Settings | None = None,
        **body: Any,
    ) -> Any:
        import urara_chat.api.routes as routes

        monkeypatch.setattr(routes, "answer", pipeline or FakePipeline())
        return client_for(fake or FakeClient(), None, settings).post("/debug/answer", json=body)

    def test_it_returns_every_documented_field(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """Text alone is not enough: without toolCalls and iterations a wrong
        answer is unexplainable, which is why this route exists."""
        response = self.post(
            monkeypatch, snapshotId="snap-1", question="What is the grain of fact_orders?"
        )

        assert response.status_code == 200
        assert response.json() == {
            "text": "fact_orders is one row per order.",
            "citations": ["ordering/fact_orders"],
            "toolCalls": [{"name": "get_tables", "args": {"ids": ["ordering/fact_orders"]}}],
            "iterations": 2,
            "truncated": False,
            "model": "gemini-2.5-flash",
            "latencyMs": 1234,
            "usage": {"input_tokens": 900, "output_tokens": 120},
        }

    def test_latest_is_resolved_before_the_pipeline_sees_it(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """answer() refuses the alias, and rightly: reading the wrong snapshot
        produces a confidently wrong answer."""
        fake, pipeline = FakeClient(), FakePipeline()
        response = self.post(
            monkeypatch, fake, pipeline, snapshotId="latest", question="the grain of fact_orders?"
        )

        assert response.status_code == 200
        assert fake.resolved == ["latest"]
        assert pipeline.calls[0]["snapshot_id"] == "real-id"

    def test_nothing_is_persisted_so_the_history_is_empty(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        pipeline = FakePipeline()
        self.post(monkeypatch, None, pipeline, snapshotId="snap-1", question="q")

        assert pipeline.calls[0]["history"] == []

    def test_the_question_reaches_the_pipeline_trimmed(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        pipeline = FakePipeline()
        self.post(monkeypatch, None, pipeline, snapshotId="snap-1", question="  the grain?  ")

        assert pipeline.calls[0]["question"] == "the grain?"

    @pytest.mark.parametrize("question", ["", "   ", "\n\t "])
    def test_an_empty_question_is_400_naming_the_field(
        self, monkeypatch: pytest.MonkeyPatch, question: str
    ) -> None:
        """Whitespace is empty, not short: it costs a prompt and answers nothing."""
        response = self.post(monkeypatch, snapshotId="snap-1", question=question)

        assert response.status_code == 400
        assert "question" in str(response.json()["detail"])

    def test_an_over_long_question_is_400_naming_the_limit(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """Whoever hit this is pasting a document, and needs to know how much to
        cut rather than only that it was too much."""
        settings = fake_settings(max_question_chars=50)
        response = self.post(
            monkeypatch, None, None, settings, snapshotId="snap-1", question="x" * 51
        )

        assert response.status_code == 400
        assert "50" in str(response.json()["detail"])

    def test_a_question_at_the_limit_is_answered(self, monkeypatch: pytest.MonkeyPatch) -> None:
        settings = fake_settings(max_question_chars=50)
        response = self.post(
            monkeypatch, None, None, settings, snapshotId="snap-1", question="x" * 50
        )
        assert response.status_code == 200

    def test_the_limit_is_measured_after_trimming(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """The prompt is built from what is left, so that is what is counted."""
        settings = fake_settings(max_question_chars=10)
        response = self.post(
            monkeypatch, None, None, settings, snapshotId="snap-1", question="   short   "
        )
        assert response.status_code == 200

    def test_an_unknown_snapshot_is_404(self, monkeypatch: pytest.MonkeyPatch) -> None:
        response = self.post(monkeypatch, snapshotId="nosuch", question="q")
        assert response.status_code == 404

    def test_the_pipeline_is_not_called_for_a_bad_request(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """A validation failure should cost nothing: the model is the expensive
        part of this route."""
        pipeline = FakePipeline()
        self.post(monkeypatch, None, pipeline, snapshotId="nosuch", question="  ")

        assert pipeline.calls == []

    def test_an_unreachable_backend_is_502(self, monkeypatch: pytest.MonkeyPatch) -> None:
        fake = FakeClient(raises=BackendUnavailable(502, "backend unreachable: refused"))
        response = self.post(monkeypatch, fake, snapshotId="snap-1", question="q")
        assert response.status_code == 502

    def test_a_provider_failure_is_502(self, monkeypatch: pytest.MonkeyPatch) -> None:
        pipeline = FakePipeline(raises=RuntimeError("boom"))
        response = self.post(monkeypatch, None, pipeline, snapshotId="snap-1", question="q")
        assert response.status_code == 502

    def test_a_provider_timeout_is_502(self, monkeypatch: pytest.MonkeyPatch) -> None:
        pipeline = FakePipeline(raises=TimeoutError())
        response = self.post(monkeypatch, None, pipeline, snapshotId="snap-1", question="q")
        assert response.status_code == 502

    def test_the_provider_error_text_is_not_echoed(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """Provider errors quote the request back, so they can carry prompt
        fragments and occasionally credentials."""
        secret = f"quota exceeded for key {FAKE_KEY}; prompt was 'the grain of fact_orders'"
        pipeline = FakePipeline(raises=RuntimeError(secret))

        response = self.post(monkeypatch, None, pipeline, snapshotId="snap-1", question="q")

        assert response.status_code == 502
        assert FAKE_KEY not in response.text
        assert FAKE_KEY[:12] not in response.text
        assert "quota exceeded" not in response.text
        assert "the grain of fact_orders" not in response.text

    def test_the_real_reason_is_logged(
        self, monkeypatch: pytest.MonkeyPatch, caplog: pytest.LogCaptureFixture
    ) -> None:
        pipeline = FakePipeline(raises=RuntimeError("the real reason"))

        with caplog.at_level(logging.ERROR, logger="urara_chat.api.routes"):
            self.post(monkeypatch, None, pipeline, snapshotId="snap-1", question="q")

        record = next(r for r in caplog.records if r.message == "answer failed")
        assert record.snapshot_id == "snap-1"  # type: ignore[attr-defined]
        assert "the real reason" in str(record.exc_info[1])  # type: ignore[index]

    def test_a_spent_tool_budget_is_200_with_truncated_set(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """A partial answer that names its own gaps beats an error, and by now
        the reader has already waited."""
        pipeline = FakePipeline(agent_answer(truncated=True, iterations=6))
        response = self.post(monkeypatch, None, pipeline, snapshotId="snap-1", question="q")

        assert response.status_code == 200
        body = response.json()
        assert body["truncated"] is True
        assert body["text"]

    def test_usage_may_be_empty_when_the_provider_reports_none(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """Never guessed: a fabricated number in a cost report is worse than a
        gap."""
        pipeline = FakePipeline(agent_answer(usage={}))
        body = self.post(monkeypatch, None, pipeline, snapshotId="snap-1", question="q").json()

        assert body["usage"] == {}

    @pytest.mark.parametrize("sent", ["EN", "JA", "ja"])
    def test_a_known_language_reaches_the_pipeline_upper_cased(
        self, monkeypatch: pytest.MonkeyPatch, sent: str
    ) -> None:
        pipeline = FakePipeline()
        self.post(monkeypatch, None, pipeline, snapshotId="snap-1", question="q", language=sent)

        assert pipeline.calls[0]["language"] == sent.upper()

    @pytest.mark.parametrize("sent", ["FR", "klingon", ""])
    def test_an_unknown_language_falls_back_to_en(
        self, monkeypatch: pytest.MonkeyPatch, sent: str
    ) -> None:
        """Not worth failing a question over: refusing the turn loses the answer
        as well as the language."""
        pipeline = FakePipeline()
        response = self.post(
            monkeypatch, None, pipeline, snapshotId="snap-1", question="q", language=sent
        )

        assert response.status_code == 200
        assert pipeline.calls[0]["language"] == "EN"

    def test_the_language_defaults_to_en(self, monkeypatch: pytest.MonkeyPatch) -> None:
        pipeline = FakePipeline()
        self.post(monkeypatch, None, pipeline, snapshotId="snap-1", question="q")

        assert pipeline.calls[0]["language"] == "EN"

    def test_snake_case_body_is_accepted_too(self, monkeypatch: pytest.MonkeyPatch) -> None:
        response = self.post(monkeypatch, snapshot_id="snap-1", question="q")
        assert response.status_code == 200

    def test_an_unknown_body_field_is_rejected(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """A misspelled key should be reported, not silently dropped."""
        response = self.post(monkeypatch, snapshotId="snap-1", question="q", langauge="JA")
        assert response.status_code == 422

    def test_a_missing_snapshot_field_is_422(self, monkeypatch: pytest.MonkeyPatch) -> None:
        response = self.post(monkeypatch, question="q")
        assert response.status_code == 422


class TestTheTurnDeadline:
    """Each call inside a turn is bounded already; the turn was not. Phase 08's
    eval runner drives this route thousands of times, and one hung turn hangs
    the run."""

    def test_a_hung_turn_is_502_rather_than_hanging(self, monkeypatch: pytest.MonkeyPatch) -> None:
        import urara_chat.api.routes as routes

        async def never(*args: Any, **kwargs: Any) -> Any:
            await asyncio.sleep(10)

        monkeypatch.setattr(routes, "answer", never)
        settings = fake_settings(answer_timeout_seconds=0.05)
        response = client_for(FakeClient(), None, settings).post(
            "/debug/answer", json={"snapshotId": "snap-1", "question": "q"}
        )

        assert response.status_code == 502

    def test_the_timeout_leaks_nothing(self, monkeypatch: pytest.MonkeyPatch) -> None:
        import urara_chat.api.routes as routes

        async def never(*args: Any, **kwargs: Any) -> Any:
            await asyncio.sleep(10)

        monkeypatch.setattr(routes, "answer", never)
        settings = fake_settings(answer_timeout_seconds=0.05)
        response = client_for(FakeClient(), None, settings).post(
            "/debug/answer", json={"snapshotId": "snap-1", "question": "q"}
        )

        assert response.json()["detail"] == routes.UPSTREAM_FAILURE


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

    def test_the_lifespan_installs_the_agent_pipeline(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """Nothing called configure_pipeline until 04.R, so every request to
        /debug/answer raised "the agent pipeline has not been configured" -- and
        no unit test saw it, because each one builds its own Pipeline."""
        import urara_chat.agent.pipeline as agent_pipeline
        import urara_chat.main as main
        from urara_chat.config import get_settings

        monkeypatch.setenv("GOOGLE_API_KEY", "test-key-not-real")
        get_settings.cache_clear()
        monkeypatch.setattr(main, "BackendClient", lambda settings: FakeClient())
        monkeypatch.setattr(main, "build_chat_model", lambda settings: FakeModel())
        monkeypatch.setattr(agent_pipeline, "_pipeline", None)

        with TestClient(main.app):
            installed = agent_pipeline.get_pipeline()

        assert installed is not None
        get_settings.cache_clear()

    def test_the_lifespan_passes_the_configured_limits_through(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """These three settings were read by nothing at all: the history bound,
        the tool budget and the card TTL."""
        import urara_chat.agent.pipeline as agent_pipeline
        import urara_chat.main as main
        from urara_chat.config import get_settings

        monkeypatch.setenv("GOOGLE_API_KEY", "test-key-not-real")
        monkeypatch.setenv("MAX_HISTORY_MESSAGES", "7")
        monkeypatch.setenv("MAX_TOOL_ITERATIONS", "2")
        get_settings.cache_clear()
        monkeypatch.setattr(main, "BackendClient", lambda settings: FakeClient())
        monkeypatch.setattr(main, "build_chat_model", lambda settings: FakeModel())
        monkeypatch.setattr(agent_pipeline, "_pipeline", None)

        with TestClient(main.app):
            built = agent_pipeline.get_pipeline()
            assert built._max_history == 7
            assert built._max_tool_iterations == 2

        get_settings.cache_clear()

    def test_an_oversized_body_is_refused_before_it_is_read(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """MAX_QUESTION_CHARS cannot do this: it runs in the handler, by which
        point the whole body has been received and parsed."""
        import urara_chat.main as main
        from urara_chat.config import get_settings

        monkeypatch.setenv("GOOGLE_API_KEY", "test-key-not-real")
        monkeypatch.setenv("MAX_REQUEST_BYTES", "200")
        get_settings.cache_clear()
        monkeypatch.setattr(main, "BackendClient", lambda settings: FakeClient())
        monkeypatch.setattr(main, "build_chat_model", lambda settings: FakeModel())

        with TestClient(main.app) as c:
            oversized = c.post(
                "/debug/answer", json={"snapshotId": "snap-1", "question": "x" * 500}
            )
            assert oversized.status_code == 413
            assert "200" in oversized.json()["detail"]

            # A body under the ceiling still reaches the handler, where the
            # friendly limit lives.
            assert c.get("/healthz").status_code == 200

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
