# sat_order_details

## Overview

| Property | Value |
|---|---|
| **Table Name** | `sat_order_details` |
| **Type** | Satellite |
| **Domain** | Raw Vault |
| **Bounded Context** | Raw Vault |
| **Layer** | Raw Vault |
| **Update Frequency** | hourly |
| **Parent Hub** | `hub_order` |
| **Grain** | One row per order per load date. |

The order's own attributes over time. `shipped_date` is null until the order ships and then is not, so most orders have two rows here and the second one is what `fact_orders.days_to_ship` is computed from.

## Columns

| Column | Type | Description |
|---|---|---|
| `order_hk` | BINARY | Hash of the order business key (FK) |
| `load_date` | TIMESTAMP | When this version arrived (PK, with order_hk) |
| `effective_from` | TIMESTAMP | When this version became current |
| `hashdiff` | BINARY | Hash of every descriptive column, for change detection |
| `order_date` | DATE | Date the order was placed |
| `required_date` | DATE | Date the customer asked for |
| `shipped_date` | DATE | Date the order shipped; null until it does |
| `freight` | FLOAT64 | Freight charged |
| `ship_city` | STRING | Ship-to city |
| `ship_country` | STRING | Ship-to country |
| `record_source` | STRING | The staging model this version arrived from |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `order_hk` | `northwind.stg_orders` | `order_id` | Derived: `MD5` of the business key |
| `load_date` | `northwind.stg_orders` | `load_date` |  |
| `effective_from` | `northwind.stg_orders` | `load_date` | Derived: the load date of the first row in this version |
| `hashdiff` | `northwind.stg_orders` | `order_date` | Derived: `MD5` over every descriptive column |
| `order_date` | `northwind.stg_orders` | `order_date` |  |
| `required_date` | `northwind.stg_orders` | `required_date` |  |
| `shipped_date` | `northwind.stg_orders` | `shipped_date` |  |
| `freight` | `northwind.stg_orders` | `freight` |  |
| `ship_city` | `northwind.stg_orders` | `ship_city` |  |
| `ship_country` | `northwind.stg_orders` | `ship_country` |  |
| `record_source` | `northwind.stg_orders` | `record_source` |  |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `hub_order` | `order_hk = order_hk` | Many-to-one |
