# dim_customer

## Overview

| Property | Value |
|---|---|
| **Table Name** | `dim_customer` |
| **Type** | Dimension (Conformed — primary authority) |
| **Domain** | Customer Identity |
| **Bounded Context** | Customer Identity |
| **Aggregate Root** | Customer |
| **Grain** | One row per customer. |
| **Update Frequency** | hourly |
| **Layer** | Star Schema (proposed) |
| **Semantic Entity** | customer |

The Customer aggregate root, and the conformed customer dimension for the model. `customer_key` is the surrogate hash of `customer_code`, so a customer keeps the same key when the core banking platform renumbers its own identifiers — which it has done twice. Every context that means "customer" means this row.

## Columns

| Column | Type | Description |
|---|---|---|
| `customer_key` | STRING | Surrogate key over `customer_code` (PK) |
| `customer_code` | STRING | Natural key from the core banking platform |
| `customer_name` | STRING | Legal name as held for KYC |
| `date_of_birth` | DATE | Date of birth, used for age-banded reporting |
| `country` | STRING | Country of residence, ISO 3166-1 alpha-2 |
| `region` | STRING | Sales region, from the country mapping table |
| `postal_code` | STRING | Postal code of the registered address |
| `email` | STRING | Primary contact email |
| `phone` | STRING | Primary contact number, E.164 |
| `kyc_status` | STRING | `verified`, `pending`, `expired` or `rejected` |
| `kyc_verified_date` | DATE | Date the current KYC decision was made |
| `risk_band` | STRING | `low`, `medium` or `high`, from onboarding scoring |
| `first_account_open_date` | DATE | Earliest account opening across the customer |
| `recent_activity_date` | DATE | Most recent transaction or repayment |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `customer_key` | `core_banking.stg_customer` | `customer_code` | Primary Key; `generate_surrogate_key(['customer_code'])` |
| `customer_code` | `core_banking.stg_customer` | `customer_code` | |
| `customer_name` | `core_banking.stg_customer` | `customer_name` | |
| `date_of_birth` | `core_banking.stg_customer` | `date_of_birth` | |
| `country` | `core_banking.stg_customer` | `country` | |
| `region` | `core_banking.stg_customer` | `region` | |
| `postal_code` | `core_banking.stg_customer` | `postal_code` | |
| `email` | `core_banking.stg_customer` | `email` | |
| `phone` | `core_banking.stg_customer` | `phone` | |
| `kyc_status` | `core_banking.stg_customer` | `kyc_status` | Slowly changing dimension type 1 |
| `kyc_verified_date` | `core_banking.stg_customer` | `kyc_verified_at` | Cast to date |
| `risk_band` | `core_banking.stg_customer` | `onboarding_risk_band` | |
| `first_account_open_date` | `core_banking.stg_account` | `opened_at` | Derived: `MIN(opened_at)` per customer, type 0 |
| `recent_activity_date` | `card_network.stg_transaction` | `booked_at` | Derived: `MAX(booked_at)` per customer, type 1 |

## Relationships

Account is an entity inside this aggregate, so the join below is internal to the context. Every other join to this table is declared from the other side, by the fact that borrows it.

| Related Table | Join Key | Relationship |
|---|---|---|
| `dim_account` | `customer_key = customer_key` | One-to-many |

## Notes / Caveats

- `risk_band` is the band assigned at onboarding and is never recomputed here. Risk & Compliance maintains its own current band, which is why that context ended up with a local copy of this dimension.
- `recent_activity_date` is a type 1 attribute overwritten in place, so a snapshot of this table cannot be used to reconstruct activity as at a past date. Use `fact_payment_transactions` for that.
