# dim_account

## Overview

| Property | Value |
|---|---|
| **Table Name** | `dim_account` |
| **Type** | Dimension |
| **Domain** | Customer Identity |
| **Bounded Context** | Customer Identity |
| **Aggregate Root** | Customer |
| **Grain** | One row per account. |
| **Update Frequency** | hourly |
| **Layer** | Star Schema (proposed) |
| **Semantic Entity** | account |

The Account entity inside the Customer aggregate: one row per account a customer holds, current-state only. Cards are issued against an account and loans are originated from one, so this table is where two other contexts reach into this one.

## Columns

| Column | Type | Description |
|---|---|---|
| `account_key` | STRING | Surrogate key over `account_number` (PK) |
| `account_number` | STRING | Natural key from the core banking platform |
| `customer_key` | STRING | Customer holding the account (FK) |
| `account_type` | STRING | `current`, `savings` or `credit` |
| `currency_code` | STRING | Account currency, ISO 4217 |
| `opened_date` | DATE | Date the account was opened |
| `closed_date` | DATE | Date the account was closed, null while open |
| `account_status` | STRING | `open`, `dormant`, `frozen` or `closed` |
| `overdraft_limit` | NUMERIC | Agreed overdraft, in the account currency |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `account_key` | `core_banking.stg_account` | `account_number` | Primary Key; `generate_surrogate_key(['account_number'])` |
| `account_number` | `core_banking.stg_account` | `account_number` | |
| `customer_key` | `core_banking.stg_account` | `customer_code` | Foreign Key; hashed to match `dim_customer` |
| `account_type` | `core_banking.stg_account` | `account_type` | |
| `currency_code` | `core_banking.stg_account` | `currency_code` | |
| `opened_date` | `core_banking.stg_account` | `opened_at` | Cast to date |
| `closed_date` | `core_banking.stg_account` | `closed_at` | Cast to date |
| `account_status` | `core_banking.stg_account` | `account_status` | Slowly changing dimension type 1 |
| `overdraft_limit` | `core_banking.stg_account` | `overdraft_limit` | |

## Relationships

Two of the joins below leave this context: a card is issued by Payments against an account, and a loan is originated by Lending from one. Both are declared from this side as well as the other, because the account is the thing being borrowed.

| Related Table | Join Key | Relationship |
|---|---|---|
| `dim_customer` | `customer_key = customer_key` | Many-to-one |
| `dim_card` | `account_key = account_key` | One-to-many |
| `dim_loan` | `origination_account_key = account_key` | One-to-many |

## Notes / Caveats

- The `dim_loan` join key above is written loan-column-first on a `One-to-many` row, which is the wrong way round for the declaring side. It is left that way on purpose: the orientation rule should recover `account_key = origination_account_key` from the column lists rather than trusting the written order.
- `overdraft_limit` is held in the account currency and is not converted, so summing it across a multi-currency portfolio is meaningless without joining a rate table this model does not yet document.
- A closed account keeps its row. Filtering to live accounts means `account_status = 'open'`, not `closed_date IS NULL` — a frozen account has neither.
