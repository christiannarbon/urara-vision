# HTTP API

Everything under `/api/v1` except `POST /api/v1/auth/login` needs a signed-in
session cookie or the service bearer token (see [Authentication](#authentication)).
The probes do not, because kubelet cannot carry a credential.

Every read route accepts `latest` in place of a snapshot ID.

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/api/v1/ingest` | Parse an uploaded directory into a snapshot |
| `GET` | `/api/v1/snapshots` | List snapshots |
| `GET` | `/api/v1/snapshots/{sid}` | Snapshot metadata and stats |
| `DELETE` | `/api/v1/snapshots/{sid}` | Delete a snapshot from both stores |
| `GET` | `/api/v1/projects` | List projects, most recently updated first |
| `GET` | `/api/v1/projects/{project}` | One project by slug, with its version count and latest snapshot |
| `DELETE` | `/api/v1/projects/{project}` | Delete a project and every snapshot in it from both stores |
| `GET` | `/api/v1/projects/{project}/versions` | A project's snapshots, newest first |
| `GET` | `/api/v1/projects/{project}/versions/{version}` | One version's snapshot; `latest` is the newest |
| `DELETE` | `/api/v1/projects/{project}/versions/{version}` | Delete one version from both stores |
| `GET` | `/api/v1/projects/{project}/diff?from=&to=` | What changed in the model between two versions |
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
| `PATCH` | `/api/v1/conversations/{cid}` | Retitle a conversation |
| `DELETE` | `/api/v1/conversations/{cid}` | Delete a conversation |
| `POST` | `/api/v1/conversations/{cid}/messages` | Append a turn |
| `GET` | `/api/v1/features` | Whether chat is deployed and switched on |
| `PATCH` | `/api/v1/settings` | Switch chat on or off at runtime |
| `POST` | `/api/v1/auth/login` | Sign in; sets the session cookie |
| `POST` | `/api/v1/auth/logout` | End the current session |
| `GET` | `/api/v1/auth/me` | The caller and their permissions |
| `POST` | `/api/v1/auth/password` | Change your own password |
| `GET` | `/api/v1/auth/session` | `204` with identity headers, for nginx `auth_request` |
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
and `name`, `sourceLabel` and `project` are plain fields. Either way anything that is
neither a `.md` file nor the manifest is dropped rather than rejected, so
posting a whole directory is fine.

The upload must include the directory's
[`projectmeta.toml`](../../usage/documentation-format.md#the-manifest), at the
root: it is read and validated before any document is, and an upload without a
valid one is `400` with every problem the manifest has listed at once. It is
stored with the snapshot and comes back on it as `project`, so a caller reading
`/snapshots` sees what project and version each ingest documented.

`project` is optional: the slug the caller expects the manifest to name. When
it does not match, the upload is `400` naming both slugs.

A project holds each version once. Importing a version it already has is `409`,
checked before any document is parsed:

```json
{"error": "version 0.1.0 of jaffle-shop-ddd already exists; delete it first to re-import",
 "project": "jaffle-shop-ddd", "version": "0.1.0"}
```

Two imports racing for the same version get the same `409` from the database.

It returns `201` with the snapshot, its stats, the normalised edge count and
every diagnostic — so a caller knows what its documentation resolved to without
a second request. `MAX_FILES` and `MAX_UPLOAD_BYTES` bound what it will accept.
The response also carries `"project": {"id", "slug"}`, the project the snapshot
was saved under.

Paths arrive from the browser and are the one piece of genuinely untrusted
input the server takes; they are normalised before anything else looks at them.

## Projects

A project groups the snapshots of one documentation set. Its slug comes from
`project.name` in the manifest: lower-cased, every run of characters other than
`a–z` and `0–9` turned into one `-`, trimmed, and cut to 64 characters. Every
ingest with the same slug joins the same project and takes over its name and
description. A snapshot from before manifests named projects has a project of
its own, slugged `legacy-` plus 12 hex characters.

Snapshots carry `projectId` and `projectSlug`. A project summary looks like:

```json
{
  "id": "…", "slug": "jaffle-shop-ddd", "name": "jaffle-shop-ddd", "description": "…",
  "createdAt": "…", "updatedAt": "…",
  "versionCount": 3,
  "latest": {"snapshotId": "…", "version": "0.1.0", "createdAt": "…"}
}
```

`GET /api/v1/projects` returns `{"projects": [...]}` and `GET
/api/v1/projects/{project}` one summary; an unknown slug is `404 {"error":
"project not found"}`. `DELETE /api/v1/projects/{project}` removes the project
and every snapshot in it, then clears each snapshot's graph projection, and
answers `204`. As with a snapshot delete, Postgres is the record of truth: a
graph projection that fails to clear is logged, not reported as a failure.

### Versions

A version turns a project slug and a version label into a snapshot; every read
after that goes through `/snapshots/{sid}/...`.

- `GET …/versions` returns `{"versions": [...]}`, newest import first. An
  unknown project is `404 {"error": "project not found"}`.
- `GET …/versions/{version}` returns the snapshot. `latest` is the most recent
  import. An unknown version is `404 {"error": "version not found"}`.
- `DELETE …/versions/{version}` removes that snapshot, clears its graph
  projection, and answers `204`. Deleting the last version removes the project
  too. `latest` is refused with `400`: name the version you mean.

Percent-encode the label in the path, e.g. `1.0.0%2Bbuild.7` or `2024%20Q1`.
It is decoded once; a malformed escape is `400`.

### Diff

`GET …/diff?from=<version>&to=<version>` compares two versions of one project.
Both parameters are required (`400 {"error": "from is required"}`), and
`latest` works for either. An unknown project is `404 {"error": "project not
found"}`; an unknown version is `404 {"error": "version <v> not found"}`.
`from == to` is an empty diff, not an error.

```json
{
  "project": "jaffle-shop-ddd",
  "from": { "version": "0.1.0", "snapshotId": "…" },
  "to":   { "version": "0.2.0", "snapshotId": "…" },
  "summary": {
    "domains":       { "added": 0, "removed": 0, "changed": 1 },
    "tables":        { "added": 1, "removed": 1, "changed": 2 },
    "columns":       { "added": 3, "removed": 0, "changed": 1 },
    "relationships": { "added": 1, "removed": 0, "changed": 0 },
    "lineage":       { "added": 0, "removed": 2, "changed": 0 }
  },
  "domains": [ { "id": "ordering", "change": "changed",
                 "fields": [ { "field": "description", "from": "…", "to": "…" } ] } ],
  "tables":  [ { "id": "ordering/fact_orders", "domainId": "ordering", "change": "changed",
                 "fields": [ { "field": "grain", "from": "…", "to": "…" } ],
                 "columns": [ { "name": "amount", "change": "changed",
                                "fields": [ { "field": "type", "from": "int", "to": "numeric" } ] } ] } ],
  "relationships": [ { "fromTableId": "…", "toTableId": "…", "targetRef": "…",
                       "fromColumn": "…", "toColumn": "…", "change": "added", "fields": [] } ],
  "lineage": [ { "tableId": "…", "column": "…", "sourceTable": "…", "sourceColumn": "…",
                 "change": "removed", "fields": [] } ]
}
```

- `change` is `added`, `removed` or `changed`. Unchanged things are left out,
  and every list is sorted, so the same pair always gives the same body.
- Things are matched by name, not by ID or position. Tables match by
  `domain/table`, columns by name, joins by from-table, the target reference
  as written and both columns, lineage by table, column and source. A renamed
  table is one removed and one added; so is a join whose reference is
  rewritten.
- `fields` lists only what differs. Domains compare `title`, `description`;
  tables `kind`, `grain`, `updateFrequency`, `layer`, `description`,
  `conformed`; columns `type`, `description`, `isPk`, `isFk`; joins
  `cardinality`, `resolution`, `toTableId`; lineage `notes`, `derived`. A join
  whose target stops resolving is `changed`, with `toTableId` going to `""`.
- A table whose only changes are in its columns is `changed` with empty
  `fields`. Added and removed tables list no columns, and their columns are not
  counted in `summary.columns`.
- `project` is the slug from the path. `toTableId` is `""` for an unresolved
  join. Values are as stored, so `cardinality` reads as the document wrote it
  (`Many-to-one`).

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

## Features and settings

Chat has two switches, and `GET /api/v1/features` combines them so no caller
has to:

```json
{"chat": {"available": true, "enabled": false}}
```

`available` is `CHAT_ENABLED`, fixed at deploy time. `enabled` is `available`
and the runtime `chat.enabled` setting, which defaults to on.

`PATCH /api/v1/settings` changes the runtime setting:

```bash
curl -X PATCH localhost:8080/api/v1/settings \
  -H 'Content-Type: application/json' \
  -d '{"chatEnabled": false}'
```

It answers `200` with the same body as `/features`. A body without
`chatEnabled`, or with any other field, is `400`. Turning chat on while
`CHAT_ENABLED=false` is `409`, because there is no chat service to turn on.

## Authentication

A request is identified in this order:

1. `AUTH_DISABLED=true`: every request is an anonymous admin. Local use only.
2. A `urara_session` cookie: the signed-in user. An unknown or expired session is
   `401` and clears the cookie.
3. `Authorization: Bearer <API_TOKEN>`: the service (chat). With
   `X-Acting-User: <user id>` it acts as that user instead. `X-Acting-User`
   without the token is ignored.

A bearer token that does not match is `401`, even alongside a valid cookie.

**CSRF.** Cookie-authenticated `POST`, `PUT`, `PATCH` and `DELETE` must send
`X-Requested-With: urara`, or they get `403`. Bearer calls are exempt.

### Login

```bash
curl -c jar -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"..."}' localhost:8080/api/v1/auth/login
```

`200 {"user": {...}}` and a `urara_session` cookie: `HttpOnly`,
`SameSite=Lax`, `Path=/`, `Secure` unless `COOKIE_SECURE=false`, expiring after
`SESSION_TTL_HOURS` (168). The token is never in the body; only its SHA-256 is
stored.

An unknown user and a wrong password both answer `401 {"error":"invalid
username or password"}`, in the same time.

**Rate limits**, on failures in a sliding 15 minutes: 5 per username and 20 per
client IP. Past either, `429` with `Retry-After` in seconds. A successful login
clears the username's count. Limits are per replica.

### The other routes

| Route | Answer |
|---|---|
| `POST /auth/logout` | `204`. For a cookie session, deletes it and clears the cookie |
| `GET /auth/me` | `{"user": {...} or null, "kind": "user\|service\|anonymous", "permissions": [...]}` |
| `POST /auth/password` | `{"current","new"}` → `204`, ending the user's other sessions. Wrong current `401` (rate-limited like login), weak new `400`, non-user `403` |
| `GET /auth/session` | `204` with `X-User-Id` and `X-User-Role` for a user; `401` otherwise |

## Errors

Failures are JSON with an `error` field and the status the outcome maps to:

| Status | When |
|---|---|
| `400` | A parameter the handler can see is wrong, a body that will not decode, a missing or invalid `projectmeta.toml`, an ingest whose manifest names a different `project`, a malformed version escape or a delete of version `latest`, no `.md` files, too many files, or an upload past `MAX_UPLOAD_BYTES` |
| `401` | Not signed in, an expired session, a wrong bearer token, an unknown `X-Acting-User`, or a failed login |
| `403` | A cookie-authenticated write without `X-Requested-With: urara`, or a password change by a non-user |
| `404` | No such snapshot, table or project — including `latest` when nothing has been ingested yet, which says so rather than returning an empty graph |
| `409` | Importing a version the project already has, or turning chat on while `CHAT_ENABLED=false` |
| `429` | Too many failed logins or password checks; see `Retry-After` |
| `500` | Anything the stores report; the detail is logged with the request ID rather than returned |
