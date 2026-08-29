# film_category

## Overview

| Property | Value |
|---|---|
| **Table Name** | `film_category` |
| **Type** | Associative |
| **Domain** | Catalog |
| **Bounded Context** | Catalog |
| **Grain** | One row per film-and-category pair. |
| **Update Frequency** | daily |
| **Layer** | Operational Replica (3NF) |

The other junction table. It was replicated before anybody documented the category table it points at, so it currently names its parents in prose and joins to nothing.

## Columns

| Column | Type | Description |
|---|---|---|
| `film_id` | INT64 | The film (PK, with category_id; FK) |
| `category_id` | INT64 | The category (PK, with film_id; FK) |
| `last_update` | TIMESTAMP | When the row was last changed in the source |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `film_id` | `sakila.raw_film_category` | `film_id` | Foreign Key |
| `category_id` | `sakila.raw_film_category` | `category_id` | Foreign Key |
| `last_update` | `sakila.raw_film_category` | `last_update` |  |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| The Category Tables | `category_id` | Many-to-one |

## Notes / Caveats

- This table names its parents in prose rather than naming tables, so nothing it declares resolves and it ends up joined to nothing. A junction table joined to nothing cannot do the one job it has, which is why the check treats it the same way it treats a fact with no dimensions.
- It is the same shape of gap as `supply/lnk_part_supplier` in the Data Vault set, on a different vocabulary's connective table.
