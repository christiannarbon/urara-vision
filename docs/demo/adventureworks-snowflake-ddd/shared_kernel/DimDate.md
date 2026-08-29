# DimDate

## Overview

| Property | Value |
|---|---|
| **Table Name** | `DimDate` |
| **Type** | Dimension (Conformed) |
| **Domain** | Shared Kernel |
| **Bounded Context** | Shared Kernel |
| **Grain** | One row per calendar day. |
| **Update Frequency** | yearly |
| **Layer** | Snowflake Schema |

The conformed calendar, carrying AdventureWorks' calendar and fiscal years side by side. Both fact tables join to it; `customer/DimDate` is a stale copy that should not exist.

## Columns

| Column | Type | Description |
|---|---|---|
| `DateKey` | INT64 | Date as an integer, yyyymmdd (PK) |
| `FullDateAlternateKey` | DATE | The same date, as a date |
| `EnglishDayNameOfWeek` | STRING | Day name |
| `MonthNumberOfYear` | INT64 | Month, 1-12 |
| `EnglishMonthName` | STRING | Month name |
| `CalendarQuarter` | INT64 | Calendar quarter, 1-4 |
| `CalendarYear` | INT64 | Calendar year |
| `FiscalQuarter` | INT64 | Fiscal quarter, 1-4 |
| `FiscalYear` | INT64 | Fiscal year; AdventureWorks' fiscal year starts in July |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `DateKey` | `adventureworks.dim_date_generated` | `DateKey` |  |
| `FullDateAlternateKey` | `adventureworks.dim_date_generated` | `FullDateAlternateKey` |  |
| `EnglishDayNameOfWeek` | `adventureworks.dim_date_generated` | `EnglishDayNameOfWeek` |  |
| `MonthNumberOfYear` | `adventureworks.dim_date_generated` | `MonthNumberOfYear` |  |
| `EnglishMonthName` | `adventureworks.dim_date_generated` | `EnglishMonthName` |  |
| `CalendarQuarter` | `adventureworks.dim_date_generated` | `CalendarQuarter` |  |
| `CalendarYear` | `adventureworks.dim_date_generated` | `CalendarYear` |  |
| `FiscalQuarter` | `adventureworks.dim_date_generated` | `FiscalQuarter` |  |
| `FiscalYear` | `adventureworks.dim_date_generated` | `FiscalYear` |  |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| Both Fact Tables | `DateKey` | One-to-many |

## Notes / Caveats

- The row above names a group of tables in prose rather than a table, which is what calendar documentation usually says. The parser should record it as narrative rather than as a broken reference.
- `customer/DimDate` is a stale copy of this table. This one is the authority.
