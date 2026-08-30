# dim_invoice

## Overview

| Property | Value |
|---|---|
| **Table Name** | `dim_invoice` |
| **Type** | Dimension |
| **Domain** | Sales |
| **Bounded Context** | Sales |
| **Grain** | One row per invoice. [JP] 請求書ごとに 1 行。 |
| **Update Frequency** | Hourly [JP] 毎時 |
| **Layer** | Star Schema (proposed) |

The invoice header, kept as a dimension of its own so that the billing address can be read without reaching through a line. [JP] 請求書ヘッダ。明細を経由せずに請求先住所を参照できるよう、独立したディメンションとして保持します。

## Columns

| Column | Type | Description |
|---|---|---|
| `invoice_id` | STRING | Invoice identifier (PK) [JP] 請求書識別子（主キー） |
| `customer_id` | STRING | Customer billed (FK) [JP] 請求先の顧客（外部キー） |
| `billing_country` | STRING | Country on the invoice [JP] 請求書上の国 |
| `billing_city` | STRING | City on the invoice [JP] 請求書上の市区町村 |
| `invoice_total` | FLOAT64 | Sum of the invoice's lines [JP] 明細行の合計 |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `invoice_id` | `chinook.stg_invoices` | `invoice_id` | Primary Key [JP] 主キー |
| `customer_id` | `chinook.stg_invoices` | `customer_id` | Foreign Key [JP] 外部キー |
| `billing_country` | `chinook.stg_invoices` | `billing_country` | |
| `billing_city` | `chinook.stg_invoices` | `billing_city` | |
| `invoice_total` | `chinook.stg_invoices` | `total` | |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `dim_customer` | `customer_id = customer_id` | Many-to-one |

## Notes / Caveats

- The billing address is the one on the invoice at the time it was raised, not the customer's current address. [JP] 請求先住所は発行時点のものであり、顧客の現住所ではありません。
- `invoice_total` is the source's own total and is not recomputed from the lines, so a line loaded late will disagree with it until the next full load. [JP] `invoice_total` はソース側の合計値であり、明細から再計算していません。遅れて取り込まれた明細があると、次の全件ロードまで数値が一致しません。
