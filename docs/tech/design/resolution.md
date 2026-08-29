# Resolution

`internal/graph` is the stage that needs the whole directory in view. It runs
over plain structs with no store anywhere near it, which is why every rule
below is unit-tested against a handful of in-memory documents.

`Build` runs the stages in order, and the order is observable: diagnostics are
appended as they are found and sorted stably at the end, so two findings of
equal severity keep the sequence the stages produced.

## The registry

Tables are indexed twice up front — by ID, and by bare name to every ID
carrying it, sorted — because resolution asks the same two questions over and
over: "is there a table with this ID?" and "what else is called this?"

## Conformed dimensions

A table is conformed either because its name is documented in more than one
domain, or because the document declares it — a `Domain` or `Type` cell
containing the word `conformed`, the way a shared kernel writes
`Shared Kernel (Conformed Dimensions)`. The declaration counts on its own: a
kernel dimension no other directory has got round to documenting is still a
conformed dimension.

Matching requires that word specifically. A domain merely named something like
`Cross-Domain Reporting` is not claiming to be an authority.

## Binding a reference

For each declared relationship, in order of preference:

1. **A table in the same domain.** Local always wins.
2. **A conformed instance elsewhere.** Ranked by: a document that declares
   itself conformed, then the richest definition (most columns), then
   alphabetically, so the choice is deterministic. The alternatives are kept on
   the relationship as candidates and shown in the UI, and the binding is
   reported as `cross_domain_reference`.
3. **Nothing.** If the target looks like a table identifier —
   a snake_case token — that is an `unresolved_reference` error. If it is prose
   (`Various Fact Tables`), it is a `narrative_reference` and no edge is drawn.

## Join-key orientation

The one place the documents cannot be taken at their word. The order a join key
is written in does not reliably say which column belongs to which side: some
documents write the *fact's* column first even on their own `One-to-many` rows.

So the sides are matched against the tables' real column lists instead. If
exactly one reading places both columns in the tables that own them, that
reading wins; if both readings work, the written order is kept; if only one
column can be placed, it anchors the pair. When neither side can be matched to
either table, the columns stay as written and it becomes an
`unmatched_join_key` warning — the join is still real, but the key drawn on it
is fiction.

Identical column names on both sides are the easy case: order carries no
information and none is needed.

## Edge normalisation

A relationship declared from both sides is one edge, not two. Every declaration
is rewritten to point from the many side to the one side — reversing the join
columns along with the direction, so the key still names the many side's column
first — and then deduplicated on `(from, to, fromColumn, toColumn)`.

Two details survive the merge:

- **`declaredBy`** accumulates every table that asserted the join, so the UI can
  show when only one side documents a relationship.
- **A conformed binding wins over a local one.** It is the more interesting fact
  about an edge.

Nothing in this stage reads a table's role. That is exactly why a snowflake's
dimension-to-dimension joins and a Data Vault's link-to-hub joins need no
special case: cardinality alone decides the direction.

## Source canonicalisation

Documents cite the same upstream model both ways — `warehouse.upstream_model`
in one file, `upstream_model` in another. Left unfolded that is two unrelated
nodes, and "what else reads this?" quietly returns the wrong answer.

References are folded onto one identity, preferring the qualified spelling, and
each canonical source carries a count of how many columns cite it. A lineage
cell that is prose rather than a model name is left on the column so the detail
pane still shows what the document said, but is not modelled as a source, and
is reported as `undocumented_lineage`.

## Conformed drift

Once the instances of a repeated name are known, each is compared against the
authority `pickConformed` chose, and the columns that differ become a
`conformed_drift` warning naming both sides. That is the check that catches a
shared dimension slowly becoming two different dimensions with one name.

## Isolation

A table whose whole purpose is to join others — a fact, a Data Vault link, a
junction table — that resolved no relationship at all is almost always a
documentation gap rather than a real standalone table, and is reported as
`isolated_fact` or `isolated_table`. Roles that are legitimately standalone are
left alone: a conformed dimension nothing in this directory happens to join to
is ordinary, not a problem.
