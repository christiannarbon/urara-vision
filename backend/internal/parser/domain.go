// Reading a domain index document: the directory-level description, its
// diagram, and the upstream models feeding the tables it proposes.
package parser

import (
	"path"
	"strings"

	"urara-vision/backend/internal/model"
)

// parseDomainDoc reads one domain index document.
func parseDomainDoc(f File, secs []section) model.Domain {
	base := strings.TrimSuffix(path.Base(f.Path), path.Ext(f.Path))
	d := model.Domain{
		ID:      base,
		Name:    base,
		DocPath: f.Path,
	}
	d.Title = strings.TrimSpace(firstHeading(secs))
	if d.Title == "" {
		d.Title = humanise(base)
	}

	if body, ok := findSection(secs, "Description"); ok {
		d.Description = prose(stripTables(body))
	}
	if body, ok := findSection(secs, domainDiagramHeadings...); ok {
		d.Mermaid = mermaidBlock(body)
	}
	if d.Mermaid == "" {
		d.Mermaid = mermaidBlock(f.Content)
	}

	if body, ok := findSection(secs, "Lineage"); ok {
		for _, tbl := range parseTables(body) {
			pi := headerIndex(tbl.Header, "proposed table", "table")
			si := headerIndex(tbl.Header, "source model", "source")
			if pi < 0 || si < 0 {
				continue
			}
			for _, row := range tbl.Rows {
				name := unticked(cell(row, pi))
				if name == "" {
					continue
				}
				sources := allTicked(cell(row, si))
				if len(sources) == 0 {
					if v := strings.TrimSpace(cell(row, si)); v != "" {
						sources = []string{v}
					}
				}
				d.Lineage = append(d.Lineage, model.DomainLineage{
					ProposedTable: name,
					SourceModels:  sources,
				})
			}
		}
	}
	return d
}
