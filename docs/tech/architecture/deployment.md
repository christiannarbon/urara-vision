# Deployment and configuration

Three ways to run the same two images: compose for a laptop, Kubernetes for
anything else, and the two apps natively for hot reload while developing.

## Configuration

Everything the server needs comes from the environment:

| Variable | Default | Notes |
|---|---|---|
| `APP_ADDR` | `:8080` | Listen address |
| `POSTGRES_DSN` | `postgres://relviz:relviz@localhost:5432/relviz?sslmode=disable` | |
| `NEO4J_URI` | `bolt://localhost:7687` | |
| `NEO4J_USER` | `neo4j` | |
| `NEO4J_PASSWORD` | — | Required; the server refuses to start without it |
| `API_TOKEN` | _(unset)_ | Shared bearer token for `/api/v1`. Unset disables authentication; if set it must be 24+ characters |
| `CORS_ORIGINS` | `http://localhost:5173,http://localhost:8081` | Unused when served behind the frontend's proxy |
| `MAX_UPLOAD_BYTES` | `67108864` | 64 MB |
| `MAX_FILES` | `5000` | |
| `SHUTDOWN_TIMEOUT_SECONDS` | `20` | Grace period for in-flight requests on SIGTERM |
| `LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` |

Two of those refuse rather than warn. A missing `NEO4J_PASSWORD` stops the
server: there is no sensible default for a credential. An `API_TOKEN` shorter
than 24 characters is refused outright, because a short token looks like a
control while being trivially guessable — unset it deliberately instead, and
the server logs a warning on every start to say authentication is off.

The frontend needs no runtime configuration: nginx proxies `/api` to the
backend, so the browser only ever talks to one origin. `VITE_API_BASE` (a build
arg) overrides the API base if you want to point it elsewhere.

`relviz` survives the rename as the Postgres role and database, the Kubernetes
secret names (`relviz-postgres`, `relviz-neo4j`) and the `localStorage` key.
Renaming those is not free — a new role and database means an existing volume no
longer matches, and a new secret name means recreating it by hand on any prod
cluster — and none of it is user-visible, so it stayed put.

## Compose

```bash
make up       # build and start everything
make down     # stop it, keeping the volumes
make clean    # stop it and delete the volumes
make logs     # follow the backend
```

Postgres is published on `5433` so it cannot collide with a local install on
the default port.

## Kubernetes

Kustomize, with a `base` and two overlays. Everything enters through the
frontend, which serves the SPA and proxies `/api` to the backend, so the
browser stays on a single origin and CORS never applies.

```bash
make k8s-up      # build, load into the cluster, deploy, wait, open a tunnel
make k8s-status  # pods, volumes, ingress, ingested sets, tunnel state
make k8s-logs    # follow backend logs in-cluster
make k8s-down    # tear it down again, keeping the data
```

`k8s-up` starts minikube and enables the ingress addon if needed, so it works
from a cold machine. It blocks until every pod is genuinely ready and then
prints the URL — about 35 seconds from nothing, 10 on a warm cluster. Both
commands are idempotent; re-running `k8s-up` on a live stack changes nothing
and does not restart pods.

### The data outlives the teardown

`k8s-down` deletes the workloads and keeps the namespace, so the claims holding
Postgres and the graph projection survive and the next `k8s-up` rebinds them.
It says what it found on the way up:

```
==> storage
    reusing the volumes a previous run left behind:
      data-neo4j-0      Bound   8Gi
      data-postgres-0   Bound   8Gi
      logs-neo4j-0      Bound   2Gi
...
  Urara Vision is up:  http://localhost:18081
  already ingested:      sakila-oltp-ddd
```

To start genuinely clean, `make k8s-clean` deletes the namespace and takes the
volumes with it — the same pairing as `make down` and `make clean` on the
compose stack.

One consequence: a datastore keeps the credentials it was initialised with, so
changing the dev overlay's Postgres password does not take effect on an
existing volume. `k8s-clean` is the way through that.

### Overlays

| Overlay | Replicas | Secrets | Storage |
|---|---|---|---|
| `dev` | 1 backend, 1 frontend | Generated, committed on purpose | 8Gi / 8Gi / 2Gi, default storage class |
| `prod` | 3 backend, 2 frontend | Externally managed; the overlay creates none | 50Gi / 50Gi / 5Gi on `standard-rwo`, TLS ingress |

The dev overlay also drops the HPAs and PodDisruptionBudgets, which fight a
single replica.

[`k8s/README.md`](../../../k8s/README.md) has the rest: creating the prod
secrets, resizing a live volume, and the image-specific gotchas the live deploy
exposed.
