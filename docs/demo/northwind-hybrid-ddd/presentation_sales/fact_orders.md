# fact_orders

## Overview

| Property | Value |
|---|---|
| **Table Name** | `fact_orders` |
| **Type** | Fact |
| **Domain** | Presentation - Sales |
| **Bounded Context** | Presentation - Sales |
| **Grain** | One row per order. |
| **Update Frequency** | hourly |
| **Layer** | Star Schema |

One row per order, built by joining the order-to-customer link to the order satellite and taking the current satellite row. It keeps `order_hk` so that any figure on a dashboard can be traced back to the vault row it came from — the join below to `hub_order` is that path, written down.

## Columns

| Column | Type | Description |
|---|---|---|
| `order_key` | STRING | Surrogate key for the order (PK) |
| `order_hk` | BINARY | The raw vault's hash key for the same order (FK) |
| `customer_key` | STRING | Customer who placed the order (FK) |
| `order_date_key` | DATE | Date the order was placed (FK) |
| `shipper_key` | STRING | Shipper the order went out with (FK) |
| `freight` | FLOAT64 | Freight charged on the order |
| `order_total` | FLOAT64 | Sum of the line extended prices |
| `line_count` | INT64 | Number of line items |
| `days_to_ship` | INT64 | Days between order date and shipped date; null if unshipped |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `order_key` | `northwind.lnk_order_customer` | `order_hk` | Derived: surrogate key from the hash key |
| `order_hk` | `northwind.lnk_order_customer` | `order_hk` | Foreign Key |
| `customer_key` | `northwind.lnk_order_customer` | `customer_hk` | Derived: surrogate key from the hash key |
| `order_date_key` | `northwind.sat_order_details` | `order_date` | Foreign Key; cast to date |
| `shipper_key` | Not in the vault yet; the Shipping context has not shipped |  | Placeholder column, always null |
| `freight` | `northwind.sat_order_details` | `freight` |  |
| `order_total` | `northwind.stg_order_details` | `unit_price` | Derived: `SUM(unit_price * quantity * (1 - discount))` per order |
| `line_count` | `northwind.stg_order_details` | `order_id` | Derived: `COUNT(*)` per order |
| `days_to_ship` | `northwind.sat_order_details` | `shipped_date` | Derived: `shipped_date - order_date` |

## Relationships

The join to `hub_order` is the one that makes this a hybrid warehouse rather than two unrelated schemas: a fact table joined to a Data Vault hub, on the hash key the fact carries for exactly this purpose.

| Related Table | Join Key | Relationship |
|---|---|---|
| `fact_order_items` | `order_key = order_key` | One-to-many |
| `dim_customer` | `customer_key = customer_key` | Many-to-one |
| `dim_date` | `date_key = order_date_key` | Many-to-one |
| `hub_order` | `order_hk = order_hk` | Many-to-one |
| `dim_shipper` | `shipper_key = shipper_key` | Many-to-one |

## Notes / Caveats

- The `dim_date` join key above is written dimension-column-first, which is the wrong way round for a `Many-to-one` row. It is left that way on purpose: the orientation rule should recover it from the column lists rather than trusting the written order.
- `dim_shipper` belongs to the Shipping context, which is on the context map but has no table documents yet, so this reference cannot resolve. It is the only error in the set.
- `shipper_key` records its source as prose for the same reason.
