# lnk_order_customer

## Overview

| Property | Value |
|---|---|
| **Table Name** | `lnk_order_customer` |
| **Type** | Link |
| **Domain** | Raw Vault |
| **Bounded Context** | Raw Vault |
| **Layer** | Raw Vault |
| **Update Frequency** | hourly |
| **Grain** | One row per unique order-and-customer pair. |

Which customer placed which order, recorded once and never updated. This link is what `fact_orders` is built from: the presentation layer reads the relationship from here rather than from the source.

## Columns

| Column | Type | Description |
|---|---|---|
| `order_customer_hk` | BINARY | Hash of both parent hash keys together (PK) |
| `order_hk` | BINARY | The order side of the relationship (FK) |
| `customer_hk` | BINARY | The customer side of the relationship (FK) |
| `load_date` | TIMESTAMP | When this pairing was first seen |
| `record_source` | STRING | The staging model this pairing arrived from |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `order_customer_hk` | `stg_orders` | `order_id` | Derived: `MD5` of both business keys |
| `order_hk` | `stg_orders` | `order_id` | Derived: `MD5` of the order business key |
| `customer_hk` | `stg_orders` | `customer_id` | Derived: `MD5` of the customer business key |
| `load_date` | `northwind.stg_orders` | `load_date` |  |
| `record_source` | `northwind.stg_orders` | `record_source` |  |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `hub_order` | `order_hk = order_hk` | Many-to-one |
| `hub_customer` | `customer_hk = customer_hk` | Many-to-one |

## Notes / Caveats

- This document cites `stg_orders` unqualified where `hub_order` and `sat_order_details` both write `northwind.stg_orders`. Both spellings mean the same dbt model and have to fold onto one lineage node.
