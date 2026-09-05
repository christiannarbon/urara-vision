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
| `GET` | `/api/v1/snapshots/{sid}/context` | Compact catalogue of a whole snapshot |
| `GET` | `/api/v1/snapshots/{sid}/domains` | Domains, with descriptions and mermaid |
| `GET` | `/api/v1/snapshots/{sid}/tables` | Table summaries (`?domain=`) |
| `GET` | `/api/v1/snapshots/{sid}/tables/detail?ids=` | Several table documents in one call |
| `GET` | `/api/v1/snapshots/{sid}/table?id=` | One table in full, plus referrers and lineage |
| `GET` | `/api/v1/snapshots/{sid}/graph` | Node-link graph (`?domain=&kind=&sources=&crossDomainOnly=`) |
| `GET` | `/api/v1/snapshots/{sid}/neighborhood?table=` | Subgraph within `?depth=` hops |
| `GET` | `/api/v1/snapshots/{sid}/paths?from=&to=` | Shortest join paths between two tables |
| `GET` | `/api/v1/snapshots/{sid}/lineage?id=` | Upstream or `?direction=downstream` |
| `GET` | `/api/v1/snapshots/{sid}/search?q=` | Full-text over tables and columns |
| `GET` | `/api/v1/snapshots/{sid}/diagnostics` | Documentation problems (`?severity=`) |
| `GET` | `/api/v1/snapshots/{sid}/sources` | Upstream source models by reference count |
| `POST` | `/api/v1/conversations` | Start a conversation about a snapshot |
| `GET` | `/api/v1/conversations?snapshot=` | Conversations for a snapshot |
| `GET` | `/api/v1/conversations/{cid}` | One conversation with its messages |
| `DELETE` | `/api/v1/conversations/{cid}` | Delete a conversation |
| `POST` | `/api/v1/conversations/{cid}/messages` | Append a turn |
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
          {"path": "projectmeta.toml", "content": "[project]\nname = \"my model\"\n..."},
          {"path": "domain_one.md", "content": "# Domain One\n\n## Description\n..."},
          {"path": "domain_one/fact_primary.md", "content": "# fact_primary\n..."}
        ]
      }'
```

In multipart, each file part carries its relative path as the form field name,
and `name` and `sourceLabel` are plain fields. Either way anything that is
neither a `.md` file nor the manifest is dropped rather than rejected, so
posting a whole directory is fine.

The upload must include the directory's
[`projectmeta.toml`](../../usage/documentation-format.md#the-manifest), at the
root: it is read and validated before any document is, and an upload without a
valid one is `400` with every problem the manifest has listed at once. It is
stored with the snapshot and comes back on it as `project`, so a caller reading
`/snapshots` sees what project and version each ingest documented.

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

## The context endpoint

`/context` is the whole of a snapshot in one response: its metadata and stats,
every domain, every table, and a count of diagnostics by severity. It exists to
be read in full before anything else is asked — a catalogue small enough to
prime a prompt with, so a caller can go straight to the table it wants instead
of paging the lists to find out what exists.

```bash
curl 'localhost:8080/api/v1/snapshots/latest/context'
```

Being bounded is the point, so prose is cut to fit: domain descriptions to 400
characters and table grains to 200, counted in characters rather than bytes so
the bilingual corpora survive it. Past `MAX_CONTEXT_TABLES` tables the list is
dropped entirely and `truncated` is `true` — `tables` comes back empty rather
than shortened, because a partial catalogue would read as a complete one and
send the caller looking for tables it had simply not been shown. Domains are
never dropped: they are few, and they are what is left to navigate by.

All three severity keys — `error`, `warning`, `info` — are always present, at
zero if need be, so nothing has to distinguish an absent key from a count of
none.

## Batch table detail

`/tables/detail?ids=` returns up to eight table documents in one call, each
entry identical to what `/table?id=` returns for the same ID. Fetching four
tables one at a time costs four round trips; this costs one.

Eight is the cap because a caller wanting more than that wants the table list,
not the documents. Asking for none, or for more than eight, is a `400`.

An ID that does not exist is not an error. It comes back in `missing` alongside
the tables that were found, and the status is still `200`: a caller that guessed
an ID wrong should get a usable answer telling it which ones missed, rather than
a failed call it has to unpick. Both `tables` and `missing` are always lists,
`[]` rather than `null`, so neither needs a guard before it is read.

## Conversations

A conversation is a thread about one snapshot. It is addressed by its own ID
rather than under `/snapshots/{sid}`, because the snapshot it concerns is
settled once, when it is created.

That is also where `latest` is resolved. `POST /api/v1/conversations` accepts
the alias in `snapshotId` and stores the concrete ID it resolved to, so a later
ingest does not change what an existing conversation is about — a thread pinned
to whatever was ingested most recently would silently change subject, and its
earlier answers would then cite tables from a different model.

Turns are appended one at a time and their order is the database's to decide:
`POST /api/v1/conversations/{cid}/messages` returns the stored message with the
`ordinal` it was given. A turn's `role` must be `user`, `assistant` or `system`,
and empty content is refused; both are `400` naming the problem. `citations` is
the table IDs an answer drew on, and comes back as `[]` when it drew on none.

## Errors

Failures are JSON with an `error` field and the status the outcome maps to:

| Status | When |
|---|---|
| `400` | A parameter the handler can see is wrong, a body that will not decode, a missing or invalid `projectmeta.toml`, no `.md` files, too many files, or an upload past `MAX_UPLOAD_BYTES` |
| `401` | `API_TOKEN` is set and the request did not carry it as `Authorization: Bearer <token>` |
| `404` | No such snapshot or table — including `latest` when nothing has been ingested yet, which says so rather than returning an empty graph |
| `500` | Anything the stores report; the detail is logged with the request ID rather than returned |
