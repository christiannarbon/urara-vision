"""The models against real captured backend JSON.

The fixtures in `fixtures/` came off a running stack with the jaffle-shop demo
set ingested, not from invented shapes: the point is to catch a field name that
was guessed wrong, and an invented fixture would agree with whatever the model
happens to say.
"""

import json
from pathlib import Path
from typing import Any

import pytest
from pydantic import ValidationError

from urara_chat.backend.models import (
    Conversation,
    Diagnostic,
    Domain,
    Graph,
    JoinPath,
    LineageEntry,
    Message,
    SearchHit,
    SnapshotContext,
    SourceTable,
    TableDetail,
    TablesDetailResponse,
    TableSummary,
)

FIXTURES = Path(__file__).parent / "fixtures"


def load(name: str) -> Any:
    return json.loads((FIXTURES / f"{name}.json").read_text())


class TestRealResponsesParse:
    """Every captured response parses into its model with no validation error."""

    def test_context(self) -> None:
        ctx = SnapshotContext.model_validate(load("context"))
        assert ctx.snapshot.id
        assert ctx.domains and ctx.tables
        # All three severity keys are always present, per Phase 01.
        assert set(ctx.diagnostics) == {"error", "warning", "info"}
        assert ctx.truncated is False

    def test_domains(self) -> None:
        domains = [Domain.model_validate(d) for d in load("domains")["domains"]]
        assert domains
        assert all(d.id and d.title for d in domains)

    def test_tables(self) -> None:
        tables = [TableSummary.model_validate(t) for t in load("tables")["tables"]]
        assert tables
        assert all(t.id and t.name for t in tables)

    def test_table_detail(self) -> None:
        detail = TableDetail.model_validate(load("table"))
        assert detail.table.id == "ordering/fact_orders"
        assert detail.table.columns

    def test_tables_detail_batch(self) -> None:
        batch = TablesDetailResponse.model_validate(load("tables_detail"))
        assert len(batch.tables) == 2
        assert batch.missing == ["nope/missing"]

    def test_search(self) -> None:
        hits = [SearchHit.model_validate(h) for h in load("search")["hits"]]
        assert hits
        assert all(h.table_id for h in hits)

    def test_graph(self) -> None:
        graph = Graph.model_validate(load("graph"))
        assert graph.nodes and graph.links
        assert all(link.source and link.target for link in graph.links)

    def test_paths(self) -> None:
        paths = [JoinPath.model_validate(p) for p in load("paths")["paths"]]
        assert paths
        assert paths[0].hops

    def test_lineage(self) -> None:
        entries = [LineageEntry.model_validate(e) for e in load("lineage")["entries"]]
        assert entries
        assert all(e.id for e in entries)

    def test_diagnostics(self) -> None:
        diags = [Diagnostic.model_validate(d) for d in load("diagnostics")["diagnostics"]]
        assert diags
        assert all(d.severity for d in diags)

    def test_sources(self) -> None:
        sources = [SourceTable.model_validate(s) for s in load("sources")["sources"]]
        assert sources
        assert all(s.id for s in sources)

    def test_conversation(self) -> None:
        conv = Conversation.model_validate(load("conversation"))
        assert conv.id and conv.snapshot_id
        assert len(conv.messages) == 2

    def test_message(self) -> None:
        msg = Message.model_validate(load("message"))
        assert msg.role == "user"
        assert msg.ordinal == 0


class TestKnownValuesSurvive:
    """A field name guessed wrong parses to its default rather than failing, so
    the aliases are checked against values that are actually in the fixture."""

    def test_grain_survives_the_round_trip(self) -> None:
        detail = TableDetail.model_validate(load("table"))
        assert detail.table.grain == "One row per order."

    def test_camel_case_fields_are_populated(self) -> None:
        detail = TableDetail.model_validate(load("table"))
        assert detail.table.domain_id == "ordering"
        assert detail.table.column_lineage, "columnLineage did not map to column_lineage"
        assert any(c.is_pk for c in detail.table.columns), "isPk did not map to is_pk"

    def test_path_hop_reads_the_from_key(self) -> None:
        """ "from" is a Python keyword, so this alias is written by hand and is
        the one most likely to be silently wrong."""
        path = JoinPath.model_validate(load("paths")["paths"][0])
        assert path.hops[0].from_table
        assert path.hops[0].to

    def test_snapshot_stats_and_project_are_nested(self) -> None:
        ctx = SnapshotContext.model_validate(load("context"))
        assert ctx.snapshot.stats.tables > 0
        assert ctx.snapshot.project.project.name


class TestTolerance:
    def test_an_unknown_field_does_not_raise(self) -> None:
        """The backend will grow fields; parsing must not break on a release."""
        raw = load("table")
        raw["table"]["somethingAddedLater"] = {"nested": True}
        raw["anEntirelyNewKey"] = 1

        detail = TableDetail.model_validate(raw)
        assert detail.table.id == "ordering/fact_orders"

    def test_a_missing_optional_field_parses(self) -> None:
        raw = load("table")
        raw["table"].pop("notes", None)
        raw["table"].pop("conformedIn", None)
        assert TableDetail.model_validate(raw).table.notes == []


class TestCitations:
    """ "Cited nothing" is a real answer, so it must arrive as [] and never None."""

    def test_a_message_with_no_citations_parses_to_an_empty_list(self) -> None:
        msg = Message.model_validate({"role": "user", "content": "hi"})
        assert msg.citations == []

    def test_citations_none_is_not_constructible(self) -> None:
        with pytest.raises(ValidationError):
            Message(role="user", citations=None)  # type: ignore[arg-type]

    def test_real_citations_survive(self) -> None:
        conv = Conversation.model_validate(load("conversation"))
        cited = [m for m in conv.messages if m.citations]
        assert cited, "the fixture should carry a message with citations"
        assert cited[0].citations == ["ordering/fact_orders"]
