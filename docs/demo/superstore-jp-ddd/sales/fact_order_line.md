# fact_order_line

## Overview

| Property | Value |
|---|---|
| **Table Name** | `fact_order_line` |
| **Type** | Fact |
| **Domain** | 販売 |
| **Bounded Context** | Sales |
| **Aggregate Root** | Order |
| **Grain** | 注文 1 件の中の商品 1 点ごとに 1 行。 [EN] One row per product on an order. |
| **Update Frequency** | 毎日 [EN] Daily |
| **Layer** | Star Schema (proposed) |

店舗の粒度そのもの。1 件の注文に含まれる 1 商品を、実際の売上・数量・値引き・利益とともに記録します。利益はソース側で計算済みの値をそのまま取り込んでおり、ここでは再計算しません。 [EN] The grain of the store: one product on one order, with the sales, quantity, discount and profit as they were recorded. Profit arrives already calculated in the source and is not recomputed here.

## Columns

| Column | Type | Description |
|---|---|---|
| `order_line_id` | STRING | 注文明細識別子（主キー） [EN] Order line identifier (PK) |
| `order_id` | STRING | 注文番号。注文ヘッダは別テーブルに持ちません [EN] Order number; there is no separate order header table |
| `product_id` | STRING | 販売した商品（外部キー） [EN] Product sold (FK) |
| `customer_id` | STRING | 購入した顧客（外部キー） [EN] Customer who bought it (FK) |
| `ordered_at_date_key` | DATE | 注文日（外部キー） [EN] Date the order was placed (FK) |
| `ship_mode_id` | STRING | 配送方法（外部キー） [EN] Shipping method (FK) |
| `region_id` | STRING | 販売地域。地域ディメンションは未整備（外部キー） [EN] Sales region; the region dimension is not documented yet (FK) |
| `sales_amount` | FLOAT64 | 値引き後の売上金額 [EN] Sales amount, after discount |
| `quantity` | INT64 | 販売数量 [EN] Units sold |
| `discount` | FLOAT64 | 値引率（0〜1） [EN] Discount rate, 0 to 1 |
| `profit` | FLOAT64 | 明細の利益額 [EN] Profit on the line |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `order_line_id` | `superstore.stg_orders` | `row_id` | 主キー [EN] Primary Key |
| `order_id` | `superstore.stg_orders` | `order_id` | |
| `product_id` | `superstore.stg_orders` | `product_id` | 外部キー [EN] Foreign Key |
| `customer_id` | `superstore.stg_orders` | `customer_id` | 外部キー [EN] Foreign Key |
| `ordered_at_date_key` | `superstore.stg_orders` | `order_date` | 外部キー。日付にキャスト [EN] Foreign Key; cast to date |
| `ship_mode_id` | `superstore.stg_ship_modes` | `ship_mode_id` | 外部キー。名称で結合 [EN] Foreign Key; joined on the name |
| `region_id` | 地域マスタが未整備のため、ソースなし | | 常に NULL のプレースホルダ列 [EN] Placeholder column, always null |
| `sales_amount` | `superstore.stg_orders` | `sales` | |
| `quantity` | `superstore.stg_orders` | `quantity` | |
| `discount` | `superstore.stg_orders` | `discount` | |
| `profit` | `superstore.stg_orders` | `profit` | |

## Relationships

5 つの参照先のうち 3 つは他の境界づけられたコンテキストが所有しています。 [EN] Three of these five targets are owned by another bounded context.

| Related Table | Join Key | Relationship |
|---|---|---|
| `dim_ship_mode` | `ship_mode_id = ship_mode_id` | Many-to-one |
| `dim_product` | `product_id = product_id` | Many-to-one |
| `dim_customer` | `customer_id = customer_id` | Many-to-one |
| `dim_date` | `date_key = ordered_at_date_key` | Many-to-one |
| `dim_region` | `region_id = region_id` | Many-to-one |

## Notes / Caveats

- `dim_region` の文書はこのセットのどこにもありません。これは意図的で、本セット唯一の `unresolved_reference` エラーです。 [EN] `dim_region` has no document anywhere in this set. It is deliberate: this is the set's one `unresolved_reference` error.
- 注文ヘッダを持たないため、注文単位の集計は明細の合計で行います。同じ注文の明細が複数日に分かれることはありません。 [EN] There is no order header, so an order-level total is a sum over its lines. The lines of one order never span more than one day.
- `discount` は率であり金額ではありません。金額として合計すると意味のない数値になります。 [EN] `discount` is a rate, not an amount. Summing it as money produces a meaningless figure.
