# dim_date

## Overview

| Property | Value |
|---|---|
| **Table Name** | `dim_date` |
| **Type** | Dimension (Conformed) |
| **Domain** | Shared Kernel |
| **Bounded Context** | Shared Kernel |
| **Grain** | One row per calendar day. |
| **Update Frequency** | yearly |
| **Layer** | Shared |

The conformed calendar, read by both layers. It is generated rather than loaded, which is why its lineage cites a model with no upstream of its own.

## Columns

| Column | Type | Description |
|---|---|---|
| `date_key` | DATE | The date itself (PK) |
| `full_date` | DATE | The date, unabbreviated |
| `calendar_year` | INT64 | Calendar year |
| `calendar_quarter` | INT64 | Calendar quarter, 1-4 |
| `calendar_month` | INT64 | Calendar month, 1-12 |
| `day_of_week` | STRING | Day name |
| `is_weekday` | BOOLEAN | Whether the date is a working day |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `date_key` | `northwind.dim_date_generated` | `date_key` | Primary Key |
| `full_date` | `northwind.dim_date_generated` | `full_date` |  |
| `calendar_year` | `northwind.dim_date_generated` | `calendar_year` |  |
| `calendar_quarter` | `northwind.dim_date_generated` | `calendar_quarter` |  |
| `calendar_month` | `northwind.dim_date_generated` | `calendar_month` |  |
| `day_of_week` | `northwind.dim_date_generated` | `day_of_week` |  |
| `is_weekday` | `northwind.dim_date_generated` | `is_weekday` |  |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| Every Fact Table in the Presentation Layer | `date_key` | One-to-many |

## Notes / Caveats

- The row above names a group of tables in prose rather than a table. Real calendar documentation says exactly this, and the parser should record it as narrative rather than as a broken reference.
- `presentation_catalog/dim_date` is a stale copy of this table. This one is the authority.
