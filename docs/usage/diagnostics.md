# Diagnostics

The reason the tool exists. Reading one document at a time, a documentation set
looks fine; the problems are all in the spaces between documents, and this is
the list of them.

| Check | Severity | What it catches |
|---|---|---|
| `unresolved_reference` | error | A join points at a table no document defines |
| `conformed_drift` | warning | The same dimension documented differently in different domains |
| `cross_domain_reference` | warning | A domain borrows a dimension it does not document |
| `unmatched_join_key` | warning | A join key naming columns neither table declares |
| `isolated_fact` | warning | A fact with no resolvable join |
| `isolated_table` | warning | Any other connective table — a link, a junction — joined to nothing |
| `no_columns` | warning | A table document with no readable Columns table |
| `missing_domain_index` | warning | A directory of tables with no index document |
| `empty_domain` | warning | An index document whose directory holds nothing |
| `empty_document` | warning | A document with no content, skipped |
| `narrative_reference` | info | Prose (`Various Fact Tables`) where a table name belongs |
| `undocumented_lineage` | info | A column whose source is prose rather than a model name |
| `name_filename_mismatch` | info | A document whose Table Name and filename disagree |
| `unrecognised_document` | info | A document matching neither layout, ignored |

## What to do about the common ones

**`unresolved_reference`** is the only error, and it means what it says: a
`Related Table` cell names something no document defines. Either the table is
undocumented, or the name is misspelled, or it lives in another domain and
nothing there documents it either.

**`cross_domain_reference`** is not a mistake on its own — it is how a borrowed
conformed dimension looks. It becomes a finding when a domain is borrowing
something it ought to own, or when there is no shared definition anywhere and
each domain has quietly written its own.

**`conformed_drift`** compares every instance of a repeated table name against
the conformed authority and reports the columns that differ. This is the check
that catches a shared dimension slowly becoming two different dimensions with
one name.

**`unmatched_join_key`** means the join key cell names a column that neither
table declares. Usually a rename that only landed in one document. The join
still resolves — the tables are real — but the key it is drawn with is fiction.

**`narrative_reference`** is prose in a column reserved for a table name:
`Various Fact Tables`, `Catalog Dimensions`. Readable, and invisible to
everything downstream. Filed as info rather than a warning because it is often
deliberate.

**`isolated_fact`** and **`isolated_table`** flag a table whose whole purpose is
to join others — a fact, a Data Vault link, a junction table — that resolved no
relationship at all. A conformed dimension nothing in this directory happens to
join to is ordinary and is left alone.

## Reading them

In the app, the diagnostics pane lists them by severity and links each one to
the document that produced it.

Outside the app, [`relctl`](cli.md) prints the same list, and
`relctl -strict` exits non-zero when any error is present — which makes it a
documentation linter you can put in CI.
