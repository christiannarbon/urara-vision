//go:build integration

// The version diff over two real ingests of the diff fixture pair.
package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"urara-vision/backend/internal/diff"
)

const diffFixtures = "../../fixtures/diff"

// ingestDir uploads a documentation directory as it sits on disk.
func ingestDir(t *testing.T, base, dir string) string {
	t.Helper()
	type file struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	req := struct {
		Name        string `json:"name"`
		SourceLabel string `json:"sourceLabel"`
		Files       []file `json:"files"`
	}{Name: "integration", SourceLabel: dir}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		req.Files = append(req.Files, file{Path: filepath.ToSlash(rel), Content: string(b)})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(req)
	res, err := http.Post(base+"/api/v1/ingest", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /ingest: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("POST /ingest %s = %d: %s", dir, res.StatusCode, raw)
	}
	var out struct {
		Project struct {
			Slug string `json:"slug"`
		} `json:"project"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out.Project.Slug
}

func deleteProject(t *testing.T, base, slug string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodDelete, base+"/api/v1/projects/"+slug, nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Errorf("DELETE project %s: %v", slug, err)
		return
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNoContent && res.StatusCode != http.StatusNotFound {
		t.Errorf("DELETE project %s = %d", slug, res.StatusCode)
	}
}

func TestDiffReportsExactlyTheFixtureChanges(t *testing.T) {
	base := stack(t)
	// The fixture's project name is fixed, so clear anything a crashed run left.
	deleteProject(t, base, "diff-fixture")
	slug := ingestDir(t, base, filepath.Join(diffFixtures, "v1"))
	t.Cleanup(func() { deleteProject(t, base, slug) })
	if got := ingestDir(t, base, filepath.Join(diffFixtures, "v2")); got != slug {
		t.Fatalf("v2 landed in project %q, want %q", got, slug)
	}

	res, err := http.Get(base + "/api/v1/projects/" + slug + "/diff?from=1.0.0&to=latest")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("diff = %d: %s", res.StatusCode, raw)
	}
	var body struct {
		From struct{ Version string } `json:"from"`
		To   struct{ Version string } `json:"to"`
		diff.Result
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if body.From.Version != "1.0.0" || body.To.Version != "2.0.0" {
		t.Errorf("versions = %s → %s", body.From.Version, body.To.Version)
	}

	// The seven changes in fixtures/diff/README.md, plus the added table's join.
	none := []diff.FieldChange{}
	want := diff.Result{
		Summary: diff.Summary{
			Tables:        diff.Counts{Added: 1, Removed: 1, Changed: 3},
			Columns:       diff.Counts{Added: 1, Changed: 1},
			Relationships: diff.Counts{Added: 1, Changed: 1},
			Lineage:       diff.Counts{Removed: 1},
		},
		Domains: []diff.DomainDiff{},
		Tables: []diff.TableDiff{
			{ID: "catalog/dim_products", DomainID: "catalog", Change: diff.Changed, Columns: []diff.ColumnDiff{},
				Fields: []diff.FieldChange{{Field: "grain", From: "One row per product.", To: "One row per product variant."}}},
			{ID: "catalog/fact_restocks", DomainID: "catalog", Change: diff.Added, Fields: none, Columns: []diff.ColumnDiff{}},
			{ID: "sales/dim_customers", DomainID: "sales", Change: diff.Changed, Fields: none,
				Columns: []diff.ColumnDiff{{Name: "phone", Change: diff.Added, Fields: none}}},
			{ID: "sales/dim_promotions", DomainID: "sales", Change: diff.Removed, Fields: none, Columns: []diff.ColumnDiff{}},
			{ID: "sales/fact_orders", DomainID: "sales", Change: diff.Changed, Fields: none,
				Columns: []diff.ColumnDiff{{Name: "amount", Change: diff.Changed,
					Fields: []diff.FieldChange{{Field: "type", From: "INT64", To: "NUMERIC"}}}}},
		},
		Relationships: []diff.RelationshipDiff{
			{FromTableID: "catalog/fact_inventory", ToTableID: "catalog/dim_products", TargetRef: "dim_products",
				FromColumn: "product_id", ToColumn: "product_id", Change: diff.Changed,
				Fields: []diff.FieldChange{{Field: "cardinality", From: "Many-to-one", To: "One-to-one"}}},
			{FromTableID: "catalog/fact_restocks", ToTableID: "catalog/dim_products", TargetRef: "dim_products",
				FromColumn: "product_id", ToColumn: "product_id", Change: diff.Added, Fields: none},
		},
		Lineage: []diff.LineageDiff{
			{TableID: "sales/fact_orders", Column: "amount", SourceTable: "shop.refunds", SourceColumn: "amount",
				Change: diff.Removed, Fields: none},
		},
	}
	if !reflect.DeepEqual(body.Result, want) {
		t.Errorf("diff mismatch\n got: %+v\nwant: %+v", body.Result, want)
	}
}
