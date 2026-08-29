# Urara Vision

[![backend](https://github.com/christiannarbon/urara-vision/actions/workflows/backend.yml/badge.svg?branch=main)](https://github.com/christiannarbon/urara-vision/actions/workflows/backend.yml)
[![frontend](https://github.com/christiannarbon/urara-vision/actions/workflows/frontend.yml/badge.svg?branch=main)](https://github.com/christiannarbon/urara-vision/actions/workflows/frontend.yml)
[![images](https://github.com/christiannarbon/urara-vision/actions/workflows/images.yml/badge.svg?branch=main)](https://github.com/christiannarbon/urara-vision/actions/workflows/images.yml)
[![manifests](https://github.com/christiannarbon/urara-vision/actions/workflows/manifests.yml/badge.svg?branch=main)](https://github.com/christiannarbon/urara-vision/actions/workflows/manifests.yml)
[![security](https://github.com/christiannarbon/urara-vision/actions/workflows/security.yml/badge.svg?branch=main)](https://github.com/christiannarbon/urara-vision/actions/workflows/security.yml)
[![workflows](https://github.com/christiannarbon/urara-vision/actions/workflows/workflows.yml/badge.svg?branch=main)](https://github.com/christiannarbon/urara-vision/actions/workflows/workflows.yml)

![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)
![Vue](https://img.shields.io/badge/Vue-3-4FC08D?logo=vuedotjs&logoColor=white)
![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16-4169E1?logo=postgresql&logoColor=white)
![Neo4j](https://img.shields.io/badge/Neo4j-5-4581C3?logo=neo4j&logoColor=white)
![Kubernetes](https://img.shields.io/badge/Kubernetes-ready-326CE5?logo=kubernetes&logoColor=white)

A visualization tool for Open Knowledge Format documentation of domain-driven
data models. Point it at a directory of markdown model documents and it becomes
an interactive relationship graph: the backend parses every document, resolves
the joins between tables, projects the result into a graph, and reports every
reference it could not satisfy.

It knows nothing about any particular business domain, and nothing about any
particular modelling style. The layout it reads is the convention — a domain
index per directory, a document per table — so any model documented that way is
a model it can draw: a Kimball star, a snowflake, a Data Vault, a plain
third-normal-form schema, or a vocabulary of roles it has never met.

---

## Quick start

```bash
docker compose up -d --build
open http://localhost:8081
```

Then click **Choose folder…** and select your documentation directory. Every
`.md` file beneath it is read in the browser and posted to the parser; nothing
is written back to disk.

No documentation of your own to hand? Seven complete sample sets ship under
[`docs/demo/`](docs/demo/README.md) — a star, a snowflake, a Data Vault, a
hybrid and a third-normal-form schema among them — each with deliberate flaws
so every diagnostic has something to find.

```bash
make demo-docs                    # parse all seven on the command line
make demo-docs SET=eshop-ddd      # or just one
```

Full walkthrough: [getting started](docs/usage/getting-started.md).

---

## Documentation

| | |
|---|---|
| **[Using it](docs/usage/)** | [Getting started](docs/usage/getting-started.md) · [The documentation format](docs/usage/documentation-format.md) · [Exploring the graph](docs/usage/exploring-the-graph.md) · [Diagnostics](docs/usage/diagnostics.md) · [Themes](docs/usage/themes.md) · [The CLI](docs/usage/cli.md) |
| **[Architecture](docs/tech/architecture/)** | [Overview](docs/tech/architecture/overview.md) · [The two stores](docs/tech/architecture/data-model.md) · [HTTP API](docs/tech/architecture/api.md) · [Deployment and configuration](docs/tech/architecture/deployment.md) |
| **[Design notes](docs/tech/design/)** | [Parsing](docs/tech/design/parsing.md) · [Resolution](docs/tech/design/resolution.md) · [Theme pipeline](docs/tech/design/theming.md) · [Frontend](docs/tech/design/frontend.md) · [Testing](docs/tech/design/testing.md) · [CI](docs/tech/design/ci.md) |
| **[Troubleshooting](docs/tech/troubleshooting/)** | [The local stack](docs/tech/troubleshooting/local-stack.md) · [Kubernetes](docs/tech/troubleshooting/kubernetes.md) · [CI and act](docs/tech/troubleshooting/ci-and-act.md) |

All of it is under [`docs/`](docs/README.md). Three READMEs stay next to the
code they describe: [`k8s/`](k8s/README.md) for the manifests,
[`backend/tests/`](backend/tests/README.md) for the suites, and
[`docs/demo/`](docs/demo/README.md) for the sample sets.

---

## What it does

**Parses** a documentation layout it does not own: a domain index per directory
plus one document per table, each with `Overview`, `Columns`,
`Column-Level Lineage`, `Relationships` and `Notes / Caveats` sections. The
role vocabulary is open — a `Type` nobody here anticipated keeps its own name,
shape and colour instead of collapsing into a canvas of identical circles.

**Resolves** relationships into a real graph, which is more work than it
sounds. A join declared from both sides is one edge, not two. Join keys are
matched against the tables' real column lists rather than trusting the order
they were written in. A dimension referenced but not documented in its own
domain is bound to a conformed instance elsewhere, and the alternatives are
kept and shown. → [resolution](docs/tech/design/resolution.md)

**Reports** what is wrong with the documentation — the part that is invisible
when reading one file at a time. A join pointing at a table nobody defined, a
conformed dimension whose two copies have drifted apart, a join key naming
columns neither table has, prose where a model name belongs.
→ [diagnostics](docs/usage/diagnostics.md)

**Renders** the model as a graph you can explore: filter by domain and role,
switch between three layouts, focus on one table's neighbourhood, overlay
upstream source models, search across tables and columns, and click any table
for its description, grain, columns, column-level lineage, joins and caveats.
→ [exploring the graph](docs/usage/exploring-the-graph.md)

---

## How it fits together

```
Browser ──pick folder──▶ Vue 3 app
                          │ reads .md client-side
                          ▼
                    POST /api/v1/ingest        (JSON or multipart)
                          │
                    ┌─────▼──────┐
                    │ Go backend │  parse ─▶ resolve ─▶ project
                    └──┬──────┬──┘
                       │      │
              ┌────────▼─┐  ┌─▼──────────────┐
              │ Postgres │  │ Neo4j          │
              │ system   │  │ graph          │
              │ of record│  │ projection     │
              └──────────┘  └────────────────┘
```

Postgres is the system of record and carries the weighted full-text index;
Neo4j holds the projected graph and answers the traversals that are a single
Cypher clause and an awkward recursive CTE in SQL — neighbourhoods, shortest
join paths, and shared-source siblings. The parse and resolve stages touch
neither, which is why [`relctl`](docs/usage/cli.md) can lint a documentation
tree with no server and no database.

→ [architecture](docs/tech/architecture/overview.md)

---

## Working on it

```bash
make test              # backend unit tests, frontend typecheck and unit tests
make test-integration  # backend against real Postgres and Neo4j
make ci                # what a pull request runs, locally, through act
make k8s-up            # build, deploy to minikube, and tunnel to it
make help              # everything else
```

```
backend/    Go: parser, resolver, both stores, HTTP API, relctl
frontend/   Vue 3 + Pinia + Cytoscape, no Tailwind
k8s/        kustomize base and dev/prod overlays
docs/       usage guides, technical documentation, demo sets
```

Every workflow runs on your machine through [nektos/act], so a broken one costs
no Actions minutes to find. → [CI](docs/tech/design/ci.md) ·
[testing](docs/tech/design/testing.md)

[nektos/act]: https://github.com/nektos/act

---

## Why it is called that

This is a passion project. Nobody asked for it, it is not on a roadmap, and the
parts of it that took the longest — the contrast pipeline behind the themes, the
join-key orientation rules — are the parts nobody would have specified.

It is named after **Haru Urara**, and yes, that is an Uma Musume thing. I am a
fan, and the name is not decoration; it is the design brief.

The real Haru Urara ran over a hundred races at Kochi and never won one. That is
not usually how a horse becomes famous, but in the early 2000s she became
exactly that — a national favourite, precisely *because* she kept turning up and
losing. People who were not winning either went to watch her run. Her losing
tickets sold as charms. In Uma Musume she is drawn as relentlessly bright: pink,
undiscouraged, and genuinely delighted by a track she has no realistic chance
against.

Which is the tone I wanted, because of what this tool actually does. Point it at
a documentation tree and it will tell you that a join key is written as `"N/A"`,
that a conformed dimension has drifted apart, that a column cites prose where an
upstream model belongs. That is a list of everything you got wrong, delivered
all at once, about work you probably did carefully. A tool that delivers it in
the usual severity-red on grey-slate reads like a performance review.

So the default theme is her — sakura pink, spring sky, blossom paper, corners
rounder than any grown-up tool would choose — and the whole app is light-mode
only, on purpose. The idea is that opening it should feel like being looked at
by someone who is pleased you showed up, and who is going to tell you about the
`"N/A"` anyway, cheerfully, because that is what you came for.

You can switch the theme. The [eight paintings](docs/usage/themes.md) are still
there. But this is the one it opens on.
