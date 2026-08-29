# dim_category

## Overview

| Property | Value |
|---|---|
| **Table Name** | `dim_category` |
| **Type** | Outrigger |
| **Domain** | Presentation - Catalog |
| **Bounded Context** | Presentation - Catalog |
| **Grain** | One row per product category. |
| **Update Frequency** | daily |
| **Layer** | Snowflake Schema |

The category outrigger. Eight rows, one long description each, hanging off `dim_product` rather than folded into it.

## Columns

| Column | Type | Description |
|---|---|---|
| `category_key` | STRING | Surrogate key for the category (PK) |
| `category_id` | INT64 | Northwind's CategoryID |
| `category_name` | STRING | Category name |
| `category_description` | STRING | The long description; the reason this is not folded in |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `category_key` | `northwind.stg_categories` | `category_id` | Derived: surrogate key from the business key |
| `category_id` | `northwind.stg_categories` | `category_id` | Primary Key |
| `category_name` | `northwind.stg_categories` | `category_name` |  |
| `category_description` | `northwind.stg_categories` | `description` |  |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `dim_product` | `category_key = category_key` | One-to-many |
