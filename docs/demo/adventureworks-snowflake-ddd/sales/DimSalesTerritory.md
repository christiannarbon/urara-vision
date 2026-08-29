# DimSalesTerritory

## Overview

| Property | Value |
|---|---|
| **Table Name** | `DimSalesTerritory` |
| **Type** | Dimension |
| **Domain** | Sales |
| **Bounded Context** | Sales |
| **Grain** | One row per sales territory. |
| **Update Frequency** | daily |
| **Layer** | Snowflake Schema |

Ten territories grouped into three regions. It sits at the top of the geography chain: a city rolls up to a territory, so a question about sales by territory can be answered either from the fact's own `SalesTerritoryKey` or by walking `DimCustomer` to `DimGeography` to here -- and the two do not always agree, which is noted below.

## Columns

| Column | Type | Description |
|---|---|---|
| `SalesTerritoryKey` | INT64 | Territory key (PK) |
| `SalesTerritoryAlternateKey` | INT64 | Territory ID in the source |
| `SalesTerritoryRegion` | STRING | Region name |
| `SalesTerritoryCountry` | STRING | Country the territory sits in |
| `SalesTerritoryGroup` | STRING | Group: North America, Europe or Pacific |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `SalesTerritoryKey` | `adventureworks.stg_salesterritory` | `TerritoryID` | Primary Key |
| `SalesTerritoryAlternateKey` | `adventureworks.stg_salesterritory` | `TerritoryID` |  |
| `SalesTerritoryRegion` | `adventureworks.stg_salesterritory` | `Name` |  |
| `SalesTerritoryCountry` | `adventureworks.stg_salesterritory` | `CountryRegionCode` |  |
| `SalesTerritoryGroup` | `adventureworks.stg_salesterritory` | `Group` |  |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `DimGeography` | `SalesTerritoryKey = SalesTerritoryKey` | One-to-many |

## Notes / Caveats

- A sale's territory is the one it is credited to, which is not always the one its customer's city rolls up to -- a salesperson can be credited for a sale outside their own patch. Answering by territory therefore gives two different numbers depending on which path is walked, and both are correct answers to different questions.
