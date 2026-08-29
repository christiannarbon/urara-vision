# DimProductCategory

## Overview

| Property | Value |
|---|---|
| **Table Name** | `DimProductCategory` |
| **Type** | Outrigger |
| **Domain** | Product |
| **Bounded Context** | Product |
| **Grain** | One row per product category. |
| **Update Frequency** | daily |
| **Layer** | Snowflake Schema |

Four rows: Bikes, Components, Clothing, Accessories. The top of the product chain, and the clearest illustration of the trade -- four rows in a table of their own rather than four strings repeated six hundred times.

## Columns

| Column | Type | Description |
|---|---|---|
| `ProductCategoryKey` | INT64 | Surrogate key (PK) |
| `ProductCategoryAlternateKey` | INT64 | Category ID in the source |
| `EnglishProductCategoryName` | STRING | Category name |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `ProductCategoryKey` | `adventureworks.stg_productcategory` | `ProductCategoryID` | Primary Key |
| `ProductCategoryAlternateKey` | `adventureworks.stg_productcategory` | `ProductCategoryID` |  |
| `EnglishProductCategoryName` | `adventureworks.stg_productcategory` | `Name` |  |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `DimProductSubcategory` | `ProductCategoryKey = ProductCategoryKey` | One-to-many |
