# hub_order

## Overview

| Property | Value |
|---|---|
| **Table Name** | `hub_order` |
| **Type** | Hub |
| **Domain** | Raw Vault |
| **Bounded Context** | Raw Vault |
| **Layer** | Raw Vault |
| **Update Frequency** | hourly |
| **Business Key** | `OrderID` |
| **Grain** | One row per order business key. |

Northwind's `OrderID`, hashed. One row per order ever seen.

## Columns

| Column | Type | Description |
|---|---|---|
| `order_hk` | BINARY | Hash of the order business key (PK) |
| `order_id` | INT64 | Northwind's OrderID, as it arrives |
| `load_date` | TIMESTAMP | When this key was first seen |
| `record_source` | STRING | The staging model this key arrived from |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `order_hk` | `northwind.stg_orders` | `order_id` | Derived: `MD5` of the business key |
| `order_id` | `northwind.stg_orders` | `order_id` | Business key |
| `load_date` | `northwind.stg_orders` | `load_date` |  |
| `record_source` | `northwind.stg_orders` | `record_source` |  |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `sat_order_details` | `order_hk = order_hk` | One-to-many |
| `lnk_order_customer` | `order_hk = order_hk` | One-to-many |
