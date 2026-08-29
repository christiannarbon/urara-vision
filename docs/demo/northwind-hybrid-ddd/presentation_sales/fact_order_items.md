# fact_order_items

## Overview

| Property | Value |
|---|---|
| **Table Name** | `fact_order_items` |
| **Type** | Fact |
| **Domain** | Presentation - Sales |
| **Bounded Context** | Presentation - Sales |
| **Grain** | One row per product on an order. |
| **Update Frequency** | hourly |
| **Layer** | Star Schema |

The line items of an order. This is the one table in the presentation layer built straight from the source rather than from the vault: Northwind's `Order Details` has no key of its own and was never given a hub, so there was nothing in the raw layer to build it from.

## Columns

| Column | Type | Description |
|---|---|---|
| `order_item_key` | STRING | Surrogate key for the line (PK) |
| `order_key` | STRING | Order the line belongs to (FK) |
| `product_key` | STRING | Product ordered (FK) |
| `order_date_key` | DATE | Date the parent order was placed (FK) |
| `unit_price` | FLOAT64 | Price at the time of the order |
| `quantity` | INT64 | Units ordered |
| `discount` | FLOAT64 | Discount applied, 0 to 1 |
| `extended_price` | FLOAT64 | unit_price * quantity * (1 - discount) |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `order_item_key` | `northwind.stg_order_details` | `order_id` | Derived: `MD5` of order and product |
| `order_key` | `northwind.stg_order_details` | `order_id` | Foreign Key |
| `product_key` | `northwind.stg_order_details` | `product_id` | Foreign Key |
| `order_date_key` | `northwind.stg_orders` | `order_date` | Foreign Key; cast to date |
| `unit_price` | `northwind.stg_order_details` | `unit_price` |  |
| `quantity` | `northwind.stg_order_details` | `quantity` |  |
| `discount` | `northwind.stg_order_details` | `discount` |  |
| `extended_price` | `northwind.stg_order_details` | `unit_price` | Derived: `unit_price * quantity * (1 - discount)` |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `fact_orders` | `order_key = order_key` | Many-to-one |
| `dim_product` | `product_id = product_number` | Many-to-one |

## Notes / Caveats

- The join to `dim_product` names `product_id` and `product_number`, and neither column exists on either table. It is what a join key looks like when written from the source schema rather than from the warehouse model, and the check should catch it.
- It has a second consequence worth knowing about. `dim_product` declares the same join from its own side, correctly, as `product_key = product_key`. Because the two spellings disagree they do not merge, so the graph draws two edges between these tables rather than one. The warning is the loud symptom; the duplicated edge is the one that actually misleads a reader.
