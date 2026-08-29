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
| **Service of Record** | Ordering.API |

The Order aggregate root: one row per order placed, with the money and the item counts rolled up from its line items. The shipping address is flattened out of eShop's `Address` value object — it is an EF Core owned entity with no identity of its own, so it has no dimension to become and its five fields ride on the fact.

## Columns

| Column | Type | Description |
|---|---|---|
| `order_key` | STRING | Surrogate key over `order_id` (PK) |
| `order_id` | INT64 | Order id from the Ordering database |
| `buyer_key` | STRING | Buyer who placed the order (FK) |
| `order_status_key` | STRING | Current lifecycle status (FK) |
| `order_date_key` | DATE | Date the order was placed (FK) |
| `payment_method_key` | STRING | Payment method used (FK) |
| `payment_id` | INT64 | Payment method id held on the order |
| `ship_street` | STRING | Shipping address, street |
| `ship_city` | STRING | Shipping address, city |
| `ship_state` | STRING | Shipping address, state |
| `ship_country` | STRING | Shipping address, country |
| `ship_zip_code` | STRING | Shipping address, postal code |
| `order_total` | NUMERIC | Sum of units times unit price across the lines |
| `order_item_count` | INT64 | Units across the order |
| `distinct_product_count` | INT64 | Distinct products on the order |
| `is_draft` | BOOLEAN | Whether the order is a draft |
| `description` | STRING | Status narration written by the aggregate |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `order_key` | `orderingdb.stg_orders` | `id` | Primary Key |
| `order_id` | `orderingdb.stg_orders` | `id` | |
| `buyer_key` | `orderingdb.stg_orders` | `buyer_id` | Foreign Key; resolved through `stg_buyers` to the identity GUID |
| `order_status_key` | `orderingdb.stg_orders` | `order_status` | Foreign Key |
| `order_date_key` | `orderingdb.stg_orders` | `order_date` | Foreign Key; cast to date |
| `payment_method_key` | The Payment context has not published a payment method model | | Placeholder column, always null |
| `payment_id` | `orderingdb.stg_orders` | `payment_id` | Nullable until the payment method is verified |
| `ship_street` | `orderingdb.stg_orders` | `address_street` | Flattened from the `Address` owned entity |
| `ship_city` | `orderingdb.stg_orders` | `address_city` | Flattened from the `Address` owned entity |
| `ship_state` | `orderingdb.stg_orders` | `address_state` | Flattened from the `Address` owned entity |
| `ship_country` | `orderingdb.stg_orders` | `address_country` | Flattened from the `Address` owned entity |
| `ship_zip_code` | `orderingdb.stg_orders` | `address_zip_code` | Flattened from the `Address` owned entity |
| `order_total` | `orderingdb.stg_order_items` | `unit_price` | Derived: `SUM(units * unit_price)` per order |
| `order_item_count` | `orderingdb.stg_order_items` | `units` | Derived: `SUM(units)` per order |
| `distinct_product_count` | `orderingdb.stg_order_items` | `product_id` | Derived: `COUNT(DISTINCT product_id)` per order |
| `is_draft` | Never persisted; the aggregate assigns `_isDraft` but EF Core does not map it | | Placeholder column, always false |
| `description` | `orderingdb.stg_orders` | `description` | Set by the aggregate on each status change |

## Relationships

Two of the five targets below are owned by another bounded context, and the fifth belongs to a context that has not shipped.

| Related Table | Join Key | Relationship |
|---|---|---|
| `fact_order_items` | `order_key = order_key` | One-to-many |
| `dim_order_status` | `order_status_key = order_status_key` | Many-to-one |
| `dim_buyer` | `buyer_key = buyer_key` | Many-to-one |
| `dim_date` | `date_key = order_date_key` | Many-to-one |
| `dim_payment_method` | `payment_method_key = payment_method_key` | Many-to-one |

## Notes / Caveats

- The `dim_date` join key above is written dimension-column-first, which is the wrong way round for a `Many-to-one` row. It is left that way on purpose: the orientation rule should recover `order_date_key = date_key` from the column lists rather than trusting the written order.
- `dim_payment_method` belongs to the Payment context, which is on the context map but has no table documents yet, so this reference cannot resolve.
- Two columns record their source as prose rather than a model name, which keeps them out of the lineage graph. Both are honest: one waits on Payment, and the other describes a field eShop's own `Order` assigns and never stores.
- `order_total` is recomputed from the line items rather than read from `Order.GetTotal()`, which is a method rather than a column and so is not in the database at all. The two agree only while no line carries a discount, because `GetTotal()` ignores `Discount`.
- `order_status_key` is the *current* status. An order that has been paid and shipped shows only as shipped; this fact carries no status history, and eShop publishes no status-change event the warehouse subscribes to.
