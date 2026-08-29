# DimReseller

## Overview

| Property | Value |
|---|---|
| **Table Name** | `DimReseller` |
| **Type** | Dimension |
| **Domain** | Reseller |
| **Bounded Context** | Reseller |
| **Grain** | One row per reseller. |
| **Update Frequency** | daily |
| **Layer** | Snowflake Schema |

701 resellers. It shares `DimGeography` with `DimCustomer`, which is the whole argument for having normalised geography out in the first place.

## Columns

| Column | Type | Description |
|---|---|---|
| `ResellerKey` | INT64 | Surrogate key (PK) |
| `GeographyKey` | INT64 | Where the reseller trades from (FK) |
| `ResellerAlternateKey` | STRING | Reseller account number in the source |
| `ResellerName` | STRING | Business name |
| `BusinessType` | STRING | Warehouse, Value Added Reseller or Specialty Bike Shop |
| `NumberEmployees` | INT64 | Headcount |
| `AnnualSales` | FLOAT64 | Reported annual sales |
| `YearOpened` | INT64 | Year the business opened |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `ResellerKey` | `adventureworks.stg_store` | `BusinessEntityID` | Primary Key |
| `GeographyKey` | `adventureworks.stg_store` | `BusinessEntityID` | Foreign Key; resolved through the address |
| `ResellerAlternateKey` | `adventureworks.stg_store` | `BusinessEntityID` |  |
| `ResellerName` | `adventureworks.stg_store` | `Name` |  |
| `BusinessType` | `adventureworks.stg_store` | `Demographics` | Derived: read from the demographics XML |
| `NumberEmployees` | `adventureworks.stg_store` | `Demographics` | Derived: read from the demographics XML |
| `AnnualSales` | `adventureworks.stg_store` | `Demographics` | Derived: read from the demographics XML |
| `YearOpened` | `adventureworks.stg_store` | `Demographics` | Derived: read from the demographics XML |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `DimGeography` | `GeographyID = GeoKey` | Many-to-one |
| `FactResellerSales` | `ResellerKey = ResellerKey` | One-to-many |

## Notes / Caveats

- The join to `DimGeography` names `GeographyID` and `GeoKey`, and neither column exists on either table -- somebody wrote the join from memory using the source schema's names. The check should catch it.
- Note the consequence: `DimGeography` declares the same join correctly from its own side, so the two declarations do not merge and the graph draws two edges here rather than one.
