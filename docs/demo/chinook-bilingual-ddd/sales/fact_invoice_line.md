# fact_invoice_line

## Overview

| Property | Value |
|---|---|
| **Table Name** | `fact_invoice_line` |
| **Type** | Fact |
| **Domain** | Sales |
| **Bounded Context** | Sales |
| **Aggregate Root** | Invoice |
| **Grain** | One row per line on an invoice. [JP] 請求書の明細行ごとに 1 行。 |
| **Update Frequency** | Hourly [JP] 毎時 |
| **Layer** | Star Schema (proposed) |

The grain of the store: one track, on one invoice, at the price it was actually charged. The list price lives in the catalogue and is not what this row records. [JP] ストアの粒度そのもの。1 つの請求書上の 1 曲を、実際に請求された価格で記録します。定価はカタログ側にあり、この行が記録するものではありません。

## Columns

| Column | Type | Description |
|---|---|---|
| `invoice_line_id` | STRING | Invoice line identifier (PK) [JP] 請求明細識別子（主キー） |
| `invoice_id` | STRING | Invoice the line belongs to (FK) [JP] 明細が属する請求書（外部キー） |
| `track_id` | STRING | Track sold (FK) [JP] 販売された楽曲（外部キー） |
| `customer_id` | STRING | Customer billed (FK) [JP] 請求先の顧客（外部キー） |
| `invoiced_at_date_key` | DATE | Date the invoice was raised (FK) [JP] 請求日（外部キー） |
| `promotion_id` | STRING | Promotion applied to the line, null when none (FK) [JP] 明細に適用されたプロモーション。無い場合は NULL（外部キー） |
| `unit_price` | FLOAT64 | Price charged for one unit [JP] 1 単位あたりの請求価格 |
| `quantity` | INT64 | Units sold on the line [JP] 明細の販売数量 |
| `line_total` | FLOAT64 | Unit price times quantity [JP] 単価 × 数量 |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `invoice_line_id` | `chinook.stg_invoice_items` | `invoice_line_id` | Primary Key [JP] 主キー |
| `invoice_id` | `chinook.stg_invoice_items` | `invoice_id` | Foreign Key [JP] 外部キー |
| `track_id` | `chinook.stg_invoice_items` | `track_id` | Foreign Key [JP] 外部キー |
| `customer_id` | `chinook.stg_invoices` | `customer_id` | Foreign Key; joined on `invoice_id` [JP] 外部キー。`invoice_id` で結合 |
| `invoiced_at_date_key` | `chinook.stg_invoices` | `invoice_date` | Foreign Key; cast to date [JP] 外部キー。日付にキャスト |
| `promotion_id` | Not in the source yet; the promotions feed has not shipped | | Placeholder column, always null [JP] 常に NULL のプレースホルダ列 |
| `unit_price` | `chinook.stg_invoice_items` | `unit_price` | |
| `quantity` | `chinook.stg_invoice_items` | `quantity` | |
| `line_total` | `chinook.stg_invoice_items` | `unit_price` | Derived: `unit_price * quantity` [JP] 導出：`unit_price * quantity` |

## Relationships

Four of these five targets are owned by another bounded context. [JP] 5 つの参照先のうち 4 つは他の境界づけられたコンテキストが所有しています。

| Related Table | Join Key | Relationship |
|---|---|---|
| `dim_invoice` | `invoice_id = invoice_id` | Many-to-one |
| `dim_track` | `track_id = track_id` | Many-to-one |
| `dim_customer` | `customer_id = customer_id` | Many-to-one |
| `dim_date` | `date_key = invoiced_at_date_key` | Many-to-one |
| `dim_promotion` | `promotion_id = promotion_id` | Many-to-one |

## Notes / Caveats

- `dim_promotion` has no document anywhere in this set. It is left in on purpose: this is the set's one `unresolved_reference` error, so `uraractl -strict` exits non-zero here as it does on every other set. [JP] `dim_promotion` はこのセットのどこにも文書がありません。これは意図的なもので、本セット唯一の `unresolved_reference` エラーです。他のセットと同様に `uraractl -strict` は非ゼロで終了します。
- The `dim_date` join key is written dimension-column-first on a Many-to-one row, which is the wrong way round. It is left that way so the orientation rule has something to recover. [JP] `dim_date` の結合キーは Many-to-one の行でディメンション列を先に書いており、順序が逆です。方向解決の規則に働く余地を残すため、そのままにしています。
- `line_total` is stored rather than computed at query time, so a rounding change in the source will not be reflected in rows already loaded. [JP] `line_total` はクエリ時計算ではなく保存値です。ソース側の丸め方が変わっても、既に取り込まれた行には反映されません。
