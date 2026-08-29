# sat_customer_details

## Overview

| Property | Value |
|---|---|
| **Table Name** | `sat_customer_details` |
| **Type** | Satellite |
| **Domain** | Party |
| **Bounded Context** | Party |
| **Parent Hub** | `hub_customer` |
| **Grain** | One row per customer per load date. |
| **Update Frequency** | hourly |
| **Layer** | Raw Vault |

Everything TPC-H records about a customer, tracked over time. This is the table a presentation-layer dimension would be built from, and the table `pit_customer` indexes.

## Columns

| Column | Type | Description |
|---|---|---|
| `customer_hk` | BINARY | Hash of the customer business key (FK) |
| `load_date` | TIMESTAMP | When this version arrived (PK, with customer_hk) |
| `effective_from` | TIMESTAMP | When this version became current |
| `hashdiff` | BINARY | Hash of every descriptive column, for change detection |
| `c_name` | STRING | Customer name |
| `c_address` | STRING | Street address |
| `c_nationkey` | INT64 | Nation the customer is registered in |
| `c_phone` | STRING | Phone number |
| `c_acctbal` | FLOAT64 | Account balance at the time of load |
| `c_mktsegment` | STRING | Market segment |
| `c_comment` | STRING | Free-text comment from the source |
| `record_source` | STRING | The staging model this version arrived from |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `customer_hk` | `tpch.v_stg_customer` | `c_custkey` | Derived: `MD5` of the business key |
| `load_date` | `tpch.v_stg_customer` | `load_date` |  |
| `effective_from` | `tpch.v_stg_customer` | `load_date` | Derived: the load date of the first row in this version |
| `hashdiff` | `tpch.v_stg_customer` | `c_name` | Derived: `MD5` over every descriptive column |
| `c_name` | `tpch.v_stg_customer` | `c_name` |  |
| `c_address` | `tpch.v_stg_customer` | `c_address` |  |
| `c_nationkey` | `tpch.v_stg_customer` | `c_nationkey` |  |
| `c_phone` | `tpch.v_stg_customer` | `c_phone` |  |
| `c_acctbal` | `tpch.v_stg_customer` | `c_acctbal` |  |
| `c_mktsegment` | `tpch.v_stg_customer` | `c_mktsegment` |  |
| `c_comment` | Not carried through staging; the source comment is truncated upstream |  | Placeholder column, always null |
| `record_source` | `tpch.v_stg_customer` | `record_source` |  |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `hub_customer` | `customer_hk = customer_hk` | Many-to-one |

## Notes / Caveats

- `c_comment` records its source as prose because the column is not carried through the staging view. It is a real gap, left visible rather than tidied away.
- `c_nationkey` is an attribute here, not a link. See `shared_kernel/sat_nation_details` for why.
