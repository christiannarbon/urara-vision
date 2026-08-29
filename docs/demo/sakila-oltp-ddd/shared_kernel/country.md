# country

## Overview

| Property | Value |
|---|---|
| **Table Name** | `country` |
| **Type** | Reference (Conformed) |
| **Domain** | Shared Kernel |
| **Bounded Context** | Shared Kernel |
| **Grain** | One row per country. |
| **Update Frequency** | daily |
| **Layer** | Operational Replica (3NF) |

109 countries. Reference data in the strict sense: it is not owned by any context, it changes almost never, and every other table that needs a country reaches it through `city` rather than storing a name.

## Columns

| Column | Type | Description |
|---|---|---|
| `country_id` | INT64 | Country identifier (PK) |
| `country` | STRING | Country name |
| `last_update` | TIMESTAMP | When the row was last changed in the source |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `country_id` | `sakila.raw_country` | `country_id` | Primary Key |
| `country` | `sakila.raw_country` | `country` |  |
| `last_update` | `sakila.raw_country` | `last_update` |  |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `city` | `country_id = country_id` | One-to-many |
| Every Address Table | `country_id` | One-to-many |

## Notes / Caveats

- The second row names a group of tables in prose rather than a table. It is what reference-data documentation usually says, and the parser should record it as narrative rather than as a broken reference.
- `party/country` is a copy of this table. This one is the authority.
