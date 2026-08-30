// Loading a shipped demo documentation set the way uraractl and the frontend do.
//
// Three sets ship under docs/demo, and each has its own suite pinning what it
// resolves to. The walk is identical for all of them, so it lives here rather
// than three times over.
package demo_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"urara-vision/backend/internal/graph"
	"urara-vision/backend/internal/model"
	"urara-vision/backend/internal/parser"
)

// loadSet walks one demo directory and resolves it. The snapshot ID and name
// are the directory's base name, matching what uraractl produces.
func loadSet(t *testing.T, dir string) *model.Model {
	t.Helper()
	var files []parser.File
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.EqualFold(filepath.Ext(p), ".md") {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		files = append(files, parser.File{Path: filepath.ToSlash(rel), Content: string(b)})
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", dir, err)
	}
	if len(files) == 0 {
		t.Fatalf("no documents found under %s", dir)
	}
	name := filepath.Base(dir)
	return graph.Build("demo", name, dir, parser.Parse(files))
}

// stat is one pinned figure from a set's snapshot statistics.
type stat struct {
	name      string
	got, want int
}

// checkStats reports every figure that has moved, rather than stopping at the
// first, so a resolver change shows its whole blast radius in one run.
func checkStats(t *testing.T, stats []stat) {
	t.Helper()
	for _, c := range stats {
		if c.got != c.want {
			t.Errorf("%s = %d, want %d", c.name, c.got, c.want)
		}
	}
}

// checkDiagnostics pins a set's diagnostics exactly: every code the sample is
// built around at its expected count, and nothing else. A count dropping to
// zero means that check lost its coverage; an unexpected code means the sample
// has acquired a flaw nobody meant to put there.
func checkDiagnostics(t *testing.T, m *model.Model, want map[string]int, wantErrors int) {
	t.Helper()
	counts := map[string]int{}
	for _, d := range m.Diagnostics {
		counts[d.Code]++
	}
	for code, n := range want {
		if counts[code] != n {
			t.Errorf("%s = %d, want %d", code, counts[code], n)
		}
		delete(counts, code)
	}
	for code, n := range counts {
		t.Errorf("unexpected diagnostic %s (%d); the sample should only carry deliberate flaws", code, n)
	}

	errors := 0
	for _, d := range m.Diagnostics {
		if d.Severity == model.SeverityError {
			errors++
		}
	}
	if errors != wantErrors {
		t.Errorf("error diagnostics = %d, want %d", errors, wantErrors)
	}
}

// checkTwoSided asserts that every join a set declares from both sides
// collapses to exactly one edge, credited to both documents.
func checkTwoSided(t *testing.T, m *model.Model, twoSided map[string][]string) {
	t.Helper()
	seen := map[string]int{}
	for _, e := range graph.Edges(m) {
		key := e.From + "->" + e.To
		seen[key]++
		want, ok := twoSided[key]
		if !ok {
			continue
		}
		if strings.Join(e.DeclaredBy, ",") != strings.Join(want, ",") {
			t.Errorf("%s declaredBy = %v, want %v", key, e.DeclaredBy, want)
		}
	}
	for key := range twoSided {
		if seen[key] != 1 {
			t.Errorf("%s appears %d times, want exactly 1 merged edge", key, seen[key])
		}
	}
}

// relationshipTo returns the relationship the named table declares to the
// named target, for asserting on how a single declaration was resolved.
func relationshipTo(m *model.Model, tableID, targetRef string) (model.Relationship, bool) {
	for _, tb := range m.Tables {
		if tb.ID != tableID {
			continue
		}
		for _, r := range tb.Relationships {
			if r.TargetRef == targetRef {
				return r, true
			}
		}
	}
	return model.Relationship{}, false
}
