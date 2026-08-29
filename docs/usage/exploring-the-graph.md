# Exploring the graph

Once a directory is ingested, the canvas is the model: every table a node,
every resolved join an edge, every domain a hull around its own tables.

## Layouts

No single arrangement suits every way a warehouse gets modelled:

| Layout | Reads well for | Groups by domain |
|---|---|---|
| Force | Stars, and any model with no natural reading direction | Yes |
| Layered | Snowflake normalisation depth, Data Vault tiers | No |
| Radial | A star seen the way it is drawn on a whiteboard | No |

Grouping is a Force-mode feature because neither of the other two layout engines
understands compound nodes; the toggle disables itself rather than silently
dropping the clusters, and remembers your preference for when you switch back.

## Narrowing what is drawn

The sidebar filters by domain and by role, and both compose. Two of its
controls do more than filter:

- **Cross-domain joins only** keeps just the edges that leave the domain they
  were declared in. On a set where each domain keeps its own copy of the
  conformed dimensions, that is the fastest way to see the seams.
- **Overlay source models** adds the upstream models the column lineage cites
  as their own nodes, so a table's provenance is on the same canvas as its
  joins.

## Following one table

Click a table for the detail pane: its description, grain, columns,
column-level lineage, the joins it declares, who joins to *it*, and its
caveats.

From there:

- **Neighbourhood** redraws the canvas as just that table and everything within
  a chosen number of hops.
- **Paths** answers "how do I join these two?" with the shortest join paths
  between them.
- **Lineage** walks upstream to the source models a table derives from, or
  downstream from a source model to everything that reads it — which is the
  "what else breaks if this upstream model changes?" question.

## Search

Search runs over table names, domains, descriptions, grains and column names
and descriptions, weighted in that order. It is a prefix search, so it matches
as you type: `prim` finds `fact_primary` before you have finished the word. A
hit that matched on a column rather than the table name says which column, so
it can explain itself.

## Reading the graph

A documentation set where each domain keeps its own copy of the conformed
dimensions will draw as several mostly-separate components rather than one
connected model, and the only joins crossing a domain boundary will be the ones
a domain could not satisfy locally — exactly what the `cross_domain_reference`
warnings list. Merging those dimensions into single shared documents is what
connects the graph up.

Until then, **Cross-domain joins only** in the sidebar is the fastest way to see
where the seams are.

A canvas of identical shapes usually means the `Type` cells are not saying what
you think. See [roles](documentation-format.md#roles).
