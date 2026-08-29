# Parsing

`internal/parser` turns a directory of markdown into domains and tables, and it
deliberately never looks beyond the document in hand. Relationship targets are
left exactly as written; matching them against the rest of the directory is
`internal/graph`'s job. That boundary is what lets every parser test be one
document long.

## Sections

A document is split on its headings, and a section's body runs to the next
heading of the same or shallower depth — so a level-2 section carries its
level-3 children. Headings are matched case-insensitively, exactly first and
then by prefix, which is how `Notes / Caveats` also answers to `Notes`.

## Deciding what a document is

`classify` reads the headings before it trusts the path, so a reorganised
directory still parses:

1. `Columns` **and** `Overview` → a table document.
2. A diagram heading, or a proposed-tables heading → a domain index.
3. Nested in a directory: `Columns` alone is enough for a table document;
   anything else is unrecognised.
4. At the root: a `Description` makes it a domain index.
5. Otherwise unrecognised, and reported as `unrecognised_document`.

The bare heading `Diagram` is deliberately absent from the list that decides
what a document *is*, and present in the list `parseDomainDoc` reads once the
decision is made. Reading a diagram out of a document already known to be an
index can afford to be generous; deciding what a document is cannot, or a table
document that happens to carry a diagram stops being a table.

A domain index is `domain_one.md` beside its `domain_one/` directory. A
directory with table documents but no index still forms a domain, synthesised
from the directory name and reported as `missing_domain_index`.

## Reading a table document

The `Overview` block is a two-column property table, read by matching the
property name rather than by position, because documents order those rows
however they like. `Type` becomes the role, `Domain` the domain label, and both
feed the conformed rules.

Columns keep their document order — the ordinal is stored — because a column
list is a reading order, not a set.

Key flags are inferred from three places, because documents mark keys in
whichever one they happen to use: a description or a lineage note matching
`PK` / `primary key` sets the primary-key flag, `FK` / `foreign key` sets the
foreign-key flag, and a column that a declared relationship joins *from* is
taken to be a foreign key unless it is already a primary key.

Column lineage rows are `Column | Source Table | Source Column | Notes`, and a
`Notes` cell starting with `Derived` sets the derived flag. Relationship rows are
`Related Table | Join Key | Relationship`, and every cell of all of them is
kept raw as well as parsed, so a diagnostic can quote what the document
actually said.

## Normalising a role

`Type` cells are prose written by people: `Dimension (Conformed)`,
`Conformed Dimension`, `Fact Table`. `normalise.go` maps the spellings that
recur onto the roles that have a name, and leaves anything else as a role of
its own — lower-cased and trimmed, but otherwise the author's word. A `Type`
holding a whole sentence is prose in the wrong column and becomes `unknown`.

## Determinism

Files are sorted before parsing and domains and tables sorted after, so two
ingests of the same directory produce byte-identical output whatever order the
browser handed the files over in. A test pins that: parse a fixture, parse it
reversed, compare.
