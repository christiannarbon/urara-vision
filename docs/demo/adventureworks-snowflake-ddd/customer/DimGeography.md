# DimGeography

## Overview

| Property | Value |
|---|---|
| **Table Name** | `DimGeography` |
| **Type** | Outrigger |
| **Domain** | Customer |
| **Bounded Context** | Customer |
| **Grain** | One row per city. |
| **Update Frequency** | daily |
| **Layer** | Snowflake Schema |

655 cities. The outrigger two dimensions share, and the one that makes normalising worth it here: `DimCustomer` and `DimReseller` both point at it, so a correction to a city's country is one row rather than two tables.

It also points upward, at `DimSalesTerritory`, which makes the customer chain three levels deep.

## Columns

| Column | Type | Description |
|---|---|---|
| `GeographyKey` | INT64 | Surrogate key (PK) |
| `City` | STRING | City name |
| `StateProvinceCode` | STRING | State or province code |
| `StateProvinceName` | STRING | State or province name |
| `CountryRegionCode` | STRING | ISO country code |
| `EnglishCountryRegionName` | STRING | Country name |
| `PostalCode` | STRING | Postal code |
| `SalesTerritoryKey` | INT64 | Territory the city rolls up to (FK) |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `GeographyKey` | `adventureworks.stg_address` | `AddressID` | Primary Key |
| `City` | `adventureworks.stg_address` | `City` |  |
| `StateProvinceCode` | `adventureworks.stg_address` | `StateProvinceID` | Derived: resolved to the code |
| `StateProvinceName` | `adventureworks.stg_address` | `StateProvinceID` | Derived: resolved to the name |
| `CountryRegionCode` | `adventureworks.stg_address` | `StateProvinceID` | Derived: resolved to the country |
| `EnglishCountryRegionName` | `adventureworks.stg_address` | `StateProvinceID` | Derived: resolved to the country name |
| `PostalCode` | `adventureworks.stg_address` | `PostalCode` |  |
| `SalesTerritoryKey` | `adventureworks.stg_address` | `StateProvinceID` | Foreign Key; resolved through the state |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `DimCustomer` | `GeographyKey = GeographyKey` | One-to-many |
| `DimReseller` | `GeographyKey = GeographyKey` | One-to-many |
| `DimSalesTerritory` | `SalesTerritoryKey = SalesTerritoryKey` | Many-to-one |

## Notes / Caveats

- This table is the reason to prefer a snowflake here rather than a flat star. It is shared by two dimensions in two different contexts, and a shared attribute with two copies is a disagreement waiting to happen.
