# sat_customer_details

## Overview

| Property | Value |
|---|---|
| **Table Name** | `sat_customer_details` |
| **Type** | Satellite |
| **Domain** | Raw Vault |
| **Bounded Context** | Raw Vault |
| **Layer** | Raw Vault |
| **Update Frequency** | hourly |
| **Parent Hub** | `hub_customer` |
| **Grain** | One row per customer per load date. |

Everything Northwind records about a customer, with a row per change. `presentation_sales/dim_customer` is a flattening of this table down to its current row, which is the ordinary way a star gets built on top of a vault.

## Columns

| Column | Type | Description |
|---|---|---|
| `customer_hk` | BINARY | Hash of the customer business key (FK) |
| `load_date` | TIMESTAMP | When this version arrived (PK, with customer_hk) |
| `effective_from` | TIMESTAMP | When this version became current |
| `hashdiff` | BINARY | Hash of every descriptive column, for change detection |
| `company_name` | STRING | Company name |
| `contact_name` | STRING | Named contact |
| `contact_title` | STRING | The contact's job title |
| `city` | STRING | City |
| `country` | STRING | Country |
| `phone` | STRING | Phone number |
| `record_source` | STRING | The staging model this version arrived from |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `customer_hk` | `northwind.stg_customers` | `customer_id` | Derived: `MD5` of the business key |
| `load_date` | `northwind.stg_customers` | `load_date` |  |
| `effective_from` | `northwind.stg_customers` | `load_date` | Derived: the load date of the first row in this version |
| `hashdiff` | `northwind.stg_customers` | `company_name` | Derived: `MD5` over every descriptive column |
| `company_name` | `northwind.stg_customers` | `company_name` |  |
| `contact_name` | `northwind.stg_customers` | `contact_name` |  |
| `contact_title` | `northwind.stg_customers` | `contact_title` |  |
| `city` | `northwind.stg_customers` | `city` |  |
| `country` | `northwind.stg_customers` | `country` |  |
| `phone` | `northwind.stg_customers` | `phone` |  |
| `record_source` | `northwind.stg_customers` | `record_source` |  |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `hub_customer` | `customer_hk = customer_hk` | Many-to-one |
