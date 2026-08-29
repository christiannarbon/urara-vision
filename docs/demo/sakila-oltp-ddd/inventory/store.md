# store

## Overview

| Property | Value |
|---|---|
| **Table Name** | `store` |
| **Type** | Entity |
| **Domain** | Inventory |
| **Bounded Context** | Inventory |
| **Grain** | One row per store. |
| **Update Frequency** | yearly |
| **Layer** | Operational Replica (3NF) |

Two stores. It points at an address the same way a customer does, which is the reason `address` is a table rather than a set of columns.

## Columns

| Column | Type | Description |
|---|---|---|
| `store_id` | INT64 | Store identifier (PK) |
| `manager_staff_id` | INT64 | Staff member managing the store (FK) |
| `address_id` | INT64 | Where the store is (FK) |
| `last_update` | TIMESTAMP | When the row was last changed in the source |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `store_id` | `sakila.raw_store` | `store_id` | Primary Key |
| `manager_staff_id` | Not replicated yet; the Staffing context has not shipped |  | Placeholder column, always null |
| `address_id` | `sakila.raw_store` | `address_id` | Foreign Key |
| `last_update` | `sakila.raw_store` | `last_update` |  |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `address` | `address_id = address_id` | Many-to-one |
| `inventory` | `store_id = store_id` | One-to-many |
| `customer` | `store_id = store_id` | One-to-many |

## Notes / Caveats

- `manager_staff_id` records its source as prose because the staff table has not been replicated. The column exists so the shape is right; it is null on both rows.
