# dim_promotions

## Overview

| Property | Value |
|---|---|
| **Table Name** | `dim_promotions` |
| **Type** | Dimension |
| **Domain** | Sales |
| **Grain** | One row per promotion. |
| **Update Frequency** | Weekly |
| **Layer** | Star Schema (proposed) |

Promotions that have run.

## Columns

| Column | Type | Description |
|---|---|---|
| `promotion_id` | STRING | Promotion identifier (PK) |
| `promotion_name` | STRING | Name of the promotion |
