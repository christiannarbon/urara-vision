# fact_fraud_alerts

## Overview

| Property | Value |
|---|---|
| **Table Name** | `fact_fraud_alerts` |
| **Type** | Fact |
| **Domain** | Risk & Compliance |
| **Bounded Context** | Risk & Compliance |
| **Aggregate Root** | Fraud Alert |
| **Grain** | One row per alert raised by the fraud engine. |
| **Update Frequency** | hourly |
| **Layer** | Star Schema (proposed) |
| **Semantic Entity** | fraud_alert |

One row per alert the fraud engine raises, with the outcome once an analyst has dispositioned it. The alert carries the keys of the customer and the transaction that triggered it, but this document was written before either of those dimensions was documented, so it names them in prose instead.

## Columns

| Column | Type | Description |
|---|---|---|
| `fraud_alert_key` | STRING | Surrogate key over the alert (PK) |
| `customer_key` | STRING | Customer the alert was raised against (FK) |
| `payment_transaction_key` | STRING | Transaction that triggered the alert (FK) |
| `alert_date_key` | STRING | Date the alert was raised (FK) |
| `alert_rule_code` | STRING | Rule that fired, e.g. `VELOCITY_04` |
| `alert_score` | NUMERIC | Engine score between 0 and 1 |
| `alert_status` | STRING | `open`, `cleared`, `confirmed` or `expired` |
| `is_confirmed_fraud` | BOOLEAN | Whether an analyst confirmed the alert |
| `loss_amount` | NUMERIC | Loss booked against the alert, zero if none |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `fraud_alert_key` | `fraud_platform.stg_fraud_alert` | `alert_id` | Primary Key; `generate_surrogate_key(['alert_id'])` |
| `customer_key` | `fraud_platform.stg_fraud_alert` | `customer_code` | Foreign Key; hashed to match `dim_customer` |
| `payment_transaction_key` | `fraud_platform.stg_fraud_alert` | `authorisation_id` | Foreign Key; hashed to match `fact_payment_transactions` |
| `alert_date_key` | `fraud_platform.stg_fraud_alert` | `raised_at` | Foreign Key; cast to date, then hashed |
| `alert_rule_code` | `fraud_platform.stg_fraud_alert` | `rule_code` | |
| `alert_score` | `fraud_platform.stg_fraud_alert` | `score` | |
| `alert_status` | `fraud_platform.stg_fraud_alert` | `disposition` | |
| `is_confirmed_fraud` | `fraud_platform.stg_fraud_alert` | `disposition` | Derived: `disposition = 'confirmed'` |
| `loss_amount` | `fraud_platform.stg_fraud_alert` | `booked_loss` | |

## Relationships

The alert is raised against a transaction and a customer, but this document names those in prose rather than pointing at their documents.

| Related Table | Join Key | Relationship |
|---|---|---|
| `Payments Context Facts` | `payment_transaction_key` | Many-to-one |

## Notes / Caveats

- The row above is prose where a table document belongs, and it is the only relationship this fact declares. Both flaws are deliberate and they compound: the reference is unusable, and because it is the only one, this fact resolves to nothing at all. A fact table joined to no dimension is almost always a documentation gap, and here it is exactly that.
- The columns are correct and the keys are real. What is missing is the declaration — `dim_customer`, `fact_payment_transactions` and `dim_date` are all documented in this model and none of them is named here.
- `loss_amount` is booked at disposition, so an alert still open carries zero rather than null. Averaging loss over all alerts understates it.
