# Kubernetes

## The backend pod sits in `CreateContainerConfigError`

The Deployment references the `relviz-api` secret, so the pod will not start
until it exists. That is deliberate: forgetting the API token should stop a
rollout rather than quietly publish an open API.

The dev overlay generates it. On prod you create it yourself:

```bash
kubectl -n urara-vision create secret generic relviz-api \
  --from-literal=token="$(openssl rand -hex 32)"
```

To run without authentication on purpose, delete the `API_TOKEN` block from
`base/backend.yaml`.

## The app asks for an API token

The frontend prompts when nginx forwarded the request without a credential and
the backend answered 401. The dev overlay avoids that by giving the frontend
`API_AUTHORIZATION`, built from the `relviz-api` secret; check it survived on
the pod:

```bash
kubectl -n urara-vision exec deploy/frontend -- printenv API_AUTHORIZATION
```

Empty means the patch did not apply, or that `API_TOKEN` is declared after it
in the env list — `$(VAR)` expansion only sees names defined before the one
using them. Prod prompts on purpose; read the token out of the secret and hand
it over.

## Postgres will not initialise its data directory

`postgres:16-alpine` runs as **uid 70**, not the 999 the Debian-based tags use,
and the `securityContext` reflects that. Switching to `postgres:16` without
changing `runAsUser` / `fsGroup` / `runAsGroup` to 999 fails here.

The data also lives in a *subdirectory* of the mount (`PGDATA` is
`/var/lib/postgresql/data/pgdata`) because the image refuses to initialise into
a non-empty mount root.

## Neo4j refuses to start: `No declared setting with name: PASSWORD`

```
Failed to read config: Unrecognized setting. No declared setting with name: PASSWORD.
```

The Neo4j entrypoint maps every `NEO4J_`-prefixed environment variable onto a
server setting, so a variable named `NEO4J_PASSWORD` becomes a setting called
`PASSWORD` and strict config validation refuses to start. That is why the
holding variable in `base/neo4j.yaml` is called `RELVIZ_NEO4J_PASSWORD`, and
why it must be declared *before* `NEO4J_AUTH` in the env list — `NEO4J_AUTH`
is assembled from it with Kubernetes' `$(VAR)` expansion, which only sees
variables defined earlier.

The backend reads the same secret under plain `NEO4J_PASSWORD`, which is fine:
the restriction applies only to the Neo4j container.

## The backend cannot authenticate to Postgres after a redeploy

A datastore keeps the credentials it was initialised with, and the volume
outlives the workload. Changing the password in the dev overlay therefore has
no effect on an existing volume — the pod comes up with the old one and the
backend retries its connection at startup.

`make k8s-clean` deletes the namespace and its volumes, which is the way
through.

The same applies within the secret itself: the backend reads `dsn` while the
Postgres pod reads `username` / `password` / `database`, so all four have to
agree.

## Ingested data disappeared

`make k8s-down` keeps it; `make k8s-clean` does not, and neither does
`kubectl delete ns urara-vision` — a namespace takes its PVCs with it. Check
what survived:

```bash
kubectl -n urara-vision get pvc
```

`make k8s-up` prints the same list under `==> storage` as it comes up, along
with the documentation sets it found already ingested.

## The tunnel is not there

`make k8s-status` says whether the port-forward is running. `make k8s-tunnel`
restarts it; `make k8s-open` runs one in the foreground instead. A port-forward
does not survive the pod it points at being replaced, so a rollout usually
means restarting it.

## Everything is Pending on a fresh cluster

Nothing has a `storageClassName` in the dev overlay, so it takes the cluster
default. A cluster with no default storage class leaves the claims `Pending`
and the StatefulSets waiting on them forever. `kubectl get storageclass` will
show whether one is marked default.

## Something else

[`k8s/README.md`](../../../k8s/README.md) covers the deployment itself:
creating the prod secrets, resizing a live volume, and the read-only frontend's
writable mounts.
