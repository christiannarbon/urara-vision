# hub_nation

## Overview

| Property | Value |
|---|---|
| **Table Name** | `hub_nation` |
| **Type** | Hub (Conformed) |
| **Domain** | Shared Kernel |
| **Bounded Context** | Shared Kernel |
| **Business Key** | `n_nationkey` |
| **Grain** | One row per nation business key. |
| **Update Frequency** | daily |
| **Layer** | Raw Vault |

The conformed nation hub: one row per nation the business has ever seen, keyed on a hash of `n_nationkey`. Every context that needs a country should read this hub. Supply does not, which is the point of the copy sitting in `supply/`.

## Columns

| Column | Type | Description |
|---|---|---|
| `nation_hk` | BINARY | Hash of the nation business key (PK) |
| `n_nationkey` | INT64 | Nation business key, as it arrives from the source |
| `load_date` | TIMESTAMP | When this key was first seen |
| `record_source` | STRING | The staging model this key arrived from |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `nation_hk` | `tpch.v_stg_nation` | `n_nationkey` | Derived: `MD5` of the business key |
| `n_nationkey` | `tpch.v_stg_nation` | `n_nationkey` | Business key |
| `load_date` | `tpch.v_stg_nation` | `load_date` |  |
| `record_source` | `tpch.v_stg_nation` | `record_source` |  |

## Relationships

A hub declares its satellites and links from its own side as well, so the graph has both statements to collapse.

| Related Table | Join Key | Relationship |
|---|---|---|
| `sat_nation_details` | `nation_hk = nation_hk` | One-to-many |
| Every Satellite in the Raw Vault | `nation_hk` | One-to-many |

## Notes / Caveats

- The second row above names a group of tables in prose rather than a table. It is left that way on purpose: real vault documentation says exactly this, and the parser should record it as narrative rather than as a broken reference.
- `supply/hub_nation` is a copy of this table loaded from a different feed. This one is the authority.
