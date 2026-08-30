# dim_date

## Overview

| Property | Value |
|---|---|
| **Table Name** | `dim_date` |
| **Type** | Dimension |
| **Domain** | Shared Kernel (Conformed Dimensions) |
| **Bounded Context** | Shared Kernel |
| **Grain** | One row per day. [JP] 1 日ごとに 1 行。 |
| **Update Frequency** | Yearly [JP] 年次 |
| **Layer** | Star Schema (proposed) |

The calendar every context joins to. It is conformed on purpose: a second calendar is how two contexts start disagreeing about what a week is. [JP] すべてのコンテキストが結合する暦。意図的に統一（コンフォームド）しています。暦が二つあることが、週の定義について二つのコンテキストが食い違い始める原因です。 [JP] 一度作成した行は変更しません。

## Columns

| Column | Type | Description |
|---|---|---|
| `date_key` | DATE | The day itself (PK) [JP] 日付そのもの（主キー） |
| `calendar_year` | INT64 | Calendar year [JP] 暦年 |
| `calendar_month` | INT64 | Month of the year, 1-12 [JP] 月（1〜12） |
| `day_of_week` | STRING | Day name [JP] 曜日名 |
| `is_weekend` | BOOLEAN | Whether the day is a Saturday or a Sunday [JP] 土曜日または日曜日かどうか |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `date_key` | `chinook.stg_calendar` | `date_day` | Primary Key [JP] 主キー |
| `calendar_year` | `chinook.stg_calendar` | `date_day` | Derived: `EXTRACT(YEAR)` [JP] 導出：`EXTRACT(YEAR)` |
| `calendar_month` | `chinook.stg_calendar` | `date_day` | Derived: `EXTRACT(MONTH)` [JP] 導出：`EXTRACT(MONTH)` |
| `day_of_week` | `chinook.stg_calendar` | `date_day` | Derived: day name [JP] 導出：曜日名 |
| `is_weekend` | `chinook.stg_calendar` | `date_day` | Derived: Saturday or Sunday [JP] 導出：土曜または日曜 |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `fact_invoice_line` | `date_key = invoiced_at_date_key` | One-to-many |

## Notes / Caveats

- The description above tags Japanese twice, which is this set's one `duplicate_language_tag` warning. The two Japanese parts are joined in the order they appear rather than one being dropped. [JP] 上の説明は日本語のタグを 2 回使っており、これが本セット唯一の `duplicate_language_tag` 警告です。2 つの日本語部分は、どちらかを捨てるのではなく出現順に連結されます。
- `is_weekend` is Saturday and Sunday everywhere, with no regional calendar. [JP] `is_weekend` はどの地域でも土曜と日曜であり、地域別の休日暦は持ちません。
