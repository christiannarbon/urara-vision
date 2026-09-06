"""All nine tools against a real backend.

This is the phase's point: retrieval checked before a model's judgement is in
the way. A tool that returns something thin or wrong is invisible later, because
the model will write fluent prose around it and the prose reads as an answer.

Expected counts come from `backend/tests/unit/demo/demo_test.go`, which already
pins this set's statistics. Two suites disagreeing about one fixture is worse
than one suite asserting less.
"""

import json
from typing import Any

import pytest

from urara_chat.backend.client import BackendClient
from urara_chat.tools.registry import TOOL_NAMES, ToolSpec, build_tools

pytestmark = pytest.mark.integration

DEMO_DOMAINS = 6
DEMO_TABLES = 10
DEMO_SOURCE_TABLES = 7
DEMO_ERRORS = 1

FACT_ORDERS = "ordering/fact_orders"
DIM_CUSTOMERS = "customer_identity/dim_customers"


@pytest.fixture
def tools(client: BackendClient, snapshot_id: str) -> dict[str, ToolSpec]:
    return {spec.name: spec for spec in build_tools(client, snapshot_id)}


async def test_list_domains(tools: dict[str, ToolSpec]) -> None:
    result = await tools["list_domains"].fn()

    assert result["total"] == DEMO_DOMAINS
    ids = {d["id"] for d in result["items"]}
    assert {"ordering", "customer_identity"} <= ids
    # This set deliberately carries a domain with no table directory, so a
    # domain count and a directory count are not the same number.
    assert "delivery_logistics" in ids


async def test_list_tables(tools: dict[str, ToolSpec]) -> None:
    everything = await tools["list_tables"].fn()
    assert everything["total"] == DEMO_TABLES
    assert everything["truncated"] is False

    ordering = await tools["list_tables"].fn(domain="ordering")
    assert 0 < ordering["total"] < DEMO_TABLES
    assert all(t["domainId"] == "ordering" for t in ordering["items"])


async def test_get_tables_returns_a_shrunk_document(tools: dict[str, ToolSpec]) -> None:
    result = await tools["get_tables"].fn(ids=[FACT_ORDERS])

    assert result["missing"] == []
    table = result["items"][0]["table"]
    assert table["id"] == FACT_ORDERS
    assert table["grain"] == "One row per order."
    assert table["columns"], "a table document without columns is not worth returning"
    assert "docPath" not in json.dumps(result), "the model cannot open a file"


async def test_get_tables_reports_a_missing_id(tools: dict[str, ToolSpec]) -> None:
    result = await tools["get_tables"].fn(ids=[FACT_ORDERS, "nope/missing"])

    assert result["total"] == 1
    assert result["missing"] == ["nope/missing"]


async def test_search_model(tools: dict[str, ToolSpec]) -> None:
    result = await tools["search_model"].fn(query="orders")
    assert FACT_ORDERS in {h["tableId"] for h in result["items"]}


async def test_get_neighbourhood(tools: dict[str, ToolSpec]) -> None:
    result = await tools["get_neighbourhood"].fn(table_id=FACT_ORDERS, depth=1)

    ids = {n["id"] for n in result["items"]}
    assert FACT_ORDERS in ids
    assert len(ids) > 1, "a fact table at depth 1 should reach its dimensions"
    assert any(n.get("kind") == "dimension" for n in result["items"])
    assert result["links"]


async def test_find_join_paths_carries_the_join_columns(tools: dict[str, ToolSpec]) -> None:
    result = await tools["find_join_paths"].fn(from_table=FACT_ORDERS, to_table=DIM_CUSTOMERS)

    assert result["total"] >= 1
    hops = result["items"][0]["hops"]
    assert hops
    assert hops[0]["from"] and hops[0]["to"]
    assert any(h["fromColumn"] and h["toColumn"] for h in hops), (
        "a join path without columns cannot be turned into SQL"
    )


async def test_get_lineage_both_directions(tools: dict[str, ToolSpec]) -> None:
    upstream = await tools["get_lineage"].fn(table_id=FACT_ORDERS)
    assert upstream["direction"] == "upstream"
    assert upstream["total"] >= 1

    source_id = upstream["items"][0]["id"]
    downstream = await tools["get_lineage"].fn(table_id=source_id, direction="downstream")
    assert downstream["direction"] == "downstream"
    assert FACT_ORDERS in {e["id"] for e in downstream["items"]}


async def test_list_diagnostics(tools: dict[str, ToolSpec]) -> None:
    everything = await tools["list_diagnostics"].fn()
    assert everything["total"] > 0, "the demo set is built around deliberate flaws"

    errors = await tools["list_diagnostics"].fn(severity="error")
    assert errors["total"] == DEMO_ERRORS
    assert all(d["severity"] == "error" for d in errors["items"])


async def test_list_source_models(tools: dict[str, ToolSpec]) -> None:
    result = await tools["list_source_models"].fn()

    assert result["total"] == DEMO_SOURCE_TABLES
    assert all(s["dataset"] for s in result["items"])
    assert all(s["refs"] > 0 for s in result["items"])


# The arguments each tool needs to run once, for the sweep below.
LIVE_CALLS: dict[str, dict[str, Any]] = {
    "list_domains": {},
    "list_tables": {},
    "get_tables": {"ids": [FACT_ORDERS]},
    "search_model": {"query": "orders"},
    "get_neighbourhood": {"table_id": FACT_ORDERS},
    "find_join_paths": {"from_table": FACT_ORDERS, "to_table": DIM_CUSTOMERS},
    "get_lineage": {"table_id": FACT_ORDERS},
    "list_diagnostics": {},
    "list_source_models": {},
}


def test_the_sweep_covers_every_tool() -> None:
    """Guards the parameterisation below against a tenth tool being added and
    quietly not exercised."""
    assert set(LIVE_CALLS) == set(TOOL_NAMES)


@pytest.mark.parametrize("name", TOOL_NAMES)
async def test_every_result_is_json_serialisable(tools: dict[str, ToolSpec], name: str) -> None:
    """Whatever a tool returns goes into a prompt as text. Something
    unserialisable fails at the model call, where it reads as a model problem."""
    result = await tools[name].fn(**LIVE_CALLS[name])

    encoded = json.dumps(result)
    assert encoded
    assert json.loads(encoded) == result


@pytest.mark.parametrize("name", TOOL_NAMES)
async def test_every_result_has_the_uniform_shape(tools: dict[str, ToolSpec], name: str) -> None:
    """One shape either way. A result that is sometimes a list and sometimes an
    object is one the model handles inconsistently."""
    result = await tools[name].fn(**LIVE_CALLS[name])

    assert isinstance(result, dict)
    assert set(result) >= {"items", "truncated", "total"}
    assert isinstance(result["truncated"], bool)
    assert result["total"] == len(result["items"]) or result["truncated"]
