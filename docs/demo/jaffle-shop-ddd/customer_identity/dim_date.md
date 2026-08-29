# dim_date

## Overview

| Property | Value |
|---|---|
| **Table Name** | `dim_date` |
| **Type** | Dimension |
| **Domain** | Customer Identity |
| **Bounded Context** | Customer Identity |
| **Aggregate Root** | None — reference data |
| **Grain** | One row per calendar date. |
| **Update Frequency** | daily |
| **Layer** | Star Schema (deprecated) |

A local calendar copy that predates the Shared Kernel, kept because the cohort reports were written against its fiscal columns. It carries fewer calendar attributes than the kernel's `dim_date` and two the kernel does not have, which is exactly the drift a conformed dimension is supposed to prevent.

## Columns

| Column | Type | Description |
|---|---|---|
| `date_key` | DATE | Calendar date (PK) |
| `date_day` | DATE | The date itself, for display |
| `month_start_date` | DATE | First day of the month |
| `year_number` | INT64 | Calendar year |
| `fiscal_period` | STRING | Fiscal period label, e.g. 'FY24-P03' |
| `fiscal_quarter` | INT64 | Fiscal quarter, year starting in February |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `date_key` | `jaffle_shop.metricflow_time_spine` | `date_day` | Primary Key |
| `date_day` | `jaffle_shop.metricflow_time_spine` | `date_day` | |
| `month_start_date` | | | Derived |
| `year_number` | | | Derived |
| `fiscal_period` | Agreed with the finance team, not modelled anywhere | | Derived: hand-maintained mapping |
| `fiscal_quarter` | Agreed with the finance team, not modelled anywhere | | Derived: hand-maintained mapping |

## Notes / Caveats

- Deprecated. The fiscal columns are the only reason this table still exists; once they move into the Shared Kernel's `dim_date` this document should be deleted.
- No relationships are documented on purpose. Nothing new should join this copy.
