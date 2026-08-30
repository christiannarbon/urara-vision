# dim_customer

## Overview

| Property | Value |
|---|---|
| **Table Name** | `dim_customer` |
| **Type** | Dimension |
| **Domain** | Customer |
| **Bounded Context** | Customer |
| **Grain** | One row per customer. [JP] 顧客ごとに 1 行。 |
| **Update Frequency** | Daily [JP] 毎日 |
| **Layer** | Star Schema (proposed) |

The buying account, with the country it bills from and the representative who looks after it. [JP] 購入アカウント。請求元の国と、担当するサポート担当者を伴います。

## Columns

| Column | Type | Description |
|---|---|---|
| `customer_id` | STRING | Customer identifier (PK) [JP] 顧客識別子（主キー） |
| `customer_name` | STRING | Account name, first and last joined [JP] 姓名を結合したアカウント名 |
| `country` | STRING | Country the account bills from [JP] 請求元の国 |
| `city` | STRING | City the account bills from [JP] 請求元の市区町村 |
| `email` | STRING | Contact address [JP] 連絡先メールアドレス |
| `support_rep_id` | STRING | Employee who looks after the account (FK) [JP] アカウントを担当する従業員（外部キー） |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `customer_id` | `chinook.stg_customers` | `customer_id` | Primary Key [JP] 主キー |
| `customer_name` | `chinook.stg_customers` | `first_name` | Derived: first and last name joined [JP] 導出：姓と名を結合 |
| `country` | `chinook.stg_customers` | `country` | |
| `city` | `chinook.stg_customers` | `city` | |
| `email` | `chinook.stg_customers` | `email` | |
| `support_rep_id` | `chinook.stg_customers` | `support_rep_id` | Foreign Key [JP] 外部キー |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `dim_employee` | `support_rep_id = employee_id` | Many-to-one |

## Notes / Caveats

- [JP] この表は緩やかに変化するディメンションではありません。顧客が国を変更すると、過去の請求書も新しい国で集計されます。
- An account with no support representative keeps a null rather than pointing at a placeholder employee. [JP] 担当者のいないアカウントは、ダミー従業員を指すのではなく NULL のままにします。
- The first note above opens with a Japanese tag and has nothing in English before it, which is this set's one `missing_primary_language` finding. English readers are shown the Japanese, which is the documented fallback. [JP] 上の 1 つ目の注記は日本語のタグで始まり、その前に英語がありません。これが本セット唯一の `missing_primary_language` の指摘です。英語の読者には日本語が表示されます。これは仕様どおりのフォールバックです。
