package chateval_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"urara-vision/backend/internal/chateval"
)

var (
	evalDir = filepath.Join("..", "..", "eval")
	demoDir = filepath.Join("..", "..", "..", "..", "docs", "demo")
)

func load(t *testing.T) []chateval.Question {
	t.Helper()
	qs, err := chateval.Load(filepath.Join(evalDir, "questions.yaml"))
	if err != nil || len(qs) == 0 {
		t.Fatalf("%d questions, %v", len(qs), err)
	}
	return qs
}

func TestQuestionsLoadAndEverySetExists(t *testing.T) {
	for _, q := range load(t) {
		if q.ID == "" || q.Set == "" || q.Language == "" || q.Question == "" || q.Category == "" {
			t.Errorf("%q: a required field is empty", q.ID)
		}
		found := false
		for _, dir := range []string{demoDir, filepath.Join(evalDir, "fixtures")} {
			if info, err := os.Stat(filepath.Join(dir, q.Set)); err == nil && info.IsDir() {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: set %q is in neither docs/demo nor tests/eval/fixtures", q.ID, q.Set)
		}
	}
}

func TestAbsentAndEmptyListsStayDistinct(t *testing.T) {
	path := filepath.Join(t.TempDir(), "q.yaml")
	_ = os.WriteFile(path, []byte("- id: q\n  expect_citations: []\n"), 0o600)
	qs, err := chateval.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if qs[0].ExpectCitations == nil || len(qs[0].ExpectCitations) != 0 {
		t.Errorf("[] loaded as %#v", qs[0].ExpectCitations)
	}
	if qs[0].AllowCitations != nil {
		t.Errorf("absent loaded as %#v", qs[0].AllowCitations)
	}
}

func TestSelect(t *testing.T) {
	qs := load(t)
	if got := chateval.Select(qs, nil, nil, nil); len(got) != len(qs) {
		t.Errorf("no filter kept %d of %d", len(got), len(qs))
	}
	got := chateval.Select(qs, []string{"jaffle-shop-ddd"}, []string{"lookup", "refusal"}, nil)
	if len(got) == 0 {
		t.Fatal("nothing selected")
	}
	for _, q := range got {
		if q.Set != "jaffle-shop-ddd" || (q.Category != "lookup" && q.Category != "refusal") {
			t.Errorf("%s (%s, %s) selected", q.ID, q.Set, q.Category)
		}
	}
	if got := chateval.Select(qs, nil, nil, []string{"jaffle-grain-orders"}); len(got) != 1 {
		t.Errorf("by id: %d", len(got))
	}
}

func loadText(t *testing.T, text string) error {
	t.Helper()
	path := filepath.Join(t.TempDir(), "q.yaml")
	_ = os.WriteFile(path, []byte(text), 0o600)
	_, err := chateval.Load(path)
	return err
}

func TestLoadRefusesUnknownKeys(t *testing.T) {
	if err := loadText(t, "- id: q\n  expect_citation: [a/b]\n"); err == nil || !strings.Contains(err.Error(), "expect_citation") {
		t.Errorf("err = %v", err)
	}
}

func TestLoadRefusesDuplicateIDs(t *testing.T) {
	if err := loadText(t, "- id: q\n- id: q\n"); err == nil || !strings.Contains(err.Error(), `"q"`) {
		t.Errorf("err = %v", err)
	}
}
