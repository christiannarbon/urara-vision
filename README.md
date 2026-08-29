# Urara Vision

A visualization tool for Open Knowledge Format documentation of domain-driven
data models. Point it at a directory of markdown model documents and it becomes
an interactive relationship graph: the backend parses every document, resolves
the joins between tables, projects the result into a graph, and reports every
reference it could not satisfy.

It knows nothing about any particular business domain, and nothing about any
particular modelling style. The layout it reads is the convention -- a domain
index per directory, a document per table -- so any model documented that way is
a model it can draw: a Kimball star, a snowflake, a Data Vault, a plain
third-normal-form schema, or a vocabulary of roles it has never met.

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

You can switch the theme. The eight paintings are still there. But this is the
one it opens on.

---

## What it does

**Parses** the documentation layout: a domain index per directory
(`domain_one.md`) plus one document per table (`domain_one/fact_primary.md`),
each with
`Overview`, `Columns`, `Column-Level Lineage`, `Relationships` and
`Notes / Caveats` sections.

**Resolves** relationships into a real graph. This is more work than it sounds:

- A relationship declared from both sides (`fact_primary → dim_alpha` as
  *Many-to-one*, and `dim_alpha → fact_primary` as *One-to-many*) is a single
  edge, not two. Declarations are normalised to point from the many side to the
  one side and deduplicated.
- Join keys are matched against the tables' real column lists rather than
  trusting the order they were written in. Some documents write the *fact's*
  column first even on their own `One-to-many` rows — reversing those silently
  would have pointed `dim_beta.beta_id → fact_primary.beta_id_2` the wrong way.
- A dimension referenced but not documented in its own domain is bound to a
  conformed instance elsewhere, preferring one that declares itself
  `Conformed`, then the richest definition. The alternatives are kept and shown.

**Reports** what is wrong with the documentation — the part that is invisible
when reading one file at a time:

| Check | What it catches |
|---|---|
| `unresolved_reference` | A join points at a table no document defines |
| `conformed_drift` | The same dimension documented differently in different domains |
| `cross_domain_reference` | A domain borrows a dimension it does not document |
| `unmatched_join_key` | A join key naming columns neither table declares |
| `narrative_reference` | Prose (`Various Fact Tables`) where a table name belongs |
| `isolated_fact` | A fact with no resolvable join |
| `isolated_table` | Any other connective table -- a link, a junction -- joined to nothing |
| `undocumented_lineage` | A column whose source is prose rather than a model name |


### Roles

A table's role comes from its `Type` property, or failing that from its name.
The vocabulary is open on purpose. These are the roles that get a name, a
family and a shape of their own:

| Family | Roles |
|---|---|
| Kimball | fact, factless fact, dimension, outrigger, bridge, junk dimension, degenerate dimension |
| Data Vault | hub, link, satellite, point-in-time |
| Relational / 3NF | entity, associative, lookup, reference |

Anything else is *not* discarded. A `Type` of `Anchor` becomes the role
`anchor`, keeps its own name through the parser, the stores and the API, and is
drawn with a shape and a colour of its own. That is what stops a model built on
a vocabulary nobody here anticipated rendering as a canvas of identical circles.
A `Type` holding a whole sentence is prose in the wrong column, and does become
`unknown`.

Roles matter to the drawing and to two diagnostics. They matter to nothing else:
relationships are resolved from cardinality and column names alone, which is why
a snowflake's dimension-to-dimension joins and a Data Vault's link-to-hub joins
need no special case anywhere in the resolver.

### Layouts

No single arrangement suits every way a warehouse gets modelled:

| Layout | Reads well for | Groups by domain |
|---|---|---|
| Force | Stars, and any model with no natural reading direction | Yes |
| Layered | Snowflake normalisation depth, Data Vault tiers | No |
| Radial | A star seen the way it is drawn on a whiteboard | No |

Grouping is a Force-mode feature because neither of the other two layout engines
understands compound nodes; the toggle disables itself rather than silently
dropping the clusters, and remembers your preference for when you switch back.

Source references are also canonicalised before the lineage graph is built.
Documents cite the same model both ways — `warehouse.upstream_model` in one
file, `upstream_model` in another — and left unfolded that becomes two unrelated
nodes, so "what else reads this?" quietly returns the wrong answer.

**Renders** the model as a graph you can explore: filter by domain and role,
switch between three layouts, focus on one table's neighbourhood, overlay
upstream source models, search across tables and columns, and click any table
for its description, grain, columns, column-level lineage, joins and caveats.

---

## Quick start

```bash
docker compose up -d --build
open http://localhost:8081
```

Then click **Choose folder…** and select your documentation directory, e.g.

```
.../docs/data-modelling/
```

Every `.md` file beneath it is read in the browser and posted to the parser.
Nothing is written back to disk.

No documentation of your own to hand? Seven complete sample sets ship under
`docs/demo/`, each a real project's tables arranged as DDD bounded contexts, and
each with deliberate flaws — one per check — so every diagnostic has something
to find:

| Set | Modelled on |
|---|---|
| `jaffle-shop-ddd` | dbt's [Jaffle Shop][jaffle-demo], the canonical DuckDB demo project |
| `fintech-bi-ddd` | a retail bank, in the conventions of [dbt-business-intelligence][flexbi] |
| `eshop-ddd` | Microsoft's [eShop][eshop] reference microservices application |

See [docs/demo/README.md](docs/demo/README.md), or:

```bash
make demo-docs                    # parse all seven
make demo-docs SET=eshop-ddd      # or just one
```

[jaffle-demo]: https://github.com/dbt-labs/jaffle-shop
[flexbi]: https://github.com/flexanalytics/dbt-business-intelligence
[eshop]: https://github.com/dotnet/eShop

| Service | URL |
|---|---|
| Frontend | http://localhost:8081 |
| Backend API | http://localhost:8080 |
| Neo4j browser | http://localhost:7474 (`neo4j` / `relviz-dev-password`) |
| Postgres | `localhost:5433` (`relviz` / `relviz`) |

### Local development

Run the datastores in containers and the two apps natively for hot reload:

```bash
docker compose up -d postgres neo4j

cd backend && POSTGRES_DSN='postgres://relviz:relviz@localhost:5433/relviz?sslmode=disable' \
  NEO4J_URI=bolt://localhost:7687 NEO4J_PASSWORD=relviz-dev-password go run ./cmd/server

cd frontend && npm install && npm run dev   # http://localhost:5173, proxies /api
```

---

## Architecture

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

**Postgres** is the system of record: snapshots, domains, tables, columns,
relationships, column lineage, diagnostics, and a weighted `tsvector` index so
searching a column name finds its table.

**Neo4j** holds the projected graph and answers the traversals:

```cypher
(:Table)-[:JOINS {fromColumn, toColumn, cardinality, crossDomain}]->(:Table)
(:Table)-[:DERIVED_FROM {columns}]->(:Source)
(:Table)-[:IN_DOMAIN]->(:Domain)
(:Table)-[:CONFORMS_TO]->(:Conformed)
```

That is what powers neighbourhood queries, `allShortestPaths` between two
tables ("how do I join these?"), and the shared-source sibling lookup ("what
else breaks if this upstream model changes?").

Ingest writes Postgres first, then projects to Neo4j; a failed projection rolls
the snapshot back so the list never shows a half-working entry.

### Layout

```
backend/
  cmd/server/        HTTP server
  cmd/relctl/        CLI: parse a directory, print stats and diagnostics
  internal/parser/   markdown → structured documents
  internal/graph/    resolution, edge normalisation, drift detection
  internal/store/    postgres (record) + neo4j (graph)
  internal/api/      routes, ingest
frontend/
  src/components/    GraphCanvas, TableDetail, FilterSidebar, Search, Diagnostics
  src/stores/        Pinia workspace state
  src/composables/   directory picker, theme
  src/styles/        design tokens + base CSS  (no Tailwind)
k8s/
  base/  overlays/{dev,prod}
docs/
  demo/jaffle-shop-ddd/   sample OKF documentation sets, used by the demo suite
  demo/fintech-bi-ddd/      star schemas
  demo/eshop-ddd/
  demo/adventureworks-snowflake-ddd/   a snowflake
  demo/tpch-vault-ddd/                 a Data Vault
  demo/northwind-hybrid-ddd/           a vault feeding a star
  demo/sakila-oltp-ddd/                third normal form
```

---

## Theming

Ten themes: two of the app's own, and eight derived from paintings, taken from
the Material 3 token sets in [art_inspired_design_system_for_AI][art]. The app
is **light-mode only** — a theme is a single palette, and there is no dark
variant, no `prefers-color-scheme` block and no mode toggle.

| Theme | Canvas | Display / body face |
|---|---|---|
| **Haru Urara** — sakura pink & spring sky | blossom paper | M PLUS Rounded 1c / Nunito Sans |
| **Studio Paper** — teal & burnt orange | warm paper | Inter |
| Cézanne — *Mont Sainte-Victoire* | blue-grey | Bitter / Nunito Sans |
| Hokusai — *The Great Wave* | sand | Noto Serif JP / Inter |
| Hopper — *Nighthawks* | brass | DM Sans / IBM Plex Sans |
| Matisse — *The Red Studio* | rose | Space Grotesk / DM Sans |
| Monet — *Water Lilies* | misty blue | Lora / Source Serif 4 |
| Van Gogh — *Green Wheat Fields* | pale green | Lora / Source Sans 3 |
| Van Gogh — *Irises* | ochre | Archivo Black / Work Sans |
| Wang Ximeng — *A Thousand Li of Rivers and Mountains* | parchment | Noto Serif Display / Noto Sans |

**Haru Urara** is the default and the one the app is named for. Sakura pink
carries the fact role, a clear spring sky the dimensions, and the sunlit gold of
her accessories the conformed markers, all on blossom-tinted paper — with the
roundest corners and the only rounded display face in the set.

The pink is deepened well past her actual hair colour, which is the one place
the theme argues with its source. `--fact` is a ten-pixel node square and also
the accent behind every button and focus ring, so it owes 4:1 on white; a true
sakura pink manages about 1.6:1 and the contrast pass would have dragged it down
there regardless. Choosing the deeper pink up front keeps the hue relationships
deliberate instead of derived.

**Studio Paper** is the palette the app shipped with: warm paper, a teal fact
colour, a burnt-orange dimension colour. Neither house palette is hand-written
into the stylesheet — both are expressed in the same M3 roles the paintings use
and go through the identical pipeline, so they carry the same guarantees.

The choice persists in `localStorage`. Webfonts load per theme rather than
upfront — fifteen families across ten themes is far too much to ship on first
paint — and every family chains onto a fallback stack of the same species, so a
theme reads correctly before its font lands or if the network never delivers it.
The default theme is the `DEFAULT_ART` constant in `composables/useTheme.ts`.

### How it is wired

Components speak a small semantic vocabulary — `--panel`, `--text`, `--fact`,
`--edge` — and never M3 role names. `scripts/gen-art-themes.mjs` maps each
theme's tokens onto that vocabulary and writes `src/styles/art-themes.css`,
selected by a `[data-art]` attribute on the root element. Adding a theme is a
data change, not a refactor.

```bash
git clone https://github.com/peiqingzhang/art_inspired_design_system_for_AI /tmp/art
cd frontend && node scripts/gen-art-themes.mjs /tmp/art/themes
```

The generated CSS is committed, so the build never depends on the upstream
repository. `src/styles/theme.css` holds what does not vary with the theme: the
spacing scale, motion, the monospace face, and a fallback palette.

### The mapping is not a copy

An upstream palette is designed to look like its painting, not to survive a
dense read-only tool, so every derived colour is checked against the surfaces
it will actually sit on and nudged in lightness — hue and saturation held —
until it clears its target. `src/styles/art-themes.audit.md` is regenerated
alongside the CSS and records the worst-case ratio for every pairing.

Four problems the audit caught, and what the generator now does about them:

- **Matisse's ramp runs from a deep red surface to a pale salmon container.**
  No single text colour spans it: black reaches 3.81:1 on one end, white
  2.15:1 on the other. The ramp is chosen, not assumed — first the full one,
  then containers only, and finally the containers washed toward white until
  they can carry AA text *and* a red and an amber that stay apart. Matisse
  lands on a 40% wash and keeps every hue relationship intact.
- **Several paintings hand M3 two near-identical roles.** Hokusai's primary and
  secondary are both blues; Van Gogh's wheat fields a green and a green-teal.
  Facts keep the primary; the dimension takes whichever remaining role sits
  furthest from it, and is then walked in lightness and chroma until the two
  dots are plainly different. Shape still carries the distinction on the canvas
  — facts square, dimensions round — but colour now reinforces it.

  Every other role derives its colour from one of those two at runtime rather
  than adding a token here: a small hue and lightness shift off `--fact` for
  the roles that carry events and keys, off `--dim` for the roles that carry
  context. Twenty palettes times sixteen roles is not a contrast pipeline worth
  running, and shape is the channel doing the real work anyway — it survives
  greyscale, which colour does not. Fact and dimension sit at zero shift, so a
  star schema looks exactly as it did before any other role existed.
- **Upstream harmonises the M3 `error` role into the artwork**, which for Hopper
  and Wang Ximeng lands on an orange indistinguishable from the amber warning.
  In a tool that prints "1 error, 20 warnings" side by side, danger's hue is
  held inside a red band.
- **A chip fill and a label on that fill need different lightnesses of one
  hue.** An amber light enough to read as amber as a fill is too light to be
  text on a pale amber wash. Each soft wash ships its own ink — `--fact-soft`
  with `--on-fact-soft` — the way M3 pairs `error-container` with
  `on-error-container`.

The pairs the generator guards are the ones that share a surface, and Haru
Urara is where that limit shows. Its conformed gold lands 15 from the amber
warning, closer than any other theme manages and well under the ~45 where two
colours stop reading as two. It is kept anyway, because the two are never drawn
together: conformed is a node border on the canvas, where its neighbours are the
fact (83), the dimension (101) and danger (60), while warning is a chip in the
diagnostics pane. The obvious alternative — a burnt orange — buys separation
from warning only by closing on danger, to 37, which *is* a collision on the
canvas. The audit reports the numbers; which ones matter is still a judgement.

Graph hull and legend colours key off the *canvas colour*, not the app's
nominal mode. Light-mode does not mean a light canvas: Matisse's is a deep
rose, and hulls tuned for white paper vanished into it.

[art]: https://github.com/peiqingzhang/art_inspired_design_system_for_AI

---

## The CLI

`relctl` parses a directory without needing the server or any datastore, which
makes it useful as a documentation linter in CI:

```bash
cd backend
go run ./cmd/relctl -dir /path/to/model-docs
go run ./cmd/relctl -dir /path/to/model-docs -json     # full model as JSON
go run ./cmd/relctl -dir /path/to/model-docs -strict   # exit 1 on any error
```

It prints a summary of what it found, then the diagnostics themselves:

```
files parsed   <n> (skipped <n>)
domains        <n>
tables         <n>  (conformed instances <n>)
columns        <n>
relationships  <n> declared -> <n> normalised edges
lineage edges  <n> across <n> source tables
roles          <role>=<n> <role>=<n> ...
diagnostics    error=<n> warning=<n> info=<n>
```

The same binary ships inside the backend image at `/app/relctl`.

---

## API

Every read route accepts `latest` in place of a snapshot ID.

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/api/v1/ingest` | Parse an uploaded directory into a snapshot |
| `GET` | `/api/v1/snapshots` | List snapshots |
| `GET` | `/api/v1/snapshots/{sid}` | Snapshot metadata and stats |
| `DELETE` | `/api/v1/snapshots/{sid}` | Delete a snapshot from both stores |
| `GET` | `/api/v1/snapshots/{sid}/domains` | Domains, with descriptions and mermaid |
| `GET` | `/api/v1/snapshots/{sid}/tables` | Table summaries (`?domain=`) |
| `GET` | `/api/v1/snapshots/{sid}/table?id=` | One table in full, plus referrers and lineage |
| `GET` | `/api/v1/snapshots/{sid}/graph` | Node-link graph (`?domain=&kind=&sources=&crossDomainOnly=`) |
| `GET` | `/api/v1/snapshots/{sid}/neighborhood?table=` | Subgraph within `?depth=` hops |
| `GET` | `/api/v1/snapshots/{sid}/paths?from=&to=` | Shortest join paths between two tables |
| `GET` | `/api/v1/snapshots/{sid}/lineage?id=` | Upstream or `?direction=downstream` |
| `GET` | `/api/v1/snapshots/{sid}/search?q=` | Full-text over tables and columns |
| `GET` | `/api/v1/snapshots/{sid}/diagnostics` | Documentation problems (`?severity=`) |
| `GET` | `/api/v1/snapshots/{sid}/sources` | Upstream source models by reference count |
| `GET` | `/healthz`, `/readyz` | Liveness; readiness includes both datastores |

Table IDs are `domain/table` and contain a slash, so they travel as a query
parameter rather than a path segment.

```bash
curl 'localhost:8080/api/v1/snapshots/latest/paths?from=domain_one/fact_primary&to=domain_two/dim_beta'
```

---

## Configuration

| Variable | Default | Notes |
|---|---|---|
| `APP_ADDR` | `:8080` | Listen address |
| `POSTGRES_DSN` | `postgres://relviz:relviz@localhost:5432/relviz?sslmode=disable` | |
| `NEO4J_URI` | `bolt://localhost:7687` | |
| `NEO4J_USER` | `neo4j` | |
| `NEO4J_PASSWORD` | — | Required; the server refuses to start without it |
| `CORS_ORIGINS` | `http://localhost:5173,http://localhost:8081` | Unused when served behind the frontend's proxy |
| `API_TOKEN` | _(unset)_ | Shared bearer token for `/api/v1`. Unset disables authentication; if set it must be 24+ characters. |
| `MAX_UPLOAD_BYTES` | `67108864` | 64 MB |
| `MAX_FILES` | `5000` | |
| `LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` |

The frontend needs no runtime configuration: nginx proxies `/api` to the
backend, so the browser only ever talks to one origin. `VITE_API_BASE` (build
arg) overrides the API base if you want to point it elsewhere.

`relviz` survives the rename as the Postgres role and database, the Kubernetes
secret names (`relviz-postgres`, `relviz-neo4j`) and the `localStorage` key.
Renaming those is not free — a new role and database means an existing volume no
longer matches, and a new secret name means recreating it by hand on any prod
cluster — and none of it is user-visible, so it stayed put.

---

## Kubernetes

Two commands, tested on minikube:

```bash
make k8s-up      # build, load into the cluster, deploy, wait, open a tunnel
make k8s-down    # tear it all down again
```

`k8s-up` starts minikube and enables the ingress addon if needed, so it works
from a cold machine. It blocks until every pod is genuinely ready and then
prints the URL — about 35 seconds from nothing, 10 on a warm cluster. Both
commands are idempotent; re-running `k8s-up` on a live stack changes nothing
and does not restart pods.

```bash
make k8s-status  # pods, volumes, ingress, tunnel state
make k8s-logs    # follow backend logs in-cluster
```

`k8s-down` deletes the namespace, which takes its PVCs with it — the ingested
data does not survive. Re-ingesting takes seconds.

Full detail, including the two image-specific gotchas the live deploy exposed,
is in [`k8s/README.md`](k8s/README.md).

---

## Tests

```bash
make test              # backend unit tests, frontend typecheck and unit tests
make test-integration  # backend against real Postgres and Neo4j
make test-all          # everything
make test-cover        # backend coverage summary
```

Tests live in a dedicated tree, split by what they need to run:

```
backend/tests/unit/          no databases; runs anywhere
backend/tests/integration/   real Postgres and Neo4j, behind a build tag
backend/tests/fixtures/      the model documents both suites parse
backend/tests/unit/demo/     the seven shipped sample sets in docs/demo, pinned
frontend/tests/unit/         pure functions and the store, API client mocked
frontend/tests/integration/  the real client over a stubbed network, and
                             mounted components
```

The unit suites cover the cases that are easy to get wrong: two-sided
relationship deduplication, direction and column flipping for `One-to-many`,
join-key orientation against authoring order, conformed-authority selection,
drift detection, and the hull geometry that must actually enclose its nodes.
The integration suites cover what no fake can stand in for — the SQL, the
`ON DELETE CASCADE`s, the full-text index and the Cypher. The demo suite pins
what each set under `docs/demo` resolves to — statistics, diagnostic counts by
code, and the case that set exists to demonstrate — so a sample whose flaws get
tidied up fails rather than quietly leaving a check uncovered.

See `backend/tests/README.md` for how to run the integration suites against
something other than the bundled compose stack.

---

## CI

Six workflows in `.github/workflows`, each with its own path filter so a change
to one part of the tree does not start runners for the rest:

| Workflow | Runs when | Does |
| --- | --- | --- |
| `backend.yml` | `backend/**`, `docs/demo/**` | gofmt, vet, build, unit tests; integration tests against service containers |
| `frontend.yml` | `frontend/**` | typecheck, unit and component tests, production build, CSP assertions |
| `manifests.yml` | `k8s/**` | renders both overlays and schema-checks them with kubeconform |
| `images.yml` | Dockerfiles, nginx config, lockfiles | builds both images; asserts the backend image is non-root |
| `security.yml` | dependency manifests, plus Mondays | govulncheck, `npm audit`, and a check that every dependency still resolves to the public registry |
| `workflows.yml` | `.github/workflows/**` | ghalint and actionlint over the workflows themselves |

Every action is pinned to a full commit SHA, every job declares its own
`permissions` and a `timeout-minutes`, and checkout never persists credentials.
`workflows.yml` is what keeps that true — ghalint fails the build otherwise.

### Running CI locally

All of it runs on your machine through [nektos/act], so a broken workflow costs
no Actions minutes to find:

```bash
make ci-lint       # ghalint + actionlint, no containers, about a second
make ci            # what a pull request runs, minus the service containers
make ci-backend    # one workflow at a time
make ci-frontend
make ci-manifests
make ci-security
make ci-images
make ci-workflows
make ci-backend-integration   # needs the compose stack stopped; see below
make ci-clean      # drop the containers act keeps between runs
```

`make ci-lint` is the one worth running every time you touch a workflow. The
rest matter when you change what a job *does*.

Settings live in `.actrc`, and two of them are load-bearing:

- **`--bind`** runs the job against your real working tree instead of a copy.
  Steps that shell out to `docker run -v "$PWD/...":...` hand a path to the
  *host* daemon, so it has to be a host path. On a GitHub runner the job is not
  containerised and `$PWD` already is one; without `--bind` those mounts come up
  empty locally and the job fails for a reason that has nothing to do with your
  change.
- **`--no-cache-server`** stops the `setup-*` actions spending minutes failing
  to save a toolchain cache act cannot serve. `--reuse` keeps the module and
  npm caches warm inside the job container instead. GitHub's caching is
  configured in the workflows and is unaffected.

Two differences from a real runner are worth knowing:

- **act's shell does not set `pipefail`; GitHub's does.** A step that pipes a
  test run into `tee` will pass locally and fail on GitHub unless it sets
  `pipefail` itself. `backend.yml` does.
- **act runs natively on Apple silicon**, GitHub runners are amd64. Set
  `ACT_ARCH=linux/amd64` to reproduce a runner exactly; it emulates, so it is
  much slower. Native is right for almost everything.

`make ci-backend-integration` publishes Postgres and Neo4j on the same ports the
compose stack uses, so the two cannot both be up. The target checks first and
tells you to `make down`; if a previous run died mid-setup, `make ci-clean`
clears the service container it left behind.

[nektos/act]: https://github.com/nektos/act

---

## Reading the graph

A documentation set where each domain keeps its own copy of the conformed
dimensions will draw as several mostly-separate components rather than one
connected model, and the only joins crossing a domain boundary will be the ones
a domain could not satisfy locally — exactly what the `cross_domain_reference`
warnings list. Merging those dimensions into single shared documents is what
connects the graph up.

Until then, **Cross-domain joins only** in the sidebar is the fastest way to see
where the seams are.
