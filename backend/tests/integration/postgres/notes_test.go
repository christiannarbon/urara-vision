//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"testing"

	"urara-vision/backend/internal/model"
	"urara-vision/backend/internal/notes"
	"urara-vision/backend/internal/store/postgres"
	"urara-vision/backend/tests/integration/harness"
)

func createNote(t *testing.T, ctx context.Context, pg *postgres.Store, n model.Note) *model.Note {
	t.Helper()
	if n.Body == "" {
		n.Body = "a note"
	}
	if n.AuthorName == "" {
		n.AuthorName = "Tester"
	}
	out, err := pg.CreateNote(ctx, n)
	if err != nil {
		t.Fatalf("CreateNote(%+v): %v", n, err)
	}
	return out
}

// realAnchors picks one existing anchor of each kind from the saved fixture.
func realAnchors(t *testing.T, m *model.Model) map[notes.Kind]string {
	t.Helper()
	a := map[notes.Kind]string{notes.KindDomain: m.Domains[0].ID, notes.KindTable: m.Tables[0].ID}
	for _, tb := range m.Tables {
		if len(tb.Columns) > 0 && a[notes.KindColumn] == "" {
			a[notes.KindColumn] = tb.ID + "#" + tb.Columns[0].Name
		}
		if len(tb.Relationships) > 0 && a[notes.KindRelationship] == "" {
			a[notes.KindRelationship] = tb.Relationships[0].ID
		}
		if len(tb.ColumnLineage) > 0 && a[notes.KindLineage] == "" {
			a[notes.KindLineage] = tb.ID + "#" + tb.ColumnLineage[0].Column
		}
	}
	if len(a) != 5 {
		t.Fatalf("fixture lacks an anchor kind: %v", a)
	}
	return a
}

func TestAnchorExists(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)
	m := harness.SavedModel(t, ctx, pg)
	table := m.Tables[0].ID

	fake := map[notes.Kind]string{
		notes.KindDomain:       "no_such_domain",
		notes.KindTable:        "no/such_table",
		notes.KindColumn:       table + "#no_such_column",
		notes.KindRelationship: "no-such-relationship",
		notes.KindLineage:      table + "#no_such_column",
	}
	for kind, id := range realAnchors(t, m) {
		if ok, err := pg.AnchorExists(ctx, m.Snapshot.ID, kind, id); err != nil || !ok {
			t.Errorf("AnchorExists(%s, %q) = %v, %v; want true", kind, id, ok, err)
		}
		if ok, err := pg.AnchorExists(ctx, m.Snapshot.ID, kind, fake[kind]); err != nil || ok {
			t.Errorf("AnchorExists(%s, %q) = %v, %v; want false", kind, fake[kind], ok, err)
		}
	}
}

func TestNoteThread(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)
	m := harness.SavedModel(t, ctx, pg)
	sid, table := m.Snapshot.ID, m.Tables[0].ID

	top := createNote(t, ctx, pg, model.Note{SnapshotID: sid, AnchorKind: "table", AnchorID: table, Body: "top"})
	// A reply's own anchor is ignored in favour of the parent's.
	r1 := createNote(t, ctx, pg, model.Note{SnapshotID: sid, ParentID: top.ID, AnchorKind: "domain", AnchorID: "x", Body: "first"})
	r2 := createNote(t, ctx, pg, model.Note{SnapshotID: sid, ParentID: top.ID, Body: "second"})
	if r1.AnchorKind != "table" || r1.AnchorID != table {
		t.Errorf("reply anchor = %s %s, want the parent's", r1.AnchorKind, r1.AnchorID)
	}

	got, err := pg.ListNotes(ctx, sid, notes.KindTable, table)
	if err != nil {
		t.Fatalf("ListNotes: %v", err)
	}
	if len(got) != 1 || got[0].ID != top.ID {
		t.Fatalf("ListNotes = %+v, want just the top note", got)
	}
	if rs := got[0].Replies; len(rs) != 2 || rs[0].ID != r1.ID || rs[1].ID != r2.ID {
		t.Errorf("replies = %+v, want first then second", rs)
	}

	if _, err := pg.CreateNote(ctx, model.Note{SnapshotID: sid, ParentID: r1.ID, Body: "deep", AuthorName: "x"}); !errors.Is(err, postgres.ErrReplyDepth) {
		t.Errorf("reply to reply err = %v", err)
	}
	if _, err := pg.SetNoteResolved(ctx, r1.ID, true, "x"); !errors.Is(err, postgres.ErrReplyDepth) {
		t.Errorf("resolve reply err = %v", err)
	}

	other := harness.SavedModel(t, ctx, pg)
	if _, err := pg.CreateNote(ctx, model.Note{SnapshotID: other.Snapshot.ID, ParentID: top.ID, Body: "b", AuthorName: "x"}); !errors.Is(err, postgres.ErrNotFound) {
		t.Errorf("parent in another snapshot err = %v", err)
	}

	edited, err := pg.UpdateNoteBody(ctx, top.ID, "edited")
	if err != nil || edited.Body != "edited" || !edited.UpdatedAt.After(top.UpdatedAt) {
		t.Errorf("UpdateNoteBody = %+v, %v", edited, err)
	}

	if err := pg.DeleteNote(ctx, top.ID); err != nil {
		t.Fatalf("DeleteNote: %v", err)
	}
	if n := countRows(t, `SELECT count(*) FROM notes WHERE snapshot_id = $1`, sid); n != 0 {
		t.Errorf("%d notes left after deleting the parent", n)
	}
	if _, err := pg.GetNote(ctx, r2.ID); !errors.Is(err, postgres.ErrNotFound) {
		t.Errorf("GetNote on deleted reply err = %v", err)
	}
}

func TestCountNotesAndResolve(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)
	m := harness.SavedModel(t, ctx, pg)
	sid, table, domain := m.Snapshot.ID, m.Tables[0].ID, m.Domains[0].ID

	a := createNote(t, ctx, pg, model.Note{SnapshotID: sid, AnchorKind: "table", AnchorID: table})
	createNote(t, ctx, pg, model.Note{SnapshotID: sid, AnchorKind: "table", AnchorID: table})
	createNote(t, ctx, pg, model.Note{SnapshotID: sid, ParentID: a.ID})
	createNote(t, ctx, pg, model.Note{SnapshotID: sid, AnchorKind: "domain", AnchorID: domain})

	counts := func() map[string]model.NoteCount {
		t.Helper()
		cs, err := pg.CountNotes(ctx, sid)
		if err != nil {
			t.Fatalf("CountNotes: %v", err)
		}
		out := map[string]model.NoteCount{}
		for _, c := range cs {
			out[c.AnchorKind+":"+c.AnchorID] = c
		}
		return out
	}
	check := func(key string, open, resolved int) {
		t.Helper()
		if c := counts()[key]; c.Open != open || c.Resolved != resolved {
			t.Errorf("%s = %d open, %d resolved; want %d, %d", key, c.Open, c.Resolved, open, resolved)
		}
	}
	check("table:"+table, 2, 0)
	check("domain:"+domain, 1, 0)

	res, err := pg.SetNoteResolved(ctx, a.ID, true, "Resolver")
	if err != nil || res.ResolvedAt == nil || res.ResolvedByName != "Resolver" {
		t.Fatalf("resolve = %+v, %v", res, err)
	}
	check("table:"+table, 1, 1)

	res, err = pg.SetNoteResolved(ctx, a.ID, false, "Resolver")
	if err != nil || res.ResolvedAt != nil || res.ResolvedByName != "" {
		t.Fatalf("unresolve = %+v, %v", res, err)
	}
	check("table:"+table, 2, 0)
}

func TestNoteCascades(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)
	m := harness.SavedModel(t, ctx, pg)
	sid := m.Snapshot.ID
	u := createdUser(t, ctx, pg, "$2a$12$hash")

	n := createNote(t, ctx, pg, model.Note{
		SnapshotID: sid, AnchorKind: "table", AnchorID: m.Tables[0].ID, AuthorID: u.ID, AuthorName: "Test User",
	})
	if err := pg.DeleteUser(ctx, u.ID); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	got, err := pg.GetNote(ctx, n.ID)
	if err != nil || got.AuthorID != "" || got.AuthorName != "Test User" {
		t.Errorf("after user delete = %+v, %v", got, err)
	}

	if err := pg.DeleteSnapshot(ctx, sid); err != nil {
		t.Fatalf("DeleteSnapshot: %v", err)
	}
	if c := countRows(t, `SELECT count(*) FROM notes WHERE snapshot_id = $1`, sid); c != 0 {
		t.Errorf("%d notes left after deleting the snapshot", c)
	}
}
