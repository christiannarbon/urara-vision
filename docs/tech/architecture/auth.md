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
`/api/`. A wrong bearer token is `401` even alongside a valid cookie. The role
is read from `users` on every request, never from the session or a header.

## Sessions

`POST /api/v1/auth/login` sets `urara_session`: `HttpOnly`, `SameSite=Lax`,
`Path=/`, `Secure` unless `COOKIE_SECURE=false`, lasting `SESSION_TTL_HOURS`
(168). Only the SHA-256 of the token is stored. Failed logins are limited to 5
per username and 20 per IP in 15 minutes. See [the API](api.md#authentication)
for the routes.

**CSRF.** Cookie-authenticated `POST`, `PUT`, `PATCH` and `DELETE` must carry
`X-Requested-With: urara`; a cross-site form cannot set it. Bearer calls are
exempt.

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

## Adding Google login

A person (`users`) is separate from how they log in (`user_identities`). Google
would be a new `provider = 'google'` row with the Google subject in `subject`
and no `password_hash`, plus a callback route that finds or links the user and
creates the same session. Nothing about sessions, roles or chat changes.
