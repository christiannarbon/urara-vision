# dim_buyer

## Overview

| Property | Value |
|---|---|
| **Table Name** | `dim_buyer` |
| **Type** | Dimension (Conformed — primary authority) |
| **Domain** | Identity |
| **Bounded Context** | Identity |
| **Aggregate Root** | Buyer |
| **Grain** | One row per buyer. |
| **Update Frequency** | hourly |
| **Layer** | Star Schema (proposed) |
| **Service of Record** | Identity.API, Ordering.API |

The conformed buyer dimension, and the only table in this model assembled from two services. `identity_guid` is the join: `Identity.API` issues it as the subject claim, and `Ordering.API` stores it on the Buyer aggregate exactly so the two can be reconciled without either service reading the other's database. `buyer_key` is hashed from that GUID rather than from either service's integer id, because both services number their rows independently and neither numbering survives a reseed.

## Columns

| Column | Type | Description |
|---|---|---|
| `buyer_key` | STRING | Surrogate key over `identity_guid` (PK) |
| `buyer_id` | INT64 | Buyer id from the Ordering database |
| `identity_guid` | STRING | Subject claim issued by Identity.API |
| `user_name` | STRING | Login name |
| `name` | STRING | Given name |
| `last_name` | STRING | Family name |
| `email` | STRING | Contact email |
| `street` | STRING | Registered address, street |
| `city` | STRING | Registered address, city |
| `state` | STRING | Registered address, state |
| `country` | STRING | Registered address, country |
| `zip_code` | STRING | Registered address, postal code |
| `card_holder_name` | STRING | Name on the stored card |
| `card_type_id` | INT64 | Card type id, 1 = Amex, 2 = Visa, 3 = MasterCard |
| `card_type_name` | STRING | Card type name from the reference table |
| `card_expiration` | STRING | Card expiry as `MM/YY` |
| `first_order_date` | DATE | Date of the buyer's first order |
| `recent_order_date` | DATE | Date of the buyer's most recent order |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `buyer_key` | `orderingdb.stg_buyers` | `identity_guid` | Primary Key; hashed from the subject claim |
| `buyer_id` | `orderingdb.stg_buyers` | `id` | |
| `identity_guid` | `orderingdb.stg_buyers` | `identity_guid` | |
| `user_name` | `identitydb.stg_aspnet_users` | `user_name` | |
| `name` | `identitydb.stg_aspnet_users` | `name` | |
| `last_name` | `identitydb.stg_aspnet_users` | `last_name` | |
| `email` | `identitydb.stg_aspnet_users` | `email` | |
| `street` | `identitydb.stg_aspnet_users` | `street` | |
| `city` | `identitydb.stg_aspnet_users` | `city` | |
| `state` | `identitydb.stg_aspnet_users` | `state` | |
| `country` | `identitydb.stg_aspnet_users` | `country` | |
| `zip_code` | `identitydb.stg_aspnet_users` | `zip_code` | |
| `card_holder_name` | `orderingdb.stg_payment_methods` | `card_holder_name` | Most recently added payment method |
| `card_type_id` | `orderingdb.stg_payment_methods` | `card_type_id` | |
| `card_type_name` | `orderingdb.stg_card_types` | `name` | |
| `card_expiration` | `orderingdb.stg_payment_methods` | `expiration` | Formatted `MM/YY` |
| `first_order_date` | `orderingdb.stg_orders` | `order_date` | Derived: `MIN(order_date)` per buyer |
| `recent_order_date` | `orderingdb.stg_orders` | `order_date` | Derived: `MAX(order_date)` per buyer |

## Relationships

Both joins below leave this context. Ordering and Basket each own a fact that is measured per buyer, and neither of them owns the buyer.

| Related Table | Join Key | Relationship |
|---|---|---|
| `fact_orders` | `buyer_key = buyer_key` | One-to-many |
| `fact_basket_events` | `buyer_key = buyer_key` | One-to-many |

## Notes / Caveats

- The card number and security number that `ApplicationUser` and `PaymentMethod` both persist are deliberately not brought into the warehouse. They are dropped in staging, not masked here, so no downstream model can reconstruct them.
- A buyer exists in `Identity.API` from registration but is only created in `Ordering.API` when their first order is placed. Rows for buyers who have registered and never ordered therefore have a null `buyer_id` and no order dates — they are real buyers, not broken rows.
- `card_holder_name`, `card_type_id` and `card_expiration` describe the buyer's most recently added payment method, not the one used on any particular order. An order's payment method is a fact-level attribute and is not resolvable from this dimension.
- The `fact_basket_events` join declared above does not land where this document assumes. Basket keeps its own local `dim_buyer` and its fact joins that copy instead, so this declaration and Basket's produce two separate edges to two different tables. That is exactly the disagreement worth seeing on a graph.
