# DimDate

## Overview

| Property | Value |
|---|---|
| **Table Name** | `DimDate` |
| **Type** | Dimension |
| **Domain** | Customer |
| **Bounded Context** | Customer |
| **Grain** | One row per calendar day, as this context sees it. |
| **Update Frequency** | yearly |
| **Layer** | Snowflake Schema |

A stale local copy of the kernel's calendar, taken when this context needed a cohort month the kernel did not have. The kernel has since gained four columns this copy has never heard of.

## Columns

| Column | Type | Description |
|---|---|---|
| `DateKey` | INT64 | Date as an integer, yyyymmdd (PK) |
| `FullDateAlternateKey` | DATE | The same date, as a date |
| `CalendarYear` | INT64 | Calendar year |
| `CalendarQuarter` | INT64 | Calendar quarter, 1-4 |
| `CohortMonth` | STRING | Acquisition cohort label, which the kernel's calendar does not carry |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `DateKey` | `adventureworks.dim_date_generated` | `DateKey` | Primary Key |
| `FullDateAlternateKey` | `adventureworks.dim_date_generated` | `FullDateAlternateKey` |  |
| `CalendarYear` | `adventureworks.dim_date_generated` | `CalendarYear` |  |
| `CalendarQuarter` | `adventureworks.dim_date_generated` | `CalendarQuarter` |  |
| `CohortMonth` | Computed in the cohort model rather than read from a column |  | Derived: month of first purchase |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|

## Notes / Caveats

- Deprecated. Read `shared_kernel/DimDate` instead; the cohort month belongs on a customer attribute, not on a calendar.
- `CohortMonth` records its source as prose because it is computed rather than read.
