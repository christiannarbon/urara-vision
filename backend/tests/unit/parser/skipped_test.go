// Documents the parser cannot use, and the stability of what it produces.
package parser_test

import (
	"testing"

	"urara-vision/backend/internal/parser"
	"urara-vision/backend/tests/fixtures"
)

// TestSkippedDocumentsAreReported covers the two codes the frontend treats as
// parse failures rather than findings: the document contributed nothing, so the
// reader has to be told the model is incomplete.
func TestSkippedDocumentsAreReported(t *testing.T) {
	res := parser.Parse([]parser.File{
		fixtures.File("d/blank.md", "   \n\n"),
		fixtures.File("notes.md", "# Just prose\n\nNothing structured here.\n"),
		fixtures.File("d/README.txt", "not markdown"),
	})
	if res.Parsed != 0 {
		t.Errorf("parsed = %d, want 0", res.Parsed)
	}
	if res.Skipped != 3 {
		t.Errorf("skipped = %d, want 3", res.Skipped)
	}
	codes := map[string]bool{}
	for _, d := range res.Diagnostics {
		codes[d.Code] = true
	}
	if !codes["empty_document"] {
		t.Error("expected an empty_document diagnostic")
	}
	if !codes["unrecognised_document"] {
		t.Error("expected an unrecognised_document diagnostic")
	}
}

// TestParseIsOrderIndependent: two ingests of the same directory must produce
// identical output whatever order the files arrive in, or a re-ingest looks
// like a change.
func TestParseIsOrderIndependent(t *testing.T) {
	files := fixtures.StarSchema()
	reversed := make([]parser.File, len(files))
	for i, f := range files {
		reversed[len(files)-1-i] = f
	}
	a, b := parser.Parse(files), parser.Parse(reversed)
	if len(a.Tables) != len(b.Tables) || len(a.Domains) != len(b.Domains) {
		t.Fatalf("counts differ: %d/%d tables, %d/%d domains",
			len(a.Tables), len(b.Tables), len(a.Domains), len(b.Domains))
	}
	for i := range a.Tables {
		if a.Tables[i].ID != b.Tables[i].ID {
			t.Errorf("table %d = %q vs %q", i, a.Tables[i].ID, b.Tables[i].ID)
		}
	}
}
