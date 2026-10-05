# Accounts

## Description
The Accounts context owns the customer account and the plan it subscribes to. An account is the legal entity that is invoiced; a plan is the price list it is billed against. Billing reads both across the context boundary rather than keeping copies.

## Proposed Star Schema

### Fact Table(s)

The Accounts context proposes no fact tables.

### Dimension Tables

1. **`dim_account`**
   The Account aggregate root.
   - **Grain**: One row per account.
   - **Columns**: `account_key`, `account_id`, `account_name`, `billing_email`, `country_code`, `plan_key`, `created_at`

2. **`dim_plan`**
   The plans an account can subscribe to.
   - **Grain**: One row per plan.
   - **Columns**: `plan_key`, `plan_code`, `plan_name`, `monthly_price`, `currency_code`, `billing_interval`, `is_active`

## Lineage

| Proposed Table | Source Model(s) |
| :--- | :--- |
| `dim_account` | `billing_app.stg_accounts` |
| `dim_plan` | `billing_app.stg_plans` |

The table list and lineage above are generated from the per-table documents in this directory. If they disagree with a child document, the child is authoritative.
