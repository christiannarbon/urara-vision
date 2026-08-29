# address

## Overview

| Property | Value |
|---|---|
| **Table Name** | `address` |
| **Type** | Entity |
| **Domain** | Party |
| **Bounded Context** | Party |
| **Grain** | One row per address. |
| **Update Frequency** | continuous |
| **Layer** | Operational Replica (3NF) |

603 addresses, shared by customers and stores and eventually by staff. An entity rather than a set of columns precisely because three different things point at it.

## Columns

| Column | Type | Description |
|---|---|---|
| `address_id` | INT64 | Address identifier (PK) |
| `address` | STRING | First line |
| `address2` | STRING | Second line, usually null |
| `district` | STRING | District or state |
| `city_id` | INT64 | City the address is in (FK) |
| `postal_code` | STRING | Postal code |
| `phone` | STRING | Phone number |
| `last_update` | TIMESTAMP | When the row was last changed in the source |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `address_id` | `sakila.raw_address` | `address_id` | Primary Key |
| `address` | `sakila.raw_address` | `address` |  |
| `address2` | `sakila.raw_address` | `address2` |  |
| `district` | `sakila.raw_address` | `district` |  |
| `city_id` | `sakila.raw_address` | `city_id` | Foreign Key |
| `postal_code` | `sakila.raw_address` | `postal_code` |  |
| `phone` | `sakila.raw_address` | `phone` |  |
| `last_update` | `sakila.raw_address` | `last_update` |  |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `city` | `city = city_name` | Many-to-one |
| `customer` | `address_id = address_id` | One-to-many |

## Notes / Caveats

- The join to `city` names `city` and `city_name`, and while this table does have a `city_id`, neither of the columns actually written exists on either side -- somebody joined on the name instead of the key. The check should catch it, and joining geography on names rather than keys is how duplicate cities get created in the first place.
