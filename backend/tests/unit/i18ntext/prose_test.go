// Which fields carry translations, what an ingest reports about them, and
// what resolving a whole model to one language does.
package i18ntext_test

import (
	"strings"
	"testing"

	"urara-vision/backend/internal/i18ntext"
	"urara-vision/backend/internal/model"
	"urara-vision/backend/internal/parser"
	"urara-vision/backend/tests/fixtures"
)

// bilingual builds a model whose prose is written in both languages, the way a
// parsed one arrives.
func bilingual() *model.Model {
	return &model.Model{
		Snapshot: model.Snapshot{ID: "snap", Project: fixtures.ProjectMeta()},
		Domains: []model.Domain{{
			ID:          "ordering",
			Name:        "ordering",
			Title:       "Ordering [JP] 受注",
			Description: "Orders and their lines. [JP] 受注とその明細。",
			DocPath:     "ordering.md",
		}},
		Tables: []model.Table{{
			ID:              "ordering/fact_orders",
			Name:            "fact_orders",
			DomainID:        "ordering",
			DocPath:         "ordering/fact_orders.md",
			Description:     "One row per order. [JP] 受注ごとに 1 行。",
			Grain:           "Order [JP] 受注",
			UpdateFrequency: "Daily",
			Notes:           []string{"Excludes cancelled rows. [JP] キャンセル分を除きます。"},
			Columns: []model.Column{
				{Name: "order_id", Description: "This is a column [JP] これはコラムです。"},
			},
			ColumnLineage: []model.ColumnLineage{
				{Column: "order_id", Notes: "Primary Key [JP] 主キー"},
			},
		}},
	}
}

// TestCheckCountsTranslatedFields across every field a document writes prose
// in, and no others.
func TestCheckCountsTranslatedFields(t *testing.T) {
	rep := i18ntext.Check(bilingual())

	// Title, domain description, table description, grain, note, column
	// description, lineage note. The update frequency carries one language, so
	// it is not among them.
	if rep.Translated != 7 {
		t.Errorf("translated = %d, want 7", rep.Translated)
	}
	if len(rep.Diagnostics) != 0 {
		t.Errorf("a well-formed model produced diagnostics: %+v", rep.Diagnostics)
	}
}

// TestCheckReportsADuplicateTag, since the joined result reads as a mistake
// and nothing else would say where it came from.
func TestCheckReportsADuplicateTag(t *testing.T) {
	m := bilingual()
	m.Tables[0].Description = "One row per order. [JP] 受注ごとに 1 行。 [JP] 続き。"

	rep := i18ntext.Check(m)
	d, ok := find(rep.Diagnostics, i18ntext.CodeDuplicate)
	if !ok {
		t.Fatalf("no %s diagnostic: %+v", i18ntext.CodeDuplicate, rep.Diagnostics)
	}
	if d.Severity != model.SeverityWarning {
		t.Errorf("severity = %q, want warning", d.Severity)
	}
	if d.TableID != "ordering/fact_orders" || d.DocPath != "ordering/fact_orders.md" {
		t.Errorf("diagnostic does not point at the document: %+v", d)
	}
	for _, want := range []string{"description", "fact_orders", "JP"} {
		if !strings.Contains(d.Message, want) {
			t.Errorf("message = %q, want it to mention %q", d.Message, want)
		}
	}
}

// TestCheckReportsAMissingPrimaryLanguage: the field still reads, but every
// English reader is being shown Japanese, which is worth knowing.
func TestCheckReportsAMissingPrimaryLanguage(t *testing.T) {
	m := bilingual()
	m.Tables[0].Description = "[JP] 受注ごとに 1 行。"

	rep := i18ntext.Check(m)
	d, ok := find(rep.Diagnostics, i18ntext.CodeMissingPrimary)
	if !ok {
		t.Fatalf("no %s diagnostic: %+v", i18ntext.CodeMissingPrimary, rep.Diagnostics)
	}
	if d.Severity != model.SeverityInfo {
		t.Errorf("severity = %q, want info: the fallback works, it is just worth saying", d.Severity)
	}
}

// TestCheckSaysNothingWithoutAManifest, so an ingest older than the manifest
// gains no diagnostics it cannot act on.
func TestCheckSaysNothingWithoutAManifest(t *testing.T) {
	m := bilingual()
	m.Snapshot.Project = model.ProjectMeta{}

	rep := i18ntext.Check(m)
	if rep.Translated != 0 || len(rep.Diagnostics) != 0 {
		t.Errorf("report = %+v, want an empty one", rep)
	}
}

// TestResolveRewritesEveryProseFieldToOneLanguage, and leaves the identifiers
// alone: a name resolved per language would be a different graph per language.
func TestResolveRewritesEveryProseFieldToOneLanguage(t *testing.T) {
	m := bilingual()
	i18ntext.Resolve(m, "JP")

	tb := m.Tables[0]
	checks := map[string]string{
		"domain title":       m.Domains[0].Title,
		"domain description": m.Domains[0].Description,
		"description":        tb.Description,
		"grain":              tb.Grain,
		"note":               tb.Notes[0],
		"column description": tb.Columns[0].Description,
		"lineage note":       tb.ColumnLineage[0].Notes,
	}
	for field, got := range checks {
		if strings.Contains(got, "[JP]") {
			t.Errorf("%s still carries a tag: %q", field, got)
		}
		if containsASCIILetters(got) {
			t.Errorf("%s = %q, want only the Japanese text", field, got)
		}
	}

	if tb.Name != "fact_orders" || tb.ID != "ordering/fact_orders" {
		t.Errorf("an identifier was rewritten: %q / %q", tb.Name, tb.ID)
	}
	// A field written in one language stays as it is rather than emptying.
	if tb.UpdateFrequency != "Daily" {
		t.Errorf("update frequency = %q, want it left alone", tb.UpdateFrequency)
	}
}

// TestAnIngestReadsTheLanguagesTheManifestDeclares: the check is wired into
// the build rather than bolted on afterwards, so a parsed directory arrives
// with its translation count and findings already in the snapshot.
func TestAnIngestReadsTheLanguagesTheManifestDeclares(t *testing.T) {
	doc := fixtures.Doc("fact_orders", "Fact", "Ordering", []string{"order_id"}, "")
	doc = strings.Replace(doc,
		"| `order_id` | STRING | col |",
		"| `order_id` | STRING | This is a column [JP] これはコラムです。 |", 1)

	m := fixtures.Build([]parser.File{fixtures.File("ordering/fact_orders.md", doc)})

	if m.Snapshot.Stats.Translated != 1 {
		t.Errorf("translated = %d, want 1", m.Snapshot.Stats.Translated)
	}
	if m.Snapshot.Project.Internationalization.Primary != "EN" {
		t.Errorf("the manifest did not reach the snapshot: %+v", m.Snapshot.Project)
	}
	if got := m.Tables[0].Columns[0].Description; !strings.Contains(got, "[JP]") {
		t.Errorf("column description = %q, want both languages kept as written", got)
	}
}

func find(ds []model.Diagnostic, code string) (model.Diagnostic, bool) {
	for _, d := range ds {
		if d.Code == code {
			return d, true
		}
	}
	return model.Diagnostic{}, false
}

func containsASCIILetters(s string) bool {
	return strings.ContainsFunc(s, func(r rune) bool {
		return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
	})
}
