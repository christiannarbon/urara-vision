# DimCustomer

## Overview

| Property | Value |
|---|---|
| **Table Name** | `DimCustomer` |
| **Type** | Dimension |
| **Domain** | Customer |
| **Bounded Context** | Customer |
| **Grain** | One row per customer. |
| **Update Frequency** | daily |
| **Layer** | Snowflake Schema |

18,484 individual customers. Its address is not on it: `GeographyKey` points at the shared geography outrigger, which the reseller dimension uses too.

## Columns

| Column | Type | Description |
|---|---|---|
| `CustomerKey` | INT64 | Surrogate key (PK) |
| `GeographyKey` | INT64 | Where the customer lives (FK) |
| `CustomerAlternateKey` | STRING | Customer account number in the source |
| `FirstName` | STRING | First name |
| `LastName` | STRING | Last name |
| `BirthDate` | DATE | Date of birth |
| `MaritalStatus` | STRING | S or M |
| `Gender` | STRING | M or F |
| `YearlyIncome` | FLOAT64 | Banded yearly income |
| `EnglishEducation` | STRING | Highest education level |
| `EnglishOccupation` | STRING | Occupation category |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `CustomerKey` | `adventureworks.stg_customer` | `CustomerID` | Primary Key |
| `GeographyKey` | `adventureworks.stg_customer` | `AddressID` | Foreign Key; resolved through the address |
| `CustomerAlternateKey` | `adventureworks.stg_customer` | `AccountNumber` |  |
| `FirstName` | `adventureworks.stg_person` | `FirstName` |  |
| `LastName` | `adventureworks.stg_person` | `LastName` |  |
| `BirthDate` | `adventureworks.stg_person` | `BusinessEntityID` | Derived: joined in from the demographics XML |
| `MaritalStatus` | `adventureworks.stg_person` | `BusinessEntityID` | Derived: joined in from the demographics XML |
| `Gender` | `adventureworks.stg_person` | `BusinessEntityID` | Derived: joined in from the demographics XML |
| `YearlyIncome` | `adventureworks.stg_person` | `BusinessEntityID` | Derived: banded from the demographics XML |
| `EnglishEducation` | `adventureworks.stg_person` | `BusinessEntityID` | Derived: joined in from the demographics XML |
| `EnglishOccupation` | `adventureworks.stg_person` | `BusinessEntityID` | Derived: joined in from the demographics XML |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `DimGeography` | `GeographyKey = GeographyKey` | Many-to-one |
| `FactInternetSales` | `CustomerKey = CustomerKey` | One-to-many |
