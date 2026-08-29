# dim_products

## Overview

| Property | Value |
|---|---|
| **Table Name** | `dim_products` |
| **Type** | Dimension |
| **Domain** | Product Catalog |
| **Bounded Context** | Product Catalog |
| **Aggregate Root** | Product |
| **Grain** | One row per product in the catalog. |
| **Update Frequency** | daily |
| **Layer** | Star Schema (proposed) |

The Product aggregate root: one row per item on the menu, carrying the current catalog price and whether it is food or drink. Ordering borrows this dimension across the context boundary; it does not document its own copy.

## Columns

| Column | Type | Description |
|---|---|---|
| `product_id` | STRING | Product identifier (PK) |
| `product_name` | STRING | Name as it appears on the menu |
| `product_type` | STRING | 'jaffle' or 'beverage' |
| `product_description` | STRING | Menu description |
| `product_price` | FLOAT64 | Current catalog price |
| `is_food_item` | BOOLEAN | Derived from `product_type` |
| `is_drink_item` | BOOLEAN | Derived from `product_type` |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `product_id` | `jaffle_shop.stg_products` | `product_id` | Primary Key |
| `product_name` | `jaffle_shop.stg_products` | `product_name` | |
| `product_type` | `jaffle_shop.stg_products` | `product_type` | |
| `product_description` | `jaffle_shop.stg_products` | `product_description` | |
| `product_price` | `jaffle_shop.stg_products` | `product_price` | |
| `is_food_item` | `jaffle_shop.stg_products` | `product_type` | Derived: `product_type = 'jaffle'` |
| `is_drink_item` | `jaffle_shop.stg_products` | `product_type` | Derived: `product_type = 'beverage'` |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `dim_supplies` | `product_id = product_id` | One-to-many |

## Notes / Caveats

- `product_price` is the current price, with no history. Anything asking what a product cost on a past date must read `fact_order_items.product_price` instead.
- The join to `fact_order_items` is documented only from the Ordering side, because that reference crosses out of this context rather than into it.
