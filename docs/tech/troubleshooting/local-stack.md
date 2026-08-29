# The local stack

## The backend exits immediately

```
NEO4J_PASSWORD must be set
```

There is no default for a credential, so the server refuses to start without
one. The compose stack sets it; running `go run ./cmd/server` by hand does not.

```
API_TOKEN must be at least 24 characters (leave it unset to disable authentication)
```

A short token looks like a control while being trivially guessable, so it is
refused rather than accepted. Unset it to run without authentication — the
server logs a warning on every start to say so.

## The backend starts but never becomes ready

It blocks until both datastores accept connections, logging
`waiting for dependency` with the attempt number and the error each time, and
gives up after 40 attempts. On a cold start Neo4j is usually the slow one:
first boot takes noticeably longer than a restart. `docker compose logs neo4j`
will say whether it is still initialising or actually failing.

If Postgres is the one refusing, check the DSN's port. The compose stack
publishes Postgres on **5433**, not 5432, so it cannot collide with a local
install — and the server's default DSN points at 5432. That is why the
local-development command in [getting started](../../usage/getting-started.md)
passes `POSTGRES_DSN` explicitly.

## Port already allocated

A bare `port is already allocated` from the Docker daemon means something else
holds it. The stack uses 8080 (backend), 8081 (frontend),
5433 (Postgres), 7474 and 7687 (Neo4j). The act-based CI targets publish
Postgres and Neo4j on the *same* ports the compose stack uses, so the two
cannot both be up: `make ci-backend-integration` checks first and tells you to
`make down`.

## `make ci-backend-integration` says a port is in use after a failed run

A previous run died before removing its service containers. `make ci-clean`
drops the containers act keeps around.

## The graph is empty after an ingest

If the snapshot lists but draws nothing, the documents parsed but nothing
resolved. Check the diagnostics pane: a set where every `Related Table` cell is
prose produces `narrative_reference` for each one and no edges at all.

If the snapshot does not list, the ingest itself failed — the response carries
the reason, and `400` with `no markdown files were supplied` means the picker
handed over a directory with no `.md` files beneath it.

## Search finds nothing for a word you can see

Search is a prefix search over a weighted `tsvector`, so it matches from the
start of a word: `prim` finds `fact_primary`, `mary` does not. Underscores are
word separators, which is why `primary` matches `primary_id`.

## The integration suites all skip

```
--- SKIP: TestListDomains (0.00s)
    set TEST_POSTGRES_DSN to run this test (see: make test-integration)
```

That is the designed behaviour, not a failure: the suites skip unless told where
to connect, so `go test ./...` needs nothing running. `make test-integration`
starts the stack and passes the variables. CI additionally fails the job when a
suite skips, so a rename cannot quietly stop testing the stores.

## A demo suite fails after you edited a sample set

The seven sets under `docs/demo` are pinned by `backend/tests/unit/demo` —
statistics, diagnostic counts by code, and the specific case each set exists to
demonstrate. Tidying up a deliberate flaw fails the suite on purpose. If the
edit was intended, update the pin in the same commit, and the totals in
[`docs/demo/README.md`](../../demo/README.md) with it.
