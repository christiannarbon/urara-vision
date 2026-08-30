// The two shipped bilingual demo documentation sets, parsed and resolved.
//
// docs/demo/chinook-bilingual-ddd is documented in English and translated into
// Japanese; docs/demo/superstore-jp-ddd is documented in Japanese and
// translated into English. Between them they are the sets that prove inline
// translations work in both directions, so what they resolve to is pinned here
// exactly as the other sets are.
//
// The assertions below are the specified behaviour of the format, read out of
// real documents rather than a fixture: a tagged field gives each reader their
// own language, an untagged field gives every reader the same one whatever
// script it is written in, and an untranslated field falls back rather than
// blanking.
package demo_test

import (
	"strings"
	"testing"

	"urara-vision/backend/internal/graph"
	"urara-vision/backend/internal/i18ntext"
	"urara-vision/backend/internal/model"
)

const (
	chinookDir    = "../../../../docs/demo/chinook-bilingual-ddd"
	superstoreDir = "../../../../docs/demo/superstore-jp-ddd"
)

func loadChinook(t *testing.T) *model.Model    { return loadSet(t, chinookDir) }
func loadSuperstore(t *testing.T) *model.Model { return loadSet(t, superstoreDir) }

// field returns one table's field from a set, by table name.
func field(t *testing.T, m *model.Model, table string, get func(model.Table) string) string {
	t.Helper()
	for _, tb := range m.Tables {
		if tb.Name == table {
			return get(tb)
		}
	}
	t.Fatalf("no table named %s in the set", table)
	return ""
}

// reads resolves one field into a language the way the interface does.
func reads(m *model.Model, text, lang string) string {
	return i18ntext.New(m.Snapshot.Project.Internationalization).Split(text).In(lang)
}

func TestChinookStats(t *testing.T) {
	m := loadChinook(t)
	s := m.Snapshot.Stats
	checkStats(t, []stat{
		{"files parsed", s.FilesParsed, 12},
		{"files skipped", s.FilesSkipped, 0},
		{"domains", s.Domains, 4},
		{"tables", s.Tables, 8},
		{"columns", s.Columns, 42},
		{"conformed instances", s.Conformed, 1},
		{"declared relationships", s.Relationships, 13},
		{"normalised edges", len(graph.Edges(m)), 9},
		{"lineage edges", s.LineageEdges, 41},
		{"source tables", s.SourceTables, 10},
		{"translated fields", s.Translated, 115},
	})
}

// TestChinookDiagnostics pins the flaws the set is built around, two of which
// are flaws in the translations rather than in the model.
func TestChinookDiagnostics(t *testing.T) {
	checkDiagnostics(t, loadChinook(t), map[string]int{
		"unresolved_reference":     1, // dim_promotion, a feed that has not shipped
		"cross_domain_reference":   6, // every dimension the fact reads is owned elsewhere
		"duplicate_language_tag":   1, // dim_date's description tags Japanese twice
		"missing_primary_language": 1, // a note on dim_customer opens with a tag
		"undocumented_lineage":     1, // prose in the Source Table column
	}, 1) // exactly one error, so `uraractl -strict` on the set exits non-zero
}

// TestChinookReadsBothWays is the headline case: one field, two readers, two
// languages, from a document nobody wrote twice.
func TestChinookReadsBothWays(t *testing.T) {
	m := loadChinook(t)
	grain := field(t, m, "dim_track", func(tb model.Table) string { return tb.Grain })

	if got, want := reads(m, grain, "EN"), "One row per track."; got != want {
		t.Errorf("dim_track grain in EN = %q, want %q", got, want)
	}
	if got, want := reads(m, grain, "JP"), "楽曲ごとに 1 行。"; got != want {
		t.Errorf("dim_track grain in JP = %q, want %q", got, want)
	}
}

// TestChinookUntaggedFieldsReadTheSameInBothLanguages: dim_genre is documented
// only in Japanese and tags nothing, and dim_employee only in English. Neither
// is a translation, so both are shown to every reader as written.
func TestChinookUntaggedFieldsReadTheSameInBothLanguages(t *testing.T) {
	m := loadChinook(t)

	japaneseOnly := field(t, m, "dim_genre", func(tb model.Table) string { return tb.Description })
	if !strings.HasPrefix(japaneseOnly, "ジャンルは") {
		t.Fatalf("dim_genre description = %q, want the Japanese the document carries", japaneseOnly)
	}
	if en, jp := reads(m, japaneseOnly, "EN"), reads(m, japaneseOnly, "JP"); en != jp {
		t.Errorf("an untagged Japanese field read differently per language:\n EN %q\n JP %q", en, jp)
	}

	englishOnly := field(t, m, "dim_employee", func(tb model.Table) string { return tb.Description })
	if en, jp := reads(m, englishOnly, "EN"), reads(m, englishOnly, "JP"); en != jp {
		t.Errorf("an untranslated English field read differently per language:\n EN %q\n JP %q", en, jp)
	}
}

func TestSuperstoreStats(t *testing.T) {
	m := loadSuperstore(t)
	s := m.Snapshot.Stats
	checkStats(t, []stat{
		{"files parsed", s.FilesParsed, 9},
		{"files skipped", s.FilesSkipped, 0},
		{"domains", s.Domains, 4},
		{"tables", s.Tables, 5},
		{"columns", s.Columns, 29},
		{"conformed instances", s.Conformed, 1},
		{"declared relationships", s.Relationships, 9},
		{"normalised edges", len(graph.Edges(m)), 4},
		{"lineage edges", s.LineageEdges, 28},
		{"source tables", s.SourceTables, 5},
		{"translated fields", s.Translated, 75},
	})
}

func TestSuperstoreDiagnostics(t *testing.T) {
	checkDiagnostics(t, loadSuperstore(t), map[string]int{
		"unresolved_reference":   1, // dim_region, a master nobody has documented
		"cross_domain_reference": 6, // the fact is read from three contexts that own no fact
		"undocumented_lineage":   1, // prose in the Source Table column
	}, 1)
}

// TestSuperstoreReadsBothWays is the mirror of the Chinook case: the same rules
// applied to a project that documents in Japanese and translates into English.
func TestSuperstoreReadsBothWays(t *testing.T) {
	m := loadSuperstore(t)
	if got, want := m.Snapshot.Project.Internationalization.Primary, "JP"; got != want {
		t.Fatalf("primary language = %q, want %q", got, want)
	}

	grain := field(t, m, "dim_product", func(tb model.Table) string { return tb.Grain })
	if got, want := reads(m, grain, "JP"), "商品ごとに 1 行。"; got != want {
		t.Errorf("dim_product grain in JP = %q, want %q", got, want)
	}
	if got, want := reads(m, grain, "EN"), "One row per product."; got != want {
		t.Errorf("dim_product grain in EN = %q, want %q", got, want)
	}

	// An English-only note in a Japanese-primary set: untranslated, so both
	// readers get it as written.
	note := field(t, m, "dim_product", func(tb model.Table) string { return tb.Notes[0] })
	if en, jp := reads(m, note, "EN"), reads(m, note, "JP"); en != jp {
		t.Errorf("an untranslated note read differently per language:\n EN %q\n JP %q", en, jp)
	}
}

// TestBilingualSetsTranslateNoIdentifiers. A name that resolved per language
// would resolve to a different graph per language, so the sets are checked for
// a tag having crept into one.
func TestBilingualSetsTranslateNoIdentifiers(t *testing.T) {
	for _, m := range []*model.Model{loadChinook(t), loadSuperstore(t)} {
		for _, tb := range m.Tables {
			if strings.ContainsAny(tb.Name+tb.ID, "[]") {
				t.Errorf("table identifier carries a tag: %q / %q", tb.ID, tb.Name)
			}
			for _, c := range tb.Columns {
				if strings.ContainsAny(c.Name, "[]") {
					t.Errorf("%s: column name carries a tag: %q", tb.ID, c.Name)
				}
			}
		}
	}
}
