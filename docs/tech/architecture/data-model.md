# The two stores

Both are snapshot-scoped from top to bottom: every row and every node carries a
`snapshot_id` / `snapshotId`, every query filters on it, and a write replaces
whatever was previously stored under the same ID. That is what makes ingest
idempotent, deletion exact, and the integration suites able to share one
instance without isolating by database.

## Postgres — the system of record

Eight tables, all cascading from `snapshots`, so deleting a snapshot is one
`DELETE` and the rest follows:

| Table | Holds | Key |
|---|---|---|
| `snapshots` | Name, source label, creation time, stats JSON, and the `projectmeta.toml` the directory declared | `id` |
| `domains` | Title, description, mermaid diagram, lineage JSON, table count | `(snapshot_id, id)` |
| `tables` | Every Overview property, notes, conformed flags, and the `tsvector` | `(snapshot_id, id)` |
| `columns` | Name, type, description, PK/FK flags, in document order | `(snapshot_id, table_id, ordinal)` |
| `relationships` | Both endpoints, both columns, cardinality, resolution, candidates | `(snapshot_id, id)` |
| `column_lineage` | Column, source table, source column, notes, derived flag | `(snapshot_id, table_id, ordinal)` |
| `source_tables` | Canonicalised upstream models with a reference count | `(snapshot_id, id)` |
| `diagnostics` | Severity, code, message and where it points | `(snapshot_id, ordinal)` |

The schema is `CREATE TABLE IF NOT EXISTS` throughout and applied on every
start, so there is no migration step to forget and no state where the server is
running against a schema it does not recognise. Columns added after the first
release follow it as `ADD COLUMN IF NOT EXISTS`, which is idempotent in the same
way: a fresh database takes them from the `CREATE`, an existing one from the
`ALTER`, and neither has to know which it is.

An ingest is one transaction: the snapshot row is deleted and rewritten, and
every child table is bulk-loaded with `COPY`. A snapshot is therefore wholly
present or wholly absent, never half-written.

### The search index

`tables.search` is a weighted `tsvector`, rebuilt at the end of each ingest
inside the same transaction:

| Weight | From |
|---|---|
| A | Table name |
| B | Domain ID |
| C | Description, grain |
| D | Every column name and description |

Weighting is what makes a table whose *name* matches outrank one where the term
only appears in a column description, while still finding the latter — which is
the whole point of indexing columns into the table's own vector rather than
searching two tables and merging.

Queries are prefix queries (`prim:*`), built by splitting the user's text on
anything that is not a letter, digit or underscore, so the overlay can search
as the reader types. Text with no usable characters compiles to a term that
matches nothing rather than erroring.

## Neo4j — the graph projection

```cypher
(:Table)-[:JOINS {fromColumn, toColumn, cardinality, crossDomain}]->(:Table)
(:Table)-[:DERIVED_FROM {columns}]->(:Source)
(:Table)-[:IN_DOMAIN]->(:Domain)
(:Table)-[:CONFORMS_TO]->(:Conformed)
```

Four labels: `Table`, `Domain`, `Source` and `Conformed` — the last a synthetic
node per conformed name, so every instance of a shared dimension hangs off one
place.

Uniqueness constraints on `(snapshotId, id)` for tables, domains and sources
make the projection's `MERGE`s idempotent; indexes on `snapshotId` and on table
`name` keep the scoped traversals from touching other snapshots' subgraphs.
`EnsureConstraints` runs at startup and is safe to run concurrently — several
replicas do exactly that.

These are the queries the second store exists for:

| Question | Query |
|---|---|
| What is near this table? | Variable-length `JOINS` traversal to a depth |
| How do I join these two? | `allShortestPaths` over `JOINS` |
| Where does this column come from? | `DERIVED_FROM` upstream |
| What else reads this model? | `DERIVED_FROM` downstream from a `Source` |
| What else shares my upstream? | Two `DERIVED_FROM` hops through a `Source` |

Every one of them is a single Cypher clause and an awkward recursive CTE in
SQL. That is the entire justification for the second store; nothing that a
plain indexed read can answer is asked of it.
