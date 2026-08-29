# country

## Overview

| Property | Value |
|---|---|
| **Table Name** | `country` |
| **Type** | Reference |
| **Domain** | Party |
| **Bounded Context** | Party |
| **Grain** | One row per country, as this context sees it. |
| **Update Frequency** | yearly |
| **Layer** | Operational Replica (3NF) |

A stale local copy of the kernel's country table, taken when this context wanted an ISO code the kernel did not have. It has never been reconciled since.

## Columns

| Column | Type | Description |
|---|---|---|
| `country_id` | INT64 | Country identifier (PK) |
| `country` | STRING | Country name |
| `iso_code` | STRING | ISO 3166 alpha-2 code, which the kernel's table does not carry |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `country_id` | `sakila.raw_country` | `country_id` | Primary Key |
| `country` | `sakila.raw_country` | `country` |  |
| `iso_code` | Maintained by hand against a Wikipedia table |  | Derived: matched on country name |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|

## Notes / Caveats

- Deprecated. Read `shared_kernel/country` instead; the ISO code belongs on the kernel's table and moving it there is the fix.
- `iso_code` records its source as prose because it genuinely is a hand-maintained list, matched on name rather than on key -- which is the same mistake the `address` document makes in its join, and for the same reason.
