# syntax=docker/dockerfile:1

# --- build ---------------------------------------------------------------
FROM golang:1.27.0-alpine AS build

WORKDIR /src

# Dependencies are copied first so the module cache survives source edits.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .

ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build \
      -trimpath \
      -ldflags="-s -w -X main.version=${VERSION}" \
      -o /out/server ./cmd/server && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/uraractl ./cmd/uraractl

# --- runtime -------------------------------------------------------------
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app
COPY --from=build /out/server /app/server
COPY --from=build /out/uraractl /app/uraractl

USER nonroot:nonroot
EXPOSE 8080

ENV APP_ADDR=:8080

ENTRYPOINT ["/app/server"]
