# FactInternetSales

## Overview

| Property | Value |
|---|---|
| **Table Name** | `FactInternetSales` |
| **Type** | Fact |
| **Domain** | Sales |
| **Bounded Context** | Sales |
| **Grain** | One row per sales order line. |
| **Update Frequency** | hourly |
| **Layer** | Snowflake Schema |

Direct-to-customer sales at line grain. Every dimension it reaches is two or three joins deep on the other side: the product's category is two tables past `DimProduct`, and the customer's country is one table past `DimCustomer`. That depth is the cost of the snowflake, and it is what the Layered view is worth looking at this set in.

## Columns

| Column | Type | Description |
|---|---|---|
| `SalesOrderNumber` | STRING | Order number (PK, with the line number) |
| `SalesOrderLineNumber` | INT64 | Line number within the order (PK) |
| `ProductKey` | INT64 | Product sold (FK) |
| `CustomerKey` | INT64 | Customer who bought (FK) |
| `OrderDateKey` | INT64 | Date the order was placed (FK) |
| `SalesTerritoryKey` | INT64 | Territory the sale is credited to (FK) |
| `OrderQuantity` | INT64 | Units on the line |
| `UnitPrice` | FLOAT64 | Price per unit |
| `SalesAmount` | FLOAT64 | Line revenue |
| `TotalProductCost` | FLOAT64 | Line cost |
| `TaxAmt` | FLOAT64 | Tax on the line |
| `Freight` | FLOAT64 | Freight allocated to the line |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `SalesOrderNumber` | `adventureworks.stg_salesorderheader` | `SalesOrderNumber` | Primary Key |
| `SalesOrderLineNumber` | `adventureworks.stg_salesorderdetail` | `SalesOrderDetailID` | Primary Key |
| `ProductKey` | `adventureworks.stg_salesorderdetail` | `ProductID` | Foreign Key |
| `CustomerKey` | `adventureworks.stg_salesorderheader` | `CustomerID` | Foreign Key |
| `OrderDateKey` | `adventureworks.stg_salesorderheader` | `OrderDate` | Foreign Key; cast to yyyymmdd |
| `SalesTerritoryKey` | `adventureworks.stg_salesorderheader` | `TerritoryID` | Foreign Key |
| `OrderQuantity` | `adventureworks.stg_salesorderdetail` | `OrderQty` |  |
| `UnitPrice` | `adventureworks.stg_salesorderdetail` | `UnitPrice` |  |
| `SalesAmount` | `adventureworks.stg_salesorderdetail` | `LineTotal` |  |
| `TotalProductCost` | `adventureworks.stg_product` | `StandardCost` | Derived: `StandardCost * OrderQty` at the time of sale |
| `TaxAmt` | `adventureworks.stg_salesorderheader` | `TaxAmt` | Derived: allocated across the order's lines |
| `Freight` | `adventureworks.stg_salesorderheader` | `Freight` | Derived: allocated across the order's lines |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `DimProduct` | `ProductKey = ProductKey` | Many-to-one |
| `DimCustomer` | `CustomerKey = CustomerKey` | Many-to-one |
| `DimDate` | `DateKey = OrderDateKey` | Many-to-one |
| `DimSalesTerritory` | `SalesTerritoryKey = SalesTerritoryKey` | Many-to-one |

## Notes / Caveats

- The `DimDate` join key above is written dimension-column-first, which is the wrong way round for a `Many-to-one` row. It is left that way on purpose: the orientation rule should recover it from the column lists rather than trusting the written order.
- This document and `FactResellerSales` both cite `stg_salesorderheader` for the order-level columns. AdventureWorks stores both channels in one header table and separates them by whether `OnlineOrderFlag` is set.
