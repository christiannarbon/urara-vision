"""The client against the real Go API.

Everything up to here was mocked, which proves the client asks for the right
thing but not that the backend answers it. The assertions are on real values
from the jaffle-shop demo set; the counts are the ones
`backend/tests/unit/demo/demo_test.go` already pins, so the two suites cannot
drift into disagreeing about the same fixture.
"""

import pytest

from urara_chat.backend.client import BackendClient
from urara_chat.backend.errors import BackendNotFound

pytestmark = pytest.mark.integration

# Pinned by TestDemoStats in backend/tests/unit/demo/demo_test.go.
DEMO_DOMAINS = 6
DEMO_TABLES = 10
DEMO_COLUMNS = 80
DEMO_SOURCE_TABLES = 7
# TestDemoDiagnostics pins exactly one error, which is what makes
# `uraractl -strict` exit non-zero on the sample.
DEMO_ERRORS = 1

FACT_ORDERS = "ordering/fact_orders"
DIM_CUSTOMERS = "customer_identity/dim_customers"


async def test_resolve_snapshot_turns_latest_into_an_id(
    client: BackendClient, snapshot_id: str
) -> None:
    resolved = await client.resolve_snapshot("latest")
    assert resolved != "latest"
    assert len(resolved) == 36, "a snapshot ID is a UUID"


async def test_resolve_snapshot_keeps_a_concrete_id(
    client: BackendClient, snapshot_id: str
) -> None:
    assert await client.resolve_snapshot(snapshot_id) == snapshot_id


async def test_get_context_returns_the_pinned_counts(
    client: BackendClient, snapshot_id: str
) -> None:
    ctx = await client.get_context(snapshot_id)

    assert ctx.snapshot.id == snapshot_id
    assert ctx.snapshot.stats.domains == DEMO_DOMAINS
    assert ctx.snapshot.stats.tables == DEMO_TABLES
    assert ctx.snapshot.stats.columns == DEMO_COLUMNS
    assert len(ctx.domains) == DEMO_DOMAINS
    assert len(ctx.tables) == DEMO_TABLES
    assert ctx.truncated is False
    assert set(ctx.diagnostics) == {"error", "warning", "info"}


async def test_get_table_survives_a_slash_bearing_id(
    client: BackendClient, snapshot_id: str
) -> None:
    """The assertion that proves a table ID reached the backend intact. A
    mangled ID 404s in a way indistinguishable from a table that is absent."""
    detail = await client.get_table(snapshot_id, FACT_ORDERS)

    assert detail.table.id == FACT_ORDERS
    assert detail.table.grain == "One row per order."
    assert detail.table.columns


async def test_get_table_unknown_raises_not_found(client: BackendClient, snapshot_id: str) -> None:
    with pytest.raises(BackendNotFound):
        await client.get_table(snapshot_id, "nope/missing")


async def test_get_tables_reports_the_id_it_could_not_find(
    client: BackendClient, snapshot_id: str
) -> None:
    batch = await client.get_tables(snapshot_id, [FACT_ORDERS, "nope/missing"])

    assert [d.table.id for d in batch.tables] == [FACT_ORDERS]
    assert batch.missing == ["nope/missing"]


async def test_list_domains(client: BackendClient, snapshot_id: str) -> None:
    domains = await client.list_domains(snapshot_id)

    assert len(domains) == DEMO_DOMAINS
    ids = {d.id for d in domains}
    assert {"ordering", "customer_identity"} <= ids


async def test_list_tables_and_the_domain_filter(client: BackendClient, snapshot_id: str) -> None:
    everything = await client.list_tables(snapshot_id)
    assert len(everything) == DEMO_TABLES

    ordering = await client.list_tables(snapshot_id, domain="ordering")
    assert 0 < len(ordering) < DEMO_TABLES
    assert all(t.domain_id == "ordering" for t in ordering)


async def test_search_finds_a_table_by_name(client: BackendClient, snapshot_id: str) -> None:
    hits = await client.search(snapshot_id, "orders")
    assert FACT_ORDERS in {h.table_id for h in hits}


async def test_diagnostics_are_not_empty(client: BackendClient, snapshot_id: str) -> None:
    """Every demo set is built around deliberate flaws, so an empty list means
    the query is wrong rather than the documentation being clean."""
    diags = await client.diagnostics(snapshot_id)
    assert diags

    errors = await client.diagnostics(snapshot_id, severity="error")
    assert len(errors) == DEMO_ERRORS
    assert all(d.severity == "error" for d in errors)


async def test_neighbourhood_returns_a_subgraph(client: BackendClient, snapshot_id: str) -> None:
    graph = await client.neighbourhood(snapshot_id, FACT_ORDERS, depth=1)

    assert FACT_ORDERS in {n.id for n in graph.nodes}
    assert graph.links


async def test_join_paths_between_two_real_tables(client: BackendClient, snapshot_id: str) -> None:
    paths = await client.join_paths(snapshot_id, FACT_ORDERS, DIM_CUSTOMERS)

    assert paths
    assert paths[0].hops
    assert paths[0].tables[0] == FACT_ORDERS


async def test_lineage_in_both_directions(client: BackendClient, snapshot_id: str) -> None:
    upstream = await client.lineage(snapshot_id, FACT_ORDERS, direction="upstream")
    assert upstream

    source_id = upstream[0].id
    downstream = await client.lineage(snapshot_id, source_id, direction="downstream")
    assert FACT_ORDERS in {e.id for e in downstream}


async def test_list_sources(client: BackendClient, snapshot_id: str) -> None:
    sources = await client.list_sources(snapshot_id)

    assert len(sources) == DEMO_SOURCE_TABLES
    assert all(s.dataset for s in sources)


async def test_health_is_true_against_a_running_backend(client: BackendClient) -> None:
    assert await client.health() is True
