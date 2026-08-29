# dim_customers

## Overview

| Property | Value |
|---|---|
| **Table Name** | `dim_customers` |
| **Type** | Dimension |
| **Domain** | Customer Identity |
| **Bounded Context** | Customer Identity |
| **Aggregate Root** | Customer |
| **Grain** | One row per customer. |
| **Update Frequency** | daily |
| **Layer** | Star Schema (proposed) |

The Customer aggregate root. One row per person the shop knows about, carrying the lifetime totals that summarise their order history. Ordering borrows this dimension across the context boundary rather than documenting its own copy, which is the dependency the context map records.

## Columns

| Column | Type | Description |
|---|---|---|
| `customer_id` | STRING | Customer identifier (PK) |
| `customer_name` | STRING | Customer's name as given at first order |
| `count_lifetime_orders` | INT64 | Orders placed to date |
| `first_ordered_at` | TIMESTAMP | Timestamp of the first order |
| `last_ordered_at` | TIMESTAMP | Timestamp of the most recent order |
| `lifetime_spend_pretax` | FLOAT64 | Sum of order subtotals |
| `lifetime_spend` | FLOAT64 | Sum of order totals, tax included |
| `customer_type` | STRING | 'new' on the first order, 'returning' thereafter |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `customer_id` | `jaffle_shop.stg_customers` | `customer_id` | Primary Key |
| `customer_name` | `jaffle_shop.stg_customers` | `customer_name` | |
| `count_lifetime_orders` | `jaffle_shop.stg_orders` | `order_id` | Derived: `COUNT(*)` per customer |
| `first_ordered_at` | `jaffle_shop.stg_orders` | `ordered_at` | Derived: `MIN(ordered_at)` |
| `last_ordered_at` | `jaffle_shop.stg_orders` | `ordered_at` | Derived: `MAX(ordered_at)` |
| `lifetime_spend_pretax` | `jaffle_shop.stg_orders` | `subtotal` | Derived: `SUM(subtotal)` |
| `lifetime_spend` | `jaffle_shop.stg_orders` | `order_total` | Derived: `SUM(order_total)` |
| `customer_type` | | | Derived: `count_lifetime_orders = 1` |

## Relationships

Declared from this side as well as from the fact, so the context map shows the dependency in both directions.

| Related Table | Join Key | Relationship |
|---|---|---|
| `fact_orders` | `customer_id = customer_id` | One-to-many |

## Notes / Caveats

- `customer_name` is whatever was given at the first order and is not reconciled afterwards, so two rows can legitimately share a name.
- The lifetime totals are recomputed in full on every run rather than incremented, so a restated order changes history silently.
