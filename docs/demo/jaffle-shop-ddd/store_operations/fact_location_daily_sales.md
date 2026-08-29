# fact_location_daily_sales

## Overview

| Property | Value |
|---|---|
| **Table Name** | `fact_location_daily_sales` |
| **Type** | Fact |
| **Domain** | Store Operations |
| **Bounded Context** | Store Operations |
| **Aggregate Root** | Location |
| **Grain** | One row per store per day. |
| **Update Frequency** | daily |
| **Layer** | Star Schema (proposed) |

Daily performance per store, aggregated from the orders the Ordering context publishes. A store with no orders on a day still gets a row, with zeroes, so the series is continuous.

## Columns

| Column | Type | Description |
|---|---|---|
| `location_id` | STRING | Store (PK/FK) |
| `date_key` | DATE | Sales date (PK/FK) |
| `order_count` | INT64 | Orders taken that day |
| `gross_sales` | FLOAT64 | Sum of order totals |
| `tax_collected` | FLOAT64 | Sum of tax paid |
| `net_sales` | FLOAT64 | Gross sales less tax |
| `count_food_items` | INT64 | Food items sold |
| `count_drink_items` | INT64 | Drink items sold |
| `average_order_value` | FLOAT64 | Gross sales divided by order count |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `location_id` | `jaffle_shop.stg_orders` | `location_id` | Foreign Key |
| `date_key` | `jaffle_shop.stg_orders` | `ordered_at` | Foreign Key; cast to date |
| `order_count` | `jaffle_shop.stg_orders` | `order_id` | Derived: `COUNT(*)` per store per day |
| `gross_sales` | `jaffle_shop.stg_orders` | `order_total` | Derived: `SUM(order_total)` |
| `tax_collected` | `jaffle_shop.stg_orders` | `tax_paid` | Derived: `SUM(tax_paid)` |
| `net_sales` | | | Derived: `gross_sales - tax_collected` |
| `count_food_items` | `jaffle_shop.stg_order_items` | `product_id` | Derived: count of food items |
| `count_drink_items` | `jaffle_shop.stg_order_items` | `product_id` | Derived: count of drink items |
| `average_order_value` | | | Derived: `gross_sales / NULLIF(order_count, 0)` |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `dim_locations` | `location_id = location_id` | Many-to-one |
| `dim_date` | `sales_date = calendar_date` | Many-to-one |

## Notes / Caveats

- The `dim_date` join key names the columns by their business names rather than the physical ones. Neither `sales_date` nor `calendar_date` exists on either table; the real join is `date_key = date_key`. Left as written on purpose, so the mismatch is reported rather than guessed at.
- Days with no orders are filled with zero rows, so `average_order_value` is null rather than zero on those days.
