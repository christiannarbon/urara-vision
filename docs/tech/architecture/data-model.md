# The two stores

Both are snapshot-scoped from top to bottom: every row and every node carries a
`snapshot_id` / `snapshotId`, every query filters on it, and a write replaces
whatever was previously stored under the same ID. That is what makes ingest
idempotent, deletion exact, and the integration suites able to share one
instance without isolating by database.

Snapshots group under projects: a project is the thing with a name and a URL,
a snapshot is one version of it.

## Postgres — the system of record

Eleven tables, all cascading from `projects` — by way of `snapshots`, and for
messages by way of `conversations` — so deleting either end is one `DELETE` and
the rest follows:

| Table | Holds | Key |
|---|---|---|
| `projects` | Slug, name, description, creation and last-touched times | `id` |
| `snapshots` | `project_id`, name, source label, creation time, stats JSON, and the `projectmeta.toml` the directory declared | `id` |
| `domains` | Title, description, mermaid diagram, lineage JSON, table count | `(snapshot_id, id)` |
| `tables` | Every Overview property, notes, conformed flags, and the `tsvector` | `(snapshot_id, id)` |
| `columns` | Name, type, description, PK/FK flags, in document order | `(snapshot_id, table_id, ordinal)` |
| `relationships` | Both endpoints, both columns, cardinality, resolution, candidates | `(snapshot_id, id)` |
| `column_lineage` | Column, source table, source column, notes, derived flag | `(snapshot_id, table_id, ordinal)` |
| `source_tables` | Canonicalised upstream models with a reference count | `(snapshot_id, id)` |
| `diagnostics` | Severity, code, message and where it points | `(snapshot_id, ordinal)` |
| `conversations` | One chat thread about a snapshot: title, creation and last-touched times | `id` |
| `conversation_messages` | One turn: role, content, the table IDs it cited, and what it cost | `(conversation_id, ordinal)` |

The schema is `CREATE TABLE IF NOT EXISTS` throughout and applied on every
start, so there is no migration step to forget and no state where the server is
running against a schema it does not recognise. Columns added after the first
release follow it as `ADD COLUMN IF NOT EXISTS`, which is idempotent in the same
way: a fresh database takes them from the `CREATE`, an existing one from the
`ALTER`, and neither has to know which it is.

Applying it takes an advisory lock for the length of the transaction. `IF NOT
EXISTS` is idempotent but not concurrency-safe — two sessions both find a table
absent, both create it, and the loser fails on a duplicate key in `pg_type` —
and the backend runs more than one replica, each migrating as it starts, so a
rollout has them arriving together.

An ingest is one transaction: the snapshot row is deleted and rewritten, and
every child table is bulk-loaded with `COPY`. A snapshot is therefore wholly
present or wholly absent, never half-written.

### Projects

A project's `slug` is derived from its name, not stored separately from it:
lower-case, every run of characters outside `[a-z0-9]` replaced by `-`, trimmed
of leading and trailing `-`, cut to 64 characters and trimmed again. Two
directories whose `projectmeta.toml` declares the same name are therefore the
same project, and its snapshots are its versions.

The ingest looks the slug up and inserts it `ON CONFLICT (slug) DO NOTHING`
rather than upserting: an update would hold the project row for the length of
the ingest and queue every concurrent save to the same project behind it. The
name and description are taken from the newest save instead, in a single
`UPDATE` run last, so that lock is held only until commit.

`snapshots.project_id` is `NOT NULL`, which snapshots ingested before projects
existed could not satisfy. The schema backfills them in two passes, both
touching only `project_id IS NULL` rows and so a no-op once nothing is left:
first by slugging `project_name`, in SQL that repeats `projectmeta.Slug`
step for step (`projects_backfill_test.go` checks the two agree); then whatever
is left has no usable name and gets a project each, slugged
`legacy-<first 12 hex of md5(snapshot id)>`, matching `projectmeta.LegacySlug`.

### Versions

A version is a snapshot's `project_version`, and a project holds each one once:
`snapshots_project_version_key` is unique on `(project_id, project_version)`.
The ingest checks for an existing version before parsing and answers `409`; the
index settles two imports racing past that check.

Databases from before the index can hold the same version several times. The
schema renames them rather than deleting any: within a project and version the
newest keeps the label and the older ones become `<version>+legacy.1`,
`+legacy.2`, … in age order. A snapshot with no version is `legacy`. Both steps
touch only duplicates or empty labels, so they are a no-op once the index
exists.

`latest` is not stored. It means the most recently imported version
(`created_at`), not the highest by semver, and works on reads only: deleting by
alias is refused. Deleting a project's last version deletes the project too.

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

### Conversations

A conversation belongs to the snapshot it is about and does not outlive it.
Every citation a transcript carries is a table ID, and once the snapshot is gone
those point at nothing: a transcript full of dead links is worse than no
transcript, so `conversations.snapshot_id` cascades and the messages cascade
behind it. The snapshot ID stored is always a concrete one — the `latest` alias
is resolved when the conversation is created, never at read time.

Message ordinals are assigned by the database, inside the same statement that
stores the row:

```sql
INSERT INTO conversation_messages (conversation_id, ordinal, ...)
SELECT $1, coalesce(max(ordinal), -1) + 1, ...
  FROM conversation_messages WHERE conversation_id = $1
```

Reading the maximum into the application and writing it back would be a race:
two appends to one conversation would see the same number, and one would be lost
or would collide. Computing it in the `INSERT` narrows that to the window
between the `SELECT` and the write, and the primary key on
`(conversation_id, ordinal)` closes it — a collision becomes a unique violation
rather than a duplicate ordinal, and the loser is retried once, by which point
the winner's row is visible. The append also bumps the conversation's
`updated_at` in the same transaction, so a stored turn always leaves its thread
looking touched.

`citations` and `meta` are `jsonb` defaulting to `[]` and `{}`, never null:
"cited nothing" is a real answer and has to survive storage as one. `role` has
no `CHECK` constraint — it is validated in the API instead, so an invalid role
is a `400` with a message rather than a driver error a handler has to interpret.

### Notes

A note belongs to one snapshot and goes with it: `notes.snapshot_id` cascades,
and notes are never copied into a new version. Replies are rows with
`parent_id` set, one level deep, and cascade with their parent.

| Column | |
|---|---|
| `anchor_kind`, `anchor_id` | What the note is pinned to. Replies copy their parent's |
| `body` | Plain text, trimmed, at most 4000 characters |
| `author_id` | `ON DELETE SET NULL`: a deleted user's notes stay |
| `author_name` | Display name, or username, at write time; kept after the user goes |
| `resolved_at`, `resolved_by_name` | Top-level notes only; cleared on reopen |

| `anchor_kind` | `anchor_id` | Checked against |
|---|---|---|
| `domain` | domain ID | `domains.id` |
| `table` | table ID | `tables.id` |
| `column` | `<table id>#<column name>` | `columns (table_id, name)` |
| `relationship` | relationship ID | `relationships.id` |
| `lineage` | `<table id>#<column name>` | any `column_lineage (table_id, column_name)` row |

Column and lineage IDs are matched as a whole (`table_id || '#' || name`),
since table and column names may contain `#`. Anchors are checked when a note is
created, not enforced by foreign keys: the anchored rows have composite keys
and live only as long as the snapshot, which the cascade already covers.
`notes_anchor_idx (snapshot_id, anchor_kind, anchor_id, created_at)` serves both
the thread read and the counts.

Notes are never read by anything the chat service calls; see
[auth.md](auth.md#notes).

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
