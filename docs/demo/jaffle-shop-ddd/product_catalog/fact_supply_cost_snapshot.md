# fact_supply_cost_snapshot

## Overview

| Property | Value |
|---|---|
| **Table Name** | `fact_supply_cost_snapshot` |
| **Type** | Fact |
| **Domain** | Product Catalog |
| **Bounded Context** | Product Catalog |
| **Aggregate Root** | Product |
| **Grain** | One row per product per supply per day. |
| **Update Frequency** | daily |
| **Layer** | Star Schema (proposed) |

A daily snapshot of what each product costs to supply and what margin it earns at the current catalog price. Written up ahead of the dimensions it will join, so its relationships are still described in prose rather than declared against table documents.

## Columns

| Column | Type | Description |
|---|---|---|
| `snapshot_date_key` | DATE | Date of the snapshot (PK/FK) |
| `product_id` | STRING | Product (PK/FK) |
| `supply_id` | STRING | Supply (PK/FK) |
| `supply_cost` | FLOAT64 | Cost of the supply on the snapshot date |
| `product_price` | FLOAT64 | Catalog price on the snapshot date |
| `gross_margin` | FLOAT64 | Price less the summed supply cost |
| `is_perishable_supply` | BOOLEAN | Whether the supply is perishable |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `snapshot_date_key` | | | Derived: the run date |
| `product_id` | `jaffle_shop.stg_supplies` | `product_id` | Foreign Key |
| `supply_id` | `jaffle_shop.stg_supplies` | `supply_id` | Foreign Key |
| `supply_cost` | `jaffle_shop.stg_supplies` | `supply_cost` | |
| `product_price` | `stg_products` | `product_price` | |
| `gross_margin` | | | Derived: `product_price - SUM(supply_cost)` per product |
| `is_perishable_supply` | `jaffle_shop.stg_supplies` | `is_perishable_supply` | |

## Relationships

Joins to the catalog dimensions on `product_id` and to the calendar on the snapshot date. Neither is declared as a row yet.

| Related Table | Join Key | Relationship |
|---|---|---|
| `Product Catalog Dimensions` | `product_id` | Many-to-one |

## Notes / Caveats

- Deliberately under-documented. The relationship row names a group of tables in prose rather than a table document, so this fact resolves to nothing and reads as isolated.
- `product_price` is snapshotted from the catalog, so a price change is visible in this fact from the next run onwards but is not backfilled.
