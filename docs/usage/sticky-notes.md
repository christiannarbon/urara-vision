# Sticky notes

Notes let people discuss the model where it is drawn: a question about a
column, a caveat on a join, a reminder on a domain. Everyone who can open a
version sees its notes.

## Where notes go

| On | Where the button is |
|---|---|
| A table | The detail pane header, next to the close button |
| A column | Each row of the detail pane's *columns* tab |
| A join | Each declared join on the *joins* tab (not the *Referenced by* rows) |
| A column's lineage | The *Column lineage* list on the *lineage* tab, once per column |
| A domain | Beside each domain chip in the left rail |

The button shows how many open notes there are; hover it to see how many are
resolved. A tab with open notes on any of its rows shows a dot, and a table
with open notes gets a dashed outline on the graph.

## Writing and replying

Press the button to open the thread. Type in the box at the bottom and press
*Add note*, or Ctrl+Enter (⌘+Enter on a Mac). Notes are plain text, up to
4000 characters; formatting and HTML are shown exactly as typed.

*Reply* adds a reply under a note. Replies go one level deep: you cannot reply
to a reply.

## Resolving, editing and deleting

| Action | Who can |
|---|---|
| Add a note or reply | Anyone with `note.write`: viewers, creators and admins |
| Resolve or reopen a note | Anyone with `note.write`. Only top-level notes are resolved; replies follow their note |
| Edit a note | Its author, or an admin (`note.moderate`) |
| Delete a note | Its author, or an admin. Deleting a note deletes its replies |

A resolved note folds to one line, *Resolved by …*; click it to read it again.
If an author's account is deleted, their notes stay, under the name they had.

## Notes belong to one version

A note is attached to the version it was written on. Importing a new version
starts with no notes, and deleting a version deletes its notes. Anchors are
checked when a note is added, so a note can only point at a table, column,
join or domain that exists in that version.

## The chat assistant cannot read notes

Notes are never given to the chat assistant, and it has no way to fetch them.
Anyone who can write a note could otherwise put instructions in front of the
model and steer its answers for everyone else. Ask a person, or read the
thread.
