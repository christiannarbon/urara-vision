# dim_loan

## Overview

| Property | Value |
|---|---|
| **Table Name** | `dim_loan` |
| **Type** | Dimension |
| **Domain** | Lending |
| **Bounded Context** | Lending |
| **Aggregate Root** | Loan |
| **Grain** | One row per loan. |
| **Update Frequency** | daily |
| **Layer** | Star Schema (proposed) |
| **Semantic Entity** | loan |

The Loan aggregate root, published as a dimension so an instalment can be measured against the terms it was written under. The terms here are the terms at origination and do not move when a loan is restructured; a restructure opens a new loan with a new reference, which is the bank's own convention and not a modelling shortcut.

## Columns

| Column | Type | Description |
|---|---|---|
| `loan_key` | STRING | Surrogate key over `loan_reference` (PK) |
| `loan_reference` | STRING | Natural key from the lending platform |
| `customer_key` | STRING | Borrower (FK) |
| `origination_account_key` | STRING | Account the loan was disbursed to (FK) |
| `product_type` | STRING | `personal`, `auto`, `mortgage` or `revolving` |
| `origination_date` | DATE | Date the loan was advanced |
| `maturity_date` | DATE | Date of the final scheduled instalment |
| `principal_amount` | NUMERIC | Amount advanced, in the account currency |
| `interest_rate_apr` | NUMERIC | Annual percentage rate at origination |
| `term_months` | INT64 | Scheduled term in months |
| `loan_status` | STRING | `active`, `settled`, `defaulted` or `written_off` |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `loan_key` | `core_banking.stg_loan` | `loan_reference` | Primary Key; `generate_surrogate_key(['loan_reference'])` |
| `loan_reference` | `core_banking.stg_loan` | `loan_reference` | |
| `customer_key` | `core_banking.stg_loan` | `customer_code` | Foreign Key; hashed to match `dim_customer` |
| `origination_account_key` | `core_banking.stg_loan` | `disbursement_account_number` | Foreign Key; hashed to match `dim_account` |
| `product_type` | `core_banking.stg_loan` | `product_type` | |
| `origination_date` | `core_banking.stg_loan` | `advanced_at` | Cast to date |
| `maturity_date` | `core_banking.stg_loan` | `maturity_at` | Cast to date |
| `principal_amount` | `core_banking.stg_loan` | `principal_amount` | |
| `interest_rate_apr` | `core_banking.stg_loan` | `apr` | |
| `term_months` | `core_banking.stg_loan` | `term_months` | |
| `loan_status` | `core_banking.stg_loan` | `loan_status` | Slowly changing dimension type 1 |

## Relationships

The account and the customer below are both owned by Customer Identity. The account join is also declared from that side, because the account is the thing being borrowed across the boundary.

| Related Table | Join Key | Relationship |
|---|---|---|
| `fact_loan_repayments` | `loan_key = loan_key` | One-to-many |
| `dim_account` | `origination_account_key = account_key` | Many-to-one |
| `dim_customer` | `customer_key = customer_key` | Many-to-one |

## Notes / Caveats

- A restructured loan is a new row with a new `loan_key`, so counting loans per customer counts restructures. There is no link between the old and new reference in this model, which is a real gap rather than a deliberate one.
- `loan_status` is type 1. A loan written off today reads as written off across its whole repayment history, which makes cohort arrears analysis over this dimension alone misleading — use `days_past_due` on the fact instead.
