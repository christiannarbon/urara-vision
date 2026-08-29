# dim_catalog_brand

## Overview

| Property | Value |
|---|---|
| **Table Name** | `dim_catalog_brand` |
| **Type** | Dimension |
| **Domain** | Catalog |
| **Bounded Context** | Catalog |
| **Aggregate Root** | Catalog Item |
| **Grain** | One row per brand. |
| **Update Frequency** | daily |
| **Layer** | Star Schema (proposed) |
| **Service of Record** | Catalog.API |

The brand an item belongs to. In eShop this is a real table rather than a string column on the item, which is worth preserving: the storefront's brand filter is a foreign key lookup, and modelling it as an attribute in the warehouse would make the two disagree about how many brands exist.

## Columns

| Column | Type | Description |
|---|---|---|
| `catalog_brand_key` | STRING | Surrogate key over `catalog_brand_id` (PK) |
| `catalog_brand_id` | INT64 | Brand id from the Catalog database |
| `brand` | STRING | Brand name as shown in the storefront filter |
| `item_count` | INT64 | Items currently listed under the brand |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `catalog_brand_key` | `catalogdb.stg_catalog_brands` | `id` | Primary Key |
| `catalog_brand_id` | `catalogdb.stg_catalog_brands` | `id` | |
| `brand` | `catalogdb.stg_catalog_brands` | `brand` | |
| `item_count` | `catalogdb.stg_catalog_items` | `id` | Derived: `COUNT(*)` per brand |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `dim_catalog_item` | `catalog_brand_key = catalog_brand_key` | One-to-many |

## Notes / Caveats

- `item_count` counts listed items regardless of stock, so a brand whose entire range is sold out still reports its full count.
