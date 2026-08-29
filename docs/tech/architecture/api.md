# HTTP API

Everything under `/api/v1` is behind the bearer token when `API_TOKEN` is set;
the probes deliberately are not, because kubelet cannot carry a credential.

Every read route accepts `latest` in place of a snapshot ID.

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/api/v1/ingest` | Parse an uploaded directory into a snapshot |
| `GET` | `/api/v1/snapshots` | List snapshots |
| `GET` | `/api/v1/snapshots/{sid}` | Snapshot metadata and stats |
| `DELETE` | `/api/v1/snapshots/{sid}` | Delete a snapshot from both stores |
| `GET` | `/api/v1/snapshots/{sid}/domains` | Domains, with descriptions and mermaid |
| `GET` | `/api/v1/snapshots/{sid}/tables` | Table summaries (`?domain=`) |
| `GET` | `/api/v1/snapshots/{sid}/table?id=` | One table in full, plus referrers and lineage |
| `GET` | `/api/v1/snapshots/{sid}/graph` | Node-link graph (`?domain=&kind=&sources=&crossDomainOnly=`) |
| `GET` | `/api/v1/snapshots/{sid}/neighborhood?table=` | Subgraph within `?depth=` hops |
| `GET` | `/api/v1/snapshots/{sid}/paths?from=&to=` | Shortest join paths between two tables |
| `GET` | `/api/v1/snapshots/{sid}/lineage?id=` | Upstream or `?direction=downstream` |
| `GET` | `/api/v1/snapshots/{sid}/search?q=` | Full-text over tables and columns |
| `GET` | `/api/v1/snapshots/{sid}/diagnostics` | Documentation problems (`?severity=`) |
| `GET` | `/api/v1/snapshots/{sid}/sources` | Upstream source models by reference count |
| `GET` | `/healthz`, `/readyz` | Liveness; readiness includes both datastores |

Table IDs are `domain/table` and contain a slash, so they travel as a query
parameter rather than a path segment:

```bash
curl 'localhost:8080/api/v1/snapshots/latest/paths?from=domain_one/fact_primary&to=domain_two/dim_beta'
```

## Ingest

`POST /api/v1/ingest` takes either JSON or multipart, because the browser sends
one and a script finds the other easier:

```bash
curl -X POST localhost:8080/api/v1/ingest \
  -H 'Content-Type: application/json' \
  -d '{
        "name": "my model",
        "sourceLabel": "local",
        "files": [
          {"path": "domain_one.md", "content": "# Domain One\n\n## Description\n..."},
          {"path": "domain_one/fact_primary.md", "content": "# fact_primary\n..."}
        ]
      }'
```

In multipart, each file part carries its relative path as the form field name,
and `name` and `sourceLabel` are plain fields. Either way anything that is not
a `.md` file is dropped rather than rejected, so posting a whole directory is
fine.

It returns `201` with the snapshot, its stats, the normalised edge count and
every diagnostic — so a caller knows what its documentation resolved to without
a second request. `MAX_FILES` and `MAX_UPLOAD_BYTES` bound what it will accept.

Paths arrive from the browser and are the one piece of genuinely untrusted
input the server takes; they are normalised before anything else looks at them.

## The graph response

`/graph` returns a node-link shape — `{"nodes": [...], "links": [...]}` — which
is what the canvas consumes directly. Nodes carry their role, domain, column
count and degree; links carry both columns, the cardinality and whether they
cross a domain boundary.

## Errors

Failures are JSON with an `error` field and the status the outcome maps to:

| Status | When |
|---|---|
| `400` | A parameter the handler can see is wrong, a body that will not decode, no `.md` files, too many files, or an upload past `MAX_UPLOAD_BYTES` |
| `401` | `API_TOKEN` is set and the request did not carry it as `Authorization: Bearer <token>` |
| `404` | No such snapshot or table — including `latest` when nothing has been ingested yet, which says so rather than returning an empty graph |
| `500` | Anything the stores report; the detail is logged with the request ID rather than returned |
