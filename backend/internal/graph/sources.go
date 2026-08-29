// Upstream source models, read from the documents' column-level lineage.
//
// Documents cite the same upstream model inconsistently -- with and without its
// dataset -- and sometimes cite prose ("not available") where a model name
// belongs. Both are handled here, because a source that resolves to two nodes
// makes "what else reads this?" quietly wrong.
package graph

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"urara-vision/backend/internal/model"
)

// collectSources folds the lineage references onto canonical source models and
// records them on the model, returning the number of lineage edges and the
// total column count.
func collectSources(m *model.Model, reg *registry) (lineageEdges, colCount int) {
	rewrite := canonicaliseSources(m.Tables)
	srcRefs := map[string]int{}
	undocumented := map[string]int{}
	for i := range m.Tables {
		t := &m.Tables[i]
		colCount += len(t.Columns)
		for j := range t.ColumnLineage {
			l := &t.ColumnLineage[j]
			if l.SourceTable == "" {
				continue
			}
			if !isSourceRef(l.SourceTable) {
				// Keep the text on the column so the detail pane still shows
				// what the document said, but do not model it as a source.
				undocumented[t.ID]++
				continue
			}
			if canon, ok := rewrite[l.SourceTable]; ok {
				l.SourceTable = canon
			}
			srcRefs[l.SourceTable]++
			lineageEdges++
		}
	}

	for tableID, n := range undocumented {
		t := reg.byID[tableID]
		if t == nil {
			continue
		}
		m.Diagnostics = append(m.Diagnostics, model.Diagnostic{
			Severity: model.SeverityInfo,
			Code:     "undocumented_lineage",
			Message: fmt.Sprintf("%s has %d column(s) whose source is recorded as prose rather than a model name; those are excluded from the lineage graph.",
				t.Name, n),
			DomainID: t.DomainID,
			TableID:  t.ID,
			DocPath:  t.DocPath,
		})
	}
	for id, refs := range srcRefs {
		dataset, tname := splitQualified(id)
		m.SourceTables = append(m.SourceTables, model.SourceTable{
			ID:      id,
			Dataset: dataset,
			Name:    tname,
			Refs:    refs,
		})
	}
	sort.Slice(m.SourceTables, func(i, j int) bool { return m.SourceTables[i].ID < m.SourceTables[j].ID })
	return lineageEdges, colCount
}

// sourceRefRe matches a real upstream model reference: a bare identifier or a
// dataset-qualified one. Anything else -- "not available", "N/A", "GA event
// models" -- is prose sitting in the Source Table column, not a model.
var sourceRefRe = regexp.MustCompile(`^[A-Za-z0-9_]+(\.[A-Za-z0-9_]+)*$`)

// isSourceRef reports whether a lineage cell names an actual upstream model.
func isSourceRef(s string) bool {
	return s != "" && sourceRefRe.MatchString(s)
}

// canonicaliseSources builds a rewrite map that folds unqualified references
// onto their dataset-qualified form. Documents are inconsistent: one table
// cites "warehouse.upstream_model" while another cites the same model as
// "upstream_model". Left alone they become
// two unrelated lineage nodes, which silently breaks "what else reads this?".
func canonicaliseSources(tables []model.Table) map[string]string {
	qualified := map[string]string{} // bare name -> qualified id
	ambiguous := map[string]bool{}   // bare name seen under two datasets

	for i := range tables {
		for _, l := range tables[i].ColumnLineage {
			if !isSourceRef(l.SourceTable) {
				continue
			}
			dataset, name := splitQualified(l.SourceTable)
			if dataset == "" {
				continue
			}
			if prev, ok := qualified[name]; ok && prev != l.SourceTable {
				ambiguous[name] = true
				continue
			}
			qualified[name] = l.SourceTable
		}
	}

	rewrite := map[string]string{}
	for name, id := range qualified {
		// A bare name that maps to two different datasets cannot be resolved
		// without guessing, so it is left as written.
		if ambiguous[name] {
			continue
		}
		rewrite[name] = id
	}
	return rewrite
}

// splitQualified separates "dataset.table" into its parts.
func splitQualified(id string) (string, string) {
	if i := strings.LastIndex(id, "."); i > 0 {
		return id[:i], id[i+1:]
	}
	return "", id
}
