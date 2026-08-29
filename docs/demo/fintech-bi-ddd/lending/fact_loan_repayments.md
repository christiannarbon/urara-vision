# fact_loan_repayments

## Overview

| Property | Value |
|---|---|
| **Table Name** | `fact_loan_repayments` |
| **Type** | Fact |
| **Domain** | Lending |
| **Bounded Context** | Lending |
| **Aggregate Root** | Loan |
| **Grain** | One row per loan per scheduled instalment. |
| **Update Frequency** | daily |
| **Layer** | Star Schema (proposed) |
| **Semantic Entity** | loan_repayment |

One row per instalment on the amortisation schedule, present whether or not the instalment was paid. A missed payment is a row with `paid_amount = 0`, not a missing row, which is the whole reason arrears can be measured from this table at all.

## Columns

| Column | Type | Description |
|---|---|---|
| `loan_repayment_key` | STRING | Surrogate key over loan and instalment (PK) |
| `loan_key` | STRING | Loan being repaid (FK) |
| `customer_key` | STRING | Borrower (FK) |
| `repayment_date_key` | STRING | Date the instalment fell due (FK) |
| `instalment_number` | INT64 | Position in the amortisation schedule |
| `scheduled_amount` | NUMERIC | Amount due for the instalment |
| `paid_amount` | NUMERIC | Amount actually received, zero if missed |
| `principal_component` | NUMERIC | Portion of the payment reducing principal |
| `interest_component` | NUMERIC | Portion of the payment covering interest |
| `fees_component` | NUMERIC | Portion of the payment covering fees |
| `days_past_due` | INT64 | Days between the due date and payment |
| `is_late` | BOOLEAN | Whether the instalment was paid after its due date |
| `is_partial` | BOOLEAN | Whether less than the scheduled amount was received |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `loan_repayment_key` | `core_banking.stg_loan_repayment` | `loan_reference` | Primary Key; `generate_surrogate_key(['loan_reference','instalment_number'])` |
| `loan_key` | `core_banking.stg_loan_repayment` | `loan_reference` | Foreign Key; hashed to match `dim_loan` |
| `customer_key` | `core_banking.stg_loan` | `customer_code` | Foreign Key; hashed to match `dim_customer` |
| `repayment_date_key` | `core_banking.stg_loan_repayment` | `due_at` | Foreign Key; cast to date, then hashed |
| `instalment_number` | `core_banking.stg_loan_repayment` | `instalment_number` | |
| `scheduled_amount` | `core_banking.stg_loan_repayment` | `scheduled_amount` | |
| `paid_amount` | `core_banking.stg_loan_repayment` | `paid_amount` | |
| `principal_component` | `core_banking.stg_loan_repayment` | `principal_component` | |
| `interest_component` | `core_banking.stg_loan_repayment` | `interest_component` | |
| `fees_component` | `core_banking.stg_loan_repayment` | `fees_component` | |
| `days_past_due` | `core_banking.stg_loan_repayment` | `paid_at` | Derived: `DATEDIFF(day, due_at, paid_at)`, floored at zero |
| `is_late` | `core_banking.stg_loan_repayment` | `paid_at` | Derived: `days_past_due > 0` |
| `is_partial` | `core_banking.stg_loan_repayment` | `paid_amount` | Derived: `paid_amount < scheduled_amount` |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `dim_loan` | `loan_key = loan_key` | Many-to-one |
| `dim_customer` | `customer_key = customer_key` | Many-to-one |
| `dim_date` | `repayment_date = calendar_date` | Many-to-one |

## Notes / Caveats

- The `dim_date` join key above names the columns in business language — `repayment_date` and `calendar_date` — and neither table declares a column by either name. It is left in on purpose: this is what a join key looks like when it was written from the conversation rather than from the model, and nothing catches it by reading one document.
- The schedule runs to `maturity_date`, which for a mortgage is decades out. The conformed calendar only extends to the latest booked transaction, so future instalments have no `dim_date` row to join to even once the key is corrected.
- `days_past_due` is floored at zero, so an early payment reads as on time. Prepayment analysis needs the raw `paid_at`, which this fact does not carry.
