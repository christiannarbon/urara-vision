# hub_nation

## Overview

| Property | Value |
|---|---|
| **Table Name** | `hub_nation` |
| **Type** | Hub |
| **Domain** | Supply |
| **Bounded Context** | Supply |
| **Business Key** | `n_nationkey` |
| **Grain** | One row per nation business key, as the supplier feed sees it. |
| **Update Frequency** | weekly |
| **Layer** | Raw Vault |

A local copy of the nation hub, loaded from the supplier feed rather than the reference feed. It should not exist: `shared_kernel/hub_nation` is the authority and this table has been drifting from it since the day it was created.

## Columns

| Column | Type | Description |
|---|---|---|
| `nation_hk` | BINARY | Hash of the nation business key (PK) |
| `n_nationkey` | INT64 | Nation business key, as the supplier feed sees it |
| `load_date` | TIMESTAMP | When this key was first seen by the supplier feed |
| `supplier_count` | INT64 | How many suppliers are registered in this nation |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `nation_hk` | `tpch.v_stg_supplier` | `s_nationkey` | Derived: `MD5` of the business key |
| `n_nationkey` | `tpch.v_stg_supplier` | `s_nationkey` | Business key |
| `load_date` | `tpch.v_stg_supplier` | `load_date` |  |
| `supplier_count` | Computed in the load job rather than read from a column |  | Derived: `COUNT(*)` per nation |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|

## Notes / Caveats

- This table is deprecated. It is kept in the set because a raw vault loaded from several feeds is exactly where a duplicated hub appears, and the drift check should have something to find.
- It is missing `record_source`, which every other hub carries, and it adds `supplier_count`, which is a measure and has no business being on a hub at all.
- `supplier_count` records its source as prose because it is computed in the load job.
