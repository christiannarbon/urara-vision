# hub_supplier

## Overview

| Property | Value |
|---|---|
| **Table Name** | `hub_supplier` |
| **Type** | Hub |
| **Domain** | Supply |
| **Bounded Context** | Supply |
| **Business Key** | `s_suppkey` |
| **Grain** | One row per supplier business key. |
| **Update Frequency** | daily |
| **Layer** | Raw Vault |

The supplier business key. Supply owns no supplier satellite yet: the descriptive feed arrives weekly and on a different schedule to the key feed, so it is being landed separately.

## Columns

| Column | Type | Description |
|---|---|---|
| `supplier_hk` | BINARY | Hash of the supplier business key (PK) |
| `s_suppkey` | INT64 | Supplier business key, as it arrives from the source |
| `load_date` | TIMESTAMP | When this key was first seen |
| `record_source` | STRING | The staging model this key arrived from |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `supplier_hk` | `tpch.v_stg_supplier` | `s_suppkey` | Derived: `MD5` of the business key |
| `s_suppkey` | `tpch.v_stg_supplier` | `s_suppkey` | Business key |
| `load_date` | `tpch.v_stg_supplier` | `load_date` |  |
| `record_source` | `tpch.v_stg_supplier` | `record_source` |  |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|

## Notes / Caveats

- This hub has neither a satellite nor a resolvable link yet. That is not a documentation gap -- a hub is a list of keys and is useful on its own -- so no check fires on it, which is the difference between a hub and the link two documents below.
