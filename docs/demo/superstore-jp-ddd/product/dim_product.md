# dim_product

## Overview

| Property | Value |
|---|---|
| **Table Name** | `dim_product` |
| **Type** | Dimension |
| **Domain** | 商品 |
| **Bounded Context** | Product |
| **Grain** | 商品ごとに 1 行。 [EN] One row per product. |
| **Update Frequency** | 毎日 [EN] Daily |
| **Layer** | Star Schema (proposed) |

販売商品と、その分類。カテゴリは 3 つ、サブカテゴリは 17 で、いずれも商品行に平坦化しています。 [EN] The product and how it is classified. Three categories and seventeen sub-categories, both flattened onto the product row.

## Columns

| Column | Type | Description |
|---|---|---|
| `product_id` | STRING | 商品識別子（主キー） [EN] Product identifier (PK) |
| `product_name` | STRING | 商品名 [EN] Product name |
| `category` | STRING | 大分類 [EN] Category |
| `sub_category` | STRING | 中分類 [EN] Sub-category |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `product_id` | `superstore.stg_products` | `product_id` | 主キー [EN] Primary Key |
| `product_name` | `superstore.stg_products` | `product_name` | |
| `category` | `superstore.stg_products` | `category` | |
| `sub_category` | `superstore.stg_products` | `sub_category` | |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `fact_order_line` | `product_id = product_id` | One-to-many |

## Notes / Caveats

- The same product name appears under more than one `product_id` where the source recorded a repackaged item as a new product. Counting distinct names and counting products give different answers.
- 上の注記は英語のみで書かれています。翻訳のないフィールドは、主要言語が日本語であってもそのまま表示されます。 [EN] The note above is written in English only. A field with no translation is shown as written, even where the primary language is Japanese.
