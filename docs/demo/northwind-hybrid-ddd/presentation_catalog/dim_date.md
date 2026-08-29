# dim_date

## Overview

| Property | Value |
|---|---|
| **Table Name** | `dim_date` |
| **Type** | Dimension |
| **Domain** | Presentation - Catalog |
| **Bounded Context** | Presentation - Catalog |
| **Grain** | One row per calendar day, as this context sees it. |
| **Update Frequency** | yearly |
| **Layer** | Snowflake Schema |

A stale local copy of the kernel's calendar, taken when this context needed a fiscal period the kernel did not have. The kernel has since gained three columns this copy has never heard of, and this copy has gained one the kernel does not want.

## Columns

| Column | Type | Description |
|---|---|---|
| `date_key` | DATE | The date itself (PK) |
| `full_date` | DATE | The date, unabbreviated |
| `calendar_year` | INT64 | Calendar year |
| `calendar_month` | INT64 | Calendar month, 1-12 |
| `fiscal_period` | STRING | Fiscal period label, which the kernel's calendar does not carry |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `date_key` | `northwind.dim_date_generated` | `date_key` | Primary Key |
| `full_date` | `northwind.dim_date_generated` | `full_date` |  |
| `calendar_year` | `northwind.dim_date_generated` | `calendar_year` |  |
| `calendar_month` | `northwind.dim_date_generated` | `calendar_month` |  |
| `fiscal_period` | Maintained by hand in a spreadsheet the finance team owns |  | Derived: looked up by month |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|

## Notes / Caveats

- Deprecated. Read `shared_kernel/dim_date` instead; the fiscal period belongs on the kernel's calendar and moving it there is the fix.
- `fiscal_period` records its source as prose because it genuinely is a spreadsheet.
