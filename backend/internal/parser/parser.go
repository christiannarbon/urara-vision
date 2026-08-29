// Package parser turns a directory of markdown model documents into tables and
// domains, whatever modelling style they are written in.
//
// Nothing here looks beyond the document in hand: relationship targets are left
// as written, and matching them against the rest of the directory is the graph
// package's job.
package parser

import (
	"path"
	"sort"
	"strings"

	"urara-vision/backend/internal/model"
)

// File is one markdown document handed to the parser.
type File struct {
	// Path is relative to the selected root, e.g. "domain_one/fact_primary.md".
	Path    string
	Content string
}

// Result is the parser's output before graph resolution.
type Result struct {
	Domains     []model.Domain
	Tables      []model.Table
	Diagnostics []model.Diagnostic
	Parsed      int
	Skipped     int
}

// The headings a domain index announces itself with. Star schema wording is
// only the first of them: the tool reads whatever modelling style a directory
// is documented in, so a snowflake, a Data Vault or a plain ERD names its
// diagram section its own way and must be recognised just the same.
//
// The bare "Diagram" is deliberately absent here and present in
// domainDiagramHeadings. Reading a diagram out of a document already known to
// be an index can afford to be generous; deciding what a document *is* cannot,
// or a table document that happens to carry a diagram stops being a table.
var diagramHeadings = []string{
	"Star Schema Diagram",
	"Snowflake Schema Diagram",
	"Schema Diagram",
	"Data Model Diagram",
	"Model Diagram",
	"Entity Relationship Diagram",
	"ER Diagram",
	"ERD",
}

// domainDiagramHeadings is what parseDomainDoc looks under, once a document has
// already been classified as an index.
var domainDiagramHeadings = append(append([]string(nil), diagramHeadings...), "Diagram")

// proposedHeadings are the headings a domain index lists its planned tables
// under.
var proposedHeadings = []string{
	"Proposed Star Schema",
	"Proposed Snowflake Schema",
	"Proposed Schema",
	"Proposed Data Model",
	"Proposed Model",
	"Proposed Tables",
}

// docKind distinguishes a per-table document from a domain index.
type docKind int

const (
	docTable docKind = iota
	docDomainIndex
	docUnknown
)

// classify decides what a document is from its headings, falling back to its
// position in the tree. Content wins over path so a reorganised directory still
// parses correctly.
func classify(f File, secs []section) docKind {
	_, hasCols := findSection(secs, "Columns")
	_, hasOverview := findSection(secs, "Overview")
	if hasCols && hasOverview {
		return docTable
	}
	if _, ok := findSection(secs, diagramHeadings...); ok {
		return docDomainIndex
	}
	if _, ok := findSection(secs, proposedHeadings...); ok {
		return docDomainIndex
	}
	if strings.Contains(f.Path, "/") {
		if hasCols {
			return docTable
		}
		return docUnknown
	}
	if _, ok := findSection(secs, "Description"); ok {
		return docDomainIndex
	}
	return docUnknown
}

// Parse reads every markdown file and produces domains and tables. Files are
// processed in a stable order so repeated ingests of the same directory yield
// identical output.
func Parse(files []File) *Result {
	res := &Result{}
	sorted := append([]File(nil), files...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })

	seenDomains := map[string]bool{}

	for _, f := range sorted {
		if !strings.EqualFold(path.Ext(f.Path), ".md") {
			res.Skipped++
			continue
		}
		if strings.TrimSpace(f.Content) == "" {
			res.Skipped++
			res.Diagnostics = append(res.Diagnostics, model.Diagnostic{
				Severity: model.SeverityWarning,
				Code:     "empty_document",
				Message:  "Document is empty and was skipped.",
				DocPath:  f.Path,
			})
			continue
		}
		secs := splitSections(f.Content)
		switch classify(f, secs) {
		case docTable:
			t, diags := parseTableDoc(f, secs)
			res.Tables = append(res.Tables, t)
			res.Diagnostics = append(res.Diagnostics, diags...)
			res.Parsed++
		case docDomainIndex:
			d := parseDomainDoc(f, secs)
			seenDomains[d.ID] = true
			res.Domains = append(res.Domains, d)
			res.Parsed++
		default:
			res.Skipped++
			res.Diagnostics = append(res.Diagnostics, model.Diagnostic{
				Severity: model.SeverityInfo,
				Code:     "unrecognised_document",
				Message:  "Document matched neither a table nor a domain index layout; ignored.",
				DocPath:  f.Path,
			})
		}
	}

	// A table directory with no index document still forms a domain.
	for i := range res.Tables {
		id := res.Tables[i].DomainID
		if id == "" || seenDomains[id] {
			continue
		}
		seenDomains[id] = true
		res.Domains = append(res.Domains, model.Domain{
			ID:    id,
			Name:  id,
			Title: humanise(id),
		})
		res.Diagnostics = append(res.Diagnostics, model.Diagnostic{
			Severity: model.SeverityWarning,
			Code:     "missing_domain_index",
			Message:  "Domain has table documents but no index document; synthesised from the directory name.",
			DomainID: id,
		})
	}

	sort.Slice(res.Domains, func(i, j int) bool { return res.Domains[i].ID < res.Domains[j].ID })
	sort.Slice(res.Tables, func(i, j int) bool { return res.Tables[i].ID < res.Tables[j].ID })
	return res
}
