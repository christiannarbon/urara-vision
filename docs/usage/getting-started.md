# Getting started

## Run the stack

```bash
docker compose up -d --build
open http://localhost:8081
```

Then click **Choose folder…** and select your documentation directory, e.g.

```
.../docs/data-modelling/
```

Every `.md` file beneath it is read in the browser and posted to the parser,
along with the `projectmeta.toml` the directory must carry — see [the
documentation format](documentation-format.md#the-manifest). Nothing is written
back to disk.

| Service | URL |
|---|---|
| Frontend | http://localhost:8081 |
| Backend API | http://localhost:8080 |
| Neo4j browser | http://localhost:7474 (`neo4j` / `relviz-dev-password`) |
| Postgres | `localhost:5433` (`relviz` / `relviz`) |

Those credentials are development credentials, committed on purpose so the
stack works with no setup. They are safe only because that is all they are.

## Try it without your own documentation

Nine complete sample sets ship under [`docs/demo/`](../demo/README.md), each a
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
| `chinook-bilingual-ddd` | a star documented in English, translated into Japanese inline |
| `superstore-jp-ddd` | a star documented in Japanese, translated into English inline |

Load one through **Choose folder…**, or parse it without the UI at all:

```bash
make demo-docs                    # parse all nine
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
  NEO4J_URI=bolt://localhost:7687 NEO4J_PASSWORD=relviz-dev-password go run ./cmd/server

cd frontend && npm install && npm run dev   # http://localhost:5173, proxies /api
```

## Beyond your laptop

The same stack runs on Kubernetes — `make k8s-up` builds, deploys and tunnels
to it in one command. See [deployment](../tech/architecture/deployment.md).

If something does not come up, the [troubleshooting
guide](../tech/troubleshooting/) is organised by symptom.
