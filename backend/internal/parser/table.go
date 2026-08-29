// Reading one per-table document: its Overview properties, columns, column
// lineage, declared relationships and caveats.
package parser

import (
	"path"
	"regexp"
	"strings"

	"urara-vision/backend/internal/model"
)

var (
	pkHintRe     = regexp.MustCompile(`(?i)\bPK\b|\bprimary key\b`)
	fkHintRe     = regexp.MustCompile(`(?i)\bFK\b|\bforeign key\b`)
	derivedRe    = regexp.MustCompile(`(?i)^\s*derived\b`)
	nonTableRe   = regexp.MustCompile(`^[a-z0-9_]+$`)
	whitespaceRe = regexp.MustCompile(`\s+`)
)

// parseTableDoc reads one per-table document.
func parseTableDoc(f File, secs []section) (model.Table, []model.Diagnostic) {
	var diags []model.Diagnostic
	domainID := path.Base(path.Dir(f.Path))
	if domainID == "." || domainID == "/" {
		domainID = ""
	}
	fileBase := strings.TrimSuffix(path.Base(f.Path), path.Ext(f.Path))

	t := model.Table{
		DomainID: domainID,
		DocPath:  f.Path,
		Kind:     model.KindUnknown,
	}

	props, overview := parseOverview(secs)

	t.Name = unticked(props["table name"])
	if t.Name == "" {
		t.Name = strings.TrimSpace(firstHeading(secs))
	}
	if t.Name == "" {
		t.Name = fileBase
	}
	if t.Name != fileBase && fileBase != "" {
		diags = append(diags, model.Diagnostic{
			Severity: model.SeverityInfo,
			Code:     "name_filename_mismatch",
			Message:  "Declared table name \"" + t.Name + "\" differs from the file name \"" + fileBase + "\".",
			DomainID: domainID,
			DocPath:  f.Path,
		})
	}

	t.KindRaw = unticked(props["type"])
	t.Kind = normaliseKind(t.KindRaw)
	if t.Kind == model.KindUnknown {
		t.Kind = kindFromName(t.Name)
	}
	t.Grain = unticked(props["grain"])
	t.UpdateFrequency = unticked(props["update frequency"])
	t.Layer = unticked(props["layer"])
	t.DomainLabel = unticked(props["domain"])
	t.ID = domainID + "/" + t.Name
	t.Description = prose(stripTables(overview))

	// Columns.
	colSec, _ := findSection(secs, "Columns")
	for _, tbl := range parseTables(colSec) {
		ci := headerIndex(tbl.Header, "column")
		ti := headerIndex(tbl.Header, "type")
		di := headerIndex(tbl.Header, "description")
		if ci < 0 {
			continue
		}
		for _, row := range tbl.Rows {
			name := unticked(cell(row, ci))
			if name == "" {
				continue
			}
			desc := strings.TrimSpace(cell(row, di))
			t.Columns = append(t.Columns, model.Column{
				Name:        name,
				Type:        strings.ToUpper(unticked(cell(row, ti))),
				Description: desc,
				Ordinal:     len(t.Columns),
				IsPK:        pkHintRe.MatchString(desc),
				IsFK:        fkHintRe.MatchString(desc),
			})
		}
	}
	if len(t.Columns) == 0 {
		diags = append(diags, model.Diagnostic{
			Severity: model.SeverityWarning,
			Code:     "no_columns",
			Message:  "Table document declares no columns.",
			DomainID: domainID,
			TableID:  t.ID,
			DocPath:  f.Path,
		})
	}

	// Column-level lineage.
	linSec, _ := findSection(secs, "Column-Level Lineage", "Column Level Lineage", "Lineage")
	for _, tbl := range parseTables(linSec) {
		ci := headerIndex(tbl.Header, "column")
		sti := headerIndex(tbl.Header, "source table")
		sci := headerIndex(tbl.Header, "source column")
		ni := headerIndex(tbl.Header, "note")
		if ci < 0 || sti < 0 {
			continue
		}
		for _, row := range tbl.Rows {
			col := unticked(cell(row, ci))
			if col == "" {
				continue
			}
			notes := strings.TrimSpace(cell(row, ni))
			t.ColumnLineage = append(t.ColumnLineage, model.ColumnLineage{
				Column:       col,
				SourceTable:  unticked(cell(row, sti)),
				SourceColumn: unticked(cell(row, sci)),
				Notes:        notes,
				Derived:      derivedRe.MatchString(notes),
			})
		}
	}

	// Relationships. Targets stay unresolved here; the graph package matches
	// them against the full table registry.
	relSec, _ := findSection(secs, "Relationships")
	t.RelationshipNote = prose(stripTables(relSec))
	for _, tbl := range parseTables(relSec) {
		ri := headerIndex(tbl.Header, "related table", "table")
		ji := headerIndex(tbl.Header, "join key", "join")
		ci := headerIndex(tbl.Header, "relationship", "cardinality")
		if ri < 0 {
			continue
		}
		for _, row := range tbl.Rows {
			target := unticked(cell(row, ri))
			if target == "" {
				continue
			}
			join := strings.TrimSpace(cell(row, ji))
			from, to := splitJoinKey(join)
			t.Relationships = append(t.Relationships, model.Relationship{
				FromTableID: t.ID,
				TargetRef:   target,
				FromColumn:  from,
				ToColumn:    to,
				JoinKeyRaw:  unticked(join),
				Cardinality: normaliseCardinality(cell(row, ci)),
			})
		}
	}

	notesSec, _ := findSection(secs, "Notes / Caveats", "Notes", "Caveats")
	t.Notes = bullets(notesSec)

	markKeyColumns(&t)
	return t, diags
}

// markKeyColumns folds evidence from lineage notes and join keys back into the
// column list, so a column named only as a foreign key in a join is flagged.
func markKeyColumns(t *model.Table) {
	byName := map[string]*model.Column{}
	for i := range t.Columns {
		byName[t.Columns[i].Name] = &t.Columns[i]
	}
	for _, l := range t.ColumnLineage {
		c, ok := byName[l.Column]
		if !ok {
			continue
		}
		if pkHintRe.MatchString(l.Notes) {
			c.IsPK = true
		}
		if fkHintRe.MatchString(l.Notes) {
			c.IsFK = true
		}
	}
	for _, r := range t.Relationships {
		if c, ok := byName[r.FromColumn]; ok && r.FromColumn != "" {
			if !c.IsPK {
				c.IsFK = true
			}
		}
	}
}
