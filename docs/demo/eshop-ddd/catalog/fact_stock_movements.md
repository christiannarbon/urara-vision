# fact_stock_movements

## Overview

| Property | Value |
|---|---|
| **Table Name** | `fact_stock_movements` |
| **Type** | Fact |
| **Domain** | Catalog |
| **Bounded Context** | Catalog |
| **Aggregate Root** | Catalog Item |
| **Grain** | One row per item per stock change. |
| **Update Frequency** | hourly |
| **Layer** | Star Schema (proposed) |
| **Service of Record** | Catalog.API |

One row per change to an item's available stock: a removal when an order is validated, an addition when a restock lands. Catalog.API mutates `AvailableStock` in place and publishes no stock event of its own, so this fact is reconstructed from the catalog database's change feed rather than from the bus.

## Columns

| Column | Type | Description |
|---|---|---|
| `stock_movement_key` | STRING | Surrogate key over the change (PK) |
| `catalog_item_key` | STRING | Item whose stock changed (FK) |
| `movement_date_key` | DATE | Date of the change (FK) |
| `movement_type` | STRING | `removal`, `addition` or `correction` |
| `units_delta` | INT64 | Signed change in units |
| `available_stock_after` | INT64 | Stock level after the change |
| `crossed_restock_threshold` | BOOLEAN | Whether this change took the item below its threshold |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `stock_movement_key` | `catalogdb.stg_stock_changes` | `change_id` | Primary Key |
| `catalog_item_key` | `catalogdb.stg_stock_changes` | `catalog_item_id` | Foreign Key; hashed to match `dim_catalog_item` |
| `movement_date_key` | `catalogdb.stg_stock_changes` | `changed_at` | Foreign Key; cast to date |
| `movement_type` | `catalogdb.stg_stock_changes` | `change_reason` | |
| `units_delta` | `catalogdb.stg_stock_changes` | `units_delta` | |
| `available_stock_after` | `catalogdb.stg_stock_changes` | `available_stock_after` | |
| `crossed_restock_threshold` | `catalogdb.stg_stock_changes` | `available_stock_after` | Derived: compared to the item's threshold at the time |

## Relationships

This fact was written before the dimensions around it, and names them as a group rather than pointing at their documents.

| Related Table | Join Key | Relationship |
|---|---|---|
| `Catalog Dimensions` | `catalog_item_key` | Many-to-one |

## Notes / Caveats

- The row above is prose where a table document belongs, and it is the only relationship this fact declares. Both flaws are deliberate and they compound: the reference is unusable, and because it is the only one, this fact resolves to nothing at all. `dim_catalog_item` and `dim_date` are both documented in this model and neither is named here.
- Reconstructing movements from a change feed means a change made and reverted inside one polling interval is invisible. The stock level is right; the movement count is not.
- eShop seeds catalog stock at startup, before any order exists. Those seed rows predate the first order and so predate the conformed calendar, which has nothing for them to join to.
