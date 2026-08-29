# dim_card

## Overview

| Property | Value |
|---|---|
| **Table Name** | `dim_card` |
| **Type** | Dimension |
| **Domain** | Payments |
| **Bounded Context** | Payments |
| **Aggregate Root** | Payment Transaction |
| **Grain** | One row per issued card. |
| **Update Frequency** | daily |
| **Layer** | Star Schema (proposed) |
| **Semantic Entity** | card |

One row per card the bank has issued. The card is identified by the network token, never the PAN: the primary account number is dropped in staging and does not reach the warehouse at all, so nothing downstream can accidentally expose it. A card belongs to an account, which is how a transaction reaches the customer without the card table having to carry a customer key of its own.

## Columns

| Column | Type | Description |
|---|---|---|
| `card_key` | STRING | Surrogate key over `card_token` (PK) |
| `card_token` | STRING | Network token for the card |
| `account_key` | STRING | Account the card is issued against (FK) |
| `card_scheme` | STRING | `visa`, `mastercard` or `amex` |
| `card_product` | STRING | Product name, e.g. `debit_classic` |
| `issue_date` | DATE | Date the card was issued |
| `expiry_date` | DATE | Date the card expires |
| `card_status` | STRING | `active`, `blocked`, `expired` or `replaced` |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `card_key` | `card_network.stg_card` | `card_token` | Primary Key; `generate_surrogate_key(['card_token'])` |
| `card_token` | `card_network.stg_card` | `card_token` | |
| `account_key` | `card_network.stg_card` | `account_number` | Foreign Key; hashed to match `dim_account` |
| `card_scheme` | `card_network.stg_card` | `scheme` | |
| `card_product` | `card_network.stg_card` | `product_code` | |
| `issue_date` | `card_network.stg_card` | `issued_at` | Cast to date |
| `expiry_date` | `card_network.stg_card` | `expires_at` | Cast to date |
| `card_status` | `card_network.stg_card` | `card_status` | Slowly changing dimension type 1 |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `fact_payment_transactions` | `card_key = card_key` | One-to-many |
| `dim_account` | `account_key = account_key` | Many-to-one |

## Notes / Caveats

- A replaced card gets a new token and therefore a new `card_key`. Counting distinct cards per customer counts replacements, which is why card-level retention analysis has to roll up through `dim_account` first.
- `card_status` is type 1, so a transaction authorised before a block will read as being on a blocked card once the block lands.
