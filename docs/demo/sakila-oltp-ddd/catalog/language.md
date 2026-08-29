# language

## Overview

| Property | Value |
|---|---|
| **Table Name** | `language` |
| **Type** | Lookup |
| **Domain** | Catalog |
| **Bounded Context** | Catalog |
| **Grain** | One row per language. |
| **Update Frequency** | yearly |
| **Layer** | Operational Replica (3NF) |

Six rows: English, Italian, Japanese, Mandarin, French, German. A lookup rather than an entity -- there is nothing to know about a language here except its name, and nothing would ever be recorded against one.

## Columns

| Column | Type | Description |
|---|---|---|
| `language_id` | INT64 | Language identifier (PK) |
| `name` | STRING | Language name |
| `last_update` | TIMESTAMP | When the row was last changed in the source |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `language_id` | `sakila.raw_language` | `language_id` | Primary Key |
| `name` | `sakila.raw_language` | `name` |  |
| `last_update` | `sakila.raw_language` | `last_update` |  |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `film` | `language_id = language_id` | One-to-many |

## Notes / Caveats

- The distinction between this and `shared_kernel/country` is ownership rather than shape. A country is reference data the whole schema shares; a language is a lookup the Catalog owns and nothing else reads.
