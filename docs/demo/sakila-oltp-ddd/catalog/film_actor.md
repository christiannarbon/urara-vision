# film_actor

## Overview

| Property | Value |
|---|---|
| **Table Name** | `film_actor` |
| **Type** | Associative |
| **Domain** | Catalog |
| **Bounded Context** | Catalog |
| **Grain** | One row per film-and-actor pair. |
| **Update Frequency** | daily |
| **Layer** | Operational Replica (3NF) |

5,462 rows, two foreign keys, and a timestamp. Nothing else -- there is no such thing as an attribute of the fact that an actor was in a film, so the table has none. A junction table with attributes is usually an entity nobody has named yet; this one genuinely is not.

## Columns

| Column | Type | Description |
|---|---|---|
| `actor_id` | INT64 | The actor (PK, with film_id; FK) |
| `film_id` | INT64 | The film (PK, with actor_id; FK) |
| `last_update` | TIMESTAMP | When the row was last changed in the source |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `actor_id` | `sakila.raw_film_actor` | `actor_id` | Foreign Key |
| `film_id` | `sakila.raw_film_actor` | `film_id` | Foreign Key |
| `last_update` | `sakila.raw_film_actor` | `last_update` |  |

## Relationships

Both sides, which is what makes this an associative table rather than an entity: it exists only to hold the pair.

| Related Table | Join Key | Relationship |
|---|---|---|
| `film` | `film_id = film_id` | Many-to-one |
| `actor` | `actor_id = actor_id` | Many-to-one |

## Notes / Caveats

- Its primary key is the two foreign keys together. There is no surrogate `film_actor_id`, and adding one would let the same pair be inserted twice.
