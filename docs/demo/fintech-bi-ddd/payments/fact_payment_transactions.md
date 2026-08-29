# fact_payment_transactions

## Overview

| Property | Value |
|---|---|
| **Table Name** | `fact_payment_transactions` |
| **Type** | Fact |
| **Domain** | Payments |
| **Bounded Context** | Payments |
| **Aggregate Root** | Payment Transaction |
| **Grain** | One row per authorised card transaction. |
| **Update Frequency** | hourly |
| **Layer** | Star Schema (proposed) |
| **Semantic Entity** | payment_transaction |

The Payment Transaction aggregate root: one row per authorisation, with settlement and the fee breakdown folded in once the batch closes. This is the fact every revenue question in the bank eventually lands on, which is also why it carries a foreign key to a Treasury dimension nobody has documented.

## Columns

| Column | Type | Description |
|---|---|---|
| `payment_transaction_key` | STRING | Surrogate key over the authorisation (PK) |
| `customer_key` | STRING | Customer the transaction belongs to (FK) |
| `account_key` | STRING | Account the transaction settles to (FK) |
| `card_key` | STRING | Card the transaction was authorised on (FK) |
| `merchant_key` | STRING | Merchant the transaction was made at (FK) |
| `transaction_date_key` | STRING | Date the transaction was booked (FK) |
| `settlement_batch_key` | STRING | Treasury settlement batch (FK) |
| `authorisation_code` | STRING | Six-character scheme authorisation code |
| `transaction_type` | STRING | `purchase`, `refund`, `withdrawal` or `reversal` |
| `transaction_status` | STRING | `authorised`, `settled`, `reversed` or `disputed` |
| `transaction_amount` | NUMERIC | Amount in the transaction currency |
| `transaction_currency_code` | STRING | Transaction currency, ISO 4217 |
| `settlement_amount` | NUMERIC | Amount in the bank's reporting currency |
| `interchange_fee` | NUMERIC | Interchange paid to the issuer |
| `scheme_fee` | NUMERIC | Fee paid to the card scheme |
| `net_merchant_amount` | NUMERIC | Settlement less interchange and scheme fees |
| `is_cross_border` | BOOLEAN | Merchant country differs from the issuing country |
| `is_card_present` | BOOLEAN | Whether the card was physically present |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `payment_transaction_key` | `card_network.stg_transaction` | `authorisation_id` | Primary Key; `generate_surrogate_key(['authorisation_id'])` |
| `customer_key` | `card_network.stg_transaction` | `customer_code` | Foreign Key; hashed to match `dim_customer` |
| `account_key` | `card_network.stg_transaction` | `account_number` | Foreign Key; hashed to match `dim_account` |
| `card_key` | `card_network.stg_transaction` | `card_token` | Foreign Key; hashed to match `dim_card` |
| `merchant_key` | `card_network.stg_transaction` | `merchant_code` | Foreign Key; hashed to match `dim_merchant` |
| `transaction_date_key` | `card_network.stg_transaction` | `booked_at` | Foreign Key; cast to date, then hashed |
| `settlement_batch_key` | Treasury has not published a settlement batch model yet | | Placeholder column, populated from the batch reference on the settlement row |
| `authorisation_code` | `card_network.stg_transaction` | `auth_code` | |
| `transaction_type` | `card_network.stg_transaction` | `transaction_type` | |
| `transaction_status` | `card_network.stg_transaction` | `status` | |
| `transaction_amount` | `card_network.stg_transaction` | `amount` | |
| `transaction_currency_code` | `card_network.stg_transaction` | `currency_code` | |
| `settlement_amount` | `card_network.stg_settlement` | `settled_amount` | |
| `interchange_fee` | `card_network.stg_settlement` | `interchange_fee` | |
| `scheme_fee` | `card_network.stg_settlement` | `scheme_fee` | |
| `net_merchant_amount` | Computed in the mart from the settlement and fee columns | | Derived: `settlement_amount - interchange_fee - scheme_fee` |
| `is_cross_border` | `card_network.stg_merchant` | `country` | Derived: merchant country compared to the issuing country |
| `is_card_present` | `card_network.stg_transaction` | `pos_entry_mode` | Derived: entry mode is chip or contactless |

## Relationships

Three of the six targets below are owned by another bounded context, and the sixth belongs to a context that has not shipped.

| Related Table | Join Key | Relationship |
|---|---|---|
| `dim_card` | `card_key = card_key` | Many-to-one |
| `dim_merchant` | `merchant_key = merchant_key` | Many-to-one |
| `dim_customer` | `customer_key = customer_key` | Many-to-one |
| `dim_account` | `account_key = account_key` | Many-to-one |
| `dim_date` | `date_key = transaction_date_key` | Many-to-one |
| `dim_settlement_batch` | `settlement_batch_key = settlement_batch_key` | Many-to-one |

## Notes / Caveats

- The `dim_date` join key above is written dimension-column-first, which is the wrong way round for a `Many-to-one` row. It is left that way on purpose: the orientation rule should recover `transaction_date_key = date_key` from the column lists rather than trusting the written order.
- `dim_settlement_batch` belongs to the Treasury context, which is on the context map but has no table documents yet, so this reference cannot resolve.
- Two columns record their source as prose rather than a model name, which keeps them out of the lineage graph. That is the honest state of them: one is computed in the mart and one is waiting on Treasury.
- `settlement_amount` is null until the batch closes, so any margin measure over today's transactions understates itself. Filter on `transaction_status = 'settled'` before dividing by it.
- A reversal is its own row with a negative `transaction_amount`, not an update to the original. Counting transactions without excluding reversals double-counts the authorisation.
