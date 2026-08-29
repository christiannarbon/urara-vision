# hub_customer

## Overview

| Property | Value |
|---|---|
| **Table Name** | `hub_customer` |
| **Type** | Hub |
| **Domain** | Raw Vault |
| **Bounded Context** | Raw Vault |
| **Layer** | Raw Vault |
| **Update Frequency** | hourly |
| **Business Key** | `CustomerID` |
| **Grain** | One row per customer business key. |

Northwind's `CustomerID` — a five-letter code like `ALFKI`, not a number — hashed into a stable key the rest of the vault joins on.

## Columns

| Column | Type | Description |
|---|---|---|
| `customer_hk` | BINARY | Hash of the customer business key (PK) |
| `customer_id` | STRING | Northwind's CustomerID, as it arrives |
| `load_date` | TIMESTAMP | When this key was first seen |
| `record_source` | STRING | The staging model this key arrived from |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `customer_hk` | `northwind.stg_customers` | `customer_id` | Derived: `MD5` of the business key |
| `customer_id` | `northwind.stg_customers` | `customer_id` | Business key |
| `load_date` | `northwind.stg_customers` | `load_date` |  |
| `record_source` | `northwind.stg_customers` | `record_source` |  |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `sat_customer_details` | `customer_hk = customer_hk` | One-to-many |
| `lnk_order_customer` | `customer_hk = customer_hk` | One-to-many |

## Notes / Caveats

- The presentation layer's `dim_customer` and the business vault's bridge both join back to this hub. That is the drill-back path, and it is the reason the hash key is carried all the way into the star.
