# inventory

## Overview

| Property | Value |
|---|---|
| **Table Name** | `inventory` |
| **Type** | Entity |
| **Domain** | Inventory |
| **Bounded Context** | Inventory |
| **Grain** | One row per physical copy of a film at a store. |
| **Update Frequency** | continuous |
| **Layer** | Operational Replica (3NF) |

4,581 discs. This is the table that makes the schema work: a rental cannot point at a film, because renting `film_id = 1` says nothing about which of its four copies left the shelf.

## Columns

| Column | Type | Description |
|---|---|---|
| `inventory_id` | INT64 | Copy identifier (PK) |
| `film_id` | INT64 | Which film this is a copy of (FK) |
| `store_id` | INT64 | Which store holds it (FK) |
| `last_update` | TIMESTAMP | When the row was last changed in the source |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `inventory_id` | `sakila.raw_inventory` | `inventory_id` | Primary Key |
| `film_id` | `sakila.raw_inventory` | `film_id` | Foreign Key |
| `store_id` | `sakila.raw_inventory` | `store_id` | Foreign Key |
| `last_update` | `sakila.raw_inventory` | `last_update` |  |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `film` | `film_id = film_id` | Many-to-one |
| `store` | `store_id = store_id` | Many-to-one |
| `rental` | `inventory_id = inventory_id` | One-to-many |
