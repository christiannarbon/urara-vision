# dim_ship_mode

## Overview

| Property | Value |
|---|---|
| **Table Name** | `dim_ship_mode` |
| **Type** | Dimension |
| **Domain** | 販売 |
| **Bounded Context** | Sales |
| **Grain** | 配送方法ごとに 1 行。 [EN] One row per shipping method. |
| **Update Frequency** | 年次 [EN] Yearly |
| **Layer** | Star Schema (proposed) |

配送方法は 4 種類しかなく、増えることもほとんどありません。それでも列挙値ではなくテーブルにしているのは、目標日数という属性を持たせるためです。 [EN] There are only four shipping methods and there will rarely be more. They are a table rather than an enum because of the one attribute beside the name: the number of days the method is meant to take.

## Columns

| Column | Type | Description |
|---|---|---|
| `ship_mode_id` | STRING | 配送方法識別子（主キー） [EN] Shipping method identifier (PK) |
| `ship_mode_name` | STRING | 画面表示上の配送方法名 [EN] Shipping method as displayed |
| `target_days` | INT64 | 目標日数 [EN] Days the method is meant to take |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `ship_mode_id` | `superstore.stg_ship_modes` | `ship_mode_id` | 主キー [EN] Primary Key |
| `ship_mode_name` | `superstore.stg_ship_modes` | `ship_mode` | |
| `target_days` | `superstore.stg_ship_modes` | `target_days` | |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `fact_order_line` | `ship_mode_id = ship_mode_id` | One-to-many |

## Notes / Caveats

- `target_days` は目標であり実績ではありません。実際の配送日数は注文明細から計算します。 [EN] `target_days` is a target rather than an outcome. Actual shipping time is calculated from the order lines.
