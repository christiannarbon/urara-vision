// Package graph resolves a parsed directory into a single connected model.
//
// The parser reads each document alone; everything that needs the whole
// directory in view happens here -- matching relationship targets to real
// tables, binding cross-domain references to a conformed instance, folding
// upstream source references onto one identity, and reporting whatever could
// not be satisfied.
//
// Build runs the stages below in order. The order is observable: diagnostics
// are appended as they are found and sorted stably at the end, so two findings
// of equal severity keep the sequence the stages produced.
package graph

import (
	"urara-vision/backend/internal/i18ntext"
	"urara-vision/backend/internal/model"
	"urara-vision/backend/internal/parser"
)

// Build resolves a parse result into a model, collecting diagnostics as it goes.
//
// The manifest comes in rather than being attached afterwards because the
// languages it declares decide how the documents' prose reads: what is a
// translation tag and what is an ordinary bracket is a question only the
// project's own language list answers.
func Build(snapshotID, name, sourceLabel string, meta model.ProjectMeta, pr *parser.Result) *model.Model {
	m := &model.Model{
		Domains:     pr.Domains,
		Tables:      pr.Tables,
		Diagnostics: append([]model.Diagnostic(nil), pr.Diagnostics...),
	}
	m.Snapshot.Project = meta

	reg := newRegistry(m.Tables, snapshotID)
	conformed := reg.markConformed(m.Tables)
	relationships := resolveRelationships(m, reg)
	lineageEdges, columns := collectSources(m, reg)
	countDomainTables(m, snapshotID)
	flagIsolatedTables(m)

	m.Diagnostics = append(m.Diagnostics, detectConformedDrift(reg.byName, reg.byID)...)
	languages := i18ntext.Check(m)
	m.Diagnostics = append(m.Diagnostics, languages.Diagnostics...)
	sortDiagnostics(m.Diagnostics)

	m.Snapshot = model.Snapshot{
		ID:          snapshotID,
		Name:        name,
		SourceLabel: sourceLabel,
		Project:     meta,
		Stats: model.Stats{
			Domains:       len(m.Domains),
			Tables:        len(m.Tables),
			Columns:       columns,
			Relationships: relationships,
			LineageEdges:  lineageEdges,
			SourceTables:  len(m.SourceTables),
			Conformed:     conformed,
			FilesParsed:   pr.Parsed,
			FilesSkipped:  pr.Skipped,
			Diagnostics:   len(m.Diagnostics),
			Translated:    languages.Translated,
		},
	}
	return m
}
