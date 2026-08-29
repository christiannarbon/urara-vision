# FactSalesQuota

## Overview

| Property | Value |
|---|---|
| **Table Name** | `FactSalesQuota` |
| **Type** | Fact |
| **Domain** | Sales |
| **Bounded Context** | Sales |
| **Grain** | One row per employee per quarter. |
| **Update Frequency** | quarterly |
| **Layer** | Snowflake Schema |

Quarterly quota by salesperson. It was written before `DimEmployee` existed and before anyone decided whether a quota should join to the calendar by quarter or carry its own period key, so it currently names its dimensions in prose and joins to nothing at all.

## Columns

| Column | Type | Description |
|---|---|---|
| `EmployeeKey` | INT64 | Salesperson the quota is set for (FK) |
| `CalendarYear` | INT64 | Year the quota applies to |
| `CalendarQuarter` | INT64 | Quarter the quota applies to |
| `SalesAmountQuota` | FLOAT64 | Quota amount for the quarter |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `EmployeeKey` | `adventureworks.stg_salesperson` | `BusinessEntityID` | Foreign Key |
| `CalendarYear` | `adventureworks.stg_salesperson` | `ModifiedDate` | Derived: year of the quota period |
| `CalendarQuarter` | `adventureworks.stg_salesperson` | `ModifiedDate` | Derived: quarter of the quota period |
| `SalesAmountQuota` | `adventureworks.stg_salesperson` | `SalesQuota` |  |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| The Employee and Date Dimensions | `EmployeeKey` | Many-to-one |

## Notes / Caveats

- This fact names its dimensions in prose rather than naming tables, so nothing it declares resolves and it ends up joined to nothing. It is here on purpose so that check has something to find.
- Its grain is also wrong for this schema: a quarter is not a date, so it cannot join `DimDate` without either a quarter-grain calendar or a range join. That is the real reason it was never finished.
