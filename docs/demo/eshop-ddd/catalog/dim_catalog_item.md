# dim_catalog_item

## Overview

| Property | Value |
|---|---|
| **Table Name** | `dim_catalog_item` |
| **Type** | Dimension |
| **Domain** | Catalog |
| **Bounded Context** | Catalog |
| **Aggregate Root** | Catalog Item |
| **Grain** | One row per catalog item. |
| **Update Frequency** | hourly |
| **Layer** | Star Schema (proposed) |
| **Service of Record** | Catalog.API |

The Catalog Item aggregate root: one row per thing eShop sells, with its brand, its type and its current stock position. The stock columns are current state, not history — `AvailableStock` is mutated in place by the aggregate — which is why a separate fact exists to record the changes.

## Columns

| Column | Type | Description |
|---|---|---|
| `catalog_item_key` | STRING | Surrogate key over `catalog_item_id` (PK) |
| `catalog_item_id` | INT64 | Item id from the Catalog database |
| `name` | STRING | Display name |
| `description` | STRING | Long description shown on the item page |
| `price` | NUMERIC | Current list price |
| `catalog_brand_key` | STRING | Brand the item belongs to (FK) |
| `catalog_type_key` | STRING | Type the item belongs to (FK) |
| `available_stock` | INT64 | Units currently in stock |
| `restock_threshold` | INT64 | Stock level at which reordering is triggered |
| `max_stock_threshold` | INT64 | Maximum units the warehouse can hold |
| `on_reorder` | BOOLEAN | Whether the item is currently on reorder |
| `picture_file_name` | STRING | Image file name served by the catalog API |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `catalog_item_key` | `catalogdb.stg_catalog_items` | `id` | Primary Key |
| `catalog_item_id` | `catalogdb.stg_catalog_items` | `id` | |
| `name` | `catalogdb.stg_catalog_items` | `name` | |
| `description` | `catalogdb.stg_catalog_items` | `description` | |
| `price` | `catalogdb.stg_catalog_items` | `price` | |
| `catalog_brand_key` | `catalogdb.stg_catalog_items` | `catalog_brand_id` | Foreign Key; hashed to match `dim_catalog_brand` |
| `catalog_type_key` | `catalogdb.stg_catalog_items` | `catalog_type_id` | Foreign Key; hashed to match `dim_catalog_type` |
| `available_stock` | `catalogdb.stg_catalog_items` | `available_stock` | Slowly changing dimension type 1 |
| `restock_threshold` | `catalogdb.stg_catalog_items` | `restock_threshold` | |
| `max_stock_threshold` | `catalogdb.stg_catalog_items` | `max_stock_threshold` | |
| `on_reorder` | `catalogdb.stg_catalog_items` | `on_reorder` | Slowly changing dimension type 1 |
| `picture_file_name` | `catalogdb.stg_catalog_items` | `picture_file_name` | |

## Relationships

Brand and type are local to this context. The order line is not: Ordering owns it, and this join is declared from both sides.

| Related Table | Join Key | Relationship |
|---|---|---|
| `dim_catalog_brand` | `catalog_brand_key = catalog_brand_key` | Many-to-one |
| `dim_catalog_type` | `catalog_type_key = catalog_type_key` | Many-to-one |
| `fact_order_items` | `catalog_item_key = catalog_item_key` | One-to-many |

## Notes / Caveats

- `price` is the current list price. An order line records the price at the time it was placed, so joining this dimension to reprice historical orders gives the wrong answer — `fact_order_items.unit_price` is the one to use.
- The `Embedding` vector column on `CatalogItem` is not modelled. It exists for the semantic search feature and has no analytical meaning.
- `available_stock` is type 1 and overwritten on every order, so this dimension cannot answer "what was in stock last Tuesday". That is what `fact_stock_movements` is for, if it ever gets its relationships documented.
