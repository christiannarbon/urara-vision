// Command uraractl parses a directory of data model documentation and reports
// what it found, without needing the server or any datastore. It is useful for
// checking documentation in CI: a non-zero exit means unresolved references.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"urara-vision/backend/internal/graph"
	"urara-vision/backend/internal/model"
	"urara-vision/backend/internal/parser"
)

func main() {
	dir := flag.String("dir", ".", "documentation directory to parse")
	asJSON := flag.Bool("json", false, "emit the full model as JSON")
	strict := flag.Bool("strict", false, "exit non-zero when any error diagnostic is present")
	flag.Parse()

	var files []parser.File
	err := filepath.WalkDir(*dir, func(p string, d os.DirEntry, err error) error {
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
		rel, err := filepath.Rel(*dir, p)
		if err != nil {
			return err
		}
		files = append(files, parser.File{Path: filepath.ToSlash(rel), Content: string(b)})
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "walk:", err)
		os.Exit(2)
	}

	m := graph.Build("local", filepath.Base(*dir), *dir, parser.Parse(files))
	edges := graph.Edges(m)

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(struct {
			*model.Model
			Edges []graph.Edge `json:"edges"`
		}{m, edges})
		return
	}

	s := m.Snapshot.Stats
	fmt.Printf("files parsed   %d (skipped %d)\n", s.FilesParsed, s.FilesSkipped)
	fmt.Printf("domains        %d\n", s.Domains)
	fmt.Printf("tables         %d  (conformed instances %d)\n", s.Tables, s.Conformed)
	fmt.Printf("columns        %d\n", s.Columns)
	fmt.Printf("relationships  %d declared -> %d normalised edges\n", s.Relationships, len(edges))
	fmt.Printf("lineage edges  %d across %d source tables\n", s.LineageEdges, s.SourceTables)

	// Roles are reported as found rather than as a fixed fact/dimension/unknown
	// tally: a Data Vault or a plain relational model has neither, and a tally
	// of three zeroes would say nothing about it.
	byKind := map[model.TableKind]int{}
	for _, t := range m.Tables {
		byKind[t.Kind]++
	}
	kinds := make([]model.TableKind, 0, len(byKind))
	for k := range byKind {
		kinds = append(kinds, k)
	}
	sort.Slice(kinds, func(i, j int) bool {
		if byKind[kinds[i]] != byKind[kinds[j]] {
			return byKind[kinds[i]] > byKind[kinds[j]]
		}
		return kinds[i] < kinds[j]
	})
	parts := make([]string, 0, len(kinds))
	for _, k := range kinds {
		parts = append(parts, fmt.Sprintf("%s=%d", k, byKind[k]))
	}
	if len(parts) == 0 {
		parts = append(parts, "none")
	}
	fmt.Printf("roles          %s\n", strings.Join(parts, " "))

	counts := map[string]int{}
	for _, d := range m.Diagnostics {
		counts[d.Severity]++
	}
	fmt.Printf("diagnostics    error=%d warning=%d info=%d\n\n",
		counts[model.SeverityError], counts[model.SeverityWarning], counts[model.SeverityInfo])

	shown := map[string]int{}
	for _, d := range m.Diagnostics {
		if shown[d.Code] < 3 {
			fmt.Printf("  [%s] %s: %s\n", d.Severity, d.Code, d.Message)
		}
		shown[d.Code]++
	}
	fmt.Println()
	for code, n := range shown {
		fmt.Printf("  %-26s %d\n", code, n)
	}

	if *strict && counts[model.SeverityError] > 0 {
		os.Exit(1)
	}
}
