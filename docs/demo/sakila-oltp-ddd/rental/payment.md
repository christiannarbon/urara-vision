# payment

## Overview

| Property | Value |
|---|---|
| **Table Name** | `payment` |
| **Type** | Entity |
| **Domain** | Rental |
| **Bounded Context** | Rental |
| **Grain** | One row per payment. |
| **Update Frequency** | continuous |
| **Layer** | Operational Replica (3NF) |

14,596 payments against 16,044 rentals, which is the whole reason these are two tables: 1,448 rentals were never paid for, and a nullable amount on the rental row could not have told you which.

## Columns

| Column | Type | Description |
|---|---|---|
| `payment_id` | INT64 | Payment identifier (PK) |
| `customer_id` | INT64 | Who paid (FK) |
| `staff_id` | INT64 | Who took the payment (FK) |
| `rental_id` | INT64 | Which rental it settles (FK) |
| `amount` | FLOAT64 | Amount paid |
| `payment_date` | TIMESTAMP | When it was paid |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `payment_id` | `sakila.raw_payment` | `payment_id` | Primary Key |
| `customer_id` | `sakila.raw_payment` | `customer_id` | Foreign Key |
| `staff_id` | `sakila.raw_payment` | `staff_id` | Foreign Key |
| `rental_id` | `sakila.raw_payment` | `rental_id` | Foreign Key |
| `amount` | `sakila.raw_payment` | `amount` |  |
| `payment_date` | `sakila.raw_payment` | `payment_date` |  |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `rental` | `rental_id = rental_id` | Many-to-one |
| `customer` | `customer_id = customer_id` | Many-to-one |
