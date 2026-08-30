# The `uraractl` CLI

`uraractl` parses a directory without needing the server or any datastore, which
makes it useful as a documentation linter in CI:

```bash
cd backend
go run ./cmd/uraractl -dir /path/to/model-docs
go run ./cmd/uraractl -dir /path/to/model-docs -json     # full model as JSON
go run ./cmd/uraractl -dir /path/to/model-docs -strict   # exit 1 on any error
```

The directory needs its `projectmeta.toml` here too: it is read and validated
first, so a manifest the server would refuse fails in CI rather than at upload
time. A missing or invalid one exits 2 and says what is wrong with it.

It prints a summary of what it found, then the diagnostics themselves:

```
project        <name> <version>
languages      <tag> <tag> (primary <tag>, <type>)
files parsed   <n> (skipped <n>)
domains        <n>
tables         <n>  (conformed instances <n>)
columns        <n>
relationships  <n> declared -> <n> normalised edges
lineage edges  <n> across <n> source tables
roles          <role>=<n> <role>=<n> ...
diagnostics    error=<n> warning=<n> info=<n>
```

The same binary ships inside the backend image at `/app/uraractl`, so a CI job
that already pulls the image needs nothing else installed:

```bash
docker run --rm -v "$PWD/docs:/docs" --entrypoint /app/uraractl \
  urara-vision/backend:dev -dir /docs/data-modelling -strict
```

The `--entrypoint` is needed because the image's own entrypoint is the server.

`-strict` exits non-zero only on `error` diagnostics —
[`unresolved_reference`](diagnostics.md) is currently the only one — so
warnings inform without failing a build.

`-json` writes the whole resolved model: domains, tables, columns, normalised
edges, lineage and diagnostics. That is the same shape the API returns, so it
is a reasonable input for anything else you want to build on top.
