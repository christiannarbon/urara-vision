# DimProduct

## Overview

| Property | Value |
|---|---|
| **Table Name** | `DimProduct` |
| **Type** | Dimension |
| **Domain** | Product |
| **Bounded Context** | Product |
| **Grain** | One row per product per set of tracked attributes. |
| **Update Frequency** | daily |
| **Layer** | Snowflake Schema |

606 products. It carries no category name of its own: `ProductSubcategoryKey` points out at the first outrigger, and the category is one further join beyond that.

## Columns

| Column | Type | Description |
|---|---|---|
| `ProductKey` | INT64 | Surrogate key (PK) |
| `ProductAlternateKey` | STRING | Product number in the source |
| `ProductSubcategoryKey` | INT64 | Subcategory the product belongs to (FK) |
| `EnglishProductName` | STRING | Product name |
| `StandardCost` | FLOAT64 | Standard cost |
| `ListPrice` | FLOAT64 | List price |
| `Color` | STRING | Colour |
| `Size` | STRING | Size, where the product has one |
| `Weight` | FLOAT64 | Weight in kilograms |
| `ModelName` | STRING | Model the product belongs to |
| `Status` | STRING | Current or discontinued |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `ProductKey` | `adventureworks.stg_product` | `ProductID` | Primary Key |
| `ProductAlternateKey` | `adventureworks.stg_product` | `ProductNumber` |  |
| `ProductSubcategoryKey` | `adventureworks.stg_product` | `ProductSubcategoryID` | Foreign Key |
| `EnglishProductName` | `adventureworks.stg_product` | `Name` |  |
| `StandardCost` | `adventureworks.stg_product` | `StandardCost` |  |
| `ListPrice` | `adventureworks.stg_product` | `ListPrice` |  |
| `Color` | `adventureworks.stg_product` | `Color` |  |
| `Size` | `adventureworks.stg_product` | `Size` |  |
| `Weight` | `adventureworks.stg_product` | `Weight` |  |
| `ModelName` | `adventureworks.stg_product` | `ProductModelID` | Derived: resolved to the model's name |
| `Status` | `adventureworks.stg_product` | `SellEndDate` | Derived: current where the sell-end date is null |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `DimProductSubcategory` | `ProductSubcategoryKey = ProductSubcategoryKey` | Many-to-one |
| `FactInternetSales` | `ProductKey = ProductKey` | One-to-many |

## Notes / Caveats

- Products with no subcategory -- AdventureWorks has 209 of them, all components -- have a null `ProductSubcategoryKey` and drop out of any inner join up the chain. That is the other cost of a snowflake, and it is why the category rollup on a report has to be an outer join.
