# syntax=docker/dockerfile:1

# --- build ---------------------------------------------------------------
FROM golang:1.27.0-alpine AS build

WORKDIR /src

# Dependencies are copied first so the module cache survives source edits.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .

# One stage per image, so a target compiles only its own binaries.
FROM build AS build-chat
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/chat ./cmd/chat

FROM build AS build-server
ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build \
      -trimpath \
      -ldflags="-s -w -X main.version=${VERSION}" \
      -o /out/server ./cmd/server && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/uraractl ./cmd/uraractl

# --- chat runtime --------------------------------------------------------
# Before the server stage, so a build with no --target still ends at the server.
FROM gcr.io/distroless/static-debian12:nonroot AS chat
WORKDIR /app
COPY --from=build-chat /out/chat /app/chat
USER nonroot:nonroot
EXPOSE 8090
ENV APP_ADDR=:8090
ENTRYPOINT ["/app/chat"]

# --- runtime -------------------------------------------------------------
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app
COPY --from=build-server /out/server /app/server
COPY --from=build-server /out/uraractl /app/uraractl

USER nonroot:nonroot
EXPOSE 8080

ENV APP_ADDR=:8080

ENTRYPOINT ["/app/server"]
