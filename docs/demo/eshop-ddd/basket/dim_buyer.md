# dim_buyer

## Overview

| Property | Value |
|---|---|
| **Table Name** | `dim_buyer` |
| **Type** | Dimension (local copy) |
| **Domain** | Basket |
| **Bounded Context** | Basket |
| **Aggregate Root** | Customer Basket |
| **Grain** | One row per buyer seen on a basket. |
| **Update Frequency** | hourly |
| **Layer** | Star Schema (proposed) |
| **Service of Record** | Basket.API |

A local copy of the conformed buyer dimension, built from the buyer ids that appear on basket events. Basket.API stores a `CustomerBasket` under a Redis key and knows nothing about the person behind it beyond that key, so this table carries four attributes where the authority carries eighteen. It was made because the basket fact needed something to join to and nobody wanted to wait for the cross-service reconciliation Identity does.

## Columns

| Column | Type | Description |
|---|---|---|
| `buyer_key` | STRING | Surrogate key over `identity_guid` (PK) |
| `buyer_id` | INT64 | Buyer id, where an order has since resolved one |
| `identity_guid` | STRING | Redis key on the basket, the subject claim |
| `user_name` | STRING | Login name, backfilled where available |
| `last_seen_at` | TIMESTAMP | Most recent basket event for the buyer |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `buyer_key` | `basketdb.stg_basket_events` | `buyer_id` | Primary Key; hashed from the Redis key |
| `buyer_id` | `basketdb.stg_basket_events` | `buyer_id` | Null until the buyer places an order |
| `identity_guid` | `basketdb.stg_basket_events` | `buyer_id` | The Redis key is the subject claim |
| `user_name` | Backfilled from an operations spreadsheet, not modelled in dbt | | Populated for roughly a third of rows |
| `last_seen_at` | Computed in the mart over the event stream | | Derived: `MAX(event_at)` per buyer |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `fact_basket_events` | `buyer_key = buyer_key` | One-to-many |

## Notes / Caveats

- This table is a deliberate duplicate of `identity/dim_buyer`. It is missing every attribute Identity assembles from the two services, and adds one Identity has never heard of. The two documents disagree, and neither says so — which is the point of comparing them.
- Identity declares a join from its own `dim_buyer` to `fact_basket_events`, and this table declares one too. Both are honoured, so the basket fact ends up with two edges to two different buyer tables. On the graph that shows as exactly what it is: two contexts each believing they own the buyer this fact is measured by.
- The two columns above with a prose source have no upstream model, so they cannot be traced past this table. `user_name` in particular is a spreadsheet, and it is stale.
- A buyer who has browsed and never ordered has a null `buyer_id`, which is correct and is not a broken row.
