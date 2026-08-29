# dim_customer

## Overview

| Property | Value |
|---|---|
| **Table Name** | `dim_customer` |
| **Type** | Dimension |
| **Domain** | Presentation - Sales |
| **Bounded Context** | Presentation - Sales |
| **Grain** | One row per customer. |
| **Update Frequency** | hourly |
| **Layer** | Star Schema |

The current view of a customer: the customer satellite flattened to its latest row per hash key. Everything in this table came out of the vault, which is what the lineage below records — there is no path from here back to Northwind that does not go through `sat_customer_details`.

## Columns

| Column | Type | Description |
|---|---|---|
| `customer_key` | STRING | Surrogate key for the customer (PK) |
| `customer_hk` | BINARY | The raw vault's hash key for the same customer (FK) |
| `customer_id` | STRING | Northwind's CustomerID |
| `company_name` | STRING | Company name |
| `contact_name` | STRING | Named contact |
| `city` | STRING | City |
| `country` | STRING | Country |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `customer_key` | `northwind.sat_customer_details` | `customer_hk` | Derived: surrogate key from the hash key |
| `customer_hk` | `northwind.sat_customer_details` | `customer_hk` | Foreign Key |
| `customer_id` | `northwind.sat_customer_details` | `customer_hk` | Derived: resolved through the hub |
| `company_name` | `northwind.sat_customer_details` | `company_name` |  |
| `contact_name` | `northwind.sat_customer_details` | `contact_name` |  |
| `city` | `northwind.sat_customer_details` | `city` |  |
| `country` | `northwind.sat_customer_details` | `country` |  |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `hub_customer` | `customer_hk = customer_hk` | Many-to-one |

## Notes / Caveats

- A dimension joined to a hub is the drill-back path in the other direction: from a row on a report to the key it was built from.
