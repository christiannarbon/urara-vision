# Getting started

## Run the stack

```bash
docker compose up -d --build
open http://localhost:8081
```

Sign in as `admin` / `relviz-dev-admin-password`. Compose creates that account
on first start; set `BOOTSTRAP_ADMIN_USERNAME` and `BOOTSTRAP_ADMIN_PASSWORD`
before then to choose your own. Change the password from the name in the top
right.

Then click **Choose folder…** and select your documentation directory, e.g.

```
.../docs/data-modelling/
```

Every `.md` file beneath it is read in the browser and posted to the parser,
along with the `projectmeta.toml` the directory must carry — see [the
documentation format](documentation-format.md#the-manifest). Nothing is written
back to disk.

## Projects and versions

An import belongs to the project its manifest names, and each `version` in the
manifest is a separate version of it. The home screen lists your projects with
how many versions each has; opening one shows its newest, and the URL names the
version:

```
http://localhost:8081/projects/jaffle-shop-ddd/versions/0.1.0
```

That link keeps opening 0.1.0 after newer versions arrive. The bare
`/projects/jaffle-shop-ddd` always opens the newest. Inside a project, the
version menu in the top bar switches between versions.

To add a version, bump `version` in `projectmeta.toml` and use *Import version*
in the top bar, or import from the home screen. *Import version* refuses a
directory whose manifest names a different project. Importing a version that
already exists is refused too, with a link to the one that is there: delete it
first if you mean to replace it.

On the home screen, *Versions* on a project lists them; each can be opened or
deleted from there. Deleting a project's only version deletes the project, and
deleting a project deletes all of its versions, which is why both ask first.

| Service | URL |
|---|---|
| Frontend | http://localhost:8081 |
| Backend API | http://localhost:8080 |
| Neo4j browser | http://localhost:7474 (`neo4j` / `relviz-dev-password`) |
| Postgres | `localhost:5433` (`relviz` / `relviz`) |

Those credentials are development credentials, committed on purpose so the
stack works with no setup. They are safe only because that is all they are.

### Chat

The chat panel needs a Google Cloud project with Vertex AI. Without
`VERTEX_PROJECT` the chat service refuses to start and questions fail;
everything else works. To set it up, run `gcloud auth
application-default login`, set `VERTEX_PROJECT` in `.env` (see
`.env.example`), and mount the credentials as
[deployment](../tech/architecture/deployment.md#configuration) describes. To run
without chat at all, use `make up-without-chat`.

## Try it without your own documentation

Seven complete sample sets ship under [`docs/demo/`](../demo/README.md), each a
real project's tables arranged as DDD bounded contexts, and each with
deliberate flaws — one per check — so every diagnostic has something to find:

| Set | Modelled on |
|---|---|
| `jaffle-shop-ddd` | dbt's [Jaffle Shop][jaffle-demo], the canonical DuckDB demo project |
| `fintech-bi-ddd` | a retail bank, in the conventions of [dbt-business-intelligence][flexbi] |
| `eshop-ddd` | Microsoft's [eShop][eshop] reference microservices application |
| `adventureworks-snowflake-ddd` | a snowflake |
| `tpch-vault-ddd` | a Data Vault |
| `northwind-hybrid-ddd` | a vault feeding a star |
| `sakila-oltp-ddd` | third normal form |

Load one through **Choose folder…**, or parse it without the UI at all:

```bash
make demo-docs                    # parse all seven
make demo-docs SET=eshop-ddd      # or just one
```

[jaffle-demo]: https://github.com/dbt-labs/jaffle-shop
[flexbi]: https://github.com/flexanalytics/dbt-business-intelligence
[eshop]: https://github.com/dotnet/eShop

## Stopping it

```bash
make down     # stop the stack, keep the volumes
make clean    # stop it and delete the volumes with it
```

A snapshot you ingested survives `make down`, so the next `make up` still has
it. `make clean` is the one that starts you over.

## Local development

Run the datastores in containers and the two apps natively for hot reload:

```bash
docker compose up -d postgres neo4j

cd backend && POSTGRES_DSN='postgres://relviz:relviz@localhost:5433/relviz?sslmode=disable' \
  NEO4J_URI=bolt://localhost:7687 NEO4J_PASSWORD=relviz-dev-password COOKIE_SECURE=false \
  BOOTSTRAP_ADMIN_USERNAME=admin BOOTSTRAP_ADMIN_PASSWORD=relviz-dev-admin-password \
  go run ./cmd/server

cd frontend && npm install && npm run dev   # http://localhost:5173, proxies /api
```

## Beyond your laptop

The same stack runs on Kubernetes — `make k8s-up` builds, deploys and tunnels
to it in one command. See [deployment](../tech/architecture/deployment.md).

If something does not come up, the [troubleshooting
guide](../tech/troubleshooting/) is organised by symptom.
