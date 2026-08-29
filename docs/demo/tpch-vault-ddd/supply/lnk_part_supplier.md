# lnk_part_supplier

## Overview

| Property | Value |
|---|---|
| **Table Name** | `lnk_part_supplier` |
| **Type** | Link |
| **Domain** | Supply |
| **Bounded Context** | Supply |
| **Grain** | One row per unique part-and-supplier pair. |
| **Update Frequency** | daily |
| **Layer** | Raw Vault |

The part-to-supplier relationship from TPC-H's `partsupp`, with the availability and cost carried on the link itself. It was written before the part hub existed and still has nothing to point at.

## Columns

| Column | Type | Description |
|---|---|---|
| `part_supplier_hk` | BINARY | Hash of both parent hash keys together (PK) |
| `part_hk` | BINARY | The part side of the relationship (FK) |
| `supplier_hk` | BINARY | The supplier side of the relationship (FK) |
| `ps_availqty` | INT64 | Quantity the supplier has available |
| `ps_supplycost` | FLOAT64 | What the supplier charges for the part |
| `load_date` | TIMESTAMP | When this pairing was first seen |
| `record_source` | STRING | The staging model this pairing arrived from |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `part_supplier_hk` | `tpch.v_stg_partsupp` | `ps_partkey` | Derived: `MD5` of both business keys |
| `part_hk` | `tpch.v_stg_partsupp` | `ps_partkey` | Derived: `MD5` of the part business key |
| `supplier_hk` | `tpch.v_stg_partsupp` | `ps_suppkey` | Derived: `MD5` of the supplier business key |
| `ps_availqty` | `tpch.v_stg_partsupp` | `ps_availqty` |  |
| `ps_supplycost` | `tpch.v_stg_partsupp` | `ps_supplycost` |  |
| `load_date` | `tpch.v_stg_partsupp` | `load_date` |  |
| `record_source` | `tpch.v_stg_partsupp` | `record_source` |  |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| Part Hubs | `part_hk` | Many-to-one |

## Notes / Caveats

- This link names its parent in prose rather than naming a table, so nothing it declares resolves and it ends up joined to nothing. A link joined to nothing is the clearest documentation gap a vault can have, and it is here on purpose so that check has something to find.
- `ps_availqty` and `ps_supplycost` are measures on a link. Strict Data Vault would move them to a satellite; this vault did not, and the document says so rather than pretending otherwise.
