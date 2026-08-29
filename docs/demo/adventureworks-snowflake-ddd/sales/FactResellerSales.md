# FactResellerSales

## Overview

| Property | Value |
|---|---|
| **Table Name** | `FactResellerSales` |
| **Type** | Fact |
| **Domain** | Sales |
| **Bounded Context** | Sales |
| **Grain** | One row per sales order line. |
| **Update Frequency** | hourly |
| **Layer** | Snowflake Schema |

Sales through resellers, at the same line grain as the internet fact but with two dimensions the internet channel does not have: the reseller and the employee who sold to them.

## Columns

| Column | Type | Description |
|---|---|---|
| `SalesOrderNumber` | STRING | Order number (PK, with the line number) |
| `SalesOrderLineNumber` | INT64 | Line number within the order (PK) |
| `ProductKey` | INT64 | Product sold (FK) |
| `ResellerKey` | INT64 | Reseller who bought (FK) |
| `EmployeeKey` | INT64 | Salesperson credited with the sale (FK) |
| `OrderDateKey` | INT64 | Date the order was placed (FK) |
| `SalesTerritoryKey` | INT64 | Territory the sale is credited to (FK) |
| `OrderQuantity` | INT64 | Units on the line |
| `UnitPrice` | FLOAT64 | Price per unit |
| `SalesAmount` | FLOAT64 | Line revenue |
| `TotalProductCost` | FLOAT64 | Line cost |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `SalesOrderNumber` | `stg_salesorderheader` | `SalesOrderNumber` | Primary Key |
| `SalesOrderLineNumber` | `adventureworks.stg_salesorderdetail` | `SalesOrderDetailID` | Primary Key |
| `ProductKey` | `adventureworks.stg_salesorderdetail` | `ProductID` | Foreign Key |
| `ResellerKey` | `stg_salesorderheader` | `CustomerID` | Foreign Key; resolved through the store |
| `EmployeeKey` | Not in the warehouse yet; the Human Resources context has not shipped |  | Placeholder column, always null |
| `OrderDateKey` | `stg_salesorderheader` | `OrderDate` | Foreign Key; cast to yyyymmdd |
| `SalesTerritoryKey` | `stg_salesorderheader` | `TerritoryID` | Foreign Key |
| `OrderQuantity` | `adventureworks.stg_salesorderdetail` | `OrderQty` |  |
| `UnitPrice` | `adventureworks.stg_salesorderdetail` | `UnitPrice` |  |
| `SalesAmount` | `adventureworks.stg_salesorderdetail` | `LineTotal` |  |
| `TotalProductCost` | `adventureworks.stg_product` | `StandardCost` | Derived: `StandardCost * OrderQty` at the time of sale |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `DimProduct` | `ProductKey = ProductKey` | Many-to-one |
| `DimReseller` | `ResellerKey = ResellerKey` | Many-to-one |
| `DimDate` | `OrderDateKey = DateKey` | Many-to-one |
| `DimSalesTerritory` | `SalesTerritoryKey = SalesTerritoryKey` | Many-to-one |
| `DimEmployee` | `EmployeeKey = EmployeeKey` | Many-to-one |

## Notes / Caveats

- `DimEmployee` belongs to the Human Resources context, which is on the context map but has no table documents yet, so this reference cannot resolve. It is the only error in the set.
- This document cites `stg_salesorderheader` unqualified where `FactInternetSales` writes `adventureworks.stg_salesorderheader`. Both spellings mean the same model and have to fold onto one lineage node.
- `EmployeeKey` records its source as prose for the same reason as the unresolved join above.
