# dim_customer

## Overview

| Property | Value |
|---|---|
| **Table Name** | `dim_customer` |
| **Type** | Dimension |
| **Domain** | 顧客 |
| **Bounded Context** | Customer |
| **Grain** | 顧客ごとに 1 行。 [EN] One row per customer. |
| **Update Frequency** | 毎日 [EN] Daily |
| **Layer** | Star Schema (proposed) |

購入者と、その所在地およびセグメント。セグメントは消費者・法人・ホームオフィスの 3 種類です。 [EN] The buyer, with their location and segment. There are three segments: consumer, corporate and home office.

## Columns

| Column | Type | Description |
|---|---|---|
| `customer_id` | STRING | 顧客識別子（主キー） [EN] Customer identifier (PK) |
| `customer_name` | STRING | 顧客名 [EN] Customer name |
| `segment` | STRING | 顧客セグメント [EN] Customer segment |
| `country` | STRING | 国 [EN] Country |
| `city` | STRING | 市区町村 [EN] City |
| `state` | STRING | 州・都道府県 [EN] State |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `customer_id` | `superstore.stg_customers` | `customer_id` | 主キー [EN] Primary Key |
| `customer_name` | `superstore.stg_customers` | `customer_name` | |
| `segment` | `superstore.stg_customers` | `segment` | |
| `country` | `superstore.stg_customers` | `country` | |
| `city` | `superstore.stg_customers` | `city` | |
| `state` | `superstore.stg_customers` | `state` | |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `fact_order_line` | `customer_id = customer_id` | One-to-many |

## Notes / Caveats

- 緩やかに変化するディメンションではありません。顧客が転居すると、過去の注文も新しい所在地で集計されます。 [EN] This is not a slowly changing dimension. When a customer moves, their past orders are counted at the new location.
