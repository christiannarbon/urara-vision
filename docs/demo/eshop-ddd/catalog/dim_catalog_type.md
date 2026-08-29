# dim_catalog_type

## Overview

| Property | Value |
|---|---|
| **Table Name** | `dim_catalog_type` |
| **Type** | Dimension |
| **Domain** | Catalog |
| **Bounded Context** | Catalog |
| **Aggregate Root** | Catalog Item |
| **Grain** | One row per type. |
| **Update Frequency** | daily |
| **Layer** | Star Schema (proposed) |
| **Service of Record** | Catalog.API |

The type an item belongs to — "T-Shirt", "Mug", "Sheet" and the rest. Like brand, this is a real table in eShop and stays one here.

## Columns

| Column | Type | Description |
|---|---|---|
| `catalog_type_key` | STRING | Surrogate key over `catalog_type_id` (PK) |
| `catalog_type_id` | INT64 | Type id from the Catalog database |
| `type` | STRING | Type name as shown in the storefront filter |
| `item_count` | INT64 | Items currently listed under the type |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `catalog_type_key` | `catalogdb.stg_catalog_types` | `id` | Primary Key |
| `catalog_type_id` | `catalogdb.stg_catalog_types` | `id` | |
| `type` | `catalogdb.stg_catalog_types` | `type` | |
| `item_count` | `catalogdb.stg_catalog_items` | `id` | Derived: `COUNT(*)` per type |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `dim_catalog_item` | `catalog_type_key = catalog_type_key` | One-to-many |

## Notes / Caveats

- Type and brand are independent in the data model but not in practice: most brands carry one type. Cross-tabulating the two produces a very sparse grid.
