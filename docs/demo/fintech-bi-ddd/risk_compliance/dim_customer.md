# dim_customer

## Overview

| Property | Value |
|---|---|
| **Table Name** | `dim_customer` |
| **Type** | Dimension (local copy) |
| **Domain** | Risk & Compliance |
| **Bounded Context** | Risk & Compliance |
| **Aggregate Root** | Customer |
| **Grain** | One row per customer. |
| **Update Frequency** | daily |
| **Layer** | Star Schema (proposed) |
| **Semantic Entity** | customer |

A local copy of the conformed customer dimension, made when screening state was needed and Customer Identity did not carry it. It has not been kept level with the authority since: attributes added over there were never added here, and the two screening columns added here were never proposed for the kernel. Reading either document alone, this looks like a reasonable table.

## Columns

| Column | Type | Description |
|---|---|---|
| `customer_key` | STRING | Surrogate key over `customer_code` (PK) |
| `customer_code` | STRING | Natural key from the core banking platform |
| `customer_name` | STRING | Legal name as held for KYC |
| `country` | STRING | Country of residence, ISO 3166-1 alpha-2 |
| `risk_band` | STRING | Current risk band, recomputed nightly |
| `sanctions_screening_status` | STRING | `clear`, `hit` or `pending` |
| `pep_flag` | BOOLEAN | Whether the customer is politically exposed |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `customer_key` | `core_banking.stg_customer` | `customer_code` | Primary Key; `generate_surrogate_key(['customer_code'])` |
| `customer_code` | `core_banking.stg_customer` | `customer_code` | |
| `customer_name` | `core_banking.stg_customer` | `customer_name` | |
| `country` | `core_banking.stg_customer` | `country` | |
| `risk_band` | `core_banking.stg_customer` | `onboarding_risk_band` | Overwritten nightly by the risk engine |
| `sanctions_screening_status` | Screening vendor extract, landed as a CSV and not modelled in dbt | | Refreshed daily by an operations job |
| `pep_flag` | Screening vendor extract, landed as a CSV and not modelled in dbt | | Refreshed daily by an operations job |

## Relationships

This copy declares no joins of its own. Everything in this context that needs a customer joins the conformed instance, which is precisely why nobody noticed the copy drifting.

## Notes / Caveats

- This table is a deliberate duplicate of `customer_identity/dim_customer`. It is missing every attribute Customer Identity has added since the copy was taken, and adds two the authority has never heard of. The two documents disagree, and neither says so — which is the point of comparing them.
- `risk_band` carries the same name as the authority's column but a different meaning: there it is the band assigned at onboarding, here it is the band recomputed last night. Joining the two tables and picking either column silently answers a different question.
- The two screening columns above are sourced from a vendor CSV rather than a dbt model, so they have no upstream lineage and cannot be traced past this table.
