// The tolerances the parser extends to hand-written documents: join keys
// written either way round, and free-text type labels.
//
// Both are driven through Parse, which is the only way in from outside the
// package.
package parser_test

import (
	"testing"

	"urara-vision/backend/internal/parser"
	"urara-vision/backend/tests/fixtures"
)

// TestJoinKeySplitting drives the join-key parser through Parse, which is the
// only way in from outside the package. The two sides land on the relationship
// as written -- reorienting them against the tables' real columns is the graph
// builder's job, and is covered there.
func TestJoinKeySplitting(t *testing.T) {
	cases := []struct{ join, from, to string }{
		{"`beta_id_1 = beta_id`", "beta_id_1", "beta_id"},
		{"gamma_id = gamma_id", "gamma_id", "gamma_id"},
		{"`alpha_beta_id = beta_id`", "alpha_beta_id", "beta_id"},
		// No "=" at all: the whole cell is one column name and the far side is
		// left empty rather than guessed at.
		{"`some_id`", "some_id", ""},
	}
	for _, c := range cases {
		doc := fixtures.Doc("thing", "Fact", "D", []string{"a"},
			"| `dim_y` | "+c.join+" | Many-to-one |\n")
		res := parser.Parse([]parser.File{fixtures.File("d/thing.md", doc)})
		if len(res.Tables) != 1 || len(res.Tables[0].Relationships) != 1 {
			t.Fatalf("join %q: unexpected parse %+v", c.join, res.Tables)
		}
		r := res.Tables[0].Relationships[0]
		if r.FromColumn != c.from || r.ToColumn != c.to {
			t.Errorf("join %q = %q,%q want %q,%q", c.join, r.FromColumn, r.ToColumn, c.from, c.to)
		}
	}
}

// TestKindNormalisation drives type-label normalisation through Parse. The
// documents are named "thing" so the name-based fallback cannot classify them
// and the assertion is only about the Type cell.
func TestKindNormalisation(t *testing.T) {
	cases := map[string]string{
		"Fact":                  "fact",
		"Dimension":             "dimension",
		"Dimension (Conformed)": "dimension",
		"Dimension (Conformed — primary authority)": "dimension",
		"Dimension / slowly changing":               "dimension",
		"":                                          "unknown",
	}
	for in, want := range cases {
		doc := fixtures.Doc("thing", in, "D", []string{"a"}, "")
		res := parser.Parse([]parser.File{fixtures.File("d/thing.md", doc)})
		if len(res.Tables) != 1 {
			t.Fatalf("type %q: tables = %d", in, len(res.Tables))
		}
		if got := string(res.Tables[0].Kind); got != want {
			t.Errorf("type %q -> kind %q, want %q", in, got, want)
		}
		if got := res.Tables[0].KindRaw; got != in {
			t.Errorf("type %q: KindRaw = %q, want the label kept verbatim", in, got)
		}
	}
}

// TestKindFallsBackToName covers a document whose Type cell says nothing
// useful: the fact_/dim_ prefix is the last resort.
func TestKindFallsBackToName(t *testing.T) {
	cases := map[string]string{"fact_x": "fact", "dim_y": "dimension", "other_z": "unknown"}
	for name, want := range cases {
		doc := fixtures.Doc(name, "", "D", []string{"a"}, "")
		res := parser.Parse([]parser.File{fixtures.File("d/"+name+".md", doc)})
		if got := string(res.Tables[0].Kind); got != want {
			t.Errorf("%s -> kind %q, want %q", name, got, want)
		}
	}
}
