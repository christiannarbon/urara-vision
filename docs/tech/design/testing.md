# Testing

```bash
make test              # backend unit tests, frontend typecheck and unit tests
make test-integration  # backend against real Postgres and Neo4j
make test-all          # everything
make test-cover        # backend coverage summary
```

Tests live in a dedicated tree, split by what they need to run:

```
backend/tests/unit/          no databases; runs anywhere
backend/tests/integration/   real Postgres and Neo4j, behind a build tag
backend/tests/fixtures/      the model documents both suites parse
backend/tests/unit/demo/     the nine shipped sample sets in docs/demo, pinned
frontend/tests/unit/         pure functions and the store, API client mocked
frontend/tests/integration/  the real client over a stubbed network, and
                             mounted components
```

## What each layer is for

**The unit suites** cover the cases that are easy to get wrong: two-sided
relationship deduplication, direction and column flipping for `One-to-many`,
join-key orientation against authoring order, conformed-authority selection,
drift detection, and the hull geometry that must actually enclose its nodes.

**The integration suites** cover what no fake can stand in for — the SQL, the
`ON DELETE CASCADE`s, the full-text index and the Cypher. They are behind an
`integration` build tag and skip themselves unless told where to connect, so
`go test ./...` stays fast and needs nothing running. CI treats a skip as a
failure, because a suite that quietly stops testing anything is worse than one
that goes red.

**The demo suite** pins what each set under `docs/demo` resolves to —
statistics, diagnostic counts by code, and the case that set exists to
demonstrate — so a sample whose flaws get tidied up fails rather than quietly
leaving a check uncovered. It is also the closest thing to an end-to-end test of
the parser and resolver: nine real documentation sets, 100-odd tables, no
mocks.

## Isolation

Integration tests isolate by snapshot ID rather than by database. Both stores
are snapshot-scoped and replace whatever was written under the same ID, so each
test takes a fresh UUID and deletes it afterwards. That means the suites can
run against a shared instance without colliding, and cannot disturb snapshots
they did not create.

It also means the packages can run in parallel, which is the default for
`go test ./...` — and why anything they share, such as Neo4j's schema
constraints, has to tolerate concurrent callers.

[`backend/tests/README.md`](../../../backend/tests/README.md) has the rest,
including how to point the integration suites at something other than the
bundled compose stack.
