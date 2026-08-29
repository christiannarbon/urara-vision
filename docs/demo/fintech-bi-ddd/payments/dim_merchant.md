# dim_merchant

## Overview

| Property | Value |
|---|---|
| **Table Name** | `dim_merchant` |
| **Type** | Dimension |
| **Domain** | Payments |
| **Bounded Context** | Payments |
| **Aggregate Root** | Payment Transaction |
| **Grain** | One row per merchant. |
| **Update Frequency** | daily |
| **Layer** | Star Schema (proposed) |
| **Semantic Entity** | merchant |

One row per merchant the bank acquires transactions from, with the category code the scheme assigns and the two activity dates the merchant success team asks for every week. The merchant category code is the attribute almost every Payments question turns on, and it is the one the schemes change without telling anybody.

## Columns

| Column | Type | Description |
|---|---|---|
| `merchant_key` | STRING | Surrogate key over `merchant_code` (PK) |
| `merchant_code` | STRING | Natural key from the acquiring platform |
| `merchant_name` | STRING | Trading name as it appears on a statement |
| `merchant_category_code` | STRING | Four-digit scheme MCC |
| `merchant_category_name` | STRING | Human-readable MCC description |
| `merchant_country` | STRING | Country of the merchant outlet, ISO 3166-1 alpha-2 |
| `acquirer_name` | STRING | Acquirer routing the transaction |
| `first_transaction_date` | DATE | Earliest transaction seen for the merchant |
| `recent_transaction_date` | DATE | Most recent transaction seen for the merchant |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `merchant_key` | `card_network.stg_merchant` | `merchant_code` | Primary Key; `generate_surrogate_key(['merchant_code'])` |
| `merchant_code` | `card_network.stg_merchant` | `merchant_code` | |
| `merchant_name` | `card_network.stg_merchant` | `trading_name` | |
| `merchant_category_code` | `card_network.stg_merchant` | `mcc` | |
| `merchant_category_name` | `card_network.stg_merchant` | `mcc_description` | |
| `merchant_country` | `card_network.stg_merchant` | `country` | |
| `acquirer_name` | `card_network.stg_merchant` | `acquirer_name` | |
| `first_transaction_date` | `stg_transaction` | `booked_at` | Derived: `MIN(booked_at)` per merchant, type 0 |
| `recent_transaction_date` | `stg_transaction` | `booked_at` | Derived: `MAX(booked_at)` per merchant, type 1 |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `fact_payment_transactions` | `merchant_key = merchant_key` | One-to-many |

## Notes / Caveats

- The two activity dates above cite a bare `stg_transaction` while `fact_payment_transactions` cites `card_network.stg_transaction`. They are the same dbt model written two ways, left inconsistent on purpose: without folding them onto one node, "what else reads the transaction feed?" quietly returns the wrong answer.
- `merchant_category_code` is the value the scheme held when the transaction was acquired, but this dimension is type 1, so recategorising a merchant retroactively moves its whole history into the new category.
