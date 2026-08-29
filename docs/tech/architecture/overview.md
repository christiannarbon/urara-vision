# Architecture

```
Browser ──pick folder──▶ Vue 3 app
                          │ reads .md client-side
                          ▼
                    POST /api/v1/ingest        (JSON or multipart)
                          │
                    ┌─────▼──────┐
                    │ Go backend │  parse ─▶ resolve ─▶ project
                    └──┬──────┬──┘
                       │      │
              ┌────────▼─┐  ┌─▼──────────────┐
              │ Postgres │  │ Neo4j          │
              │ system   │  │ graph          │
              │ of record│  │ projection     │
              └──────────┘  └────────────────┘
```

## The pipeline

An ingest is three stages, and they are deliberately separable:

**Parse** reads each document alone. Nothing in `internal/parser` looks beyond
the file in hand — relationship targets are left exactly as written, and
matching them against the rest of the directory is somebody else's job. That is
what makes the parser testable one document at a time.

**Resolve** is the stage that needs the whole directory in view: matching
targets to real tables, normalising two-sided declarations into single edges,
binding cross-domain references to a conformed instance, folding upstream
source references onto one identity, and reporting whatever could not be
satisfied. `internal/graph` does this and nothing else, over plain structs, with
no store anywhere near it.

**Project** writes the result: Postgres first, then Neo4j. A failed projection
rolls the snapshot back, so the snapshot list never shows a half-working entry.

Because the first two stages touch no database, [`relctl`](../../usage/cli.md)
is the same code with the third stage removed — which is why a documentation
linter costs nothing to maintain.

## Why two stores

**Postgres** is the system of record: snapshots, domains, tables, columns,
relationships, column lineage, diagnostics, and a weighted `tsvector` index so
searching a column name finds its table. Every read the detail pane needs is a
single indexed query against it.

**Neo4j** holds the projected graph and answers the traversals:

```cypher
(:Table)-[:JOINS {fromColumn, toColumn, cardinality, crossDomain}]->(:Table)
(:Table)-[:DERIVED_FROM {columns}]->(:Source)
(:Table)-[:IN_DOMAIN]->(:Domain)
(:Table)-[:CONFORMS_TO]->(:Conformed)
```

That is what powers neighbourhood queries, `allShortestPaths` between two
tables ("how do I join these?"), and the shared-source sibling lookup ("what
else breaks if this upstream model changes?"). Those are the queries that are
awkward and slow in SQL and a single clause in Cypher, and they are the whole
reason for the second store.

Both stores are reached through interfaces rather than concrete types, so the
handlers' own work — validating parameters, resolving the `latest` alias,
mapping a store outcome to a status code — is unit-tested against fakes with no
database running. The interfaces are deliberately the exact set of methods the
handlers call, so a fake only has to implement what the HTTP surface actually
uses, and a compile-time assertion keeps the real stores honest against them.

That seam also leaves room for a Neo4j-less mode answering traversals out of
Postgres. Today `cmd/server` always wires the real graph store; nothing else
implements the interface.

## Snapshots

Every ingest is a snapshot with its own ID, and both stores are snapshot-scoped
throughout — every table, every node, every query carries it. Re-ingesting the
same directory produces a new snapshot rather than mutating one, so a model can
be compared with itself over time, and deleting one cannot disturb another.

Read routes accept `latest` in place of an ID, which resolves to the most
recent ingest.

## Repository layout

```
backend/
  cmd/server/        HTTP server
  cmd/relctl/        CLI: parse a directory, print stats and diagnostics
  internal/parser/   markdown → structured documents
  internal/graph/    resolution, edge normalisation, drift detection
  internal/store/    postgres (record) + neo4j (graph)
  internal/api/      routes, ingest
  tests/             unit, integration and fixtures, outside the packages
frontend/
  src/api/           typed client and response shapes
  src/components/    GraphCanvas, TableDetail, FilterSidebar, Search, Diagnostics
  src/graph/         layout engines, hulls, role palette
  src/stores/        Pinia workspace state
  src/composables/   directory picker, theme
  src/styles/        design tokens + base CSS  (no Tailwind)
k8s/
  base/  overlays/{dev,prod}
docs/
  usage/             guides for people using the tool
  tech/              architecture, design notes, troubleshooting
  demo/              seven sample documentation sets, pinned by the demo suite
```
