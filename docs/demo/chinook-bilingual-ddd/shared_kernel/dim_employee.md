# dim_employee

## Overview

| Property | Value |
|---|---|
| **Table Name** | `dim_employee` |
| **Type** | Dimension |
| **Domain** | Shared Kernel |
| **Bounded Context** | Shared Kernel |
| **Grain** | One row per employee. [JP] 従業員ごとに 1 行。 |
| **Update Frequency** | Daily [JP] 毎日 |
| **Layer** | Star Schema (proposed) |

The staff directory, flattened to one row per employee with the manager kept as a key. Customer reads a support representative out of it.

## Columns

| Column | Type | Description |
|---|---|---|
| `employee_id` | STRING | Employee identifier (PK) [JP] 従業員識別子（主キー） |
| `employee_name` | STRING | Name, first and last joined [JP] 姓名を結合した氏名 |
| `title` | STRING | Job title [JP] 役職 |
| `reports_to_id` | STRING | The employee's manager (FK) [JP] 上長（外部キー） |
| `hired_date_key` | DATE | Date hired (FK) [JP] 入社日（外部キー） |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `employee_id` | `chinook.stg_employees` | `employee_id` | Primary Key [JP] 主キー |
| `employee_name` | `chinook.stg_employees` | `first_name` | Derived: first and last name joined [JP] 導出：姓と名を結合 |
| `title` | `chinook.stg_employees` | `title` | |
| `reports_to_id` | `chinook.stg_employees` | `reports_to` | Foreign Key [JP] 外部キー |
| `hired_date_key` | `chinook.stg_employees` | `hire_date` | Foreign Key; cast to date [JP] 外部キー。日付にキャスト |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `dim_date` | `date_key = hired_date_key` | Many-to-one |

## Notes / Caveats

- The description of this table is written in English only, with no translation beside it. A field with no translation is shown as written to every reader, which is what an untranslated document looks like in a bilingual set. [JP] この表の説明は英語のみで書かれており、翻訳が併記されていません。翻訳のないフィールドはどの読者にもそのまま表示されます。これは二言語セットにおける未翻訳ドキュメントの見え方そのものです。
- The manager hierarchy is one level deep in Chinook, so `reports_to_id` never forms a chain worth walking. [JP] Chinook の上長階層は 1 階層のみで、`reports_to_id` が辿るべき連鎖を形成することはありません。
