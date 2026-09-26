# dim_customers

## Overview

| Property | Value |
|---|---|
| **Table Name** | `dim_customers` |
| **Type** | Dimension |
| **Domain** | Sales |
| **Grain** | One row per customer. |
| **Update Frequency** | Daily |
| **Layer** | Star Schema (proposed) |

Customers who have placed an order.

## Columns

| Column | Type | Description |
|---|---|---|
| `customer_id` | STRING | Customer identifier (PK) |
| `customer_name` | STRING | Full name |
| `email` | STRING | Contact email |
