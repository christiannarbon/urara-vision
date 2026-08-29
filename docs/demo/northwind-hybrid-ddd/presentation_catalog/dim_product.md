# dim_product

## Overview

| Property | Value |
|---|---|
| **Table Name** | `dim_product` |
| **Type** | Dimension |
| **Domain** | Presentation - Catalog |
| **Bounded Context** | Presentation - Catalog |
| **Grain** | One row per product. |
| **Update Frequency** | daily |
| **Layer** | Snowflake Schema |

One row per product. The supplier's name is folded in because nothing browses suppliers on their own; the category is not, because the reporting tool does. That asymmetry is the whole of the decision behind a snowflake.

## Columns

| Column | Type | Description |
|---|---|---|
| `product_key` | STRING | Surrogate key for the product (PK) |
| `product_id` | INT64 | Northwind's ProductID |
| `product_name` | STRING | Product name |
| `category_key` | STRING | Category the product belongs to (FK) |
| `supplier_name` | STRING | Supplier's company name, folded in |
| `unit_price` | FLOAT64 | List price |
| `units_in_stock` | INT64 | Units on hand |
| `discontinued` | BOOLEAN | Whether the product is discontinued |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `product_key` | `northwind.stg_products` | `product_id` | Derived: surrogate key from the business key |
| `product_id` | `northwind.stg_products` | `product_id` | Primary Key |
| `product_name` | `northwind.stg_products` | `product_name` |  |
| `category_key` | `northwind.stg_products` | `category_id` | Foreign Key |
| `supplier_name` | `northwind.stg_suppliers` | `company_name` | Derived: joined in from the supplier staging model |
| `unit_price` | `northwind.stg_products` | `unit_price` |  |
| `units_in_stock` | `northwind.stg_products` | `units_in_stock` |  |
| `discontinued` | `northwind.stg_products` | `discontinued` |  |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `dim_category` | `category_key = category_key` | Many-to-one |
| `fact_order_items` | `product_key = product_key` | One-to-many |

## Notes / Caveats

- The join to `dim_category` is a dimension joined to a dimension. In a pure star it would not exist; here it is the outrigger the set is meant to show.
