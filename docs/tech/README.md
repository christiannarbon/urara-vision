# Technical documentation

For working on Urara Vision rather than with it. If you are pointing the tool
at a documentation tree, [`docs/usage/`](../usage/) is the other half.

## [Architecture](architecture/)

What the pieces are and why there are two stores.

| Document | Covers |
|---|---|
| [Overview](architecture/overview.md) | The pipeline, snapshots, repository layout |
| [The two stores](architecture/data-model.md) | Postgres schema and search index, the Neo4j graph model |
| [HTTP API](architecture/api.md) | Every route, the ingest body, the error mapping |
| [Deployment and configuration](architecture/deployment.md) | Environment variables, compose, Kubernetes |

## [Design notes](design/)

What is inside each piece, and why the rules are those rules.

| Document | Covers |
|---|---|
| [Parsing](design/parsing.md) | Sections, classification, determinism |
| [Resolution](design/resolution.md) | Conformed authority, join-key orientation, edge normalisation |
| [The theme pipeline](design/theming.md) | The generator and its contrast audit |
| [The frontend](design/frontend.md) | Layout engines, hulls and petals, role colour, state |
| [Testing](design/testing.md) | What each layer covers, and isolation |
| [CI](design/ci.md) | The six workflows and running them locally |

## [Troubleshooting](troubleshooting/)

| Document | For |
|---|---|
| [The local stack](troubleshooting/local-stack.md) | compose, ports, an empty graph |
| [Kubernetes](troubleshooting/kubernetes.md) | pods that will not start, lost data, the tunnel |
| [CI and act](troubleshooting/ci-and-act.md) | a local run and a real one disagreeing |

## Documentation that lives next to its code

Three READMEs stay where they are, because they are read while looking at the
thing they describe:

- [`k8s/README.md`](../../k8s/README.md) — the manifests, the overlays, and the
  image-specific gotchas the live deploy exposed
- [`backend/tests/README.md`](../../backend/tests/README.md) — running the
  suites, and why the tests sit outside the packages they test
- [`docs/demo/README.md`](../demo/README.md) — the seven sample sets and what
  each one is built to demonstrate
