# actor

## Overview

| Property | Value |
|---|---|
| **Table Name** | `actor` |
| **Type** | Entity |
| **Domain** | Catalog |
| **Bounded Context** | Catalog |
| **Grain** | One row per actor. |
| **Update Frequency** | daily |
| **Layer** | Operational Replica (3NF) |

200 actors. It carries no film key at all -- it cannot, because an actor is in many films -- which is exactly the situation `film_actor` exists to resolve.

## Columns

| Column | Type | Description |
|---|---|---|
| `actor_id` | INT64 | Actor identifier (PK) |
| `first_name` | STRING | First name |
| `last_name` | STRING | Last name |
| `last_update` | TIMESTAMP | When the row was last changed in the source |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `actor_id` | `raw_actor` | `actor_id` | Primary Key |
| `first_name` | `sakila.raw_actor` | `first_name` |  |
| `last_name` | `sakila.raw_actor` | `last_name` |  |
| `last_update` | `sakila.raw_actor` | `last_update` |  |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `film_actor` | `actor_id = actor_id` | One-to-many |

## Notes / Caveats

- This document cites the same replicated table both ways: `raw_actor` unqualified on the first row, `sakila.raw_actor` on the rest. Both spellings mean one table and have to fold onto one lineage node, or asking what reads it would miss this document entirely.
- The inconsistency sits inside a single document here rather than across two, as it does in the other sets. That is what a one-to-one operational replica looks like: each table has exactly one source, so a model cited twice is usually cited twice in the same file.
