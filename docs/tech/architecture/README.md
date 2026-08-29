# Architecture

How the pieces fit and why there are two of them.

| Document | Covers |
|---|---|
| [Overview](overview.md) | The pipeline, why two stores, snapshots, repository layout |
| [The two stores](data-model.md) | The Postgres schema and search index, the Neo4j graph model |
| [HTTP API](api.md) | Every route, the ingest body, the error mapping |
| [Deployment and configuration](deployment.md) | Environment variables, compose, Kubernetes, the overlays |

For how a particular stage does its work — the parser, the resolver, the theme
generator — see the [design notes](../design/).
