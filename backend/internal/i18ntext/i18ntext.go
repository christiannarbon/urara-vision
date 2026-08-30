// Package i18ntext reads the translations a document carries inline.
//
// A project whose manifest declares `type = "inline"` keeps every language in
// the document itself, one field at a time, with a bracketed tag starting each
// translation:
//
//	This is a column [JP] これはコラムです。
//
// The text before the first tag is in the project's primary language, and each
// tag runs until the next one. A field with no tag at all is primary text
// whatever script it is written in -- nothing here guesses a language from the
// characters in it, so a field written only in Japanese under a primary of EN
// is EN text, and every reader sees it. That is the intended behaviour: an
// untranslated field is better shown than blanked.
//
// Only the languages the manifest lists are tags. `[TBD]` in a description is
// prose, not a translation, because TBD is not one of the project's languages.
package i18ntext

import (
	"regexp"
	"strings"

	"urara-vision/backend/internal/model"
)

// Splitter splits fields for one project's language set. It is built once per
// model and reused: the tag pattern is derived from the manifest, so building
// it per field would recompile the same expression thousands of times.
type Splitter struct {
	primary   string
	supported []string
	markers   *regexp.Regexp
}

// New builds a Splitter for a project's declared languages. A project with no
// languages -- an ingest older than the manifest -- yields a Splitter that
// finds no tags, so every field is read whole.
func New(i18n model.Internationalization) *Splitter {
	s := &Splitter{primary: NormaliseTag(i18n.Primary)}
	alts := make([]string, 0, len(i18n.Supported))
	for _, raw := range i18n.Supported {
		tag := NormaliseTag(raw)
		if tag == "" {
			continue
		}
		s.supported = append(s.supported, tag)
		alts = append(alts, regexp.QuoteMeta(tag))
	}
	if len(alts) > 0 {
		s.markers = regexp.MustCompile(`(?i)\[[ \t]*(` + strings.Join(alts, "|") + `)[ \t]*\]`)
	}
	return s
}

// Primary is the language a field's untagged text is read as.
func (s *Splitter) Primary() string { return s.primary }

// NormaliseTag upper-cases a language tag and trims it, so a manifest and a
// document written in different cases name the same language.
func NormaliseTag(v string) string { return strings.ToUpper(strings.TrimSpace(v)) }

// Field is one prose field, split into the languages it carries. A language
// whose text is empty is not carried: an abandoned `[JP]` with nothing after it
// leaves JP absent rather than blank, so In falls back instead of returning
// nothing.
type Field struct {
	// Primary is the language the untagged text was read as.
	Primary string
	// Langs are the languages present, in the order the field declares them.
	Langs []string
	// ByLang is each language's text, trimmed.
	ByLang map[string]string
	// Duplicated names any language the field tagged more than once. The parts
	// are joined rather than dropped; this is what reports that it happened.
	Duplicated []string
}

// Translated reports whether the field carries anything beyond its primary
// language.
func (f Field) Translated() bool {
	for _, l := range f.Langs {
		if l != f.Primary {
			return true
		}
	}
	return false
}

// In returns the field's text in one language.
//
// A language the field does not carry falls back to the primary language, and
// a field with no primary text falls back to whatever it does carry. Blank is
// returned only for a field that was blank to begin with: a reader asking for
// Japanese is better served English than nothing.
func (f Field) In(lang string) string {
	if t := f.ByLang[NormaliseTag(lang)]; t != "" {
		return t
	}
	if t := f.ByLang[f.Primary]; t != "" {
		return t
	}
	for _, l := range f.Langs {
		if t := f.ByLang[l]; t != "" {
			return t
		}
	}
	return ""
}

// Split reads one field.
func (s *Splitter) Split(text string) Field {
	f := Field{Primary: s.primary, ByLang: map[string]string{}}

	add := func(lang, body string) {
		body = strings.TrimSpace(body)
		if body == "" {
			return
		}
		if existing, ok := f.ByLang[lang]; ok {
			f.ByLang[lang] = existing + " " + body
			f.Duplicated = appendOnce(f.Duplicated, lang)
			return
		}
		f.ByLang[lang] = body
		f.Langs = append(f.Langs, lang)
	}

	if s.markers == nil {
		add(s.primary, text)
		return f
	}

	// Every match is a tag, and the text between one tag and the next is that
	// language's. What comes before the first tag is the primary language's.
	at := s.markers.FindAllStringSubmatchIndex(text, -1)
	if len(at) == 0 {
		add(s.primary, text)
		return f
	}

	add(s.primary, text[:at[0][0]])
	for i, m := range at {
		end := len(text)
		if i+1 < len(at) {
			end = at[i+1][0]
		}
		add(NormaliseTag(text[m[2]:m[3]]), text[m[1]:end])
	}
	return f
}

func appendOnce(xs []string, v string) []string {
	for _, x := range xs {
		if x == v {
			return xs
		}
	}
	return append(xs, v)
}
