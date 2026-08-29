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
| **Service of Record** | none — warehouse-generated |

The conformed calendar dimension. Every context that has a date joins this one, which is what makes a day comparable across a service boundary. It is generated in the warehouse rather than extracted from a service, because no eShop microservice owns a calendar and inventing an owner for one would put a shared concept inside somebody's aggregate.

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
| `date_key` | `orderingdb.stg_date_spine` | `date_day` | Primary Key |
| `date_day` | `orderingdb.stg_date_spine` | `date_day` | |
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

- The row above names a group of tables in prose rather than one table document, and is left in on purpose: a conformed dimension really is written this way, and the check should say so rather than the reader having to notice.
- The spine is built from the observed range of `orderingdb.stg_orders`, so the calendar starts at the first order eShop ever took. Catalog stock movements predate that — items are seeded with stock before anybody orders — and those rows have no date to join to.
- eShop stores every timestamp as UTC `DateTime`, and this calendar is UTC. Any report that presents a "day" to a customer in their own timezone is asking a question this dimension cannot answer.
