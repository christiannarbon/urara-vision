"""The retrieval layer: the fixed set of tools an agent may call."""

from __future__ import annotations

from collections.abc import Awaitable, Callable, Sequence
from dataclasses import dataclass
from typing import Any, Literal

from pydantic import BaseModel, ConfigDict, Field

from urara_chat.backend.client import BackendClient
from urara_chat.backend.models import Graph, JoinPath, TableDetail, TablesDetailResponse

# The most entries any tool returns. Bounds the prompt: past this a list stops
# informing an answer and starts crowding out the question.
MAX_ITEMS = 50

# Column prose is the bulkiest thing a table document carries, and the model needs enough to tell
# columns apart rather than the whole paragraph.
MAX_COLUMN_DESCRIPTION_RUNES = 300


@dataclass(frozen=True)
class ToolSpec:
    """One callable tool, and everything a binding needs to expose it."""

    name: str
    description: str
    args_schema: type[BaseModel]
    fn: Callable[..., Awaitable[Any]]


def _capped(items: list[Any]) -> dict[str, Any]:
    """The one shape every list result takes."""
    total = len(items)
    return {"items": items[:MAX_ITEMS], "truncated": total > MAX_ITEMS, "total": total}


def _truncate_runes(text: str, limit: int) -> str:
    """Shorten to `limit` characters, marking that it was cut."""
    if len(text) <= limit:
        return text
    return text[:limit] + "…"


def _prune(value: Any) -> Any:
    """Drop what carries no information, recursively."""
    if isinstance(value, dict):
        pruned = {k: _prune(v) for k, v in value.items()}
        return {k: v for k, v in pruned.items() if _worth_keeping(v)}
    if isinstance(value, list):
        return [_prune(v) for v in value]
    return value


def _worth_keeping(value: Any) -> bool:
    """Whether a pruned value earns its place in the prompt."""
    if isinstance(value, bool):
        return value
    if isinstance(value, int | float):
        return True
    return bool(value)


def _shrink_table(detail: TableDetail) -> dict[str, Any]:
    """One table document, reduced to what an answer is built from."""
    table = detail.table.model_dump(mode="json", by_alias=True, exclude={"doc_path", "snapshot_id"})
    for column in table.get("columns", []):
        column["description"] = _truncate_runes(
            column.get("description", ""), MAX_COLUMN_DESCRIPTION_RUNES
        )

    shrunk: dict[str, Any] = {"table": table}
    if detail.incoming:
        shrunk["incoming"] = [
            r.model_dump(mode="json", by_alias=True, exclude={"doc_path"}) for r in detail.incoming
        ]
    if detail.upstream:
        shrunk["upstream"] = [e.id for e in detail.upstream]
    if detail.siblings:
        shrunk["siblings"] = [e.id for e in detail.siblings]
    return dict(_prune(shrunk))


def _shrink_graph(graph: Graph) -> dict[str, Any]:
    """A neighbourhood as IDs and joins rather than as drawing instructions."""
    nodes = [
        _prune(
            {
                "id": n.id,
                "label": n.label,
                "kind": n.kind,
                "domainId": n.domain_id,
                "type": n.type,
            }
        )
        for n in graph.nodes
    ]
    kept = {n["id"] for n in nodes[:MAX_ITEMS]}

    result = _capped(nodes)
    result["links"] = [
        _prune(
            {
                "source": link.source,
                "target": link.target,
                "type": link.type,
                "fromColumn": link.from_column,
                "toColumn": link.to_column,
                "cardinality": link.cardinality,
            }
        )
        for link in graph.links
        if link.source in kept and link.target in kept
    ][:MAX_ITEMS]
    return result


def _shrink_path(path: JoinPath) -> dict[str, Any]:
    return {
        "length": path.length,
        "tables": path.tables,
        "hops": [
            {
                "from": hop.from_table,
                "to": hop.to,
                "fromColumn": hop.from_column,
                "toColumn": hop.to_column,
                "cardinality": hop.cardinality,
            }
            for hop in path.hops
        ],
    }


def _shrink_batch(batch: TablesDetailResponse) -> dict[str, Any]:
    result = _capped([_shrink_table(d) for d in batch.tables])
    # Reported even when empty: an ID that missed is something the model must act on, and an
    # absent key reads as though every ID was found.
    result["missing"] = list(batch.missing)
    return result


# Bounds live here rather than in the function body so an out-of-range value is a validation error
# the model is told about and can correct, instead of a 400 from the backend that it has to
# interpret.


class ToolArgs(BaseModel):
    """Base for every tool's arguments."""

    model_config = ConfigDict(extra="forbid")


class NoArgs(ToolArgs):
    pass


class ListTablesArgs(ToolArgs):
    domain: str | None = Field(
        default=None,
        description="Optional domain ID to restrict the list to, e.g. 'ordering'. "
        "Omit to list every table in the model.",
    )


class GetTablesArgs(ToolArgs):
    ids: list[str] = Field(
        min_length=1,
        max_length=8,
        description="Between one and eight full table IDs in 'domain/table' form, "
        "e.g. ['ordering/fact_orders']. Use search_model or list_tables first if "
        "you only have a name.",
    )


class SearchArgs(ToolArgs):
    query: str = Field(description="Words to search for in table and column names and prose.")
    limit: int = Field(default=20, ge=1, le=50, description="Maximum hits to return, 1 to 50.")


class NeighbourhoodArgs(ToolArgs):
    table_id: str = Field(description="Full table ID in 'domain/table' form.")
    depth: int = Field(default=1, ge=1, le=3, description="How many joins to follow out, 1 to 3.")


class JoinPathsArgs(ToolArgs):
    from_table: str = Field(description="Full table ID to start from, in 'domain/table' form.")
    to_table: str = Field(description="Full table ID to reach, in 'domain/table' form.")
    max_depth: int = Field(default=4, ge=1, le=6, description="Longest path to consider, 1 to 6.")


class LineageArgs(ToolArgs):
    table_id: str = Field(description="Full table ID in 'domain/table' form.")
    direction: Literal["upstream", "downstream"] = Field(
        default="upstream",
        description="'upstream' for the sources feeding this table, "
        "'downstream' for what is fed by it.",
    )


class DiagnosticsArgs(ToolArgs):
    severity: Literal["error", "warning", "info"] | None = Field(
        default=None, description="Restrict to one severity. Omit for all of them."
    )


# A description is the only thing the model reads before choosing, so each says what the tool is
# for, when to reach for it, what comes back, and the format of any ID it takes.

TOOL_NAMES: tuple[str, ...] = (
    "list_domains",
    "list_tables",
    "get_tables",
    "search_model",
    "get_neighbourhood",
    "find_join_paths",
    "get_lineage",
    "list_diagnostics",
    "list_source_models",
)


def build_tools(client: BackendClient, snapshot_id: str) -> list[ToolSpec]:
    """The tools an agent may call, bound to one snapshot."""

    async def list_domains() -> dict[str, Any]:
        domains = await client.list_domains(snapshot_id)
        return _capped(
            [
                {
                    "id": d.id,
                    "title": d.title,
                    "description": d.description,
                    "tableCount": d.table_count,
                }
                for d in domains
            ]
        )

    async def list_tables(domain: str | None = None) -> dict[str, Any]:
        tables = await client.list_tables(snapshot_id, domain)
        return _capped(
            [
                _prune(
                    {
                        "id": t.id,
                        "name": t.name,
                        "domainId": t.domain_id,
                        "kind": t.kind,
                        "grain": t.grain,
                        "columnCount": t.column_count,
                        "conformed": t.conformed,
                    }
                )
                for t in tables
            ]
        )

    async def get_tables(ids: Sequence[str]) -> dict[str, Any]:
        return _shrink_batch(await client.get_tables(snapshot_id, list(ids)))

    async def search_model(query: str, limit: int = 20) -> dict[str, Any]:
        hits = await client.search(snapshot_id, query, limit)
        return _capped(
            [
                _prune(
                    {
                        "tableId": h.table_id,
                        "name": h.name,
                        "domainId": h.domain_id,
                        "kind": h.kind,
                        "grain": h.grain,
                        "matchedOn": h.matched_on,
                    }
                )
                for h in hits
            ]
        )

    async def get_neighbourhood(table_id: str, depth: int = 1) -> dict[str, Any]:
        return _shrink_graph(await client.neighbourhood(snapshot_id, table_id, depth))

    async def find_join_paths(from_table: str, to_table: str, max_depth: int = 4) -> dict[str, Any]:
        paths = await client.join_paths(snapshot_id, from_table, to_table, max_depth)
        return _capped([_shrink_path(p) for p in paths])

    async def get_lineage(table_id: str, direction: str = "upstream") -> dict[str, Any]:
        entries = await client.lineage(snapshot_id, table_id, direction)
        result = _capped(
            [
                _prune(
                    {
                        "id": e.id,
                        "label": e.label,
                        "dataset": e.dataset,
                        "domainId": e.domain_id,
                        "columns": e.columns,
                    }
                )
                for e in entries
            ]
        )
        result["direction"] = direction
        return result

    async def list_diagnostics(severity: str | None = None) -> dict[str, Any]:
        diags = await client.diagnostics(snapshot_id, severity)
        return _capped(
            [
                _prune(
                    {
                        "severity": d.severity,
                        "code": d.code,
                        "message": d.message,
                        "domainId": d.domain_id,
                        "tableId": d.table_id,
                    }
                )
                for d in diags
            ]
        )

    async def list_source_models() -> dict[str, Any]:
        sources = await client.list_sources(snapshot_id)
        return _capped(
            [{"id": s.id, "dataset": s.dataset, "name": s.name, "refs": s.refs} for s in sources]
        )

    return [
        ToolSpec(
            name="list_domains",
            description=(
                "List the subject areas the model is divided into, with a short "
                "description and table count for each. Use this first when the reader "
                "asks what the model covers, or when you need a domain ID to narrow a "
                "later call. Returns one entry per domain."
            ),
            args_schema=NoArgs,
            fn=list_domains,
        ),
        ToolSpec(
            name="list_tables",
            description=(
                "List tables, optionally within one domain, with their kind, grain and "
                "column count. Use this to find out what exists before reading anything "
                "in full, or to answer questions about how many tables of a kind there "
                "are. Returns summaries, not documents: call get_tables for the detail. "
                "The optional 'domain' argument is a domain ID such as 'ordering'."
            ),
            args_schema=ListTablesArgs,
            fn=list_tables,
        ),
        ToolSpec(
            name="get_tables",
            description=(
                "Read up to eight table documents in full: columns with types and "
                "descriptions, declared relationships, column-level lineage and notes. "
                "This is the tool that answers questions about what a table contains or "
                "means. Ask for every table you need in one call rather than one at a "
                "time. IDs are full 'domain/table' identifiers, e.g. "
                "'ordering/fact_orders'; call search_model or list_tables first if you "
                "only have a name. IDs that do not exist come back in 'missing' rather "
                "than failing the call."
            ),
            args_schema=GetTablesArgs,
            fn=get_tables,
        ),
        ToolSpec(
            name="search_model",
            description=(
                "Full-text search over table names, grains, and column names and "
                "descriptions. Use this when the reader names something in their own "
                "words rather than by ID -- 'where do we store refunds' -- and to turn "
                "a name into the 'domain/table' ID the other tools need. Returns ranked "
                "hits with the field that matched, not the documents themselves."
            ),
            args_schema=SearchArgs,
            fn=search_model,
        ),
        ToolSpec(
            name="get_neighbourhood",
            description=(
                "Show what a table joins to, out to a given number of hops. Use this "
                "when the reader asks what surrounds a table, what it connects to, or "
                "what else they would need to answer a question from it. Returns the "
                "nearby tables and the joins between them, with the columns joined on. "
                "'table_id' is a full 'domain/table' ID; 'depth' is 1 to 3, and 1 is "
                "usually enough."
            ),
            args_schema=NeighbourhoodArgs,
            fn=get_neighbourhood,
        ),
        ToolSpec(
            name="find_join_paths",
            description=(
                "Find how to get from one table to another by following declared "
                "joins. Use this when the reader asks how two tables relate or what the "
                "join path between them is. Returns each path as an ordered list of "
                "hops with the columns joined at each step. Both arguments are full "
                "table IDs in 'domain/table' form; call search_model first if you only "
                "have a name."
            ),
            args_schema=JoinPathsArgs,
            fn=find_join_paths,
        ),
        ToolSpec(
            name="get_lineage",
            description=(
                "Trace where a table's data comes from, or what is built on it. Use "
                "'upstream' to find the source models feeding a table, and 'downstream' "
                "on a source model to find every table built from it -- the tool to "
                "reach for when the reader asks what breaks if something upstream "
                "changes. Returns the entries and the columns each contributes. "
                "'table_id' is a full 'domain/table' ID, or a source model ID such as "
                "'warehouse.stg_orders' when going downstream."
            ),
            args_schema=LineageArgs,
            fn=get_lineage,
        ),
        ToolSpec(
            name="list_diagnostics",
            description=(
                "List the problems found in the documentation itself: unresolved "
                "references, missing grains, ambiguous joins. Use this when the reader "
                "asks about the quality or completeness of the documentation, or when a "
                "table looks wrong and you want to know whether it is already known to "
                "be. Returns each with its severity, code, message and where it points. "
                "Filter with 'severity' as 'error', 'warning' or 'info'."
            ),
            args_schema=DiagnosticsArgs,
            fn=list_diagnostics,
        ),
        ToolSpec(
            name="list_source_models",
            description=(
                "List the upstream models the documented tables are built from, with "
                "how many tables reference each. Use this to answer what the model is "
                "sourced from, or to find the most depended-upon inputs. Returns source "
                "IDs such as 'warehouse.stg_orders'; pass one to get_lineage with "
                "direction 'downstream' to see what is built on it."
            ),
            args_schema=NoArgs,
            fn=list_source_models,
        ),
    ]
