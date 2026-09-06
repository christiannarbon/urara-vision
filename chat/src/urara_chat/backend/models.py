"""Pydantic mirrors of the JSON the Go backend returns.

The wire format is camelCase because Go struct tags decide it; the Python side
stays snake_case and `to_camel` bridges the two. Field names here were read off
`internal/model/model.go`, `internal/store/neo4j/types.go`,
`internal/store/postgres/{tables,relationships,search}.go` and
`internal/api/context.go` rather than inferred from a sample, because a field
that only appears when it is non-empty would not show up in one.

These are data. Nothing here has behaviour.
"""

from __future__ import annotations

from datetime import datetime

from pydantic import BaseModel, ConfigDict, Field
from pydantic.alias_generators import to_camel


class BackendModel(BaseModel):
    """A response from the Go API.

    Fields arrive camelCase, and unknown ones are ignored rather than refused:
    the backend will grow fields, and a chat service that fails to parse a
    response because the API added something is a chat service that breaks on
    every backend release.
    """

    model_config = ConfigDict(
        extra="ignore",
        populate_by_name=True,
        alias_generator=to_camel,
    )


# --- the snapshot and what it declares about itself -------------------------


class Project(BackendModel):
    name: str = ""
    version: str = ""
    description: str = ""


class Internationalization(BackendModel):
    primary: str = ""
    supported: list[str] = []
    type: str = ""


class ProjectMeta(BackendModel):
    project: Project = Project()
    internationalization: Internationalization = Internationalization()


class Stats(BackendModel):
    domains: int = 0
    tables: int = 0
    columns: int = 0
    relationships: int = 0
    lineage_edges: int = 0
    source_tables: int = 0
    conformed: int = 0
    files_parsed: int = 0
    files_skipped: int = 0
    diagnostics: int = 0
    translated: int = 0


class Snapshot(BackendModel):
    id: str
    name: str = ""
    source_label: str = ""
    created_at: datetime
    stats: Stats = Stats()
    project: ProjectMeta = ProjectMeta()


# --- domains and tables -----------------------------------------------------


class DomainLineage(BackendModel):
    proposed_table: str = ""
    source_models: list[str] = []


class Domain(BackendModel):
    id: str
    snapshot_id: str = ""
    name: str = ""
    title: str = ""
    description: str = ""
    # omitempty on the Go side, so absent rather than null when there is none.
    mermaid: str = ""
    lineage: list[DomainLineage] = []
    doc_path: str = ""
    table_count: int = 0


class Column(BackendModel):
    name: str
    type: str = ""
    description: str = ""
    ordinal: int = 0
    # Go tags these "isPk"/"isFk", which to_camel would render "isPk" from
    # is_pk -- it agrees, but the alias is written out so a rename cannot
    # silently break the mapping.
    is_pk: bool = False
    is_fk: bool = False


class ColumnLineage(BackendModel):
    column: str = ""
    source_table: str = ""
    source_column: str = ""
    notes: str = ""
    derived: bool = False


class Relationship(BackendModel):
    id: str = ""
    from_table_id: str = ""
    to_table_id: str = ""
    target_ref: str = ""
    from_column: str = ""
    to_column: str = ""
    join_key_raw: str = ""
    cardinality: str = ""
    resolution: str = ""
    candidates: list[str] = []


class Table(BackendModel):
    """One table document in full, as `/table?id=` nests it under "table"."""

    id: str
    snapshot_id: str = ""
    name: str
    domain_id: str = ""
    kind: str = ""
    kind_raw: str = ""
    grain: str = ""
    update_frequency: str = ""
    layer: str = ""
    domain_label: str = ""
    description: str = ""
    columns: list[Column] = []
    column_lineage: list[ColumnLineage] = []
    relationships: list[Relationship] = []
    notes: list[str] = []
    relationship_note: str = ""
    doc_path: str = ""
    conformed: bool = False
    conformed_in: list[str] = []


class TableSummary(BackendModel):
    """What `/tables` returns: enough to list, not enough to explain."""

    id: str
    name: str
    domain_id: str = ""
    kind: str = ""
    grain: str = ""
    conformed: bool = False
    column_count: int = 0
    description: str = ""


class Referrer(BackendModel):
    """A table declaring a relationship *into* the one being read."""

    table_id: str
    name: str = ""
    domain_id: str = ""
    from_column: str = ""
    to_column: str = ""
    cardinality: str = ""


class LineageEntry(BackendModel):
    id: str
    label: str = ""
    dataset: str = ""
    domain_id: str = ""
    columns: list[str] = []
    column_count: int = 0


class TableDetail(BackendModel):
    """The `/table?id=` envelope.

    The handler assembles four store calls into one object, so the table itself
    is nested rather than being the response.
    """

    table: Table
    incoming: list[Referrer] = []
    upstream: list[LineageEntry] = []
    siblings: list[LineageEntry] = []


class TablesDetailResponse(BackendModel):
    """The `/tables/detail?ids=` envelope.

    `missing` is how a wrong guess at an ID comes back: the call succeeds and
    names what it could not find, rather than failing the whole batch.
    """

    tables: list[TableDetail] = []
    missing: list[str] = []


# --- search, graph and lineage ----------------------------------------------


class SearchHit(BackendModel):
    table_id: str
    name: str = ""
    domain_id: str = ""
    kind: str = ""
    grain: str = ""
    rank: float = 0.0
    matched_on: list[str] = []


class Node(BackendModel):
    id: str
    label: str = ""
    type: str = ""  # table | source
    domain_id: str = ""
    kind: str = ""
    grain: str = ""
    conformed: bool = False
    column_count: int = 0
    dataset: str = ""
    refs: int = 0
    degree: int = 0


class Link(BackendModel):
    id: str
    source: str
    target: str
    type: str = ""  # joins | derived_from
    from_column: str = ""
    to_column: str = ""
    cardinality: str = ""
    resolution: str = ""
    cross_domain: bool = False
    columns: list[str] = []
    column_count: int = 0


class Graph(BackendModel):
    nodes: list[Node] = []
    links: list[Link] = []


class PathHop(BackendModel):
    # The Go tag is "from", which is a Python keyword, so this is the one field
    # that must be renamed and given an explicit alias -- the generator would
    # otherwise look for "fromTable" and quietly leave it empty.
    from_table: str = Field(default="", alias="from")
    to: str = ""
    from_column: str = ""
    to_column: str = ""
    cardinality: str = ""


class JoinPath(BackendModel):
    length: int = 0
    tables: list[str] = []
    hops: list[PathHop] = []


# --- diagnostics and sources ------------------------------------------------


class Diagnostic(BackendModel):
    severity: str
    code: str = ""
    message: str = ""
    domain_id: str = ""
    table_id: str = ""
    doc_path: str = ""


class SourceTable(BackendModel):
    id: str
    dataset: str = ""
    name: str = ""
    refs: int = 0


# --- the context catalogue --------------------------------------------------


class ContextDomain(BackendModel):
    id: str
    title: str = ""
    description: str = ""
    table_count: int = 0


class ContextTable(BackendModel):
    id: str
    name: str = ""
    domain_id: str = ""
    kind: str = ""
    grain: str = ""
    column_count: int = 0
    conformed: bool = False


class SnapshotContext(BackendModel):
    """The `/context` catalogue: a whole snapshot small enough to prime a prompt.

    `truncated` says the table list was dropped rather than shortened, so an
    empty `tables` with `truncated` true means "too many to list", not "none".
    """

    snapshot: Snapshot
    domains: list[ContextDomain] = []
    tables: list[ContextTable] = []
    diagnostics: dict[str, int] = {}
    truncated: bool = False


# --- conversations ----------------------------------------------------------


class Message(BackendModel):
    """One turn.

    `citations` is a list and never None: "drew on no tables" is a real answer
    and has to survive the round trip as an empty list rather than as absence.
    """

    ordinal: int = 0
    role: str
    content: str = ""
    citations: list[str] = []
    meta: dict[str, object] = {}
    created_at: datetime | None = None


class Conversation(BackendModel):
    id: str
    snapshot_id: str = ""
    title: str = ""
    created_at: datetime | None = None
    updated_at: datetime | None = None
    # Populated when one conversation is fetched, absent when they are listed.
    messages: list[Message] = []
