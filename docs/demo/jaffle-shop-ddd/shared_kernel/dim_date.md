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

The conformed calendar dimension. Every context that has a date joins this one, which is what makes a day comparable across context boundaries. Built from the dbt project's time spine so the calendar cannot drift away from the spine the metrics layer already uses.

## Columns

| Column | Type | Description |
|---|---|---|
| `date_key` | DATE | Calendar date (PK) |
| `date_day` | DATE | The date itself, for display |
| `day_of_week` | INT64 | 1 = Monday through 7 = Sunday |
| `day_name` | STRING | English weekday name |
| `week_start_date` | DATE | Monday of the week |
| `month_start_date` | DATE | First day of the month |
| `month_of_year` | INT64 | Month number |
| `quarter_of_year` | INT64 | Quarter number |
| `year_number` | INT64 | Calendar year |
| `is_weekend` | BOOLEAN | Saturday or Sunday |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `date_key` | `jaffle_shop.metricflow_time_spine` | `date_day` | Primary Key |
| `date_day` | `jaffle_shop.metricflow_time_spine` | `date_day` | |
| `day_of_week` | | | Derived: `EXTRACT(DOW FROM date_day)`, shifted to ISO |
| `day_name` | | | Derived from `day_of_week` |
| `week_start_date` | | | Derived: `DATE_TRUNC('week', date_day)` |
| `month_start_date` | | | Derived: `DATE_TRUNC('month', date_day)` |
| `month_of_year` | | | Derived |
| `quarter_of_year` | | | Derived |
| `year_number` | | | Derived |
| `is_weekend` | | | Derived: `day_of_week IN (6, 7)` |

## Relationships

This dimension is reused by every fact that has a date, in every context.

| Related Table | Join Key | Relationship |
|---|---|---|
| `Various Fact Tables` | `date_key` | One-to-many |

## Notes / Caveats

- Primary authority for every date reference in the model. A context that needs a calendar attribute this table does not carry should propose adding it here rather than keeping a local copy.
- `metricflow_time_spine` is generated daily and extends one year past the current date, so joining a forecast fact beyond that horizon drops rows.
