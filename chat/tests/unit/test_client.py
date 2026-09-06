"""The backend client, against a mocked transport.

Nothing here touches the network. What is worth asserting is not that httpx
works but that this client asks for the right thing: the paths, the query
parameters, and above all that a table ID's slash is encoded into a parameter
rather than becoming a path segment -- which would 404 in a way that reads
exactly like a table that does not exist.
"""

import json
from collections.abc import AsyncIterator
from pathlib import Path
from urllib.parse import parse_qs, urlparse

import httpx
import pytest
import respx

from urara_chat.backend.client import BackendClient
from urara_chat.backend.errors import BackendError, BackendNotFound, BackendUnavailable
from urara_chat.config import Settings

BASE = "http://backend:8080"
SID = "snap-1"
TABLE_ID = "ordering/fact_orders"


def settings(token: str = "") -> Settings:
    return Settings(
        backend_base_url=BASE,
        backend_api_token=token,
        backend_timeout_seconds=5.0,
        log_level="info",
        app_addr=":8090",
    )


@pytest.fixture
async def client() -> AsyncIterator[BackendClient]:
    c = BackendClient(settings())
    yield c
    await c.aclose()


def query(request: httpx.Request) -> dict[str, list[str]]:
    return parse_qs(urlparse(str(request.url)).query)


class TestReadsHitTheRightRoute:
    """Each method requests the path and parameters the Go handler expects."""

    @respx.mock
    async def test_get_context(self, client: BackendClient) -> None:
        route = respx.get(f"{BASE}/api/v1/snapshots/{SID}/context").mock(
            return_value=httpx.Response(
                200,
                json={
                    "snapshot": {"id": SID, "createdAt": "2026-01-01T00:00:00Z"},
                    "domains": [],
                    "tables": [],
                    "diagnostics": {"error": 0, "warning": 0, "info": 0},
                    "truncated": False,
                },
            )
        )
        ctx = await client.get_context(SID)
        assert route.called
        assert ctx.snapshot.id == SID

    @respx.mock
    async def test_resolve_snapshot_returns_the_concrete_id(self, client: BackendClient) -> None:
        respx.get(f"{BASE}/api/v1/snapshots/latest").mock(
            return_value=httpx.Response(
                200, json={"id": "real-id", "createdAt": "2026-01-01T00:00:00Z"}
            )
        )
        assert await client.resolve_snapshot("latest") == "real-id"

    @respx.mock
    async def test_list_domains_unwraps_the_envelope(self, client: BackendClient) -> None:
        respx.get(f"{BASE}/api/v1/snapshots/{SID}/domains").mock(
            return_value=httpx.Response(200, json={"domains": [{"id": "ordering"}]})
        )
        domains = await client.list_domains(SID)
        assert [d.id for d in domains] == ["ordering"]

    @respx.mock
    async def test_list_tables_filters_by_domain(self, client: BackendClient) -> None:
        route = respx.get(f"{BASE}/api/v1/snapshots/{SID}/tables").mock(
            return_value=httpx.Response(200, json={"tables": []})
        )
        await client.list_tables(SID, domain="ordering")
        assert query(route.calls.last.request)["domain"] == ["ordering"]

    @respx.mock
    async def test_list_tables_omits_the_filter_when_absent(self, client: BackendClient) -> None:
        route = respx.get(f"{BASE}/api/v1/snapshots/{SID}/tables").mock(
            return_value=httpx.Response(200, json={"tables": []})
        )
        await client.list_tables(SID)
        assert "domain" not in query(route.calls.last.request)

    @respx.mock
    async def test_get_tables_joins_ids_with_commas(self, client: BackendClient) -> None:
        route = respx.get(f"{BASE}/api/v1/snapshots/{SID}/tables/detail").mock(
            return_value=httpx.Response(200, json={"tables": [], "missing": ["b"]})
        )
        result = await client.get_tables(SID, ["a/one", "b/two"])
        assert query(route.calls.last.request)["ids"] == ["a/one,b/two"]
        assert result.missing == ["b"]

    @respx.mock
    async def test_search(self, client: BackendClient) -> None:
        route = respx.get(f"{BASE}/api/v1/snapshots/{SID}/search").mock(
            return_value=httpx.Response(200, json={"hits": [{"tableId": TABLE_ID}]})
        )
        hits = await client.search(SID, "orders", limit=5)
        q = query(route.calls.last.request)
        assert q["q"] == ["orders"] and q["limit"] == ["5"]
        assert hits[0].table_id == TABLE_ID

    @respx.mock
    async def test_neighbourhood_uses_the_american_route(self, client: BackendClient) -> None:
        route = respx.get(f"{BASE}/api/v1/snapshots/{SID}/neighborhood").mock(
            return_value=httpx.Response(200, json={"nodes": [], "links": []})
        )
        await client.neighbourhood(SID, TABLE_ID, depth=2, sources=True)
        q = query(route.calls.last.request)
        assert q["table"] == [TABLE_ID]
        assert q["depth"] == ["2"]
        assert q["sources"] == ["true"]

    @respx.mock
    async def test_join_paths_unwraps_paths(self, client: BackendClient) -> None:
        route = respx.get(f"{BASE}/api/v1/snapshots/{SID}/paths").mock(
            return_value=httpx.Response(
                200,
                json={
                    "paths": [
                        {"length": 1, "tables": ["a", "b"], "hops": [{"from": "a", "to": "b"}]}
                    ]
                },
            )
        )
        paths = await client.join_paths(SID, "a/one", "b/two", max_depth=3, limit=2)
        q = query(route.calls.last.request)
        assert q["from"] == ["a/one"] and q["to"] == ["b/two"]
        assert q["maxDepth"] == ["3"] and q["limit"] == ["2"]
        assert paths[0].hops[0].from_table == "a"

    @respx.mock
    async def test_lineage_unwraps_entries(self, client: BackendClient) -> None:
        route = respx.get(f"{BASE}/api/v1/snapshots/{SID}/lineage").mock(
            return_value=httpx.Response(
                200, json={"direction": "downstream", "entries": [{"id": "src.model"}]}
            )
        )
        entries = await client.lineage(SID, TABLE_ID, direction="downstream")
        q = query(route.calls.last.request)
        assert q["id"] == [TABLE_ID] and q["direction"] == ["downstream"]
        assert [e.id for e in entries] == ["src.model"]

    @respx.mock
    async def test_diagnostics_filters_by_severity(self, client: BackendClient) -> None:
        route = respx.get(f"{BASE}/api/v1/snapshots/{SID}/diagnostics").mock(
            return_value=httpx.Response(200, json={"diagnostics": [{"severity": "error"}]})
        )
        diags = await client.diagnostics(SID, severity="error")
        assert query(route.calls.last.request)["severity"] == ["error"]
        assert diags[0].severity == "error"

    @respx.mock
    async def test_list_sources(self, client: BackendClient) -> None:
        respx.get(f"{BASE}/api/v1/snapshots/{SID}/sources").mock(
            return_value=httpx.Response(200, json={"sources": [{"id": "warehouse.stg"}]})
        )
        assert [s.id for s in await client.list_sources(SID)] == ["warehouse.stg"]


class TestTableIDsTravelAsParameters:
    """The whole reason this client exists rather than f-strings around httpx."""

    @respx.mock
    async def test_the_slash_is_encoded_into_the_query_not_the_path(
        self, client: BackendClient
    ) -> None:
        route = respx.get(f"{BASE}/api/v1/snapshots/{SID}/table").mock(
            return_value=httpx.Response(
                200, json={"table": {"id": TABLE_ID, "name": "fact_orders"}}
            )
        )
        await client.get_table(SID, TABLE_ID)

        url = str(route.calls.last.request.url)
        # The path stops at /table; the ID is a parameter with its slash encoded.
        assert urlparse(url).path == f"/api/v1/snapshots/{SID}/table"
        assert "id=ordering%2Ffact_orders" in url
        assert "/table/ordering/" not in url
        assert query(route.calls.last.request)["id"] == [TABLE_ID]


class TestErrorMapping:
    @respx.mock
    async def test_404_raises_not_found(self, client: BackendClient) -> None:
        respx.get(f"{BASE}/api/v1/snapshots/nope/context").mock(
            return_value=httpx.Response(404, json={"error": "snapshot not found"})
        )
        with pytest.raises(BackendNotFound) as caught:
            await client.get_context("nope")
        assert caught.value.status == 404
        assert "snapshot not found" in caught.value.message

    @respx.mock
    async def test_500_carries_the_backends_own_message(self, client: BackendClient) -> None:
        respx.get(f"{BASE}/api/v1/snapshots/{SID}/context").mock(
            return_value=httpx.Response(500, json={"error": "boom"})
        )
        with pytest.raises(BackendError) as caught:
            await client.get_context(SID)
        assert "boom" in caught.value.message
        assert not isinstance(caught.value, BackendNotFound)

    @respx.mock
    async def test_a_non_json_body_still_raises_cleanly(self, client: BackendClient) -> None:
        """A proxy or a panic can answer with HTML; building the error message
        must not raise over the failure it is reporting."""
        respx.get(f"{BASE}/api/v1/snapshots/{SID}/context").mock(
            return_value=httpx.Response(502, text="<html>Bad Gateway</html>")
        )
        with pytest.raises(BackendError) as caught:
            await client.get_context(SID)
        assert caught.value.status == 502
        assert caught.value.message


class TestTransportFailures:
    """A refused connection or a timeout is still an upstream failure. Letting
    httpx's own exception escape makes the handler answer 500, which blames this
    service for an outage in the one it depends on."""

    @respx.mock
    async def test_a_refused_connection_raises_backend_unavailable(
        self, client: BackendClient
    ) -> None:
        respx.get(f"{BASE}/api/v1/snapshots/{SID}/domains").mock(
            side_effect=httpx.ConnectError("connection refused")
        )
        with pytest.raises(BackendUnavailable) as caught:
            await client.list_domains(SID)
        assert "unreachable" in caught.value.message
        assert isinstance(caught.value, BackendError), "must map to 502 like any upstream failure"

    @respx.mock
    async def test_a_timeout_raises_backend_unavailable(self, client: BackendClient) -> None:
        respx.get(f"{BASE}/api/v1/snapshots/{SID}/domains").mock(
            side_effect=httpx.ReadTimeout("too slow")
        )
        with pytest.raises(BackendUnavailable):
            await client.list_domains(SID)

    @respx.mock
    async def test_a_write_is_wrapped_too(self, client: BackendClient) -> None:
        respx.post(f"{BASE}/api/v1/conversations").mock(
            side_effect=httpx.ConnectError("connection refused")
        )
        with pytest.raises(BackendUnavailable):
            await client.create_conversation(SID)


class TestAuthorization:
    @respx.mock
    async def test_no_header_when_the_token_is_empty(self) -> None:
        """Empty is the backend's documented unauthenticated mode; "Bearer "
        with nothing after it would be refused rather than treated as absent."""
        route = respx.get(f"{BASE}/readyz").mock(return_value=httpx.Response(200))
        c = BackendClient(settings(token=""))
        await c.health()
        await c.aclose()
        assert "authorization" not in route.calls.last.request.headers

    @respx.mock
    async def test_bearer_header_when_a_token_is_configured(self) -> None:
        route = respx.get(f"{BASE}/readyz").mock(return_value=httpx.Response(200))
        c = BackendClient(settings(token="a-token"))
        await c.health()
        await c.aclose()
        assert route.calls.last.request.headers["authorization"] == "Bearer a-token"


class TestHealth:
    @respx.mock
    async def test_ready_is_true(self, client: BackendClient) -> None:
        respx.get(f"{BASE}/readyz").mock(return_value=httpx.Response(200, json={"postgres": "ok"}))
        assert await client.health() is True

    @respx.mock
    async def test_503_is_false_rather_than_an_exception(self, client: BackendClient) -> None:
        """A dependency being down is information, not an error in the caller."""
        respx.get(f"{BASE}/readyz").mock(
            return_value=httpx.Response(503, json={"postgres": "down"})
        )
        assert await client.health() is False

    @respx.mock
    async def test_an_unreachable_backend_is_false(self, client: BackendClient) -> None:
        respx.get(f"{BASE}/readyz").mock(side_effect=httpx.ConnectError("refused"))
        assert await client.health() is False


class TestTolerance:
    @respx.mock
    async def test_an_unknown_field_does_not_break_parsing(self, client: BackendClient) -> None:
        respx.get(f"{BASE}/api/v1/snapshots/{SID}/domains").mock(
            return_value=httpx.Response(
                200, json={"domains": [{"id": "ordering", "addedInSomeLaterRelease": 1}]}
            )
        )
        assert [d.id for d in await client.list_domains(SID)] == ["ordering"]


class TestLifecycle:
    async def test_one_client_is_shared_and_aclose_closes_it(self) -> None:
        c = BackendClient(settings())
        underlying = c._client
        assert underlying.is_closed is False
        await c.aclose()
        assert underlying.is_closed is True

    @respx.mock
    async def test_repeated_calls_reuse_the_same_client(self, client: BackendClient) -> None:
        """A client per request would leak connections and drop pooling."""
        respx.get(f"{BASE}/api/v1/snapshots/{SID}/sources").mock(
            return_value=httpx.Response(200, json={"sources": []})
        )
        before = client._client
        await client.list_sources(SID)
        await client.list_sources(SID)
        assert client._client is before


@respx.mock
async def test_get_table_returns_the_captured_fixture(client: BackendClient) -> None:
    """Driven by the real captured response rather than a hand-written body, so
    the client and the models are checked against the same wire format."""
    raw = json.loads((Path(__file__).parent / "fixtures" / "table.json").read_text())
    respx.get(f"{BASE}/api/v1/snapshots/{SID}/table").mock(
        return_value=httpx.Response(200, json=raw)
    )

    detail = await client.get_table(SID, TABLE_ID)

    assert detail.table.id == TABLE_ID
    assert detail.table.grain == "One row per order."
    assert detail.table.columns


class TestCreateConversation:
    @respx.mock
    async def test_posts_the_documented_body(self, client: BackendClient) -> None:
        route = respx.post(f"{BASE}/api/v1/conversations").mock(
            return_value=httpx.Response(
                201, json={"id": "conv-1", "snapshotId": SID, "title": "why"}
            )
        )
        conv = await client.create_conversation(SID, title="why")

        assert json.loads(route.calls.last.request.content) == {
            "snapshotId": SID,
            "title": "why",
        }
        assert conv.id == "conv-1"

    @respx.mock
    async def test_latest_is_passed_through_untouched(self, client: BackendClient) -> None:
        """The backend resolves the alias and stores the concrete ID. Resolving
        here as well would put a second opinion in the system about which
        snapshot a thread is pinned to."""
        route = respx.post(f"{BASE}/api/v1/conversations").mock(
            return_value=httpx.Response(
                201, json={"id": "conv-1", "snapshotId": "resolved-id", "title": ""}
            )
        )
        conv = await client.create_conversation("latest")

        assert json.loads(route.calls.last.request.content)["snapshotId"] == "latest"
        # No GET went out to resolve it first.
        assert len(respx.calls) == 1
        assert conv.snapshot_id == "resolved-id"

    @respx.mock
    async def test_an_unknown_snapshot_raises_not_found(self, client: BackendClient) -> None:
        respx.post(f"{BASE}/api/v1/conversations").mock(
            return_value=httpx.Response(404, json={"error": "snapshot not found"})
        )
        with pytest.raises(BackendNotFound):
            await client.create_conversation("nope")


class TestListAndGetConversations:
    @respx.mock
    async def test_list_sends_the_snapshot_parameter(self, client: BackendClient) -> None:
        route = respx.get(f"{BASE}/api/v1/conversations").mock(
            return_value=httpx.Response(200, json={"conversations": [{"id": "conv-1"}]})
        )
        convs = await client.list_conversations(SID)

        assert query(route.calls.last.request)["snapshot"] == [SID]
        assert [c.id for c in convs] == ["conv-1"]

    @respx.mock
    async def test_get_returns_the_transcript(self, client: BackendClient) -> None:
        raw = json.loads((Path(__file__).parent / "fixtures" / "conversation.json").read_text())
        respx.get(f"{BASE}/api/v1/conversations/conv-1").mock(
            return_value=httpx.Response(200, json=raw)
        )
        conv = await client.get_conversation("conv-1")

        assert conv.messages
        assert conv.messages[0].ordinal == 0

    @respx.mock
    async def test_get_unknown_raises_not_found(self, client: BackendClient) -> None:
        respx.get(f"{BASE}/api/v1/conversations/nope").mock(
            return_value=httpx.Response(404, json={"error": "conversation not found"})
        )
        with pytest.raises(BackendNotFound):
            await client.get_conversation("nope")


class TestDeleteConversation:
    @respx.mock
    async def test_204_is_success_and_returns_none(self, client: BackendClient) -> None:
        """A successful delete has no body, so nothing may try to decode one."""
        route = respx.delete(f"{BASE}/api/v1/conversations/conv-1").mock(
            return_value=httpx.Response(204)
        )
        assert await client.delete_conversation("conv-1") is None
        assert route.called

    @respx.mock
    async def test_404_raises_not_found(self, client: BackendClient) -> None:
        respx.delete(f"{BASE}/api/v1/conversations/nope").mock(
            return_value=httpx.Response(404, json={"error": "conversation not found"})
        )
        with pytest.raises(BackendNotFound):
            await client.delete_conversation("nope")


class TestAppendMessage:
    @respx.mock
    async def test_returns_the_server_assigned_ordinal(self, client: BackendClient) -> None:
        """The ordinal is the database's to assign, so what comes back is the
        stored row rather than what was sent."""
        respx.post(f"{BASE}/api/v1/conversations/conv-1/messages").mock(
            return_value=httpx.Response(
                201,
                json={"ordinal": 7, "role": "user", "content": "hi", "citations": []},
            )
        )
        msg = await client.append_message("conv-1", "user", "hi")
        assert msg.ordinal == 7
        assert msg.role == "user"

    @respx.mock
    async def test_citations_are_sent_as_a_list_when_none(self, client: BackendClient) -> None:
        """Never null, never an absent key: "cited nothing" has to be
        distinguishable from "was not asked"."""
        route = respx.post(f"{BASE}/api/v1/conversations/conv-1/messages").mock(
            return_value=httpx.Response(
                201, json={"ordinal": 0, "role": "user", "content": "hi", "citations": []}
            )
        )
        await client.append_message("conv-1", "user", "hi")

        body = json.loads(route.calls.last.request.content)
        assert "citations" in body
        assert body["citations"] == []
        assert body["citations"] is not None

    @respx.mock
    async def test_citations_and_meta_survive(self, client: BackendClient) -> None:
        route = respx.post(f"{BASE}/api/v1/conversations/conv-1/messages").mock(
            return_value=httpx.Response(
                201,
                json={
                    "ordinal": 1,
                    "role": "assistant",
                    "content": "two of them",
                    "citations": [TABLE_ID],
                },
            )
        )
        msg = await client.append_message(
            "conv-1",
            "assistant",
            "two of them",
            citations=[TABLE_ID],
            meta={"tokens": 10},
        )

        body = json.loads(route.calls.last.request.content)
        assert body["citations"] == [TABLE_ID]
        assert body["meta"] == {"tokens": 10}
        assert msg.citations == [TABLE_ID]

    @respx.mock
    async def test_meta_is_omitted_rather_than_sent_as_null(self, client: BackendClient) -> None:
        """The backend rejects unknown fields but accepts an absent one; sending
        null would be a value it has to interpret."""
        route = respx.post(f"{BASE}/api/v1/conversations/conv-1/messages").mock(
            return_value=httpx.Response(
                201, json={"ordinal": 0, "role": "user", "content": "hi", "citations": []}
            )
        )
        await client.append_message("conv-1", "user", "hi")
        assert "meta" not in json.loads(route.calls.last.request.content)

    @respx.mock
    async def test_unknown_conversation_raises_not_found(self, client: BackendClient) -> None:
        respx.post(f"{BASE}/api/v1/conversations/nope/messages").mock(
            return_value=httpx.Response(404, json={"error": "conversation not found"})
        )
        with pytest.raises(BackendNotFound):
            await client.append_message("nope", "user", "hi")

    @respx.mock
    async def test_an_invalid_role_surfaces_the_backends_message(
        self, client: BackendClient
    ) -> None:
        """Role validation lives in the Go handler; the client's job is to carry
        the reason back rather than to duplicate the rule."""
        respx.post(f"{BASE}/api/v1/conversations/conv-1/messages").mock(
            return_value=httpx.Response(
                400,
                json={"error": '"robot" is not a valid role; expected one of "user", ...'},
            )
        )
        with pytest.raises(BackendError) as caught:
            await client.append_message("conv-1", "robot", "hi")
        assert "not a valid role" in caught.value.message
        assert caught.value.status == 400
