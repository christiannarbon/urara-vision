# fact_order_items

## Overview

| Property | Value |
|---|---|
| **Table Name** | `fact_order_items` |
| **Type** | Fact |
| **Domain** | Ordering |
| **Bounded Context** | Ordering |
| **Aggregate Root** | Order |
| **Grain** | One row per product on an order. |
| **Update Frequency** | hourly |
| **Layer** | Star Schema (proposed) |
| **Service of Record** | Ordering.API |

The line items of an order. This is an entity inside the Order aggregate, not a fact in its own right: `OrderItem` has no public constructor outside `Order.AddOrderItem()`, and adding the same product twice modifies the existing line rather than adding a second one. That last detail is what makes this grain one row per *product* per order rather than one row per add-to-order.

## Columns

| Column | Type | Description |
|---|---|---|
| `order_item_key` | STRING | Surrogate key over the line (PK) |
| `order_key` | STRING | Order the line belongs to (FK) |
| `catalog_item_key` | STRING | Catalog item sold (FK) |
| `order_date_key` | DATE | Date the order was placed (FK) |
| `product_id` | INT64 | Catalog item id as recorded on the line |
| `product_name` | STRING | Product name captured at the time of sale |
| `unit_price` | NUMERIC | Price per unit at the time of sale |
| `discount` | NUMERIC | Discount applied to the line |
| `units` | INT64 | Units of the product on the order |
| `line_total` | NUMERIC | Units times unit price, less discount |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `order_item_key` | `orderingdb.stg_order_items` | `id` | Primary Key |
| `order_key` | `orderingdb.stg_order_items` | `order_id` | Foreign Key |
| `catalog_item_key` | `stg_catalog_items` | `id` | Foreign Key; matched through `product_id` |
| `order_date_key` | `orderingdb.stg_orders` | `order_date` | Foreign Key; cast to date |
| `product_id` | `orderingdb.stg_order_items` | `product_id` | |
| `product_name` | `orderingdb.stg_order_items` | `product_name` | Captured on the line, not read from the catalog |
| `unit_price` | `orderingdb.stg_order_items` | `unit_price` | |
| `discount` | `orderingdb.stg_order_items` | `discount` | |
| `units` | `orderingdb.stg_order_items` | `units` | |
| `line_total` | `orderingdb.stg_order_items` | `unit_price` | Derived: `units * unit_price - discount` |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `fact_orders` | `order_key = order_key` | Many-to-one |
| `dim_catalog_item` | `catalog_item_key = catalog_item_key` | Many-to-one |
| `dim_date` | `order_date_key = date_key` | Many-to-one |

## Notes / Caveats

- The catalog citation above is a bare `stg_catalog_items` while `dim_catalog_item` cites `catalogdb.stg_catalog_items`. They are the same model written two ways, left inconsistent on purpose: without folding them onto one node, "what else reads the catalog?" quietly returns the wrong answer.
- `product_name` and `unit_price` are snapshots taken when the line was created — eShop copies them onto the `OrderItem` deliberately, so the order does not change when the catalog does. Reporting on them will disagree with `dim_catalog_item`, and the order line is the one that is right.
- `catalog_item_key` resolves against the catalog as it is now. An item deleted from the catalog leaves order lines pointing at nothing, and this model has no unknown-member row for them.
