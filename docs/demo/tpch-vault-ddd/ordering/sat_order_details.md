# sat_order_details

## Overview

| Property | Value |
|---|---|
| **Table Name** | `sat_order_details` |
| **Type** | Satellite |
| **Domain** | Ordering |
| **Bounded Context** | Ordering |
| **Parent Hub** | `hub_order` |
| **Grain** | One row per order per load date. |
| **Update Frequency** | hourly |
| **Layer** | Raw Vault |

The order's own attributes over time. `o_orderstatus` and `o_totalprice` both change after an order is placed, so this table carries several rows per order and the current one is whichever has the latest load date.

## Columns

| Column | Type | Description |
|---|---|---|
| `order_hk` | BINARY | Hash of the order business key (FK) |
| `load_date` | TIMESTAMP | When this version arrived (PK, with order_hk) |
| `effective_from` | TIMESTAMP | When this version became current |
| `hashdiff` | BINARY | Hash of every descriptive column, for change detection |
| `o_orderstatus` | STRING | Order status at the time of load |
| `o_totalprice` | FLOAT64 | Order total at the time of load |
| `o_orderdate` | DATE | Date the order was placed; does not change |
| `o_orderpriority` | STRING | Priority band |
| `o_clerk` | STRING | Clerk who took the order |
| `o_shippriority` | INT64 | Shipping priority |
| `o_comment` | STRING | Free-text comment from the source |
| `record_source` | STRING | The staging model this version arrived from |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `order_hk` | `tpch.v_stg_orders` | `o_orderkey` | Derived: `MD5` of the business key |
| `load_date` | `tpch.v_stg_orders` | `load_date` |  |
| `effective_from` | `tpch.v_stg_orders` | `load_date` | Derived: the load date of the first row in this version |
| `hashdiff` | `tpch.v_stg_orders` | `o_orderstatus` | Derived: `MD5` over every descriptive column |
| `o_orderstatus` | `tpch.v_stg_orders` | `o_orderstatus` |  |
| `o_totalprice` | `tpch.v_stg_orders` | `o_totalprice` |  |
| `o_orderdate` | `tpch.v_stg_orders` | `o_orderdate` |  |
| `o_orderpriority` | `tpch.v_stg_orders` | `o_orderpriority` |  |
| `o_clerk` | `tpch.v_stg_orders` | `o_clerk` |  |
| `o_shippriority` | `tpch.v_stg_orders` | `o_shippriority` |  |
| `o_comment` | `tpch.v_stg_orders` | `o_comment` |  |
| `record_source` | `tpch.v_stg_orders` | `record_source` |  |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `hub_order` | `order_hk = order_hk` | Many-to-one |
