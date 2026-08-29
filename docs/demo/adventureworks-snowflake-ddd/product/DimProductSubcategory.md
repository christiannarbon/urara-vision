# DimProductSubcategory

## Overview

| Property | Value |
|---|---|
| **Table Name** | `DimProductSubcategory` |
| **Type** | Outrigger |
| **Domain** | Product |
| **Bounded Context** | Product |
| **Grain** | One row per product subcategory. |
| **Update Frequency** | daily |
| **Layer** | Snowflake Schema |

37 subcategories. The middle link of the chain: it hangs off `DimProduct` and itself points at `DimProductCategory`.

## Columns

| Column | Type | Description |
|---|---|---|
| `ProductSubcategoryKey` | INT64 | Surrogate key (PK) |
| `ProductSubcategoryAlternateKey` | INT64 | Subcategory ID in the source |
| `EnglishProductSubcategoryName` | STRING | Subcategory name |
| `ProductCategoryKey` | INT64 | Category the subcategory belongs to (FK) |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `ProductSubcategoryKey` | `adventureworks.stg_productsubcategory` | `ProductSubcategoryID` | Primary Key |
| `ProductSubcategoryAlternateKey` | `adventureworks.stg_productsubcategory` | `ProductSubcategoryID` |  |
| `EnglishProductSubcategoryName` | `adventureworks.stg_productsubcategory` | `Name` |  |
| `ProductCategoryKey` | `adventureworks.stg_productsubcategory` | `ProductCategoryID` | Foreign Key |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `DimProduct` | `ProductSubcategoryKey = ProductSubcategoryKey` | One-to-many |
| `DimProductCategory` | `ProductCategoryKey = ProductCategoryKey` | Many-to-one |

## Notes / Caveats

- An outrigger that is itself joined to another outrigger. Two levels is where a snowflake stops being a stylistic choice and starts changing how queries are written.
