"""The nine tools.

Bugs here are the hardest in the service to see later, because a model papers
over a bad tool result with plausible prose. So the assertions are about the
things that would produce a confident wrong answer: a tool reading the wrong
snapshot, a list silently cut, or a result the model cannot parse.

The client is faked. No network, and no LLM package is imported anywhere in the
chain under test.
"""

import json
from typing import Any

import pytest
from pydantic import ValidationError

from urara_chat.backend.models import (
    Diagnostic,
    Domain,
    Graph,
    JoinPath,
    LineageEntry,
    SearchHit,
    SourceTable,
    TableDetail,
    TablesDetailResponse,
    TableSummary,
)
from urara_chat.tools.registry import MAX_ITEMS, TOOL_NAMES, ToolSpec, build_tools


class FakeClient:
    """Records the snapshot each call was made against, and returns what it is told."""

    def __init__(self, **returns: Any) -> None:
        self.returns = returns
        self.snapshots: list[str] = []
        self.calls: list[tuple[str, tuple[Any, ...]]] = []

    def _record(self, name: str, sid: str, *args: Any) -> Any:
        self.snapshots.append(sid)
        self.calls.append((name, args))
        return self.returns.get(name, [])

    async def list_domains(self, sid: str) -> Any:
        return self._record("list_domains", sid)

    async def list_tables(self, sid: str, domain: str | None = None) -> Any:
        return self._record("list_tables", sid, domain)

    async def get_tables(self, sid: str, ids: Any) -> Any:
        return self._record("get_tables", sid, list(ids)) or TablesDetailResponse()

    async def search(self, sid: str, query: str, limit: int = 20) -> Any:
        return self._record("search", sid, query, limit)

    async def neighbourhood(
        self, sid: str, table_id: str, depth: int = 1, sources: bool = False
    ) -> Any:
        return self._record("neighbourhood", sid, table_id, depth) or Graph()

    async def join_paths(
        self, sid: str, frm: str, to: str, max_depth: int = 4, limit: int = 10
    ) -> Any:
        return self._record("join_paths", sid, frm, to, max_depth)

    async def lineage(self, sid: str, table_id: str, direction: str = "upstream") -> Any:
        return self._record("lineage", sid, table_id, direction)

    async def diagnostics(self, sid: str, severity: str | None = None) -> Any:
        return self._record("diagnostics", sid, severity)

    async def list_sources(self, sid: str) -> Any:
        return self._record("list_sources", sid)


def tools(**returns: Any) -> tuple[dict[str, ToolSpec], FakeClient]:
    client = FakeClient(**returns)
    built = build_tools(client, "snap-1")  # type: ignore[arg-type]
    return {t.name: t for t in built}, client


class TestTheSetItself:
    def test_there_are_exactly_nine(self) -> None:
        by_name, _ = tools()
        assert len(by_name) == 9
        assert tuple(by_name) == TOOL_NAMES

    def test_every_description_says_what_the_tool_is_for(self) -> None:
        by_name, _ = tools()
        for spec in by_name.values():
            assert len(spec.description) > 80, f"{spec.name} description is too thin to choose by"

    def test_tools_taking_a_table_id_spell_out_the_format(self) -> None:
        """The model has to be told IDs are 'domain/table' or it will guess a name."""
        by_name, _ = tools()
        for name in ("get_tables", "get_neighbourhood", "find_join_paths", "get_lineage"):
            assert "domain/table" in by_name[name].description, name

    def test_every_argument_is_described(self) -> None:
        by_name, _ = tools()
        for spec in by_name.values():
            for field, info in spec.args_schema.model_fields.items():
                assert info.description, f"{spec.name}.{field} has no description"


class TestTheSnapshotIsNotReachable:
    """The single most important property in this file."""

    def test_no_schema_mentions_a_snapshot(self) -> None:
        by_name, _ = tools()
        for spec in by_name.values():
            schema = json.dumps(spec.args_schema.model_json_schema()).lower()
            assert "snapshot" not in schema, f"{spec.name} exposes a snapshot to the model"

    async def test_each_tool_set_uses_its_own_snapshot(self) -> None:
        """Two agents on two snapshots must not be able to read each other's."""
        first, client_a = tools()
        second_client = FakeClient()
        second = {t.name: t for t in build_tools(second_client, "snap-2")}  # type: ignore[arg-type]

        await first["list_domains"].fn()
        await second["list_domains"].fn()

        assert client_a.snapshots == ["snap-1"]
        assert second_client.snapshots == ["snap-2"]

    async def test_the_snapshot_is_not_an_argument(self) -> None:
        by_name, _ = tools()
        for spec in by_name.values():
            assert "snapshot_id" not in spec.args_schema.model_fields
            assert "sid" not in spec.args_schema.model_fields


class TestArgumentBounds:
    """Out of range is a validation error the model is told about, not a 400 it
    has to interpret."""

    def test_get_tables_rejects_an_empty_list(self) -> None:
        by_name, _ = tools()
        with pytest.raises(ValidationError):
            by_name["get_tables"].args_schema(ids=[])

    def test_get_tables_rejects_nine_ids(self) -> None:
        by_name, _ = tools()
        with pytest.raises(ValidationError):
            by_name["get_tables"].args_schema(ids=[f"d/t{i}" for i in range(9)])

    def test_get_tables_accepts_eight(self) -> None:
        by_name, _ = tools()
        assert by_name["get_tables"].args_schema(ids=[f"d/t{i}" for i in range(8)])

    @pytest.mark.parametrize("depth", [0, 4])
    def test_neighbourhood_rejects_a_depth_outside_one_to_three(self, depth: int) -> None:
        by_name, _ = tools()
        with pytest.raises(ValidationError):
            by_name["get_neighbourhood"].args_schema(table_id="d/t", depth=depth)

    def test_search_rejects_a_limit_over_fifty(self) -> None:
        by_name, _ = tools()
        with pytest.raises(ValidationError):
            by_name["search_model"].args_schema(query="x", limit=51)

    def test_lineage_rejects_an_unknown_direction(self) -> None:
        by_name, _ = tools()
        with pytest.raises(ValidationError):
            by_name["get_lineage"].args_schema(table_id="d/t", direction="sideways")

    def test_an_unknown_argument_is_rejected(self) -> None:
        by_name, _ = tools()
        with pytest.raises(ValidationError):
            by_name["search_model"].args_schema(query="x", limt=5)

    def test_diagnostics_rejects_an_unknown_severity(self) -> None:
        by_name, _ = tools()
        with pytest.raises(ValidationError):
            by_name["list_diagnostics"].args_schema(severity="fatal")


class TestTruncationIsAlwaysReported:
    """A silently cut list teaches the model something false and it will pass
    that on to the reader with no hedge."""

    async def test_a_long_list_is_capped_and_says_so(self) -> None:
        many = [Domain(id=f"d{i}", title=f"D{i}") for i in range(137)]
        by_name, _ = tools(list_domains=many)

        result = await by_name["list_domains"].fn()

        assert len(result["items"]) == MAX_ITEMS
        assert result["truncated"] is True
        assert result["total"] == 137

    async def test_a_short_list_has_the_same_shape(self) -> None:
        by_name, _ = tools(list_domains=[Domain(id=f"d{i}") for i in range(3)])

        result = await by_name["list_domains"].fn()

        assert set(result) >= {"items", "truncated", "total"}
        assert len(result["items"]) == 3
        assert result["truncated"] is False
        assert result["total"] == 3

    async def test_an_empty_list_is_still_an_object(self) -> None:
        by_name, _ = tools(list_domains=[])
        result = await by_name["list_domains"].fn()
        assert result == {"items": [], "truncated": False, "total": 0}


class TestShrinking:
    def _detail(self, description: str = "") -> TableDetail:
        return TableDetail(
            table={  # type: ignore[arg-type]
                "id": "ordering/fact_orders",
                "name": "fact_orders",
                "domainId": "ordering",
                "docPath": "ordering/fact_orders.md",
                "grain": "One row per order.",
                "columns": [
                    {"name": "order_id", "type": "int", "description": description, "isPk": True}
                ],
            }
        )

    async def test_doc_path_is_dropped(self) -> None:
        by_name, _ = tools(get_tables=TablesDetailResponse(tables=[self._detail()], missing=[]))
        result = await by_name["get_tables"].fn(ids=["ordering/fact_orders"])
        assert "docPath" not in json.dumps(result)

    async def test_a_long_column_description_is_cut_to_300(self) -> None:
        by_name, _ = tools(
            get_tables=TablesDetailResponse(tables=[self._detail("x" * 500)], missing=[])
        )
        result = await by_name["get_tables"].fn(ids=["ordering/fact_orders"])

        described = result["items"][0]["table"]["columns"][0]["description"]
        assert len(described) == 301, "300 characters plus the marker that it was cut"
        assert described.endswith("…")

    async def test_empty_fields_are_dropped(self) -> None:
        by_name, _ = tools(get_tables=TablesDetailResponse(tables=[self._detail()], missing=[]))
        table = (await by_name["get_tables"].fn(ids=["ordering/fact_orders"]))["items"][0]["table"]
        assert "layer" not in table, "an empty string should not reach the prompt"
        assert "notes" not in table

    async def test_false_flags_are_dropped_and_zeros_kept(self) -> None:
        """In Python `False == 0`, so a naive zero check keeps every False flag.
        Absence already says false; a zero ordinal is the first column."""
        from urara_chat.tools.registry import _prune

        pruned = _prune({"conformed": False, "ordinal": 0, "name": "x", "empty": ""})

        assert "conformed" not in pruned
        assert pruned["ordinal"] == 0
        assert "empty" not in pruned

    async def test_missing_ids_are_always_reported(self) -> None:
        """An absent key would read as though every ID was found."""
        by_name, _ = tools(get_tables=TablesDetailResponse(tables=[], missing=["nope/missing"]))
        result = await by_name["get_tables"].fn(ids=["nope/missing"])
        assert result["missing"] == ["nope/missing"]

    async def test_neighbourhood_returns_ids_and_joins_not_canvas_data(self) -> None:
        graph = Graph(
            nodes=[  # type: ignore[arg-type]
                {"id": "a/one", "label": "one", "kind": "fact", "degree": 9, "refs": 4},
                {"id": "b/two", "label": "two", "kind": "dimension", "degree": 2},
            ],
            links=[{"id": "l1", "source": "a/one", "target": "b/two", "fromColumn": "x"}],  # type: ignore[list-item]
        )
        by_name, _ = tools(neighbourhood=graph)

        result = await by_name["get_neighbourhood"].fn(table_id="a/one")

        assert {n["id"] for n in result["items"]} == {"a/one", "b/two"}
        assert "degree" not in json.dumps(result), "canvas sizing data should not reach the prompt"
        assert result["links"][0]["fromColumn"] == "x"

    async def test_links_to_dropped_nodes_are_removed(self) -> None:
        """A link naming a node that was cut tells the model about a table it
        cannot see, which is worse than not mentioning it."""
        nodes = [{"id": f"d/t{i}", "label": f"t{i}"} for i in range(60)]
        graph = Graph(
            nodes=nodes,  # type: ignore[arg-type]
            links=[{"id": "l", "source": "d/t0", "target": "d/t59"}],  # type: ignore[list-item]
        )
        by_name, _ = tools(neighbourhood=graph)

        result = await by_name["get_neighbourhood"].fn(table_id="d/t0")

        assert result["truncated"] is True
        kept = {n["id"] for n in result["items"]}
        for link in result["links"]:
            assert link["source"] in kept and link["target"] in kept

    async def test_join_paths_return_hops_with_their_columns(self) -> None:
        path = JoinPath(
            length=1,
            tables=["a/one", "b/two"],
            hops=[{"from": "a/one", "to": "b/two", "fromColumn": "x", "toColumn": "y"}],  # type: ignore[list-item]
        )
        by_name, _ = tools(join_paths=[path])

        result = await by_name["find_join_paths"].fn(from_table="a/one", to_table="b/two")

        hop = result["items"][0]["hops"][0]
        assert hop["from"] == "a/one" and hop["to"] == "b/two"
        assert hop["fromColumn"] == "x" and hop["toColumn"] == "y"


class TestResultsAreUsable:
    async def test_every_tool_result_survives_json_dumps(self) -> None:
        """Whatever a tool returns is going into a prompt as text. A type that
        cannot be serialised fails at the worst possible moment."""
        by_name, _ = tools(
            list_domains=[Domain(id="d")],
            list_tables=[TableSummary(id="d/t", name="t")],
            get_tables=TablesDetailResponse(
                tables=[TableDetail(table={"id": "d/t", "name": "t"})],  # type: ignore[arg-type]
                missing=[],
            ),
            search=[SearchHit(tableId="d/t")],  # type: ignore[call-arg]
            neighbourhood=Graph(nodes=[{"id": "d/t"}], links=[]),  # type: ignore[list-item]
            join_paths=[JoinPath(length=1, tables=["d/t"], hops=[])],
            lineage=[LineageEntry(id="src.model")],
            diagnostics=[Diagnostic(severity="error")],
            list_sources=[SourceTable(id="src.model")],
        )
        calls: dict[str, dict[str, Any]] = {
            "list_domains": {},
            "list_tables": {},
            "get_tables": {"ids": ["d/t"]},
            "search_model": {"query": "x"},
            "get_neighbourhood": {"table_id": "d/t"},
            "find_join_paths": {"from_table": "d/t", "to_table": "d/u"},
            "get_lineage": {"table_id": "d/t"},
            "list_diagnostics": {},
            "list_source_models": {},
        }
        assert set(calls) == set(TOOL_NAMES)

        for name, kwargs in calls.items():
            result = await by_name[name].fn(**kwargs)
            json.dumps(result)  # raises if anything is not serialisable
            assert isinstance(result, dict), f"{name} did not return the uniform shape"

    async def test_arguments_reach_the_client(self) -> None:
        by_name, client = tools()

        await by_name["list_tables"].fn(domain="ordering")
        await by_name["search_model"].fn(query="orders", limit=5)
        await by_name["get_neighbourhood"].fn(table_id="d/t", depth=3)
        await by_name["find_join_paths"].fn(from_table="a/x", to_table="b/y", max_depth=2)
        await by_name["get_lineage"].fn(table_id="d/t", direction="downstream")
        await by_name["list_diagnostics"].fn(severity="error")

        assert ("list_tables", ("ordering",)) in client.calls
        assert ("search", ("orders", 5)) in client.calls
        assert ("neighbourhood", ("d/t", 3)) in client.calls
        assert ("join_paths", ("a/x", "b/y", 2)) in client.calls
        assert ("lineage", ("d/t", "downstream")) in client.calls
        assert ("diagnostics", ("error",)) in client.calls

    async def test_lineage_reports_the_direction_it_answered(self) -> None:
        by_name, _ = tools(lineage=[LineageEntry(id="src.model")])
        result = await by_name["get_lineage"].fn(table_id="d/t", direction="downstream")
        assert result["direction"] == "downstream"
