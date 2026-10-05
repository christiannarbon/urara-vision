# Chat golden files

What the Python chat service answered, captured before it was retired. The Go
chat service is tested against these files. No model is called to make them.

## Layout

| Path | Holds |
|---|---|
| `fixtures/*.json` | Backend responses copied from the Python service's unit fixtures, for fakes |
| `fixtures/jaffle-context.json` | `GET /api/v1/snapshots/<id>/context` for jaffle-shop-ddd |
| `golden/probes/` | `/healthz`, `/readyz`, and `/readyz` with the backend stopped |
| `golden/debug/` | `/debug/tools`, and `/debug/tool` for each tool plus its error cases |
| `golden/errors/` | Error responses of the `/api/chat` routes |
| `golden/conversations/` | Create, list, get and delete of one conversation |
| `golden/stats/` | `/api/chat/stats` for a user with no conversations |
| `golden/card/` | `render_context_card` over `fixtures/context.json` and `fixtures/jaffle-context.json` |
| `golden/prompt/` | `build_system_prompt` over `card/fixture.txt`, in `EN` and `JA` |

Each JSON file under `golden/` is
`{"status": <int>, "headers": {"Retry-After": <bool>, "X-Request-Id": <bool>}, "body": <JSON or null>}`.
Headers record presence only.

## Provenance

Captured once from the Python service in 16.1. The script went with that
service in 21.3, so these files are now the contract: change one only together
with the code that changes the response. The capture ingested jaffle-shop-ddd
and sakila-oltp-ddd; `stats/empty.json` assumes the bootstrap admin has no
conversations on jaffle-shop-ddd.

## Normalised values

| Value | Becomes |
|---|---|
| Any UUID, also inside a string | `<uuid>` |
| A string starting with an RFC 3339 timestamp | `<time>` |
| `requestId` | `<request-id>` |
| A number under a key containing `latency` or `duration` | `<number>` |

`fixtures/jaffle-context.json` instead gets a fixed snapshot ID and
`createdAt`, so it still decodes.

## Not captured: 429 and 409

Both need a model call to reach. Shapes from the Python service's API code.

`errors.py` `turns_busy`: 429, header `Retry-After: 5`, plus `X-Request-Id`.

```json
{"detail": "too many turns in flight; the limit is 4. Retry in 5 seconds.", "requestId": "<request-id>"}
```

`chat_routes.py` `_run_turn`: 409 when a conversation already has
`MAX_CONVERSATION_TURNS` assistant messages, rendered by `http_exception`.

```json
{"detail": "this conversation has reached its limit of 50 turns; start a new conversation to keep asking", "requestId": "<request-id>"}
```
