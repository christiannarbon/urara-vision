// The /context catalogue: the one call an agent makes to learn what a snapshot
// holds. What matters here is that it stays bounded without lying -- prose is
// cut at a rune boundary rather than a byte one, the table list is dropped
// whole rather than silently shortened, and every diagnostic severity is
// reported even when it is zero.
package api_test

import (
	"net/http"
	"strings"
	"testing"
	"unicode/utf8"

	"urara-vision/backend/internal/model"
	"urara-vision/backend/internal/store/postgres"
)

// contextMeta is a fake holding one domain, three tables and a diagnostic of
// each severity -- enough for the shape assertions to be meaningful.
func contextMeta() *fakeMeta {
	return &fakeMeta{
		snapshot: &model.Snapshot{
			ID:    "snap-1",
			Name:  "nightly",
			Stats: model.Stats{Domains: 1, Tables: 3},
		},
		domains: []model.Domain{
			{ID: "sales", Title: "Sales", Description: "orders and revenue", TableCount: 3},
		},
		tables: []postgres.TableSummary{
			{ID: "sales/fact_order", Name: "fact_order", DomainID: "sales", Kind: model.KindFact, Grain: "one row per order", ColumnCount: 12, Conformed: true},
			{ID: "sales/dim_customer", Name: "dim_customer", DomainID: "sales", Kind: model.KindDimension, ColumnCount: 8},
			{ID: "sales/dim_product", Name: "dim_product", DomainID: "sales", Kind: model.KindDimension, ColumnCount: 5},
		},
		diagnostics: []model.Diagnostic{
			{Severity: model.SeverityError, Code: "e1"},
			{Severity: model.SeverityWarning, Code: "w1"},
			{Severity: model.SeverityWarning, Code: "w2"},
		},
	}
}

// getContext issues the request and fails the test unless it returned 200.
func getContext(t *testing.T, h http.Handler) map[string]any {
	t.Helper()
	rec := do(t, h, http.MethodGet, "/api/v1/snapshots/snap-1/context", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200\n%s", rec.Code, rec.Body.String())
	}
	return decode(t, rec.Body.Bytes())
}

func TestContextReturnsCatalogue(t *testing.T) {
	h := newServerWithContextCap(t, contextMeta(), &fakeGraphs{}, 400)
	body := getContext(t, h)

	snap, _ := body["snapshot"].(map[string]any)
	if snap == nil || snap["id"] != "snap-1" || snap["name"] != "nightly" {
		t.Fatalf("snapshot = %v", body["snapshot"])
	}
	if _, ok := snap["stats"]; !ok {
		t.Error("snapshot carries no stats")
	}
	if _, ok := snap["project"]; !ok {
		t.Error("snapshot carries no project")
	}

	domains, _ := body["domains"].([]any)
	if len(domains) != 1 {
		t.Fatalf("domains = %v, want 1", body["domains"])
	}
	d, _ := domains[0].(map[string]any)
	if d["id"] != "sales" || d["title"] != "Sales" || d["tableCount"] != float64(3) {
		t.Errorf("domain = %v", d)
	}

	tables, _ := body["tables"].([]any)
	if len(tables) != 3 {
		t.Fatalf("tables = %v, want 3", body["tables"])
	}
	first, _ := tables[0].(map[string]any)
	if first["id"] != "sales/fact_order" || first["domainId"] != "sales" ||
		first["grain"] != "one row per order" || first["columnCount"] != float64(12) ||
		first["conformed"] != true {
		t.Errorf("table = %v", first)
	}
	// conformed is omitempty, so an unconformed table simply omits the key.
	second, _ := tables[1].(map[string]any)
	if _, present := second["conformed"]; present {
		t.Errorf("unconformed table carried a conformed key: %v", second)
	}

	if body["truncated"] != false {
		t.Errorf("truncated = %v, want false", body["truncated"])
	}
}

// TestContextAlwaysReportsEverySeverity: a consumer that has to tell an absent
// key from a count of zero will get it wrong, so all three are always emitted.
func TestContextAlwaysReportsEverySeverity(t *testing.T) {
	h := newServerWithContextCap(t, contextMeta(), &fakeGraphs{}, 400)
	diags, _ := getContext(t, h)["diagnostics"].(map[string]any)

	want := map[string]float64{"error": 1, "warning": 2, "info": 0}
	if len(diags) != len(want) {
		t.Fatalf("diagnostics = %v, want exactly %v", diags, want)
	}
	for severity, count := range want {
		if got, ok := diags[severity]; !ok {
			t.Errorf("severity %q missing from %v", severity, diags)
		} else if got != count {
			t.Errorf("diagnostics[%q] = %v, want %v", severity, got, count)
		}
	}
}

func TestContextTruncatesDescription(t *testing.T) {
	meta := contextMeta()
	meta.domains[0].Description = strings.Repeat("a", 500)
	h := newServerWithContextCap(t, meta, &fakeGraphs{}, 400)

	domains, _ := getContext(t, h)["domains"].([]any)
	got, _ := domains[0].(map[string]any)["description"].(string)
	if want := strings.Repeat("a", 400) + "…"; got != want {
		t.Errorf("description = %q (%d runes), want 400 runes plus an ellipsis",
			got, utf8.RuneCountInString(got))
	}
}

// TestContextTruncationIsRuneSafe is the point of truncateRunes existing: the
// corpus is bilingual, and cutting 500 Japanese characters by bytes would split
// a character and hand the agent mojibake instead of a shorter string.
func TestContextTruncationIsRuneSafe(t *testing.T) {
	meta := contextMeta()
	meta.domains[0].Description = strings.Repeat("受", 500)
	h := newServerWithContextCap(t, meta, &fakeGraphs{}, 400)

	domains, _ := getContext(t, h)["domains"].([]any)
	got, _ := domains[0].(map[string]any)["description"].(string)

	if n := utf8.RuneCountInString(got); n != 401 {
		t.Fatalf("description = %d runes, want 400 characters plus an ellipsis", n)
	}
	if !utf8.ValidString(got) {
		t.Fatal("description is not valid UTF-8: the cut split a character")
	}
	if want := strings.Repeat("受", 400) + "…"; got != want {
		t.Errorf("description = %q, want 400 unmangled characters plus an ellipsis", got)
	}
}

// TestContextKeepsGrainAtTheLimit: a string of exactly the limit is not cut, so
// no ellipsis appears on prose that fit.
func TestContextKeepsGrainAtTheLimit(t *testing.T) {
	meta := contextMeta()
	grain := strings.Repeat("受", 200)
	meta.tables[0].Grain = grain
	h := newServerWithContextCap(t, meta, &fakeGraphs{}, 400)

	tables, _ := getContext(t, h)["tables"].([]any)
	got, _ := tables[0].(map[string]any)["grain"].(string)
	if got != grain {
		t.Errorf("grain = %q (%d runes), want the 200-rune input untouched",
			got, utf8.RuneCountInString(got))
	}
}

// TestContextDropsTablesPastTheCap: past the cap the list goes away entirely
// rather than being shortened, because a partial catalogue reads as a complete
// one. Domains survive, since they are what is left to navigate by.
func TestContextDropsTablesPastTheCap(t *testing.T) {
	h := newServerWithContextCap(t, contextMeta(), &fakeGraphs{}, 2)
	body := getContext(t, h)

	tables, ok := body["tables"].([]any)
	if !ok || len(tables) != 0 {
		t.Errorf("tables = %v, want an empty list", body["tables"])
	}
	if body["truncated"] != true {
		t.Errorf("truncated = %v, want true", body["truncated"])
	}
	if domains, _ := body["domains"].([]any); len(domains) != 1 {
		t.Errorf("domains = %v, want the full list", body["domains"])
	}
}

func TestContextUnknownSnapshotIs404(t *testing.T) {
	meta := contextMeta()
	meta.errGetSnapshot = postgres.ErrNotFound
	h := newServerWithContextCap(t, meta, &fakeGraphs{}, 400)

	rec := do(t, h, http.MethodGet, "/api/v1/snapshots/nope/context", nil, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
