# fact_orders

## Overview

| Property | Value |
|---|---|
| **Table Name** | `fact_orders` |
| **Type** | Fact |
| **Domain** | Sales |
| **Grain** | One row per order. |
| **Update Frequency** | Daily |
| **Layer** | Star Schema (proposed) |

Every order placed.

## Columns

| Column | Type | Description |
|---|---|---|
| `order_id` | STRING | Order identifier (PK) |
| `customer_id` | STRING | Customer who ordered (FK) |
| `product_id` | STRING | Product ordered (FK) |
| `amount` | NUMERIC | Amount charged |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `order_id` | `shop.orders` | `id` | Primary Key |
| `amount` | `shop.orders` | `amount` | |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `dim_customers` | `customer_id = customer_id` | Many-to-one |
| `dim_products` | `product_id = product_id` | Many-to-one |
