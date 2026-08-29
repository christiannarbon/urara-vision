# hub_order

## Overview

| Property | Value |
|---|---|
| **Table Name** | `hub_order` |
| **Type** | Hub |
| **Domain** | Ordering |
| **Bounded Context** | Ordering |
| **Business Key** | `o_orderkey` |
| **Grain** | One row per order business key. |
| **Update Frequency** | hourly |
| **Layer** | Raw Vault |

The order business key. One row per order ever seen, whatever state it is now in.

## Columns

| Column | Type | Description |
|---|---|---|
| `order_hk` | BINARY | Hash of the order business key (PK) |
| `o_orderkey` | INT64 | Order business key, as it arrives from the source |
| `load_date` | TIMESTAMP | When this key was first seen |
| `record_source` | STRING | The staging model this key arrived from |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `order_hk` | `tpch.v_stg_orders` | `o_orderkey` | Derived: `MD5` of the business key |
| `o_orderkey` | `tpch.v_stg_orders` | `o_orderkey` | Business key |
| `load_date` | `tpch.v_stg_orders` | `load_date` |  |
| `record_source` | `tpch.v_stg_orders` | `record_source` |  |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `sat_order_details` | `order_hk = order_hk` | One-to-many |
| `lnk_customer_order` | `order_hk = order_hk` | One-to-many |
