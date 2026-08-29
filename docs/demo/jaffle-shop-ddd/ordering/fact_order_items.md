# fact_order_items

## Overview

| Property | Value |
|---|---|
| **Table Name** | `fact_order_items` |
| **Type** | Fact |
| **Domain** | Ordering |
| **Bounded Context** | Ordering |
| **Aggregate Root** | Order — this is the line-item entity, not a root of its own |
| **Grain** | One row per product on an order. |
| **Update Frequency** | hourly |
| **Layer** | Star Schema (proposed) |

The line items of an order. It sits inside the Order aggregate, so it is reached through `fact_orders` rather than queried on its own; the price and supply cost are captured as they were when the item was sold.

## Columns

| Column | Type | Description |
|---|---|---|
| `order_item_id` | STRING | Line item identifier (PK) |
| `order_id` | STRING | Order the item belongs to (FK) |
| `product_id` | STRING | Product ordered (FK) |
| `ordered_at_date_key` | DATE | Date the order was placed (FK) |
| `product_price` | FLOAT64 | Price charged for the item |
| `supply_cost` | FLOAT64 | Supply cost of the item when sold |
| `is_food_item` | BOOLEAN | Whether the product is food |
| `is_drink_item` | BOOLEAN | Whether the product is a drink |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `order_item_id` | `jaffle_shop.stg_order_items` | `order_item_id` | Primary Key |
| `order_id` | `jaffle_shop.stg_order_items` | `order_id` | Foreign Key |
| `product_id` | `jaffle_shop.stg_order_items` | `product_id` | Foreign Key |
| `ordered_at_date_key` | `jaffle_shop.stg_orders` | `ordered_at` | Foreign Key; cast to date |
| `product_price` | `stg_products` | `product_price` | Cited without its dataset, as the catalog documents write it |
| `supply_cost` | `jaffle_shop.stg_supplies` | `supply_cost` | Derived: `SUM(supply_cost)` per product |
| `is_food_item` | `stg_products` | `is_food_item` | |
| `is_drink_item` | `stg_products` | `is_drink_item` | |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `fact_orders` | `order_id = order_id` | Many-to-one |
| `dim_products` | `product_id = product_id` | Many-to-one |
| `dim_date` | `ordered_at_date_key = date_key` | Many-to-one |

## Notes / Caveats

- The join back to `fact_orders` is declared from both documents. It is one edge, not two.
- `product_price` is the price at the time of sale. A later catalog price change does not restate it, so this column will disagree with `dim_products.product_price` for historical rows.
- The three source citations above deliberately mix `jaffle_shop.stg_products` and a bare `stg_products`, which are the same dbt model written two ways.
