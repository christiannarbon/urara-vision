// Sticky notes are user-written, so they must never reach the agent-facing responses.
package api_test

import (
	"net/http"
	"strings"
	"testing"

	"urara-vision/backend/internal/model"
)

const secretNote = "sticky-note-body-7f3a"

// withNotes stores a note (and a reply) on every table and domain the fake knows.
func withNotes(meta *fakeMeta, sid string) *fakeMeta {
	add := func(kind, id string) {
		meta.addNote(model.Note{ID: kind + ":" + id, SnapshotID: sid, AnchorKind: kind, AnchorID: id, Body: secretNote, AuthorName: "vera"})
		meta.addNote(model.Note{ID: kind + ":" + id + ":r", SnapshotID: sid, ParentID: kind + ":" + id, AnchorKind: kind, AnchorID: id, Body: secretNote, AuthorName: "cody"})
	}
	for _, d := range meta.domains {
		add("domain", d.ID)
	}
	for _, t := range meta.tables {
		add("table", t.ID)
	}
	for id, t := range meta.tablesByID {
		t.Notes = []string{"a documented caveat"}
		add("table", id)
		add("column", id+"#id")
	}
	return meta
}

func TestContextCarriesNoNotes(t *testing.T) {
	meta := withNotes(contextMeta(), "snap-1")
	h := newServerWithContextCap(t, meta, &fakeGraphs{}, 400)
	rec := do(t, h, http.MethodGet, "/api/v1/snapshots/snap-1/context", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), secretNote) {
		t.Errorf("/context leaked a note body: %s", rec.Body)
	}
	if meta.noteCalls != 0 {
		t.Errorf("/context made %d notes reads", meta.noteCalls)
	}
}

func TestTablesDetailCarriesNoNotes(t *testing.T) {
	meta := withNotes(batchMeta(), "s1")
	h := newServer(t, meta, batchGraphs())
	rec := do(t, h, http.MethodGet, "/api/v1/snapshots/s1/tables/detail?ids=domain_one/fact_primary,domain_one/dim_alpha", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	if strings.Contains(body, secretNote) {
		t.Errorf("/tables/detail leaked a note body: %s", body)
	}
	if meta.noteCalls != 0 {
		t.Errorf("/tables/detail made %d notes reads", meta.noteCalls)
	}
	// The documents' own notes still come through.
	if !strings.Contains(body, "a documented caveat") {
		t.Errorf("document notes missing: %s", body)
	}
}
