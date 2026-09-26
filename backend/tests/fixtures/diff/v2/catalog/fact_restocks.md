# fact_restocks

## Overview

| Property | Value |
|---|---|
| **Table Name** | `fact_restocks` |
| **Type** | Fact |
| **Domain** | Catalog |
| **Grain** | One row per delivery line. |
| **Update Frequency** | Daily |
| **Layer** | Star Schema (proposed) |

Stock deliveries received.

## Columns

| Column | Type | Description |
|---|---|---|
| `restock_id` | STRING | Delivery line identifier (PK) |
| `product_id` | STRING | Product delivered (FK) |
| `quantity` | INT64 | Units delivered |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `dim_products` | `product_id = product_id` | Many-to-one |
