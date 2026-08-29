# fact_basket_events

## Overview

| Property | Value |
|---|---|
| **Table Name** | `fact_basket_events` |
| **Type** | Fact |
| **Domain** | Basket |
| **Bounded Context** | Basket |
| **Aggregate Root** | Customer Basket |
| **Grain** | One row per basket event. |
| **Update Frequency** | hourly |
| **Layer** | Star Schema (proposed) |
| **Service of Record** | Basket.API |

One row per event the basket service publishes: an item added, a quantity changed, an item removed, a basket checked out or deleted. Redis holds only the current basket, so this fact is the only record that a basket ever contained something a shopper then took out — which is the single most useful thing the Basket context has to say.

## Columns

| Column | Type | Description |
|---|---|---|
| `basket_event_key` | STRING | Surrogate key over the event (PK) |
| `buyer_key` | STRING | Buyer whose basket changed (FK) |
| `catalog_item_key` | STRING | Item the event concerns (FK) |
| `event_date_key` | DATE | Date of the event (FK) |
| `event_type` | STRING | `added`, `quantity_changed`, `removed`, `checked_out` or `deleted` |
| `quantity` | INT64 | Units on the line after the event |
| `unit_price` | NUMERIC | Price per unit at the time of the event |
| `old_unit_price` | NUMERIC | Previous price, where the catalog changed it |
| `basket_value_after` | NUMERIC | Value of the whole basket after the event |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `basket_event_key` | `basketdb.stg_basket_events` | `event_id` | Primary Key |
| `buyer_key` | `basketdb.stg_basket_events` | `buyer_id` | Foreign Key; hashed from the Redis key |
| `catalog_item_key` | `basketdb.stg_basket_events` | `product_id` | Foreign Key; matched against the catalog |
| `event_date_key` | `basketdb.stg_basket_events` | `event_at` | Foreign Key; cast to date |
| `event_type` | `basketdb.stg_basket_events` | `event_type` | |
| `quantity` | `basketdb.stg_basket_events` | `quantity` | |
| `unit_price` | `basketdb.stg_basket_events` | `unit_price` | |
| `old_unit_price` | `basketdb.stg_basket_events` | `old_unit_price` | Zero unless the catalog changed the price |
| `basket_value_after` | `basketdb.stg_basket_events` | `unit_price` | Derived: `SUM(quantity * unit_price)` over the basket |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `dim_buyer` | `buyer_key = buyer_key` | Many-to-one |
| `dim_catalog_item` | `product_id = id` | Many-to-one |
| `dim_date` | `event_date_key = date_key` | Many-to-one |

## Notes / Caveats

- The `dim_catalog_item` join key above names the application's own field names — `product_id` on the basket item and `id` on the catalog item — and neither table declares a column by either name. It is left in on purpose: this is what a join key looks like when it was written from the service code rather than from the warehouse model, and reading either document alone will not catch it.
- The `dim_buyer` join resolves to Basket's own local copy rather than the conformed one in Identity, because a table in the declaring context always wins. Identity separately declares the same join from its side, which is why this fact ends up joined to two different buyer dimensions.
- `old_unit_price` is eShop's price-change mechanism: `Basket.API` compares the catalog price against the stored one and surfaces the difference to the shopper. It is zero, not null, when nothing changed, so averaging it understates the size of a real change.
- There is no event for a basket abandoned rather than checked out. Abandonment has to be inferred from the absence of a later `checked_out`, which means the measure is only stable once enough time has passed.
