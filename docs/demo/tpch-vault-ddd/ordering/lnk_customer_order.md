# lnk_customer_order

## Overview

| Property | Value |
|---|---|
| **Table Name** | `lnk_customer_order` |
| **Type** | Link |
| **Domain** | Ordering |
| **Bounded Context** | Ordering |
| **Grain** | One row per unique customer-and-order pair. |
| **Update Frequency** | hourly |
| **Layer** | Raw Vault |

The relationship between a customer and an order, as it was loaded. A link is insert-only: if an order were ever reassigned to another customer, a second row would appear rather than the first one changing, and both would be true of the times they describe.

## Columns

| Column | Type | Description |
|---|---|---|
| `customer_order_hk` | BINARY | Hash of both parent hash keys together (PK) |
| `customer_hk` | BINARY | The customer side of the relationship (FK) |
| `order_hk` | BINARY | The order side of the relationship (FK) |
| `shipmode_hk` | BINARY | The ship mode the order was placed under (FK) |
| `load_date` | TIMESTAMP | When this pairing was first seen |
| `record_source` | STRING | The staging model this pairing arrived from |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `customer_order_hk` | `v_stg_orders` | `o_orderkey` | Derived: `MD5` of both business keys |
| `customer_hk` | `v_stg_orders` | `o_custkey` | Derived: `MD5` of the customer business key |
| `order_hk` | `v_stg_orders` | `o_orderkey` | Derived: `MD5` of the order business key |
| `shipmode_hk` | Not in the source yet; the Shipping context has not shipped |  | Placeholder column, always null |
| `load_date` | `tpch.v_stg_orders` | `load_date` |  |
| `record_source` | `tpch.v_stg_orders` | `record_source` |  |

## Relationships

Two of these three hubs are owned by another context. That is normal for a link and is the reason a link belongs to the context that cares about the relationship rather than to either context that owns a key.

| Related Table | Join Key | Relationship |
|---|---|---|
| `hub_customer` | `customer_hk = customer_hk` | Many-to-one |
| `hub_order` | `order_hk = order_hk` | Many-to-one |
| `hub_shipmode` | `shipmode_hk = shipmode_hk` | Many-to-one |

## Notes / Caveats

- `hub_shipmode` belongs to the Shipping context, which is on the context map but has no table documents yet, so this reference cannot resolve. It is the only error in the set.
- This document cites `v_stg_orders` unqualified where `hub_order` and `sat_order_details` both write `tpch.v_stg_orders`. Both spellings mean the same dbt model, and they have to fold onto one lineage node or "what else reads this?" quietly returns the wrong answer.
