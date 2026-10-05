# Authentication

Who is calling, worked out once per request by the backend. Permissions are
defined here but enforced from Phase 14.

## Principals

| Kind | How it arrives | Used by |
|---|---|---|
| `user` | `urara_session` cookie | People in the browser |
| `service` | `Authorization: Bearer <API_TOKEN>` | Chat, CI and CLI imports |
| `user` (acting) | Bearer token plus `X-Acting-User: <user id>` | Chat, calling the backend for a signed-in user |
| `anonymous` | `AUTH_DISABLED=true` | Local use only; acts as an admin |

`X-Acting-User` is ignored without a valid bearer token, and nginx blanks it on
`/api/`. Under `AUTH_DISABLED`, `/auth/session` reports user `anonymous` with
role `admin`, so chat still works through nginx. A wrong bearer token is `401` even alongside a valid cookie. The role
is read from `users` on every request, never from the session or a header.

## Sessions

`POST /api/v1/auth/login` sets `urara_session`: `HttpOnly`, `SameSite=Lax`,
`Path=/`, `Secure` unless `COOKIE_SECURE=false`, lasting `SESSION_TTL_HOURS`
(168). Only the SHA-256 of the token is stored. Failed logins are limited to 5
per username and 20 per IP in 15 minutes. See [the API](api.md#authentication)
for the routes.

**CSRF.** Cookie-authenticated `POST`, `PUT`, `PATCH` and `DELETE` must carry
`X-Requested-With: urara`; a cross-site form cannot set it. Login needs it
too, so another site cannot sign a visitor into its own account. Bearer calls
are exempt.

## The first admin

`BOOTSTRAP_ADMIN_USERNAME` and `BOOTSTRAP_ADMIN_PASSWORD` create an admin at
start when the users table is empty, under an advisory lock so concurrent
replicas create one. Compose and the dev overlay use `admin` /
`relviz-dev-admin-password`; prod reads the `relviz-bootstrap-admin` secret.

## Chat

```
Browser ──cookie──► nginx ──► backend  authenticate middleware: cookie | bearer | bearer+X-Acting-User
                      └─ /api/chat/ ─ auth_request /_auth ─► backend /api/v1/auth/session
                                       └─ sets X-User-Id ─► chat ─ bearer + X-Acting-User ─► backend
```

nginx asks the backend who the caller is before proxying to chat, and sets
`X-User-Id` and `X-User-Role`, replacing anything the browser sent. Without a
session nginx answers `401` and chat is never reached. Chat refuses any
`/api/chat/` request without `X-User-Id`, and forwards it as `X-Acting-User` on
its backend calls. Chat is reachable only through nginx: localhost-bound in
compose, limited by network policy in Kubernetes.

## Permissions

Defined once in `backend/internal/auth/roles.go`; `/api/v1/auth/me` returns the
caller's list. Enforced from Phase 14.

| Permission | Viewer | Creator | Admin | Service |
|---|:-:|:-:|:-:|:-:|
| `project.view` | ✓ | ✓ | ✓ | ✓ |
| `chat.use` | ✓ | ✓ | ✓ | ✓ |
| `note.write` | ✓ | ✓ | ✓ | |
| `project.import` | | ✓ | ✓ | ✓ |
| `version.import` | | ✓ | ✓ | ✓ |
| `project.delete` | | | ✓ | |
| `version.delete` | | | ✓ | |
| `note.moderate` | | | ✓ | |
| `settings.manage` | | | ✓ | |
| `user.manage` | | | ✓ | |
| `user.delete` | | | ✓ | |

## Enforcement

Every `/api/v1` route is listed in `backend/internal/api/permissions.go`:
`routePermissions` maps `"METHOD pattern"` to a permission, and `publicRoutes`
lists the ones any caller may use (login, and the `/auth/*` routes for a
signed-in caller). `POST /ingest` decides in its handler: `version.import` if
the project exists, `project.import` if not.

`RequirePermission` runs inside the authenticated group. It looks up the
matched pattern on the root router, because a group middleware only sees the
subrouter mount. A route missing from the table answers `500`, so it fails
closed.

**Adding a route:** add its entry to the table. `TestPermissionTableCoversEveryRoute`
walks the router and fails for any `/api/v1` route without one, and for any
stale entry. If the route needs a permission no other route uses, also add a
case to `tests/integration/api/role_matrix_test.go`, which checks every role
against the real server and fails when a routed permission has no case.

**403 vs 404.** A caller without the permission gets `403 {"error":"not
allowed"}`. Something that exists but belongs to another user, such as a
conversation, is `404`, so its existence is not revealed.

## Notes

Route permissions follow the table: reading notes needs `project.view`;
posting, editing, resolving and deleting need `note.write`. Authorship is
checked in the handler, because it depends on the note:

- Only a user principal can post. The service token (without
  `X-Acting-User`) and `AUTH_DISABLED` have no author and get `403`.
- Editing a body or deleting needs the author (`author_id` equals the caller)
  or `note.moderate`. Otherwise `403`: notes are visible to every viewer, so a
  `404` would hide nothing.
- Anyone with `note.write` can resolve or reopen a top-level note.

Notes never reach the agent. They are user-written text, so putting them in the
prompt would let anyone who can write a note inject instructions. The chat
backend client has no notes method and no tool mentions notes
(`backend/tests/unit/chat/fence/notes_test.go`), and `/context` and
`/tables/detail` never carry a note body
(`backend/tests/unit/api/context_no_notes_test.go`).

## Chat and permissions

nginx establishes who the caller is, chat forwards it as `X-Acting-User`, and
the backend decides. Chat never checks roles or permissions and never calls
`/auth/me`.

- A backend `403` becomes chat's `403 {"error":"not allowed","requestId":...}`,
  including a `403` from a tool read mid-turn: the turn ends, and the model
  never sees the refusal.
- Conversations are the acting user's own. Another user's is a `404`, and
  `/api/chat/stats` counts only the caller's conversations.

## Adding Google login

A person (`users`) is separate from how they log in (`user_identities`). Google
would be a new `provider = 'google'` row with the Google subject in `subject`
and no `password_hash`, plus a callback route that finds or links the user and
creates the same session. Nothing about sessions, roles or chat changes.
