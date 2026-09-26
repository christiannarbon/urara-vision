# Comparing versions

Once a project has more than one version, you can ask what changed in the model
between any two of them without reading both directories side by side.

## Opening a comparison

- **From the home screen:** expand a project, then press *Compare* on any
  version except the newest. It opens that version compared with the latest.
- **From the workspace:** press *Compare* next to the version picker in the top
  bar. It compares the version on screen with the one before it, or with the
  newest if you are on the oldest.
- **By link:** `/projects/<project>/diff?from=<version>&to=<version>`. The page
  keeps both versions in the address, so a comparison can be bookmarked or
  shared. Leave them out and it picks the two newest versions.

On the page, the two pickers change either side, and *Swap* reverses them, so
what was added becomes removed.

## Reading the result

The summary line counts what was added (`+`), removed (`−`) and changed (`~`)
for domains, tables, columns, joins and column lineage. When the two versions
match, it says *No differences.*

Below it, changes are grouped by domain. Each table shows the fields that
changed as `field: old → new`, then its changed columns. Joins and lineage are
listed under the domain of the table that declares them. A long description is
cut to one line; click it to read the rest. The filter chips narrow the list to
added, removed or changed.

Only these fields are compared. Everything else (document paths, the order
columns appear in, diagnostics) is ignored.

| Thing | Fields |
|---|---|
| Domain | title, description |
| Table | kind, grain, update frequency, layer, description, conformed |
| Column | type, description, primary key, foreign key |
| Join | cardinality, resolution |
| Column lineage | notes, derived |

Prose is compared exactly as written, language tags included, so a changed
translation counts as a changed description.

## How things are matched

Two versions are lined up by name, never by position:

| Thing | Treated as the same when… |
|---|---|
| Domain | it has the same domain name |
| Table | it has the same domain and table name |
| Column | it is in the same table and has the same name |
| Join | it runs from the same table, to the same table (or the same written reference, if the target could not be found), on the same two columns |
| Column lineage | it is on the same table and column, from the same source table and column |

This is why inserting a column in the middle of a table shows as one added
column, not as every later column changing. And a column fed by two sources is
two lineage entries, so losing one source shows as one removal.

**A renamed table shows as one removed and one added.** Nothing tries to guess
that the two are the same table. The same goes for a table moved to another
domain, and for a renamed column.

If a document declares the same join twice, both copies are kept and compared,
so removing the duplicate shows as one removed join.

## On the graph

*Show on graph* opens the newer version with its changes marked: added tables
and joins get a green halo, changed ones an amber halo. Clicking a table's name
in the list does the same and selects that table. A legend in the corner of the
graph gives the counts, links back to the full comparison, and has a *Clear*
button that removes the marks.

**Removed tables and joins are not on the graph.** They do not exist in the
newer version, so there is nothing to draw. The comparison page is the only
place they are listed.
