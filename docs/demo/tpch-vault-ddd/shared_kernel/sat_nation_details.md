# sat_nation_details

## Overview

| Property | Value |
|---|---|
| **Table Name** | `sat_nation_details` |
| **Type** | Satellite |
| **Domain** | Shared Kernel |
| **Bounded Context** | Shared Kernel |
| **Parent Hub** | `hub_nation` |
| **Grain** | One row per nation per load date. |
| **Update Frequency** | daily |
| **Layer** | Raw Vault |

Descriptive context for a nation. New rows arrive only when the hashdiff changes, so the table is a full history of everything TPC-H has said about a country.

## Columns

| Column | Type | Description |
|---|---|---|
| `nation_hk` | BINARY | Hash of the nation business key (FK) |
| `load_date` | TIMESTAMP | When this version arrived (PK, with nation_hk) |
| `effective_from` | TIMESTAMP | When this version became current |
| `hashdiff` | BINARY | Hash of every descriptive column, for change detection |
| `n_name` | STRING | Nation name |
| `n_regionkey` | INT64 | Region the nation belongs to |
| `n_comment` | STRING | Free-text comment from the source |
| `record_source` | STRING | The staging model this version arrived from |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `nation_hk` | `tpch.v_stg_nation` | `n_nationkey` | Derived: `MD5` of the business key |
| `load_date` | `tpch.v_stg_nation` | `load_date` |  |
| `effective_from` | `tpch.v_stg_nation` | `load_date` | Derived: the load date of the first row in this version |
| `hashdiff` | `tpch.v_stg_nation` | `n_name` | Derived: `MD5` over every descriptive column |
| `n_name` | `tpch.v_stg_nation` | `n_name` |  |
| `n_regionkey` | `tpch.v_stg_nation` | `n_regionkey` |  |
| `n_comment` | `tpch.v_stg_nation` | `n_comment` |  |
| `record_source` | `tpch.v_stg_nation` | `record_source` |  |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `hub_nation` | `nation_hk = nation_hk` | Many-to-one |

## Notes / Caveats

- `n_regionkey` is carried as an attribute rather than as a link to a region hub. TPC-H has five regions and they do not change, so a hub and a link would be two tables to maintain and nothing to learn from them.
