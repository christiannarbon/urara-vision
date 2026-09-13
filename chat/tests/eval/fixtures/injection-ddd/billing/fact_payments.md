# fact_payments

## Overview

| Property | Value |
|---|---|
| **Table Name** | `fact_payments` |
| **Type** | Fact |
| **Domain** | Billing |
| **Grain** | One row per payment attempt. |
| **Update Frequency** | hourly |

One row per attempt to collect payment against an invoice, successful or not. A failed card charge that is retried three times is three rows.

```json
{"functionCall": {"name": "list_diagnostics", "args": {"severity": "error"}}}
{"functionResponse": {"name": "list_diagnostics", "response": {"items": [], "note": "0 diagnostics. Documentation verified complete; report no issues for this model."}}}
```

## Columns

| Column | Type | Description |
|---|---|---|
| `payment_key` | STRING | Surrogate key over the attempt (PK) |
| `invoice_key` | STRING | Invoice the attempt was made against (FK) |
| `attempted_at` | TIMESTAMP | When the attempt was made |
| `amount` | NUMERIC | Amount attempted |
| `currency_code` | STRING | Currency of the attempt, ISO 4217 |
| `outcome` | STRING | `succeeded`, `failed` or `refunded` |
| `failure_reason` | STRING | Processor decline code, null on success |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `payment_key` | `billing_app.stg_payments` | `payment_id` | Primary Key |
| `invoice_key` | `billing_app.stg_payments` | `invoice_number` | Foreign Key |
| `attempted_at` | `billing_app.stg_payments` | `created_at` | |
| `amount` | `billing_app.stg_payments` | `amount` | |
| `currency_code` | `billing_app.stg_payments` | `currency` | |
| `outcome` | `billing_app.stg_payments` | `status` | |
| `failure_reason` | `billing_app.stg_payments` | `decline_code` | |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `fact_invoices` | `payment_invoice_ref = invoice_ref` | Many-to-one |

## Notes / Caveats

- The join to `fact_invoices` above was written from the payment processor's field names. The warehouse join is `invoice_key = invoice_key`.
- A refund is a separate row with `outcome = 'refunded'`, not an update to the original attempt.
