# The chat service

Answers questions about a documented data model, over the snapshot the request
names.

It reads and writes **only through the Go backend's HTTP API**. It holds no
Postgres or Neo4j credentials, by design: the graph projection's invariants stay
in one place, and the agent cannot write anything the backend has not given it
an endpoint for.

Python 3.12, FastAPI, managed with [uv](https://docs.astral.sh/uv/).

## Credentials

The service needs a model provider, and it refuses to start without the
credential the one it is configured for requires — naming the missing variable
in the log rather than starting and failing every request.

Nothing here is committed. Copy the template and fill in one path:

```bash
cp .env.example .env      # from the repository root
```

### Google AI Studio

The default, and the one with nothing to set up. Get a key from
[aistudio.google.com](https://aistudio.google.com/app/apikey):

```bash
export GOOGLE_API_KEY=...
docker compose up -d --build chat
curl -s localhost:8090/debug/llm
```

Expect `{"text": "pong", ...}`.

### Vertex AI

Vertex authenticates with **Application Default Credentials**, so there is no
key. On the host:

```bash
gcloud auth application-default login
export LLM_PROVIDER=vertex VERTEX_PROJECT=my-project VERTEX_LOCATION=us-central1
```

That is enough to run the service directly:

```bash
cd chat && uv run uvicorn urara_chat.main:app --port 8090
```

**Under compose it needs one extra step.** ADC lives at
`~/.config/gcloud/application_default_credentials.json` on the host, and the
container cannot see it. Mount it read-only and point
`GOOGLE_APPLICATION_CREDENTIALS` at the mount, with a
`docker-compose.override.yml` in the repository root:

```yaml
services:
  chat:
    volumes:
      - ${HOME}/.config/gcloud/application_default_credentials.json:/gcloud/adc.json:ro
    environment:
      GOOGLE_APPLICATION_CREDENTIALS: /gcloud/adc.json
```

Compose picks that file up automatically. It is gitignored: the path is
particular to your machine, and it is not the default configuration because on a
machine that has never run `gcloud` the mount source does not exist and compose
fails with an error about a missing path rather than about credentials.

Then:

```bash
LLM_PROVIDER=vertex VERTEX_PROJECT=my-project docker compose up -d --build chat
curl -s localhost:8090/debug/llm
```

### Telling the two apart

`/readyz` reports what is configured, without calling the provider:

```bash
curl -s localhost:8090/readyz
{"status":"ok","backend":"ok","llm":{"provider":"gemini-studio","model":"gemini-2.5-flash"}}
{"status":"ok","backend":"ok","llm":{"provider":"vertex","model":"gemini-2.5-flash","location":"us-central1"}}
```

Vertex carries a `location`; Studio has no region to report.

## The debug endpoints

Both stay in the service permanently. They are how a bad answer gets explained.

**`GET /debug/llm`** sends one fixed prompt and returns the reply and a latency.
It is the cheapest check that credentials work, and the first thing to reach for
before reading a log line:

```bash
curl -s localhost:8090/debug/llm
{"text":"pong","latencyMs":676.76,"provider":"vertex","model":"gemini-2.5-flash","location":"us-central1"}
```

A provider failure is a `502` with a deliberately generic message — provider
errors quote the request back and can carry prompt fragments — and the real
reason goes to the log:

```bash
docker compose logs chat --tail 5
```

**`POST /debug/tool`** runs one retrieval tool with no model involved, which is
the only way to tell a bad answer caused by bad retrieval from one caused by bad
reasoning:

```bash
curl -s localhost:8090/debug/tools     # the nine tools and their schemas

curl -s -X POST localhost:8090/debug/tool -H 'Content-Type: application/json' \
  -d '{"snapshotId":"latest","tool":"search_model","args":{"query":"orders"}}'
```

`snapshotId` accepts `latest`. An unknown tool or a bad argument is a `400`
naming the problem; an unknown snapshot is a `404`.

## Running it locally

The service talks to a backend, so start the stack first:

```bash
make up                   # from the repository root
```

Then, from this directory:

```bash
uv sync
uv run uvicorn urara_chat.main:app --reload --port 8090
```

## Tests

```bash
make test-chat            # unit tests, in a container; nothing need be running
make lint-chat            # ruff and mypy
make test-chat-integration # against the compose stack
```

Or directly, from this directory:

```bash
uv run pytest tests/unit -q
uv run ruff check . && uv run ruff format --check .
uv run mypy src
```

The integration suite ingests its own snapshot and deletes it afterwards. It
skips unless told where to connect, and CI treats a skip as a failure:

```bash
CHAT_TEST_BACKEND_URL=http://localhost:8080 \
CHAT_TEST_API_TOKEN=relviz-dev-token-not-for-production \
  uv run pytest tests/integration -q -m integration
```

No test calls a model provider.

## The image

```bash
docker build -t urara-vision/chat:dev .
```

It runs as UID 65532 with a read-only root filesystem, matching what the
Kubernetes manifests impose — nothing is written outside `/tmp`.
