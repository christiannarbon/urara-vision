// Inline translations: what a tag is, and which language a reader gets.
//
// The rules are short and the consequences are not, so the cases the format
// was specified with are pinned here verbatim -- both directions, since a
// project whose primary language is Japanese reads the same rules the other
// way round.
package i18ntext_test

import (
	"testing"

	"urara-vision/backend/internal/i18ntext"
	"urara-vision/backend/internal/model"
)

// langs builds a splitter for a project's declared languages.
func langs(primary string, supported ...string) *i18ntext.Splitter {
	return i18ntext.New(model.Internationalization{
		Primary:   primary,
		Supported: supported,
		Type:      "inline",
	})
}

// TestReadingEnglishFirst is the specified behaviour for a project whose
// primary language is English.
func TestReadingEnglishFirst(t *testing.T) {
	s := langs("EN", "EN", "JP")

	cases := []struct {
		name, text, en, jp string
	}{
		{
			name: "both languages",
			text: "This is a column [JP] これはコラムです。",
			en:   "This is a column",
			jp:   "これはコラムです。",
		},
		{
			// Untranslated: everyone reads the one language it has.
			name: "english only",
			text: "This is a column",
			en:   "This is a column",
			jp:   "This is a column",
		},
		{
			// Untagged text is primary text whatever script it is in. Nothing
			// guesses a language from the characters.
			name: "japanese only, untagged",
			text: "これはコラムです。",
			en:   "これはコラムです。",
			jp:   "これはコラムです。",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := s.Split(c.text)
			if got := f.In("EN"); got != c.en {
				t.Errorf("in EN = %q, want %q", got, c.en)
			}
			if got := f.In("JP"); got != c.jp {
				t.Errorf("in JP = %q, want %q", got, c.jp)
			}
		})
	}
}

// TestReadingJapaneseFirst is the same set of rules for a project that
// documents in Japanese and translates into English.
func TestReadingJapaneseFirst(t *testing.T) {
	s := langs("JP", "EN", "JP")

	cases := []struct {
		name, text, en, jp string
	}{
		{
			name: "both languages",
			text: "これはコラムです。 [EN] This is a column",
			en:   "This is a column",
			jp:   "これはコラムです。",
		},
		{
			name: "english only, untagged",
			text: "This is a column",
			en:   "This is a column",
			jp:   "This is a column",
		},
		{
			name: "japanese only",
			text: "これはコラムです。",
			en:   "これはコラムです。",
			jp:   "これはコラムです。",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := s.Split(c.text)
			if got := f.In("EN"); got != c.en {
				t.Errorf("in EN = %q, want %q", got, c.en)
			}
			if got := f.In("JP"); got != c.jp {
				t.Errorf("in JP = %q, want %q", got, c.jp)
			}
		})
	}
}

// TestOnlyDeclaredLanguagesAreTags: a bracket in a sentence is a bracket. The
// project's own language list is what makes one a tag, which is what keeps
// "[TBD]" and "[1]" out of it.
func TestOnlyDeclaredLanguagesAreTags(t *testing.T) {
	s := langs("EN", "EN", "JP")
	const text = "Pending [TBD] and cited [1] but translated [JP] 翻訳済み"

	f := s.Split(text)
	if got, want := f.In("EN"), "Pending [TBD] and cited [1] but translated"; got != want {
		t.Errorf("in EN = %q, want %q", got, want)
	}
	if got, want := f.In("JP"), "翻訳済み"; got != want {
		t.Errorf("in JP = %q, want %q", got, want)
	}
}

// TestTagsAreCaseInsensitive, so a document written [jp] and a manifest that
// says "JP" agree.
func TestTagsAreCaseInsensitive(t *testing.T) {
	s := langs("en", "en", "jp")
	f := s.Split("English [jp] 日本語")
	if got, want := f.In("JP"), "日本語"; got != want {
		t.Errorf("in JP = %q, want %q", got, want)
	}
	if got, want := f.In("En"), "English"; got != want {
		t.Errorf("in EN = %q, want %q", got, want)
	}
}

// TestFallback covers every way a reader can ask for something the field does
// not have. Blank is only ever returned for a field that was blank.
func TestFallback(t *testing.T) {
	s := langs("EN", "EN", "JP")

	t.Run("a language the project does not declare", func(t *testing.T) {
		f := s.Split("English [JP] 日本語")
		if got, want := f.In("FR"), "English"; got != want {
			t.Errorf("in FR = %q, want the primary language %q", got, want)
		}
	})

	t.Run("a tag with nothing after it", func(t *testing.T) {
		f := s.Split("English [JP]")
		if got, want := f.In("JP"), "English"; got != want {
			t.Errorf("in JP = %q, want %q", got, want)
		}
	})

	t.Run("nothing in the primary language", func(t *testing.T) {
		f := s.Split("[JP] 日本語だけ")
		if got, want := f.In("EN"), "日本語だけ"; got != want {
			t.Errorf("in EN = %q, want the only text there is (%q)", got, want)
		}
	})

	t.Run("an empty field", func(t *testing.T) {
		if got := s.Split("   ").In("EN"); got != "" {
			t.Errorf("in EN = %q, want empty", got)
		}
	})
}

// TestNoDeclaredLanguages: an ingest older than the manifest declares none, and
// its documents must read exactly as they did before.
func TestNoDeclaredLanguages(t *testing.T) {
	s := i18ntext.New(model.Internationalization{})
	const text = "This is a column [JP] これはコラムです。"

	if got := s.Split(text).In("EN"); got != text {
		t.Errorf("in EN = %q, want the field whole", got)
	}
}

// TestMultiLineFieldsKeepTheirShape: a description is a paragraph, and only the
// ends of a segment are trimmed.
func TestMultiLineFieldsKeepTheirShape(t *testing.T) {
	s := langs("EN", "EN", "JP")
	f := s.Split("First line.\nSecond line.\n\n[JP] 一行目。\n二行目。\n")

	if got, want := f.In("EN"), "First line.\nSecond line."; got != want {
		t.Errorf("in EN = %q, want %q", got, want)
	}
	if got, want := f.In("JP"), "一行目。\n二行目。"; got != want {
		t.Errorf("in JP = %q, want %q", got, want)
	}
}

// TestDuplicateTagsAreJoinedAndReported: dropping one half of a field would
// lose documentation, so both are kept and the duplication is what gets
// reported.
func TestDuplicateTagsAreJoinedAndReported(t *testing.T) {
	s := langs("EN", "EN", "JP")
	f := s.Split("English. [JP] 日本語。 [JP] 続き。")

	if got, want := f.In("JP"), "日本語。 続き。"; got != want {
		t.Errorf("in JP = %q, want the parts joined (%q)", got, want)
	}
	if len(f.Duplicated) != 1 || f.Duplicated[0] != "JP" {
		t.Errorf("duplicated = %v, want [JP]", f.Duplicated)
	}
}

// TestTranslatedReportsWhetherAnyLanguageWasAdded.
func TestTranslatedReportsWhetherAnyLanguageWasAdded(t *testing.T) {
	s := langs("EN", "EN", "JP")
	if s.Split("English only").Translated() {
		t.Error("an untranslated field reported itself as translated")
	}
	if !s.Split("English [JP] 日本語").Translated() {
		t.Error("a translated field reported itself as untranslated")
	}
}
