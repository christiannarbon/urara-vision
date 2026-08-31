// Walking a model's prose.
//
// Which fields carry translations is a decision made once, here: everything a
// reader reads as a sentence, and nothing an identifier is matched on. A table
// name resolved differently per language would resolve to a different graph
// per language, which is not internationalisation but a second model.
package i18ntext

import (
	"fmt"
	"strings"

	"urara-vision/backend/internal/model"
)

// Diagnostic codes this package raises.
const (
	// CodeDuplicate marks a field that tags one language twice.
	CodeDuplicate = "duplicate_language_tag"
	// CodeMissingPrimary marks a field translated out of nothing: it opens
	// with a tag, so the primary language has no text of its own.
	CodeMissingPrimary = "missing_primary_language"
)

// site is one prose field, with enough about where it came from to report on.
type site struct {
	field    string
	subject  string
	domainID string
	tableID  string
	docPath  string
	text     *string
}

// sites lists every field a document writes prose in. The pointers are into
// the model, so Resolve can write a language back through them.
func sites(m *model.Model) []site {
	out := make([]site, 0, len(m.Tables)*4)

	for i := range m.Domains {
		d := &m.Domains[i]
		out = append(out,
			site{field: "title", subject: d.Name, domainID: d.ID, docPath: d.DocPath, text: &d.Title},
			site{field: "description", subject: d.Name, domainID: d.ID, docPath: d.DocPath, text: &d.Description},
		)
	}

	for i := range m.Tables {
		t := &m.Tables[i]
		at := func(field string, text *string) site {
			return site{field: field, subject: t.Name, domainID: t.DomainID, tableID: t.ID, docPath: t.DocPath, text: text}
		}
		out = append(out,
			at("description", &t.Description),
			at("grain", &t.Grain),
			at("update frequency", &t.UpdateFrequency),
			at("relationship note", &t.RelationshipNote),
		)
		for j := range t.Notes {
			out = append(out, at("notes", &t.Notes[j]))
		}
		for j := range t.Columns {
			c := &t.Columns[j]
			out = append(out, at(fmt.Sprintf("description of column %s", c.Name), &c.Description))
		}
		for j := range t.ColumnLineage {
			l := &t.ColumnLineage[j]
			out = append(out, at(fmt.Sprintf("lineage note for column %s", l.Column), &l.Notes))
		}
	}
	return out
}

// Report is what one pass over a model's prose found.
type Report struct {
	// Translated is how many fields carry a language beyond the primary one.
	Translated int
	// Diagnostics are the problems found in the fields that do.
	Diagnostics []model.Diagnostic
}

// Check reads every prose field against the project's declared languages.
//
// A model whose manifest declares nothing -- an ingest older than the manifest
// -- has no tags to find, so this reports nothing rather than guessing.
func Check(m *model.Model) Report {
	s := New(m.Snapshot.Project.Internationalization)
	var rep Report

	for _, site := range sites(m) {
		if strings.TrimSpace(*site.text) == "" {
			continue
		}
		f := s.Split(*site.text)
		if !f.Translated() && len(f.Duplicated) == 0 {
			continue
		}
		if f.Translated() {
			rep.Translated++
		}

		for _, lang := range f.Duplicated {
			rep.Diagnostics = append(rep.Diagnostics, site.diagnostic(model.SeverityWarning, CodeDuplicate,
				fmt.Sprintf("The %s of %s tags %s more than once; the parts were joined in the order they appear.",
					site.field, site.subject, lang)))
		}
		if _, ok := f.ByLang[f.Primary]; !ok && f.Primary != "" {
			rep.Diagnostics = append(rep.Diagnostics, site.diagnostic(model.SeverityInfo, CodeMissingPrimary,
				fmt.Sprintf("The %s of %s opens with a %s tag, so %s -- the primary language -- has no text of its own and falls back to %s.",
					site.field, site.subject, f.Langs[0], f.Primary, f.Langs[0])))
		}
	}
	return rep
}

func (s site) diagnostic(severity, code, message string) model.Diagnostic {
	return model.Diagnostic{
		Severity: severity,
		Code:     code,
		Message:  message,
		DomainID: s.domainID,
		TableID:  s.tableID,
		DocPath:  s.docPath,
	}
}

// Resolve rewrites every prose field to one language, in place.
//
// It is for a consumer that wants a model in a single language -- uraractl's
// JSON, say. The UI does the opposite and keeps every language, because the
// reader can change their mind about which one they are reading without
// fetching the model again.
func Resolve(m *model.Model, lang string) {
	s := New(m.Snapshot.Project.Internationalization)
	for _, site := range sites(m) {
		if strings.TrimSpace(*site.text) == "" {
			continue
		}
		*site.text = s.Split(*site.text).In(lang)
	}
}
