# customer

## Overview

| Property | Value |
|---|---|
| **Table Name** | `customer` |
| **Type** | Entity |
| **Domain** | Party |
| **Bounded Context** | Party |
| **Grain** | One row per customer. |
| **Update Frequency** | continuous |
| **Layer** | Operational Replica (3NF) |

599 customers. Their country is three joins away -- customer to address to city to country -- and that is not an oversight. Each of those three tables is the single place its fact is recorded.

## Columns

| Column | Type | Description |
|---|---|---|
| `customer_id` | INT64 | Customer identifier (PK) |
| `store_id` | INT64 | Store the customer is registered at (FK) |
| `first_name` | STRING | First name |
| `last_name` | STRING | Last name |
| `email` | STRING | Email address |
| `address_id` | INT64 | Where they live (FK) |
| `activebool` | BOOLEAN | Whether the account is active |
| `create_date` | DATE | When the account was opened |
| `last_update` | TIMESTAMP | When the row was last changed in the source |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `customer_id` | `sakila.raw_customer` | `customer_id` | Primary Key |
| `store_id` | `sakila.raw_customer` | `store_id` | Foreign Key |
| `first_name` | `sakila.raw_customer` | `first_name` |  |
| `last_name` | `sakila.raw_customer` | `last_name` |  |
| `email` | `sakila.raw_customer` | `email` |  |
| `address_id` | `sakila.raw_customer` | `address_id` | Foreign Key |
| `activebool` | `sakila.raw_customer` | `activebool` |  |
| `create_date` | `sakila.raw_customer` | `create_date` |  |
| `last_update` | `sakila.raw_customer` | `last_update` |  |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `address` | `address_id = address_id` | Many-to-one |
| `store` | `store_id = store_id` | Many-to-one |
| `rental` | `customer_id = customer_id` | One-to-many |
