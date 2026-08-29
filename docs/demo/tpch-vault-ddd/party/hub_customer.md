# hub_customer

## Overview

| Property | Value |
|---|---|
| **Table Name** | `hub_customer` |
| **Type** | Hub |
| **Domain** | Party |
| **Bounded Context** | Party |
| **Business Key** | `c_custkey` |
| **Grain** | One row per customer business key. |
| **Update Frequency** | hourly |
| **Layer** | Raw Vault |

The customer business key. Nothing descriptive lives here by design: a hub is a list of keys and the date each was first seen, so that it can be loaded from several feeds at once without any of them contending over an attribute.

## Columns

| Column | Type | Description |
|---|---|---|
| `customer_hk` | BINARY | Hash of the customer business key (PK) |
| `c_custkey` | INT64 | Customer business key, as it arrives from the source |
| `load_date` | TIMESTAMP | When this key was first seen |
| `record_source` | STRING | The staging model this key arrived from |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `customer_hk` | `tpch.v_stg_customer` | `c_custkey` | Derived: `MD5` of the business key |
| `c_custkey` | `tpch.v_stg_customer` | `c_custkey` | Business key |
| `load_date` | `tpch.v_stg_customer` | `load_date` |  |
| `record_source` | `tpch.v_stg_customer` | `record_source` |  |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `sat_customer_details` | `customer_hk = customer_hk` | One-to-many |
| `lnk_customer_order` | `customer_hk = customer_hk` | One-to-many |

## Notes / Caveats

- The join to `lnk_customer_order` is declared here and again from the link's own document. Both statements describe one edge.
