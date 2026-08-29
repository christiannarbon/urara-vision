# dim_order_status

## Overview

| Property | Value |
|---|---|
| **Table Name** | `dim_order_status` |
| **Type** | Dimension |
| **Domain** | Ordering |
| **Bounded Context** | Ordering |
| **Aggregate Root** | Order |
| **Grain** | One row per order status. |
| **Update Frequency** | on change |
| **Layer** | Star Schema (proposed) |
| **Service of Record** | Ordering.API |

The order lifecycle, promoted from eShop's `OrderStatus` enum to a dimension. Six rows, and unlikely ever to be seven — but the two attributes beside the name are the reason this is a table: whether a status ends the order's life, and where it sits in the sequence, are both things every Ordering question needs and an enum column cannot hold.

## Columns

| Column | Type | Description |
|---|---|---|
| `order_status_key` | STRING | Surrogate key over `order_status_id` (PK) |
| `order_status_id` | INT64 | Enum value, 1 through 6 |
| `order_status_name` | STRING | `Submitted`, `AwaitingValidation`, `StockConfirmed`, `Paid`, `Shipped` or `Cancelled` |
| `is_terminal` | BOOLEAN | Whether the order can leave this status |
| `status_sequence` | INT64 | Position in the happy-path lifecycle |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `order_status_key` | `orderingdb.stg_order_statuses` | `id` | Primary Key |
| `order_status_id` | `orderingdb.stg_order_statuses` | `id` | |
| `order_status_name` | `orderingdb.stg_order_statuses` | `name` | |
| `is_terminal` | `orderingdb.stg_order_statuses` | `name` | Derived: `Shipped` and `Cancelled` are terminal |
| `status_sequence` | `orderingdb.stg_order_statuses` | `id` | Derived: enum order, with `Cancelled` placed last |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `fact_orders` | `order_status_key = order_status_key` | One-to-many |

## Notes / Caveats

- `status_sequence` describes the happy path only. `Cancelled` is reachable from `Submitted`, `AwaitingValidation` and `StockConfirmed`, so ordering by this column and reading it as elapsed progress is wrong for every cancelled order.
- The enum is a C# type with no table behind it in eShop; `stg_order_statuses` is a seed the warehouse maintains. If a new status is added to the application and not to the seed, orders in it will not join.
