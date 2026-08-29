# film

## Overview

| Property | Value |
|---|---|
| **Table Name** | `film` |
| **Type** | Entity |
| **Domain** | Catalog |
| **Bounded Context** | Catalog |
| **Grain** | One row per film. |
| **Update Frequency** | daily |
| **Layer** | Operational Replica (3NF) |

1,000 films. It holds a `language_id` and nothing about the language itself, which is the difference between a foreign key and a denormalised copy.

## Columns

| Column | Type | Description |
|---|---|---|
| `film_id` | INT64 | Film identifier (PK) |
| `title` | STRING | Title |
| `description` | STRING | Synopsis |
| `release_year` | INT64 | Year of release |
| `language_id` | INT64 | Language the film is in (FK) |
| `rental_duration` | INT64 | Rental period in days |
| `rental_rate` | FLOAT64 | Rental price |
| `length` | INT64 | Runtime in minutes |
| `replacement_cost` | FLOAT64 | Cost to replace a lost copy |
| `rating` | STRING | MPAA rating |
| `last_update` | TIMESTAMP | When the row was last changed in the source |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `title` | `sakila.raw_film` | `title` |  |
| `description` | `sakila.raw_film` | `description` |  |
| `release_year` | `sakila.raw_film` | `release_year` |  |
| `rental_duration` | `sakila.raw_film` | `rental_duration` |  |
| `rental_rate` | `sakila.raw_film` | `rental_rate` |  |
| `length` | `sakila.raw_film` | `length` |  |
| `replacement_cost` | `sakila.raw_film` | `replacement_cost` |  |
| `rating` | `sakila.raw_film` | `rating` |  |
| `last_update` | `sakila.raw_film` | `last_update` |  |
| `film_id` | `sakila.raw_film` | `film_id` | Primary Key |
| `language_id` | `sakila.raw_film` | `language_id` | Foreign Key |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `language` | `language_id = language_id` | Many-to-one |
| `film_actor` | `film_id = film_id` | One-to-many |
| `inventory` | `film_id = film_id` | One-to-many |

## Notes / Caveats

- Sakila's `original_language_id` is deliberately not replicated: it is null on all 1,000 rows in the sample data, and a column that is always null documents nothing.
