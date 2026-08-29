# CI and act

The point of running the workflows locally is that they agree with GitHub.
Where they do not, it is usually one of these.

## It passes locally and fails on GitHub

**`pipefail`.** act's shell does not set it; GitHub's does. A step that pipes a
test run into `tee` reports the exit status of `tee` locally — which is always
success — and the real status on GitHub. Any step that pipes has to set
`pipefail` itself; `backend.yml` does, and says why.

**Architecture.** act runs natively on Apple silicon; GitHub runners are amd64.
Set `ACT_ARCH=linux/amd64` to reproduce a runner exactly. It emulates, so it is
much slower, and native is right for almost everything.

**Parallelism.** `go test ./...` runs packages in parallel, and a runner's core
count is not yours. Anything shared between packages — a database, Neo4j's
schema constraints — sees more contention on one machine than the other. This
is not hypothetical: concurrent `CREATE CONSTRAINT` from two test packages
deadlocked in Neo4j's schema locks on GitHub while passing locally every time.

## A step mounts an empty directory

`--bind` in `.actrc` runs the job against your real working tree instead of a
copy. Steps that shell out to `docker run -v "$PWD/...":...` hand a path to the
*host* daemon, so it has to be a host path. On a GitHub runner the job is not
containerised and `$PWD` already is one; without `--bind` those mounts come up
empty and the job fails for a reason that has nothing to do with your change.

## The job spends minutes saving a cache

`--no-cache-server` stops the `setup-*` actions failing to save a toolchain
cache act cannot serve. `--reuse` keeps the module and npm caches warm inside
the job container instead. GitHub's own caching is configured in the workflows
and is unaffected.

## `make ci-backend-integration` will not start

Its service containers publish the same ports the compose stack uses, so the
two cannot both be up. The target checks first and tells you to `make down`. If
a previous run died mid-setup, `make ci-clean` removes the containers act left
behind.

## ghalint fails on a workflow you just wrote

Every action must be pinned to a full commit SHA, every job must declare its
own `permissions` and a `timeout-minutes`, and checkout must not persist
credentials. `make ci-lint` runs ghalint and actionlint in about a second — it
is the one worth running every time you touch a workflow.

## The integration job fails with "integration tests skipped"

The suites skip themselves when they are not told where to connect, so the job
greps its own output for `--- SKIP` and fails if it finds any. That means an
environment variable got renamed, or the service containers did not come up —
check the earlier steps rather than the tests.
