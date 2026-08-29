# Ordering

## Description
The Ordering context owns the transaction: what was ordered, from which store, by whom, and what it came to. Its aggregate root is Order, and `fact_order_items` is the line-item entity inside that aggregate rather than a fact in its own right — nothing outside this context should reach an order item except through its order. Every other dimension this context reads is borrowed across a boundary: Customer from Customer Identity, Location from Store Operations, Product from Product Catalog, and the calendar from the Shared Kernel.

## Proposed Star Schema

### Fact Table(s)

1. **`fact_orders`**
   The Order aggregate root. One row per order placed.
   - **Grain**: One row per order.
   - **Columns**: `order_id`, `customer_id`, `location_id`, `ordered_at_date_key`, `delivery_partner_id`, `subtotal`, `tax_paid`, `order_total`, `order_cost`, `count_food_items`, `count_drink_items`, `count_order_items`, `is_food_order`, `is_drink_order`, `customer_order_number`

2. **`fact_order_items`**
   The line items of an order, inside the Order aggregate.
   - **Grain**: One row per product on an order.
   - **Columns**: `order_item_id`, `order_id`, `product_id`, `ordered_at_date_key`, `product_price`, `supply_cost`, `is_food_item`, `is_drink_item`

### Dimension Tables

The Ordering context documents no dimensions of its own. Every dimension it reads is owned by another bounded context and borrowed through the context map.

## Star Schema Diagram

```mermaid
erDiagram
    fact_orders {
        string order_id PK
        string customer_id FK
        string location_id FK
        date ordered_at_date_key FK
        string delivery_partner_id FK
        float64 subtotal
        float64 tax_paid
        float64 order_total
        int64 count_order_items
        boolean is_food_order
    }
    fact_order_items {
        string order_item_id PK
        string order_id FK
        string product_id FK
        date ordered_at_date_key FK
        float64 product_price
        float64 supply_cost
        boolean is_food_item
        boolean is_drink_item
    }

    fact_order_items }o--|| fact_orders : "belongs to"
    fact_orders }o--|| dim_customers : "placed by"
    fact_orders }o--|| dim_locations : "placed at"
    fact_orders }o--|| dim_date : "ordered on"
    fact_order_items }o--|| dim_products : "is a"
```

## Lineage

| Proposed Table | Source Model(s) |
| :--- | :--- |
| `fact_orders` | `jaffle_shop.stg_orders`, `jaffle_shop.stg_order_items` |
| `fact_order_items` | `jaffle_shop.stg_order_items`, `jaffle_shop.stg_products`, `jaffle_shop.stg_supplies` |

The table list and lineage above are generated from the per-table documents in this directory. If they disagree with a child document, the child is authoritative.
