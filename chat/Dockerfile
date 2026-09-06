# syntax=docker/dockerfile:1

# --- build ---------------------------------------------------------------
FROM python:3.12-slim AS build

COPY --from=ghcr.io/astral-sh/uv:latest /uv /usr/local/bin/uv

WORKDIR /src

ENV UV_COMPILE_BYTECODE=1 \
    UV_LINK_MODE=copy \
    UV_PYTHON_DOWNLOADS=never

# Dependencies are installed from the lock file before the source is copied, so
# the layer survives edits to the application.
COPY pyproject.toml uv.lock ./
RUN --mount=type=cache,target=/root/.cache/uv \
    uv sync --locked --no-dev --no-install-project

COPY src/ ./src/
RUN --mount=type=cache,target=/root/.cache/uv \
    uv sync --locked --no-dev

# --- runtime -------------------------------------------------------------
# Slim rather than the builder image: no uv, no compiler, nothing that could
# install a package at runtime.
FROM python:3.12-slim

# 65532 matches the backend's securityContext, so both deployments can share one
# pod-level runAsUser without either image having to be special-cased.
RUN groupadd --gid 65532 nonroot \
 && useradd --uid 65532 --gid 65532 --no-create-home --shell /usr/sbin/nologin nonroot

WORKDIR /app
COPY --from=build /src/.venv /app/.venv
COPY src/ /app/src/

# The manifests run this read-only with all capabilities dropped, so nothing may
# be written outside the mounted /tmp: no .pyc files, no buffered log spool.
ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    TMPDIR=/tmp \
    PATH="/app/.venv/bin:$PATH" \
    PYTHONPATH=/app/src

USER 65532:65532
EXPOSE 8090

CMD ["uvicorn", "urara_chat.main:app", "--host", "0.0.0.0", "--port", "8090"]
