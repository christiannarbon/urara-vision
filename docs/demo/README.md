# Demo documentation sets

Seven complete, self-contained OKF documentation sets you can point the app at
without having any real model documentation to hand.

They divide into two groups. The first three are star schemas and were the
original samples. The last four exist because the tool is not a star schema
tool: between them they cover snowflake, Data Vault, a hybrid of vault and star,
and plain third normal form, and every one of them resolves through exactly the
same code path.

| Set | Modelled on | Modelling style | Contexts | Tables |
|---|---|---|---|---|
| [`jaffle-shop-ddd/`](#jaffle-shop-ddd) | dbt's [Jaffle Shop][jaffle] | Star | 6 | 10 |
| [`fintech-bi-ddd/`](#fintech-bi-ddd) | a retail bank, in the conventions of [dbt-business-intelligence][flexbi] | Star | 6 | 10 |
| [`eshop-ddd/`](#eshop-ddd) | Microsoft's [eShop][eshop] reference application | Star | 6 | 11 |
| [`adventureworks-snowflake-ddd/`](#adventureworks-snowflake-ddd) | Microsoft's [AdventureWorksDW][aw] | Snowflake | 6 | 12 |
| [`tpch-vault-ddd/`](#tpch-vault-ddd) | [TPC-H][tpch], in [AutomateDV][adv] conventions | Data Vault | 6 | 11 |
| [`northwind-hybrid-ddd/`](#northwind-hybrid-ddd) | Microsoft's [Northwind][nw] | Vault + star | 6 | 13 |
| [`sakila-oltp-ddd/`](#sakila-oltp-ddd) | MySQL's [Sakila][sakila] | 3NF | 6 | 14 |

```bash
# from the repo root
make demo-docs                    # parse all seven
make demo-docs SET=tpch-vault-ddd # or just one

# or directly
cd backend && go run ./cmd/uraractl -dir ../docs/demo/eshop-ddd

# or in the UI: Choose folder… -> docs/demo/<set>
```

[jaffle]: https://github.com/dbt-labs/jaffle-shop
[flexbi]: https://github.com/flexanalytics/dbt-business-intelligence
[eshop]: https://github.com/dotnet/eShop
[aw]: https://learn.microsoft.com/en-us/sql/samples/adventureworks-install-configure
[tpch]: https://www.tpc.org/tpch/
[adv]: https://automate-dv.readthedocs.io/
[nw]: https://learn.microsoft.com/en-us/dotnet/framework/data/adonet/sql/linq/downloading-sample-databases
[sakila]: https://dev.mysql.com/doc/sakila/en/

Every set carries its own `projectmeta.toml`, because an ingest without one is
refused; the sets are English-only, so each declares `EN` alone.

All seven are built the same way and to the same contract. The tables come from
a real project, so the column names, the upstream models and the grains are real
rather than invented; what is added on top is the *organisation*, because
splitting a flat `marts/` folder into bounded contexts is the arrangement this
tool exists to draw. And in each set the documents are deliberately imperfect:
every flaw below is a real documentation mistake, placed on purpose so that one
check has something to find, and noted in the `Notes / Caveats` of the document
it lives in so nobody mistakes it for an accident.

Each set carries exactly one **error** diagnostic, so `uraractl -strict` exits 1
on all seven and the severity filter in the UI has something to separate.

Every set's totals are pinned by a suite in `backend/tests/unit/demo/`. A
well-meant edit that "fixes" one of the flaws fails those tests rather than
quietly removing a check's only coverage.

### What the four non-star sets are for

Roles are a drawing concern. Relationships are resolved from cardinality and
column names alone, and nothing in the resolver ever asks what role a table
plays — which is why a snowflake's dimension-to-dimension joins, a vault's
link-to-hub joins and a junction table's two foreign keys all needed no special
case to work. These four sets are what make that claim checkable rather than
merely stated:

| Set | What it proves |
|---|---|
| `adventureworks-snowflake-ddd` | Normalised dimension chains three levels deep resolve, and roles come from the declared `Type` even when the table names follow no convention this tool knows |
| `tpch-vault-ddd` | A model containing no fact and no dimension at all draws correctly |
| `northwind-hybrid-ddd` | Two vocabularies in one model, with five joins that cross between them |
| `sakila-oltp-ddd` | A schema that is not a warehouse at all |

Two of them also cover `isolated_table`, the generalisation of `isolated_fact`
to connective tables that are not facts: a Data Vault link in one, a junction
table in the other.

---

## jaffle-shop-ddd

The tables are dbt's [Jaffle Shop][jaffle] — the canonical DuckDB demo project —
so the column names, the upstream `stg_*` models and the grains are all real.
What is added is the split of the marts into **DDD bounded contexts** instead of
one flat `marts/` folder.

| Bounded context | Aggregate root | Tables |
|---|---|---|
| `shared_kernel` | — (reference data) | `dim_date` |
| `customer_identity` | Customer | `dim_customers`, `dim_date` (deprecated local copy) |
| `ordering` | Order | `fact_orders`, `fact_order_items` |
| `product_catalog` | Product | `dim_products`, `dim_supplies`, `fact_supply_cost_snapshot` |
| `store_operations` | Location | `dim_locations`, `fact_location_daily_sales` |
| `delivery_logistics` | Delivery Partner | none yet — on the context map, unmodelled |

Splitting the marts this way makes the boundaries load-bearing. `Ordering` owns
no dimensions at all: Customer, Location, Product and the calendar are each
owned by another context and borrowed across the boundary, which is why the
demo produces cross-domain references rather than one tidy local star. The
Shared Kernel is the only context everything is allowed to depend on directly.

| What the set does | What it should produce |
|---|---|
| `fact_orders` writes its `dim_date` join key dimension-column-first (`date_key = ordered_at_date_key`) on a `Many-to-one` row | The orientation rule recovers it as `ordered_at_date_key = date_key` from the column lists, rather than trusting the written order |
| `fact_orders` and `fact_order_items` both declare the join between them, from opposite sides | One edge, not two, with both documents listed in `declaredBy`. Same for `dim_customers`/`fact_orders`, `dim_products`/`dim_supplies` and `dim_locations`/`fact_location_daily_sales` |
| `fact_order_items` cites `stg_products`; `dim_products` cites `jaffle_shop.stg_products` | One source node, not two, so "what else reads this?" is answerable |
| `customer_identity/dim_date` is a stale local copy of the kernel's calendar | `conformed_drift`: missing 6 columns, adds 2 |
| `fact_orders` references `dim_delivery_partner`, owned by a context with no documents | `unresolved_reference` — the only **error** |
| `fact_location_daily_sales` joins `dim_date` on the business names `sales_date = calendar_date` | `unmatched_join_key` — neither column exists on either table |
| `shared_kernel/dim_date` and `fact_supply_cost_snapshot` name groups of tables in prose | `narrative_reference` |
| `fact_supply_cost_snapshot` was written before the dimensions it joins | `isolated_fact` |
| `delivery_logistics.md` exists with no directory | `empty_domain` |
| `fact_orders` and `customer_identity/dim_date` record some column sources as prose | `undocumented_lineage` |

Expected totals, pinned by `demo_test.go`:

```
files parsed   16 (skipped 0)
domains        6
tables         10  (conformed instances 2)
columns        80
relationships  16 declared -> 9 normalised edges
lineage edges  60 across 7 source tables
roles          dimension=6 fact=4
diagnostics    error=1 warning=11 info=4
```

---

## fintech-bi-ddd

A retail bank's star schema, written in the conventions of
[flexanalytics/dbt-business-intelligence][flexbi]: surrogate `*_key` columns
hashed from a natural key with `generate_surrogate_key`, an `stg_*` staging
layer cited as column-level lineage, and dimensions carrying the type 0 and
type 1 activity dates that project uses. The five models in that repo —
`dim_customer`, `dim_date`, `dim_order`, `dim_product`, `fact_sales` — are the
skeleton; the domain is a bank rather than a product catalogue.

| Bounded context | Aggregate root | Tables |
|---|---|---|
| `shared_kernel` | — (reference data) | `dim_date` |
| `customer_identity` | Customer | `dim_customer`, `dim_account` |
| `payments` | Payment Transaction | `fact_payment_transactions`, `dim_card`, `dim_merchant` |
| `lending` | Loan | `fact_loan_repayments`, `dim_loan` |
| `risk_compliance` | Fraud Alert | `fact_fraud_alerts`, `dim_customer` (drifted local copy) |
| `treasury` | Settlement Batch | none yet — on the context map, unmodelled |

The interesting boundary here is Customer Identity's. It owns Customer *and*
Account, and both Payments and Lending reach across for them — a card is issued
against an account, a loan is originated from one — so between them those two
tables are the target of six of the set's ten cross-context references. Risk &
Compliance is the context that gave up and took a copy.

| What the set does | What it should produce |
|---|---|
| `fact_payment_transactions` writes its `dim_date` join key dimension-column-first (`date_key = transaction_date_key`) on a `Many-to-one` row | Recovered as `transaction_date_key = date_key` from the column lists |
| `dim_account` writes its `One-to-many` join to `dim_loan` loan-column-first (`origination_account_key = account_key`), with different names on each side | Recovered as `account_key = origination_account_key` — the written order is the only thing pointing the wrong way |
| Six joins are declared from both sides, two of them across a context boundary | Six merged edges, each listing both documents in `declaredBy` |
| `dim_merchant` cites a bare `stg_transaction`; `fact_payment_transactions` cites `card_network.stg_transaction` | One source node, not two |
| `risk_compliance/dim_customer` is a stale copy carrying two screening attributes the authority has never heard of | `conformed_drift`: missing 9 columns, adds 2 |
| Both facts and `dim_loan` borrow `dim_customer`, which exists in two domains | Each binds to `customer_identity/dim_customer`, the instance whose Type declares it conformed — not the stale copy |
| `fact_payment_transactions` references `dim_settlement_batch`, owned by Treasury | `unresolved_reference` — the only **error** |
| `fact_loan_repayments` joins `dim_date` on the business names `repayment_date = calendar_date` | `unmatched_join_key` |
| `shared_kernel/dim_date` and `fact_fraud_alerts` name groups of tables in prose | `narrative_reference` |
| `fact_fraud_alerts` declares only that prose reference, so nothing resolves | `isolated_fact` |
| `treasury.md` exists with no directory | `empty_domain` |
| `fact_payment_transactions` and `risk_compliance/dim_customer` record some column sources as prose | `undocumented_lineage` |

Expected totals, pinned by `fintech_test.go`:

```
files parsed   16 (skipped 0)
domains        6
tables         10  (conformed instances 3)
columns        108
relationships  21 declared -> 12 normalised edges
lineage edges  104 across 10 source tables
roles          dimension=7 fact=3
diagnostics    error=1 warning=14 info=4
```

---

## eshop-ddd

An analytics warehouse over Microsoft's [eShop][eshop] reference application.
The entities are eShop's own — `Order`, `OrderItem`, `Buyer`, `PaymentMethod`,
`CatalogItem`, `CatalogBrand`, `CatalogType`, `BasketItem`, `ApplicationUser` —
and the bounded contexts are not an arrangement invented for the demo: they are
the microservices, each with its own database and no permission to read anybody
else's.

| Bounded context | Service of record | Tables |
|---|---|---|
| `shared_kernel` | none — warehouse-generated | `dim_date` |
| `identity` | Identity.API + Ordering.API | `dim_buyer` |
| `catalog` | Catalog.API | `dim_catalog_item`, `dim_catalog_brand`, `dim_catalog_type`, `fact_stock_movements` |
| `ordering` | Ordering.API | `fact_orders`, `fact_order_items`, `dim_order_status` |
| `basket` | Basket.API | `fact_basket_events`, `dim_buyer` (drifted local copy) |
| `payment` | PaymentProcessor | none yet — on the context map, unmodelled |

This set exists because service boundaries produce a *different* shape of
problem than mart boundaries do. eShop keeps the shopper in two services on
purpose — the ASP.NET Identity user in one, the Buyer aggregate in the other,
reconciled on `IdentityGuid` — so `identity/dim_buyer` is the only dimension in
any of the three sets assembled from two systems. And `Payment` is unmodelled
not because nobody got to it, but because eShop's `PaymentProcessor` is a stub
with no database, which is a more honest reason for an empty context than the
other two sets have.

| What the set does | What it should produce |
|---|---|
| `fact_orders` writes its `dim_date` join key dimension-column-first (`date_key = order_date_key`) on a `Many-to-one` row | Recovered as `order_date_key = date_key` from the column lists |
| `identity/dim_buyer` declares a join to `fact_basket_events`, which declares the same join to Basket's own local `dim_buyer` | **Two** edges, not one: a table in the declaring context wins, so the basket fact ends up joined to two different buyer dimensions. Two contexts each believing they own the buyer is the disagreement this set exists to show |
| Seven joins are declared from both sides | Seven merged edges, each listing both documents in `declaredBy` |
| `fact_order_items` cites a bare `stg_catalog_items`; `dim_catalog_item` cites `catalogdb.stg_catalog_items` | One source node, not two |
| `basket/dim_buyer` knows only what the Redis key carries | `conformed_drift`: missing 14 columns, adds 1 |
| `fact_orders` references `dim_payment_method`, owned by a context with no database | `unresolved_reference` — the only **error** |
| `fact_basket_events` joins `dim_catalog_item` on the application's own field names, `product_id = id` | `unmatched_join_key` — this is what a join key looks like when written from the service code rather than the warehouse model |
| `shared_kernel/dim_date` and `fact_stock_movements` name groups of tables in prose | `narrative_reference` |
| `fact_stock_movements` declares only that prose reference, so nothing resolves | `isolated_fact` |
| `payment.md` exists with no directory | `empty_domain` |
| `fact_orders` and `basket/dim_buyer` record some column sources as prose — one waiting on Payment, one a field eShop assigns and never persists | `undocumented_lineage` |

Expected totals, pinned by `eshop_test.go`:

```
files parsed   17 (skipped 0)
domains        6
tables         11  (conformed instances 3)
columns        101
relationships  22 declared -> 12 normalised edges
lineage edges  89 across 13 source tables
roles          dimension=7 fact=4
diagnostics    error=1 warning=13 info=4
```

---

## adventureworks-snowflake-ddd

Microsoft's [AdventureWorksDW][aw] — the sample warehouse that ships with SQL
Server — split into bounded contexts. The tables, the columns and the row counts
in the prose are AdventureWorks' own; what is added is the context split and the
documentation.

It is the set to open the **Layered** layout on. Two dimension chains run three
levels deep, and a force-directed view flattens exactly the thing worth seeing.

| Bounded context | Tables |
|---|---|
| `shared_kernel` | `DimDate` |
| `sales` | `FactInternetSales`, `FactResellerSales`, `FactSalesQuota`, `DimSalesTerritory` |
| `product` | `DimProduct`, `DimProductSubcategory`, `DimProductCategory` |
| `customer` | `DimCustomer`, `DimGeography`, `DimDate` (drifted local copy) |
| `reseller` | `DimReseller` |
| `human_resources` | none yet — on the context map, unmodelled |

The two chains are `DimProduct → DimProductSubcategory → DimProductCategory` and
`DimCustomer → DimGeography → DimSalesTerritory`. The second is the more
interesting one: `DimGeography` is shared by `DimCustomer` and `DimReseller`,
which sit in different contexts, and a shared attribute with two copies is a
disagreement waiting to happen. That sharing is the actual argument for
normalising, and it is why this set produces more cross-context references than
any other — normalising a dimension out multiplies boundary crossings.

This set is also the only one whose tables are named in PascalCase. That is
deliberate: `DimProduct` matches none of the naming conventions the parser falls
back on, so every role here has to come from the document's declared `Type`.

| What the set does | What it should produce |
|---|---|
| `FactInternetSales` writes its `DimDate` join key dimension-column-first on a `Many-to-one` row | Recovered as `OrderDateKey = DateKey` from the column lists |
| Seven joins are declared from both sides, four of them links in a chain | Seven merged edges, each listing both documents in `declaredBy` |
| `FactResellerSales` cites a bare `stg_salesorderheader`; `FactInternetSales` qualifies it | One source node, not two |
| `customer/DimDate` is a stale copy carrying a cohort month the authority has never heard of | `conformed_drift`: missing 5 columns, adds 1 |
| `FactResellerSales` references `DimEmployee`, owned by an unmodelled context | `unresolved_reference` — the only **error** |
| `DimReseller` joins `DimGeography` on `GeographyID = GeoKey`, from memory | `unmatched_join_key`, and a *second* consequence: `DimGeography` declares the same join correctly, the two do not merge, and the graph draws two edges where it should draw one |
| `FactSalesQuota` names its dimensions in prose, having been written before either existed | `isolated_fact` and `narrative_reference` |
| `human_resources.md` exists with no directory | `empty_domain` |

Expected totals, pinned by `snowflake_test.go`:

```
files parsed   18 (skipped 0)
domains        6
tables         12  (conformed instances 2)
columns        91
relationships  24 declared -> 14 normalised edges
lineage edges  89 across 12 source tables
roles          dimension=6 fact=3 outrigger=3
diagnostics    error=1 warning=17 info=4
```

---

## tpch-vault-ddd

A raw Data Vault over [TPC-H][tpch], in the conventions [AutomateDV][adv] uses:
a hash key per business key, `load_date` and `record_source` on everything, and
a `hashdiff` on each satellite for change detection. The TPC-H column names are
real — `c_custkey`, `o_orderpriority`, `ps_availqty` — so the documents read the
way a vault built on that schema actually would.

**There is no fact and no dimension anywhere in it.** That is the point: this is
the set that demonstrates the tool is not a star schema tool.

| Bounded context | Tables |
|---|---|
| `shared_kernel` | `hub_nation`, `sat_nation_details` |
| `party` | `hub_customer`, `sat_customer_details` |
| `ordering` | `hub_order`, `lnk_customer_order`, `sat_order_details` |
| `supply` | `hub_supplier`, `lnk_part_supplier`, `hub_nation` (drifted local copy) |
| `business_vault` | `pit_customer` |
| `shipping` | none yet — on the context map, unmodelled |

A vault declares its joins from both sides far more often than a star does: a
hub lists its satellites, and every satellite names its hub. Five of this set's
seven edges are declared twice.

| What the set does | What it should produce |
|---|---|
| `pit_customer` joins `sat_customer_details` on `load_date = sat_customer_details_ldts`, satellite-column-first | Recovered as `sat_customer_details_ldts = load_date`. `load_date` exists on *both* tables, so the rule cannot pick a side by name and has to use the fact that only one table carries the other column |
| Five joins are declared from both sides | Five merged edges, each listing both documents in `declaredBy` |
| `lnk_customer_order` cites a bare `v_stg_orders`; the other two ordering documents qualify it | One source node, not two |
| `supply/hub_nation` was loaded from the supplier feed rather than the reference feed | `conformed_drift`: missing `record_source`, adds `supplier_count` — a measure, on a hub |
| `lnk_customer_order` references `hub_shipmode`, owned by an unmodelled context | `unresolved_reference` — the only **error** |
| `pit_customer` joins `hub_customer` on the business key names rather than the hash key names | `unmatched_join_key` — the mistake a modeller makes moving between a layer that joins on hashes and a source that joins on business keys |
| `lnk_part_supplier` names its parent hub in prose | `isolated_table` and `narrative_reference`. A link joined to nothing is the clearest gap a vault can have |
| `hub_supplier` declares no relationship at all | **Nothing.** A hub with no satellite or link yet is an ordinary hub, not a gap — which is the distinction `isolated_table` exists to draw |
| `shipping.md` exists with no directory | `empty_domain` |

Expected totals, pinned by `vault_test.go`:

```
files parsed   17 (skipped 0)
domains        6
tables         11  (conformed instances 2)
columns        69
relationships  15 declared -> 7 normalised edges
lineage edges  65 across 5 source tables
roles          hub=5 satellite=3 link=2 pit=1
diagnostics    error=1 warning=8 info=6
```

---

## northwind-hybrid-ddd

A warehouse over Microsoft's [Northwind][nw] with **two modelling styles in one
model**: a Data Vault raw layer that records what arrived, a Kimball
presentation layer built on top of it, and one business vault table straddling
the two. This is a common real warehouse and the hardest test of the claim that
roles are a drawing concern only.

| Bounded context | Layer | Tables |
|---|---|---|
| `shared_kernel` | both | `dim_date` |
| `raw_vault` | vault | `hub_customer`, `hub_order`, `lnk_order_customer`, `sat_customer_details`, `sat_order_details` |
| `presentation_sales` | star | `fact_orders`, `fact_order_items`, `dim_customer` |
| `presentation_catalog` | star | `dim_product`, `dim_category` (outrigger), `dim_date` (drifted local copy) |
| `business_vault` | both | `bridge_customer_order` |
| `shipping` | — | none yet — on the context map, unmodelled |

Five edges cross between vocabularies, and none of them needed a special case:

- `fact_orders → hub_order` — a fact joined to a hub. The fact carries the
  vault's hash key so a figure on a dashboard can be traced back to the row that
  produced it, and this join is that drill-back path written down.
- `dim_customer → hub_customer` — the same path in the other direction.
- `bridge_customer_order` → `hub_customer`, `hub_order` and `dim_customer` —
  three joins across three contexts and two styles, from one table.

The lineage matters as much as the joins: `dim_customer` cites
`sat_customer_details` as its source, not the staging model the satellite was
loaded from. The star is genuinely built on the vault rather than beside it.

`dim_product → dim_category` is a second snowflake, here for a reason worth
stating: Northwind's categories carry a long description that would be repeated
on every product row, and the reporting tool browses categories on their own.

| What the set does | What it should produce |
|---|---|
| `fact_orders` writes its `dim_date` join key dimension-column-first | Recovered as `order_date_key = date_key` |
| Six joins are declared from both sides | Six merged edges, each listing both documents |
| `lnk_order_customer` cites a bare `stg_orders`; two other documents qualify it | One source node, not two |
| `presentation_catalog/dim_date` is a stale copy carrying a fiscal period | `conformed_drift`: missing 3 columns, adds 1 |
| `fact_orders` references `dim_shipper`, owned by an unmodelled context | `unresolved_reference` — the only **error** |
| `fact_order_items` joins `dim_product` on `product_id = product_number`, from the source schema | `unmatched_join_key` — **and** a split edge: `dim_product` declares the same join correctly, the two spellings disagree, so they do not merge |
| `shipping.md` exists with no directory | `empty_domain` |

This is the one set with no `isolated_fact` or `isolated_table`. Every
connective table in it is joined to something, and inventing an orphan purely to
even the sets up would have meant adding a table with no reason to exist. The
vault and 3NF sets cover that check.

Expected totals, pinned by `hybrid_test.go`:

```
files parsed   19 (skipped 0)
domains        6
tables         13  (conformed instances 2)
columns        88
relationships  23 declared -> 15 normalised edges
lineage edges  85 across 10 source tables
roles          dimension=4 fact=2 hub=2 satellite=2 bridge=1 link=1 outrigger=1
diagnostics    error=1 warning=11 info=4
```

---

## sakila-oltp-ddd

The analytics replica of MySQL's [Sakila][sakila] sample database — a DVD rental
shop — documented as the third-normal-form schema it is. Not a warehouse:
entities, two junction tables, a lookup, and shared reference data. The tables,
columns and row counts are Sakila's own.

This is the set furthest from what the tool was originally built for, and so the
strongest evidence the role vocabulary is genuinely open.

| Bounded context | Tables |
|---|---|
| `shared_kernel` | `country`, `city` |
| `catalog` | `film`, `actor`, `film_actor`, `film_category`, `language` |
| `inventory` | `inventory`, `store` |
| `rental` | `rental`, `payment` |
| `party` | `customer`, `address`, `country` (drifted local copy) |
| `staffing` | none yet — on the context map, unmodelled |

Three role distinctions this set exists to draw:

- **Associative vs entity.** `film_actor` holds two foreign keys and a
  timestamp, and nothing else — there is no such thing as an attribute of the
  fact that an actor was in a film. A junction table *with* attributes is
  usually an entity nobody has named yet; this one genuinely is not.
- **Lookup vs reference.** `language` and `country` are the same shape. The
  difference is ownership: a country is reference data the whole schema shares
  and the kernel owns, a language is a lookup the Catalog owns and nothing else
  reads.
- **Connective vs not.** `film_category` joined to nothing is a bug and is
  reported; `language` with nothing pointing at it would merely be dead weight,
  and is not.

It also shows something the warehouse sets cannot. A well-normalised schema
names each foreign key after the primary key it points at — `rental.inventory_id`
at `inventory.inventory_id` — so the order a join key is written in carries no
information and none is needed. Exactly one join in the set is written with
different names on each side, and it is the deliberate mistake.

| What the set does | What it should produce |
|---|---|
| Eleven joins are declared from both sides | Eleven merged edges. An operational schema declares nearly everything twice, because a foreign key is a fact about both tables |
| `actor` cites the same replicated table both ways, in one document | One source node. In a one-to-one replica each table has exactly one source, so a model cited twice is usually cited twice in the same file |
| `party/country` is a stale copy carrying an ISO code matched on country *name* | `conformed_drift`: missing `last_update`, adds `iso_code` |
| `rental` references `staff`, owned by an unmodelled context | `unresolved_reference` — the only **error** |
| `address` joins `city` on `city = city_name` — on the name, not the key | `unmatched_join_key`, and a split edge, since `city` declares the same join correctly. Joining geography on names is how duplicate cities get created in the first place |
| `film_category` names its parents in prose, having been replicated before the category table was documented | `isolated_table` and `narrative_reference` — the same gap as the vault set's orphan link, on a different vocabulary's connective table |
| `staffing.md` exists with no directory | `empty_domain` |

Expected totals, pinned by `oltp_test.go`:

```
files parsed   20 (skipped 0)
domains        6
tables         14  (conformed instances 2)
columns        72
relationships  29 declared -> 15 normalised edges
lineage edges  70 across 13 source tables
roles          entity=8 reference=3 associative=2 lookup=1
diagnostics    error=1 warning=16 info=4
```

---

## Format

Each context is a root-level index document (`ordering.md`) beside a directory
of per-table documents (`ordering/fact_orders.md`). The index carries
`Description`, a proposal section, a diagram section and `Lineage`; each table
document carries `Overview`, `Columns`, `Column-Level Lineage`, `Relationships`
and `Notes / Caveats`.

The proposal and diagram headings vary by modelling style, and are recognised by
alias rather than by one literal spelling: the star sets write `Proposed Star
Schema` and `Star Schema Diagram`, the vault sets write `Proposed Raw Vault` and
`Data Model Diagram`, and the 3NF set writes `Entity Relationship Diagram`. Any
of the aliases in `backend/internal/parser/parser.go` works.

The `Overview` blocks also carry properties the parser does not model —
`Bounded Context` and `Aggregate Root` in the star sets, `Semantic Entity` in
the Fintech set, `Service of Record` in the eShop one, and `Business Key` and
`Parent Hub` in the vault sets. They are there because real documents carry
properties the tool has never heard of, and reading one must not break on them.

The `Type` property is what decides a table's role. It is matched against the
alias table in `backend/internal/parser/normalise.go`, which covers the Kimball,
Data Vault and relational vocabularies; a `Type` matching none of them is
slugified and becomes a role in its own right rather than collapsing to
`unknown`. None of these seven sets exercises that last case — they are all
written in vocabularies the tool knows — but `backend/tests/unit/parser/roles_test.go`
does.
