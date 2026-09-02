# Kubernetes deployment

Kustomize, with a `base` and two overlays. Everything enters through the
frontend, which serves the SPA and proxies `/api` to the backend, so the
browser stays on a single origin and CORS never applies.

```
k8s/
  base/                 namespace, both datastores, both apps, ingress,
                        network policies, HPAs, PDBs
  overlays/dev/         1 replica each, generated dev secrets, small footprint
  overlays/prod/        3 backend / 2 frontend, TLS ingress, 50Gi volumes,
                        externally-managed secrets
```

## Dev

The dev overlay is self-contained — it generates its own credentials, so
`apply -k` works with no setup:

```bash
docker build -t urara-vision/backend:dev  ./backend
docker build -t urara-vision/frontend:dev ./frontend

# On kind: kind load docker-image urara-vision/backend:dev urara-vision/frontend:dev
# On minikube: eval $(minikube docker-env) before building

kubectl apply -k k8s/overlays/dev
kubectl -n urara-vision rollout status deploy/backend
```

Reach it either through the ingress (`urara-vision.local`, add a hosts
entry) or by port-forwarding:

```bash
kubectl -n urara-vision port-forward svc/frontend 8081:80
```

Those dev credentials are committed deliberately and are safe only because
they are dev credentials. Do not reuse them anywhere real.

The dev overlay also hands the frontend the API token, as `API_AUTHORIZATION`
on its Deployment, so nginx presents it when it proxies `/api` and nobody has
to paste a key in to open the app. The token stays in the cluster: it is read
from the same `relviz-api` secret the backend checks against and never reaches
the browser. The backend is still authenticated, so anything talking to it
directly still needs the token:

```bash
make k8s-token
```

## Prod

The prod overlay deliberately does **not** generate secrets. Create them first,
from a sealed secret, an `ExternalSecret`, or by hand:

```bash
kubectl -n urara-vision create secret generic relviz-postgres \
  --from-literal=username=relviz \
  --from-literal=password="$PG_PASSWORD" \
  --from-literal=database=relviz \
  --from-literal=dsn="postgres://relviz:$PG_PASSWORD@postgres:5432/relviz?sslmode=disable"

kubectl -n urara-vision create secret generic relviz-neo4j \
  --from-literal=password="$NEO4J_PASSWORD"

# The shared bearer token for /api/v1. Generate one rather than choosing it:
kubectl -n urara-vision create secret generic relviz-api \
  --from-literal=token="$(openssl rand -hex 32)"
```

`relviz-api` is not optional: the backend Deployment references it, so the pod
stays in `CreateContainerConfigError` until it exists. That is deliberate --
forgetting it should stop a rollout rather than quietly publish an open API.
To run without authentication on purpose, delete the `API_TOKEN` block from
`base/backend.yaml`; the server logs a warning on every start when it is unset.

Prod leaves `API_AUTHORIZATION` empty, which is what the dev overlay overrides.
The frontend adds no credential of its own there and the app prompts for one,
so hand the token to whoever needs it:

```bash
kubectl -n urara-vision get secret relviz-api -o jsonpath='{.data.token}' | base64 -d
```

Fill it in for a prod deployment only if the ingress in front already decides
who may reach the frontend at all.

Both Postgres keys must agree: the backend reads `dsn` for Postgres, while the Postgres
pod itself reads `username` / `password` / `database`. A mismatch shows up as
the backend retrying its connection at startup.

Then set your hostname in `overlays/prod/prod-ingress.yaml`, point the image
tags at your registry in `overlays/prod/kustomization.yaml`, and apply:

```bash
kubectl apply -k k8s/overlays/prod
```

## Storage

Both datastores are `StatefulSet`s with `volumeClaimTemplates`:

| Claim | Base | Prod | Holds |
|---|---|---|---|
| `data` (postgres) | 8Gi | 50Gi | Parsed snapshots, columns, lineage, search index |
| `data` (neo4j) | 8Gi | 50Gi | Graph projection |
| `logs` (neo4j) | 2Gi | 5Gi | Neo4j server logs |

Both carry `persistentVolumeClaimRetentionPolicy: Retain` for `whenDeleted` and
`whenScaled`. That is the default, and it is spelled out because the teardown
relies on it: `make k8s-down` deletes the StatefulSets and leaves the namespace
standing, so the claims survive and the next `make k8s-up` rebinds them with
everything that was ingested still in place. `make k8s-clean` is the one that
deletes the namespace, and the volumes go with it.

Deleting by hand has the same split -- deleting the StatefulSet keeps its
claims, deleting the namespace does not:

```bash
kubectl -n urara-vision delete statefulset postgres   # data survives
kubectl delete ns urara-vision                        # data does not
```

`volumeClaimTemplates` are immutable once a StatefulSet exists. Resizing a live
install means either editing the PVCs directly (if your storage class allows
expansion) or recreating the StatefulSet with `--cascade=orphan` so the pods
and volumes survive:

```bash
kubectl -n urara-vision delete statefulset postgres --cascade=orphan
kubectl apply -k k8s/overlays/prod
```

The dev overlay leaves `storageClassName` unset so it picks up the cluster
default; prod pins `standard-rwo`. Change that to whatever your cluster offers.

## Things worth knowing

**Postgres runs as uid 70.** The `postgres:16-alpine` image uses uid 70, not the
999 that the Debian-based tags use. The `securityContext` reflects this — if you
switch to `postgres:16` (Debian), change `runAsUser`/`fsGroup`/`runAsGroup` to
999 or the pod will fail to initialise its data directory.

**`NEO4J_AUTH` is assembled at runtime, from a deliberately non-`NEO4J_` variable.**
Neo4j wants `neo4j/<password>` as one value, so it is built from the secret with
Kubernetes' `$(VAR)` expansion — which means the holding variable must be
declared *before* `NEO4J_AUTH` in the env list.

The holding variable is called `RELVIZ_NEO4J_PASSWORD` rather than
`NEO4J_PASSWORD` on purpose. The Neo4j entrypoint maps every `NEO4J_`-prefixed
environment variable onto a server setting, so a variable named
`NEO4J_PASSWORD` becomes a setting called `PASSWORD`, and strict config
validation refuses to start:

```
Failed to read config: Unrecognized setting. No declared setting with name: PASSWORD.
```

The backend reads the same secret under plain `NEO4J_PASSWORD`, which is fine —
that restriction applies only to the Neo4j container itself.

**The frontend runs read-only.** nginx needs writable `/var/cache/nginx`,
`/var/run` and `/etc/nginx/conf.d` (the entrypoint renders the config template
into it), all backed by `emptyDir`.

**Startup probes are generous.** The backend blocks until both datastores accept
connections and applies its schema, so `startupProbe` allows five minutes while
`livenessProbe` stays tight afterwards. On a cold cluster Neo4j is usually the
slow one.

**Network policies need a CNI that enforces them.** Postgres and Neo4j accept
traffic only from the backend; the backend only from the frontend. minikube's
default CNI does *not* implement NetworkPolicy, so on a stock `minikube start`
these objects are accepted and then ignored — any pod in the namespace can
still reach Postgres. That is inert, not broken, but do not read a green
`kubectl get networkpolicy` as proof of isolation. To actually exercise them:

```bash
minikube start --cni=calico
```

Managed clusters (GKE with dataplane v2, EKS with Calico, AKS with Azure NPM)
enforce them as written.

**HPAs are dropped in dev** along with the PDBs, since a single replica cannot
satisfy `minAvailable: 1` during a rollout.

## Verified on minikube

The dev overlay was deployed to minikube (k8s v1.35.1) and exercised end to
end: all three PVCs bound against the default `standard` storage class, the
120-document corpus ingested through the Ingress, and both stores held the
result (Postgres 108 tables / 1,310 columns; Neo4j 211 nodes / 153 `JOINS`).

Two behaviours worth knowing, both observed rather than assumed:

- **The backend outlives its datastores.** Deleting `postgres-0` did not
  restart the backend pod — the connection pool reconnects, `readyz` dips and
  recovers, and `healthz` never fails, so the liveness probe correctly leaves
  it alone. Data survived the pod deletion intact.
- **Cold start is slow, warm restart is not.** On first apply the backend spent
  ~50s retrying while Neo4j booted and its Service DNS appeared, which is what
  the generous `startupProbe` budget is for. A `rollout restart` against a warm
  cluster completes in about 5 seconds.

## Verifying changes

```bash
kubectl kustomize k8s/overlays/dev
kubectl kustomize k8s/overlays/prod

docker run --rm -v /tmp:/work ghcr.io/yannh/kubeconform:latest \
  -strict -summary -kubernetes-version 1.33.0 /work/dev.yaml /work/prod.yaml
```
