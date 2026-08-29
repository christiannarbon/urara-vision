# Backend tests

```
tests/
  fixtures/      the model documents both suites parse
  unit/          no databases; runs anywhere
  integration/   real Postgres and Neo4j, behind a build tag
    harness/     connecting to them, and cleaning up afterwards
```

## Running them

```bash
make test-unit         # from the repository root
make test-integration
```

`make test` runs the unit suites plus the frontend; `make test-all` adds the
integration suites.

## Why the tests are outside the packages they test

Go only lets a test file reach a package's unexported identifiers from inside
that package's own directory. Everything here therefore exercises the exported
API, which is a deliberate constraint rather than an accident of the layout: a
test that can only see what a caller can see cannot be written against an
implementation detail, and the suites survived a substantial restructuring of
`internal/graph` without a single assertion changing.

Where a rule lives in an unexported function, it is driven through the exported
entry point instead. Join-key splitting and type-label normalisation go through
`parser.Parse`; upload path filtering goes through `POST /api/v1/ingest`.

## Unit suites

No database, no network, no Docker. Both stores are reached through interfaces
(`api.MetaStore` and `api.GraphStore`), so the HTTP handlers run against fakes
and the suite covers what the handlers themselves decide: which parameters are
required, what an absent or malformed one defaults to, how the `latest` alias
resolves, and which status code a given store outcome becomes.

## Integration suites

Behind `//go:build integration`, and skipped unless they are told where to
connect. That keeps `go test ./...` fast and dependency-free.

| Variable | Purpose |
|---|---|
| `TEST_POSTGRES_DSN` | Postgres connection string. Unset means skip. |
| `TEST_NEO4J_URI` | Bolt URI. Unset means skip. |
| `TEST_NEO4J_USER` | Defaults to `neo4j`. |
| `TEST_NEO4J_PASSWORD` | |

`make test-integration` starts the compose stack, creates a `relviz_test`
database so a run cannot disturb whatever you have ingested locally, and points
the tests at both services. To run them against something else:

```bash
cd backend
TEST_POSTGRES_DSN='postgres://user:pass@host:5432/db?sslmode=disable' \
TEST_NEO4J_URI='bolt://host:7687' \
TEST_NEO4J_PASSWORD='...' \
go test -tags=integration -count=1 ./tests/integration/...
```

### Isolation

Every test owns a fresh snapshot ID and deletes it when it finishes. Both stores
scope everything they hold by snapshot ID and replace on write, so a unique ID
is all the isolation the suites need — they can share an instance without
colliding, and cannot touch a snapshot they did not create.

Postgres gets a separate database because it is cheap to create one. Neo4j
Community allows only one, so the graph suites share it and rely on
snapshot-scoping alone. That is why they clean up after themselves even when
they fail.

`-count=1` is in the Makefile target on purpose: a cached pass tells you nothing
about a database.
