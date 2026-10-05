# Chat service

The chat service answers questions about a model. It is a Go binary in the
backend module (`backend/cmd/chat`), built from the backend Dockerfile's `chat`
target, and runs as its own Deployment behind nginx's `/api/chat/`.

It holds **no database credential**. Everything it knows, it reads through the
backend's `/api/v1`, with the same bearer token any other client uses, and it
stores conversations there too. A question, however it is phrased, cannot reach
a database session, and the resolution rules stay in one place.

## Packages

All under `backend/internal/chat/`:

| Package | Does |
|---|---|
| `config` | Reads and validates every setting; the service refuses to start on a bad one |
| `apiclient` | The backend's `/api/v1`, typed |
| `httpapi` | Routes, identity, limits, turns, titles, stats, debug routes |
| `tools` | The fixed tool set the model may call, with argument checks and result shrinking |
| `agent` | The context card, the system prompt, the tool loop, citations |
| `llm` | The `Model` interface, the provider registry, redaction |
| `llm/gemini`, `llm/anthropic` | One adapter per API family |

Only an `llm/<adapter>` package imports a provider SDK. `agent`, `tools` and
`httpapi` see `llm.Model` and nothing else.

## A turn

```
load card ─▶ call model (first call: a tool is required; later: model's choice)
               │
               ├─ no tool calls ─────────────────────────────▶ finalise
               ├─ tool calls, budget notice already sent ─────▶ finalise
               ├─ tool calls, iterations ≥ max or tokens ≥ max ▶ spend budget ─▶ call model
               └─ tool calls ────────────────────────────────▶ run tools ─▶ call model
```

- **Spend budget:** each pending call is answered "budget spent", then the
  model gets one more call, told to answer with what it has.
- **Run tools:** in parallel, results kept in call order; the model reads them
  fenced as untrusted data.
- **Finalise:** the last non-empty answer text, and the citations it rests on.

The code is `agent/loop.go`. The snapshot is bound by the server; no tool takes
a snapshot ID.

## Providers

| `LLM_PROVIDER` | Models | Credential |
|---|---|---|
| `vertex` (default) | Gemini on Vertex AI | ADC |
| `vertex-anthropic` | Claude on Vertex AI | ADC |

Credentials are Application Default Credentials, never a key; see
[deployment](deployment.md#configuration) for local and cluster setup.

To add a provider:

1. An adapter package under `llm/` with a pure `Encode`/`Decode` pair and
   `New` returning an `llm.Model`, with unit tests for both directions.
2. One `llm.Register` line in `cmd/chat/main.go`.
3. Its settings and their validation in `config`.
4. A row in the smoke tests (`backend/tests/llm`, `make test-llm`).
5. An eval run (below) before it is offered.

## Limits

Every limit is a setting, listed with its default in
[deployment](deployment.md#configuration): concurrent turns and how long a
turn waits for a slot (`429` past it), question and body size, the turn
timeout, tool rounds and estimated tokens per turn, history length, and turns
per conversation (`409` past it). One conversation runs one turn at a time,
per process.

## Eval

`make eval` asks the golden questions (`backend/tests/eval/questions.yaml`)
through `POST /api/chat/answer` against the running stack, scores citations,
tools, expected text and refusals, and fails below
`backend/tests/eval/thresholds.yaml`. **It calls a real model and costs
money**, so it is never part of `make test` or CI. Filter with `SET=`,
`CATEGORY=` or `ID=`; results land in `backend/tests/eval/results/`. The runner
is `backend/cmd/chateval`; past runs are written up in
`backend/tests/eval/FINDINGS.md`.

## Fences, as tests

- `backend/tests/unit/chat/fence/fence_test.go`: `cmd/chat` must not import a
  store, the graph, the parser, the backend's API package or a database
  driver. This is what keeps "no database credential" true as the code grows.
- `backend/tests/unit/chat/fence/notes_test.go`: user-written notes never
  reach the agent, since in the prompt they would be an injection path.
- The golden files in `backend/tests/unit/chat/testdata/` fix the HTTP
  contract the frontend depends on.
