# Chat golden files

What the Python chat service (`chat/`) answers today. The Go chat service is
tested against these files. No model is called to make them.

## Layout

| Path | Holds |
|---|---|
| `fixtures/*.json` | Backend responses copied from `chat/tests/unit/fixtures/`, for fakes |
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

## Rerunning

Start the stack without the local override, so no real Vertex project is set:

```bash
LLM_PROVIDER=vertex VERTEX_PROJECT=golden-capture-calls-no-model \
  docker compose -f docker-compose.yml up -d --build postgres neo4j backend chat frontend
bash backend/tests/unit/chat/testdata/capture.sh
```

The script ingests jaffle-shop-ddd and sakila-oltp-ddd, or reuses them when
that version already exists. It turns chat off and on through the backend,
stops and restarts `backend`, and deletes the conversation it creates.
`stats/empty.json` assumes the bootstrap admin has no conversations on
jaffle-shop-ddd.

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

Both need a model call to reach. Shapes from `chat/src/urara_chat/api/`.

`errors.py` `turns_busy`: 429, header `Retry-After: 5`, plus `X-Request-Id`.

```json
{"detail": "too many turns in flight; the limit is 4. Retry in 5 seconds.", "requestId": "<request-id>"}
```

`chat_routes.py` `_run_turn`: 409 when a conversation already has
`MAX_CONVERSATION_TURNS` assistant messages, rendered by `http_exception`.

```json
{"detail": "this conversation has reached its limit of 50 turns; start a new conversation to keep asking", "requestId": "<request-id>"}
```
