package notes_test

import (
	"errors"
	"strings"
	"testing"

	"urara-vision/backend/internal/notes"
)

func TestParseKind(t *testing.T) {
	for _, s := range []string{"domain", "table", "column", "relationship", "lineage"} {
		if k, err := notes.ParseKind(s); err != nil || string(k) != s {
			t.Errorf("ParseKind(%q) = %q, %v", s, k, err)
		}
	}
	for _, s := range []string{"", "Table", "note", "columns"} {
		if _, err := notes.ParseKind(s); err == nil {
			t.Errorf("ParseKind(%q) accepted", s)
		}
	}
}

func TestSplitColumnAnchor(t *testing.T) {
	cases := []struct{ id, table, column string }{
		{"a/b#c", "a/b", "c"},
		{"a/b#c#d", "a/b#c", "d"},
	}
	for _, c := range cases {
		table, column, err := notes.SplitColumnAnchor(c.id)
		if err != nil || table != c.table || column != c.column {
			t.Errorf("SplitColumnAnchor(%q) = %q, %q, %v", c.id, table, column, err)
		}
	}
	for _, id := range []string{"a/b", "", "#c", "a/b#"} {
		if _, _, err := notes.SplitColumnAnchor(id); err == nil {
			t.Errorf("SplitColumnAnchor(%q) accepted", id)
		}
	}
}

func TestCheckBody(t *testing.T) {
	if got, err := notes.CheckBody("  hello\n"); err != nil || got != "hello" {
		t.Errorf("CheckBody trims: %q, %v", got, err)
	}
	if _, err := notes.CheckBody(" \n\t"); !errors.Is(err, notes.ErrEmptyBody) {
		t.Errorf("blank body err = %v", err)
	}
	max := strings.Repeat("é", notes.MaxBodyRunes)
	if got, err := notes.CheckBody(max); err != nil || got != max {
		t.Errorf("%d runes refused: %v", notes.MaxBodyRunes, err)
	}
	if _, err := notes.CheckBody(max + "é"); !errors.Is(err, notes.ErrBodyTooLong) {
		t.Errorf("%d runes err = %v", notes.MaxBodyRunes+1, err)
	}
}
