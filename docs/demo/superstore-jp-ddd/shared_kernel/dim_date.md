# dim_date

## Overview

| Property | Value |
|---|---|
| **Table Name** | `dim_date` |
| **Type** | Dimension |
| **Domain** | Shared Kernel (Conformed Dimensions) |
| **Bounded Context** | Shared Kernel |
| **Grain** | 1 日ごとに 1 行。 [EN] One row per day. |
| **Update Frequency** | 年次 [EN] Yearly |
| **Layer** | Star Schema (proposed) |

すべてのコンテキストが結合する暦。意図的に統一しています。暦が二つあることが、四半期の定義について二つのコンテキストが食い違い始める原因です。 [EN] The calendar every context joins to, conformed on purpose. A second calendar is how two contexts start disagreeing about what a quarter is.

## Columns

| Column | Type | Description |
|---|---|---|
| `date_key` | DATE | 日付そのもの（主キー） [EN] The day itself (PK) |
| `calendar_year` | INT64 | 暦年 [EN] Calendar year |
| `calendar_month` | INT64 | 月（1〜12） [EN] Month of the year, 1-12 |
| `fiscal_quarter` | STRING | 会計四半期。4 月開始 [EN] Fiscal quarter, starting in April |
| `is_holiday` | BOOLEAN | 日本の祝日かどうか [EN] Whether the day is a Japanese public holiday |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `date_key` | `superstore.stg_calendar` | `date_day` | 主キー [EN] Primary Key |
| `calendar_year` | `superstore.stg_calendar` | `date_day` | 導出：`EXTRACT(YEAR)` [EN] Derived: `EXTRACT(YEAR)` |
| `calendar_month` | `superstore.stg_calendar` | `date_day` | 導出：`EXTRACT(MONTH)` [EN] Derived: `EXTRACT(MONTH)` |
| `fiscal_quarter` | `superstore.stg_calendar` | `date_day` | 導出：4 月開始の会計四半期 [EN] Derived: fiscal quarter from April |
| `is_holiday` | `superstore.stg_calendar` | `date_day` | 導出：祝日マスタとの突合 [EN] Derived: matched against a holiday table |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `fact_order_line` | `date_key = ordered_at_date_key` | One-to-many |

## Notes / Caveats

- 会計四半期は 4 月開始です。暦年の四半期と取り違えると、集計が 1 四半期ずれます。 [EN] The fiscal quarter starts in April. Mistaking it for a calendar quarter shifts every total by one quarter.
- `is_holiday` は日本の祝日のみを対象としており、他国の休日は含みません。 [EN] `is_holiday` covers Japanese public holidays only, and no other country's.
