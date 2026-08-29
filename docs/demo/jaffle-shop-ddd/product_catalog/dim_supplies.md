# dim_supplies

## Overview

| Property | Value |
|---|---|
| **Table Name** | `dim_supplies` |
| **Type** | Dimension |
| **Domain** | Product Catalog |
| **Bounded Context** | Product Catalog |
| **Aggregate Root** | Product — a supply is a component, not a root |
| **Grain** | One row per supply per product. |
| **Update Frequency** | daily |
| **Layer** | Star Schema (proposed) |

The components that go into a product. A supply used by three products appears three times, once per product, because its cost is stated per product rather than per supply.

## Columns

| Column | Type | Description |
|---|---|---|
| `supply_uuid` | STRING | Surrogate key for the supply-product pair (PK) |
| `supply_id` | STRING | Supply identifier, not unique on its own |
| `product_id` | STRING | Product the supply goes into (FK) |
| `supply_name` | STRING | Name of the supply |
| `supply_cost` | FLOAT64 | Cost of this supply for this product |
| `is_perishable_supply` | BOOLEAN | Whether the supply is perishable |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `supply_uuid` | `jaffle_shop.stg_supplies` | `supply_uuid` | Primary Key |
| `supply_id` | `jaffle_shop.stg_supplies` | `supply_id` | |
| `product_id` | `jaffle_shop.stg_supplies` | `product_id` | Foreign Key |
| `supply_name` | `jaffle_shop.stg_supplies` | `supply_name` | |
| `supply_cost` | `jaffle_shop.stg_supplies` | `supply_cost` | |
| `is_perishable_supply` | `jaffle_shop.stg_supplies` | `is_perishable_supply` | |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `dim_products` | `product_id = product_id` | Many-to-one |

## Notes / Caveats

- `supply_id` is not unique. Joining on it rather than `supply_uuid` fans out a product's cost across every product that shares the supply.
