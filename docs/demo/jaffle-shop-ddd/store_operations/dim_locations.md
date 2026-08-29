# dim_locations

## Overview

| Property | Value |
|---|---|
| **Table Name** | `dim_locations` |
| **Type** | Dimension |
| **Domain** | Store Operations |
| **Bounded Context** | Store Operations |
| **Aggregate Root** | Location |
| **Grain** | One row per store. |
| **Update Frequency** | daily |
| **Layer** | Star Schema (proposed) |

The Location aggregate root: one row per shop, with the tax rate that applies to orders taken there. Ordering borrows this dimension across the context boundary to attribute an order to a store.

## Columns

| Column | Type | Description |
|---|---|---|
| `location_id` | STRING | Store identifier (PK) |
| `location_name` | STRING | Store name |
| `tax_rate` | FLOAT64 | Tax rate applied to orders at this store |
| `opened_at` | TIMESTAMP | When the store opened |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `location_id` | `jaffle_shop.stg_locations` | `location_id` | Primary Key |
| `location_name` | `jaffle_shop.stg_locations` | `location_name` | |
| `tax_rate` | `jaffle_shop.stg_locations` | `tax_rate` | |
| `opened_at` | `jaffle_shop.stg_locations` | `opened_at` | |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `fact_location_daily_sales` | `location_id = location_id` | One-to-many |

## Notes / Caveats

- `tax_rate` has no history. Restating a past order's tax against this rate will disagree with `fact_orders.tax_paid`, which captured the rate in force at the time.
