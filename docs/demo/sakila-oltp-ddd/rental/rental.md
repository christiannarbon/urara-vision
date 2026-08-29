# rental

## Overview

| Property | Value |
|---|---|
| **Table Name** | `rental` |
| **Type** | Entity |
| **Domain** | Rental |
| **Bounded Context** | Rental |
| **Grain** | One row per rental of one copy. |
| **Update Frequency** | continuous |
| **Layer** | Operational Replica (3NF) |

16,044 rentals. It points at a copy rather than at a film, and at a customer rather than at an address -- both of which are the schema refusing to repeat a fact it already stores elsewhere.

## Columns

| Column | Type | Description |
|---|---|---|
| `rental_id` | INT64 | Rental identifier (PK) |
| `rental_date` | TIMESTAMP | When the copy left the shelf |
| `inventory_id` | INT64 | Which copy was taken (FK) |
| `customer_id` | INT64 | Who took it (FK) |
| `return_date` | TIMESTAMP | When it came back; null while it is out |
| `staff_id` | INT64 | Who handled the rental (FK) |
| `last_update` | TIMESTAMP | When the row was last changed in the source |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `rental_id` | `sakila.raw_rental` | `rental_id` | Primary Key |
| `rental_date` | `sakila.raw_rental` | `rental_date` |  |
| `inventory_id` | `sakila.raw_rental` | `inventory_id` | Foreign Key |
| `customer_id` | `sakila.raw_rental` | `customer_id` | Foreign Key |
| `return_date` | `sakila.raw_rental` | `return_date` |  |
| `staff_id` | `sakila.raw_rental` | `staff_id` | Foreign Key |
| `last_update` | `sakila.raw_rental` | `last_update` |  |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `inventory` | `inventory_id = inventory_id` | Many-to-one |
| `customer` | `customer_id = customer_id` | Many-to-one |
| `payment` | `rental_id = rental_id` | One-to-many |
| `staff` | `staff_id = staff_id` | Many-to-one |

## Notes / Caveats

- `staff` belongs to the Staffing context, which is on the context map but has no table documents yet, so this reference cannot resolve. It is the only error in the set.
