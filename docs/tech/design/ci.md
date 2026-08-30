# CI

Six workflows in `.github/workflows`, each with its own path filter so a change
to one part of the tree does not start runners for the rest:

| Workflow | Runs when | Does |
| --- | --- | --- |
| `backend.yml` | `backend/**`, `docs/demo/**` | gofmt, vet, build, unit tests; integration tests against service containers |
| `frontend.yml` | `frontend/**` | typecheck, unit and component tests, production build, CSP assertions |
| `manifests.yml` | `k8s/**` | renders both overlays and schema-checks them with kubeconform |
| `images.yml` | Dockerfiles, nginx config, lockfiles | builds both images; asserts the backend image is non-root |
| `security.yml` | dependency manifests, plus Mondays | govulncheck, `npm audit`, and a check that every dependency still resolves to the public registry |
| `workflows.yml` | `.github/workflows/**` | ghalint and actionlint over the workflows themselves |

Every action is pinned to a full commit SHA, every job declares its own
`permissions` and a `timeout-minutes`, and checkout never persists credentials.
`workflows.yml` is what keeps that true — ghalint fails the build otherwise.

`backend.yml` runs the integration suites against Postgres and Neo4j service
containers, and then fails the job if any suite *skipped*: the suites skip
themselves when they are not told where to connect, so an accidental rename of
an environment variable would otherwise quietly stop testing the stores while
staying green.

## Dependency updates

[Dependabot] opens them, configured in `.github/dependabot.yml`: the Go module,
the frontend's npm tree, both Dockerfiles' base images, and the actions the
workflows pin. Nothing merges itself. An update is an ordinary pull request, so
the path filters in the table above decide what runs on it — a bumped lockfile
starts `security.yml`, a bumped base image starts `images.yml`, a bumped action
starts `workflows.yml`. Sending them through the front door like that is the
point: the checks that would catch a bad hand-written change catch a bad
automated one for free.

They arrive Monday 09:00 JST, the slot `security.yml` already runs in, so the
week opens with the advisory scan and the updates that answer it side by side.

Updates are grouped, so a quiet week is one pull request per ecosystem — two
for npm, which keeps what ships to a browser apart from what only ever runs on
a developer's machine. Majors sit outside every group and arrive on their own,
because they are the ones that need reading rather than merging: a Go major is
an import path change, and a base image major is a decision about the runtime.

Two couplings to know when reviewing one:

- **The Go toolchain is pinned in two places.** `backend/Dockerfile` names
  `golang:1.25.14-alpine`, and `backend/go.mod` names `go 1.25.14`. Dependabot
  moves the image without touching the directive, and the build still passes,
  because the directive is a minimum rather than a pin. They are meant to
  agree, so bump the directive in the same pull request.
- **The runtime image will never show up here.** The backend runs on
  `gcr.io/distroless/static-debian12:nonroot`, whose tag carries no version to
  compare, so moving to a newer Debian stays a manual decision.

[Dependabot]: https://docs.github.com/en/code-security/dependabot

## Running it locally

All of it runs on your machine through [nektos/act], so a broken workflow costs
no Actions minutes to find:

```bash
make ci-lint       # ghalint + actionlint, no containers, about a second
make ci            # what a pull request runs, minus the service containers
make ci-backend    # one workflow at a time
make ci-frontend
make ci-manifests
make ci-security
make ci-images
make ci-workflows
make ci-backend-integration   # needs the compose stack stopped
make ci-clean      # drop the containers act keeps between runs
```

`make ci-lint` is the one worth running every time you touch a workflow. The
rest matter when you change what a job *does*.

Settings live in `.actrc`, and two of them are load-bearing:

- **`--bind`** runs the job against your real working tree instead of a copy.
  Steps that shell out to `docker run -v "$PWD/...":...` hand a path to the
  *host* daemon, so it has to be a host path. On a GitHub runner the job is not
  containerised and `$PWD` already is one; without `--bind` those mounts come up
  empty locally and the job fails for a reason that has nothing to do with your
  change.
- **`--no-cache-server`** stops the `setup-*` actions spending minutes failing
  to save a toolchain cache act cannot serve. `--reuse` keeps the module and
  npm caches warm inside the job container instead. GitHub's caching is
  configured in the workflows and is unaffected.

Two differences from a real runner are worth knowing, and both have bitten:

- **act's shell does not set `pipefail`; GitHub's does.** A step that pipes a
  test run into `tee` will pass locally and fail on GitHub unless it sets
  `pipefail` itself. `backend.yml` does.
- **act runs natively on Apple silicon**, GitHub runners are amd64. Set
  `ACT_ARCH=linux/amd64` to reproduce a runner exactly; it emulates, so it is
  much slower. Native is right for almost everything.

See [troubleshooting](../troubleshooting/ci-and-act.md) when a local run and a
real one disagree.

[nektos/act]: https://github.com/nektos/act
