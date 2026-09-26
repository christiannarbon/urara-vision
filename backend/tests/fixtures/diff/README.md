# Diff fixtures

`v1` and `v2` are two versions of project `diff-fixture`. `v2` differs from
`v1` in exactly these seven ways, and the diff API integration test expects
each one exactly once:

1. Table removed: `sales/dim_promotions`.
2. Table added: `catalog/fact_restocks`, joined to `catalog/dim_products`.
3. Column type changed: `sales/fact_orders.amount`, `INT64` → `NUMERIC`.
4. Column added in the middle: `sales/dim_customers.phone`, after `customer_name`.
5. Grain changed: `catalog/dim_products`, "One row per product." → "One row per product variant.".
6. Join cardinality changed: `catalog/fact_inventory` → `dim_products`, `Many-to-one` → `One-to-one`.
7. Lineage source removed: `sales/fact_orders.amount` from `shop.refunds.amount`.

Change 2 also adds one relationship, the new table's join.
