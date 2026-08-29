# city

## Overview

| Property | Value |
|---|---|
| **Table Name** | `city` |
| **Type** | Reference |
| **Domain** | Shared Kernel |
| **Bounded Context** | Shared Kernel |
| **Grain** | One row per city. |
| **Update Frequency** | daily |
| **Layer** | Operational Replica (3NF) |

600 cities. The middle of the geography chain: an address points here, and this points at a country. Two joins to get from a customer to their country, which is the price 3NF charges for keeping each fact in one place.

## Columns

| Column | Type | Description |
|---|---|---|
| `city_id` | INT64 | City identifier (PK) |
| `city` | STRING | City name |
| `country_id` | INT64 | Country the city is in (FK) |
| `last_update` | TIMESTAMP | When the row was last changed in the source |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `city_id` | `sakila.raw_city` | `city_id` | Primary Key |
| `city` | `sakila.raw_city` | `city` |  |
| `country_id` | `sakila.raw_city` | `country_id` | Foreign Key |
| `last_update` | `sakila.raw_city` | `last_update` |  |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `country` | `country_id = country_id` | Many-to-one |
| `address` | `city_id = city_id` | One-to-many |
