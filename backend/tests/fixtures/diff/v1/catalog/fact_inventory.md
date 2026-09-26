# fact_inventory

## Overview

| Property | Value |
|---|---|
| **Table Name** | `fact_inventory` |
| **Type** | Fact |
| **Domain** | Catalog |
| **Grain** | One row per product per day. |
| **Update Frequency** | Daily |
| **Layer** | Star Schema (proposed) |

Stock on hand at the end of each day.

## Columns

| Column | Type | Description |
|---|---|---|
| `product_id` | STRING | Product held (FK) |
| `on_hand` | INT64 | Units in stock |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `on_hand` | `shop.stock` | `quantity` | |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `dim_products` | `product_id = product_id` | Many-to-one |
