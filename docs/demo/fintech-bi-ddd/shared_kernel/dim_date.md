# dim_date

## Overview

| Property | Value |
|---|---|
| **Table Name** | `dim_date` |
| **Type** | Dimension (Conformed — primary authority) |
| **Domain** | Shared Kernel (Conformed Dimensions) |
| **Bounded Context** | Shared Kernel |
| **Aggregate Root** | None — reference data, owned by no aggregate |
| **Grain** | One row per calendar date. |
| **Update Frequency** | daily |
| **Layer** | Star Schema (proposed) |
| **Semantic Entity** | date |

The conformed calendar dimension. Every context that has a date joins this one, which is what makes a day comparable across a context boundary. The key is the surrogate hash of `date_day` rather than the date itself, so that a fact carrying a null date can be pointed at an unknown-member row instead of dropping out of the join.

## Columns

| Column | Type | Description |
|---|---|---|
| `date_key` | STRING | Surrogate key over `date_day` (PK) |
| `date_day` | DATE | The date itself, for display |
| `date_week` | DATE | Monday of the week |
| `date_month` | DATE | First day of the month |
| `date_quarter` | DATE | First day of the quarter |
| `date_year` | DATE | First day of the year |
| `day_of_week` | INT64 | 1 = Monday through 7 = Sunday |
| `day_of_week_name` | STRING | English weekday name |
| `month_of_year` | INT64 | Month number |
| `year_number` | INT64 | Calendar year |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `date_key` | `core_banking.stg_date_dimension` | `date_day` | Primary Key; `generate_surrogate_key(['date_day'])` |
| `date_day` | `core_banking.stg_date_dimension` | `date_day` | |
| `date_week` | `core_banking.stg_date_dimension` | `week_start_date` | Renamed on the way into the mart |
| `date_month` | `core_banking.stg_date_dimension` | `month_start_date` | Renamed on the way into the mart |
| `date_quarter` | `core_banking.stg_date_dimension` | `quarter_start_date` | Renamed on the way into the mart |
| `date_year` | `core_banking.stg_date_dimension` | `year_start_date` | Renamed on the way into the mart |
| `day_of_week` | `core_banking.stg_date_dimension` | `day_of_week_iso` | |
| `day_of_week_name` | `core_banking.stg_date_dimension` | `day_of_week_name` | |
| `month_of_year` | `core_banking.stg_date_dimension` | `month_of_year` | |
| `year_number` | `core_banking.stg_date_dimension` | `year_number` | |

## Relationships

This dimension is reused by every fact that has a date, in every context.

| Related Table | Join Key | Relationship |
|---|---|---|
| `Various Fact Tables` | `date_key` | One-to-many |

## Notes / Caveats

- The row above names a group of tables in prose rather than one table document, which is left in on purpose: a conformed dimension really is written this way, and the check should say so rather than the reader having to notice.
- Primary authority for every date reference in the model. A context needing a calendar attribute this table does not carry should propose adding it here rather than keeping a local copy.
- `stg_date_dimension` is generated from the observed range of `card_network.stg_transaction`, so the calendar does not extend past the latest booked transaction. Lending's amortisation schedules run years beyond that, and those future instalments will not join.
