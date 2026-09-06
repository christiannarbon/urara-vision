# The chat service

Answers questions about a documented data model, over the snapshot the request
names.

It reads and writes **only through the Go backend's HTTP API**. It holds no
Postgres or Neo4j credentials, by design: the graph projection's invariants
stay in one place, and the agent cannot write anything the backend has not
given it an endpoint for.

Python 3.12, FastAPI, managed with [uv](https://docs.astral.sh/uv/).

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

Configuration comes from the environment; the settings and their defaults are
documented in `config.py` once it exists.

## Tests

```bash
uv run pytest              # unit tests; nothing needs to be running
uv run ruff check .
uv run ruff format --check .
uv run mypy src
```

Integration tests are marked `integration` and skip themselves unless
`CHAT_TEST_BACKEND_URL` names a running backend:

```bash
CHAT_TEST_BACKEND_URL=http://localhost:8080 uv run pytest -m integration
```

## The image

```bash
docker build -t urara-vision/chat:dev .
```

It runs as UID 65532 with a read-only root filesystem, matching what the
Kubernetes manifests impose — nothing is written outside `/tmp`.
