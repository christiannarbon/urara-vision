# fact_orders

## Overview

| Property | Value |
|---|---|
| **Table Name** | `fact_orders` |
| **Type** | Fact |
| **Domain** | Ordering |
| **Bounded Context** | Ordering |
| **Aggregate Root** | Order |
| **Grain** | One row per order. |
| **Update Frequency** | hourly |
| **Layer** | Star Schema (proposed) |

The Order aggregate root: one row per order placed, with the money and the item counts already rolled up from its line items. This is the fact every other context reads when it wants to know what the shop sold.

## Columns

| Column | Type | Description |
|---|---|---|
| `order_id` | STRING | Order identifier (PK) |
| `customer_id` | STRING | Customer who placed the order (FK) |
| `location_id` | STRING | Store the order was placed at (FK) |
| `ordered_at_date_key` | DATE | Date the order was placed (FK) |
| `delivery_partner_id` | STRING | Delivery partner, null for counter orders (FK) |
| `subtotal` | FLOAT64 | Order value before tax |
| `tax_paid` | FLOAT64 | Tax charged, at the store's rate |
| `order_total` | FLOAT64 | Subtotal plus tax |
| `order_cost` | FLOAT64 | Sum of the supply cost of every item |
| `count_food_items` | INT64 | Food items on the order |
| `count_drink_items` | INT64 | Drink items on the order |
| `count_order_items` | INT64 | Items on the order |
| `is_food_order` | BOOLEAN | Whether the order contains any food |
| `is_drink_order` | BOOLEAN | Whether the order contains any drink |
| `customer_order_number` | INT64 | This customer's nth order |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `order_id` | `jaffle_shop.stg_orders` | `order_id` | Primary Key |
| `customer_id` | `jaffle_shop.stg_orders` | `customer_id` | Foreign Key |
| `location_id` | `jaffle_shop.stg_orders` | `location_id` | Foreign Key |
| `ordered_at_date_key` | `jaffle_shop.stg_orders` | `ordered_at` | Foreign Key; cast to date |
| `delivery_partner_id` | Not in the source yet; the Delivery Logistics context has not shipped | | Placeholder column, always null |
| `subtotal` | `jaffle_shop.stg_orders` | `subtotal` | |
| `tax_paid` | `jaffle_shop.stg_orders` | `tax_paid` | |
| `order_total` | `jaffle_shop.stg_orders` | `order_total` | |
| `order_cost` | `jaffle_shop.stg_order_items` | `supply_cost` | Derived: `SUM(supply_cost)` per order |
| `count_food_items` | `jaffle_shop.stg_order_items` | `product_id` | Derived: count of food items per order |
| `count_drink_items` | `jaffle_shop.stg_order_items` | `product_id` | Derived: count of drink items per order |
| `count_order_items` | `jaffle_shop.stg_order_items` | `order_item_id` | Derived: `COUNT(*)` per order |
| `is_food_order` | Rolled up from the order items in the mart | | Derived: `count_food_items > 0` |
| `is_drink_order` | Rolled up from the order items in the mart | | Derived: `count_drink_items > 0` |
| `customer_order_number` | `jaffle_shop.stg_orders` | `ordered_at` | Derived: `ROW_NUMBER()` per customer |

## Relationships

Four of these five targets are owned by another bounded context. The fifth is the line-item entity inside this aggregate.

| Related Table | Join Key | Relationship |
|---|---|---|
| `fact_order_items` | `order_id = order_id` | One-to-many |
| `dim_customers` | `customer_id = customer_id` | Many-to-one |
| `dim_locations` | `location_id = location_id` | Many-to-one |
| `dim_date` | `date_key = ordered_at_date_key` | Many-to-one |
| `dim_delivery_partner` | `delivery_partner_id = delivery_partner_id` | Many-to-one |

## Notes / Caveats

- The `dim_date` join key above is written dimension-column-first, which is the wrong way round for a Many-to-one row. It is left that way on purpose: the orientation rule should recover it from the column lists rather than trusting the written order.
- `dim_delivery_partner` belongs to the Delivery Logistics context, which is on the context map but has no table documents yet, so this reference cannot resolve.
- `order_cost` is a rollup of supply cost at the time the item was sold, not the current supply cost, so it will not match a recomputation against today's `dim_supplies`.
