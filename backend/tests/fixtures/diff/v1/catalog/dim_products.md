# dim_products

## Overview

| Property | Value |
|---|---|
| **Table Name** | `dim_products` |
| **Type** | Dimension |
| **Domain** | Catalog |
| **Grain** | One row per product. |
| **Update Frequency** | Daily |
| **Layer** | Star Schema (proposed) |

Products on sale.

## Columns

| Column | Type | Description |
|---|---|---|
| `product_id` | STRING | Product identifier (PK) |
| `product_name` | STRING | Product name |
| `price` | FLOAT64 | List price |
