// Package diff compares two models of the same project. It does no I/O.
package diff

import (
	"sort"

	"urara-vision/backend/internal/model"
)

type Change string

const (
	Added   Change = "added"
	Removed Change = "removed"
	Changed Change = "changed"
)

type FieldChange struct {
	Field string `json:"field"`
	From  any    `json:"from"`
	To    any    `json:"to"`
}

type Counts struct {
	Added   int `json:"added"`
	Removed int `json:"removed"`
	Changed int `json:"changed"`
}

type Summary struct {
	Domains       Counts `json:"domains"`
	Tables        Counts `json:"tables"`
	Columns       Counts `json:"columns"`
	Relationships Counts `json:"relationships"`
	Lineage       Counts `json:"lineage"`
}

type DomainDiff struct {
	ID     string        `json:"id"`
	Change Change        `json:"change"`
	Fields []FieldChange `json:"fields"`
}

type ColumnDiff struct {
	Name   string        `json:"name"`
	Change Change        `json:"change"`
	Fields []FieldChange `json:"fields"`
}

type TableDiff struct {
	ID       string        `json:"id"`
	DomainID string        `json:"domainId"`
	Change   Change        `json:"change"`
	Fields   []FieldChange `json:"fields"`
	Columns  []ColumnDiff  `json:"columns"`
}

type RelationshipDiff struct {
	FromTableID string        `json:"fromTableId"`
	ToTableID   string        `json:"toTableId"`
	TargetRef   string        `json:"targetRef"`
	FromColumn  string        `json:"fromColumn"`
	ToColumn    string        `json:"toColumn"`
	Change      Change        `json:"change"`
	Fields      []FieldChange `json:"fields"`
}

type LineageDiff struct {
	TableID      string        `json:"tableId"`
	Column       string        `json:"column"`
	SourceTable  string        `json:"sourceTable"`
	SourceColumn string        `json:"sourceColumn"`
	Change       Change        `json:"change"`
	Fields       []FieldChange `json:"fields"`
}

type Result struct {
	Summary       Summary            `json:"summary"`
	Domains       []DomainDiff       `json:"domains"`
	Tables        []TableDiff        `json:"tables"`
	Relationships []RelationshipDiff `json:"relationships"`
	Lineage       []LineageDiff      `json:"lineage"`
}

type tableLineage struct {
	tableID string
	model.ColumnLineage
}

// Compare reports what changed going from one model to the other.
func Compare(from, to *model.Model) Result {
	r := Result{
		Domains:       []DomainDiff{},
		Tables:        []TableDiff{},
		Relationships: []RelationshipDiff{},
		Lineage:       []LineageDiff{},
	}

	walk(index(from.Domains, domainKey), index(to.Domains, domainKey), &r.Summary.Domains,
		func(a, b *model.Domain, c Change) bool {
			d := pick(a, b)
			var fs []FieldChange
			if c == Changed {
				fs = domainFields(*a, *b)
				if len(fs) == 0 {
					return false
				}
			}
			r.Domains = append(r.Domains, DomainDiff{ID: d.ID, Change: c, Fields: nonNil(fs)})
			return true
		})

	walk(index(from.Tables, tableKey), index(to.Tables, tableKey), &r.Summary.Tables,
		func(a, b *model.Table, c Change) bool {
			t := pick(a, b)
			var fs []FieldChange
			cols := []ColumnDiff{}
			if c == Changed {
				fs = tableFields(*a, *b)
				cols = compareColumns(a.Columns, b.Columns, &r.Summary.Columns)
				if len(fs) == 0 && len(cols) == 0 {
					return false
				}
			}
			r.Tables = append(r.Tables, TableDiff{
				ID: t.ID, DomainID: t.DomainID, Change: c, Fields: nonNil(fs), Columns: cols,
			})
			return true
		})

	relFrom, relTo := pairIndex(relationships(from), relationships(to), relationshipKey, sameRelationship)
	walk(relFrom, relTo, &r.Summary.Relationships,
		func(a, b *model.Relationship, c Change) bool {
			rel := pick(a, b)
			var fs []FieldChange
			if c == Changed {
				fs = field(fs, "cardinality", a.Cardinality, b.Cardinality)
				fs = field(fs, "resolution", string(a.Resolution), string(b.Resolution))
				fs = field(fs, "toTableId", a.ToTableID, b.ToTableID)
				if len(fs) == 0 {
					return false
				}
			}
			r.Relationships = append(r.Relationships, RelationshipDiff{
				FromTableID: rel.FromTableID, ToTableID: rel.ToTableID, TargetRef: rel.TargetRef,
				FromColumn: rel.FromColumn, ToColumn: rel.ToColumn, Change: c, Fields: nonNil(fs),
			})
			return true
		})

	lkey := func(l tableLineage) string { return lineageKey(l.tableID, l.ColumnLineage) }
	linFrom, linTo := pairIndex(lineage(from), lineage(to), lkey, sameLineage)
	walk(linFrom, linTo, &r.Summary.Lineage,
		func(a, b *tableLineage, c Change) bool {
			l := pick(a, b)
			var fs []FieldChange
			if c == Changed {
				fs = field(fs, "notes", a.Notes, b.Notes)
				fs = field(fs, "derived", a.Derived, b.Derived)
				if len(fs) == 0 {
					return false
				}
			}
			r.Lineage = append(r.Lineage, LineageDiff{
				TableID: l.tableID, Column: l.Column, SourceTable: l.SourceTable,
				SourceColumn: l.SourceColumn, Change: c, Fields: nonNil(fs),
			})
			return true
		})

	return r
}

func compareColumns(from, to []model.Column, counts *Counts) []ColumnDiff {
	out := []ColumnDiff{}
	walk(index(from, columnKey), index(to, columnKey), counts,
		func(a, b *model.Column, c Change) bool {
			var fs []FieldChange
			if c == Changed {
				fs = field(fs, "type", a.Type, b.Type)
				fs = field(fs, "description", a.Description, b.Description)
				fs = field(fs, "isPk", a.IsPK, b.IsPK)
				fs = field(fs, "isFk", a.IsFK, b.IsFK)
				if len(fs) == 0 {
					return false
				}
			}
			out = append(out, ColumnDiff{Name: pick(a, b).Name, Change: c, Fields: nonNil(fs)})
			return true
		})
	return out
}

func domainFields(a, b model.Domain) []FieldChange {
	var fs []FieldChange
	fs = field(fs, "title", a.Title, b.Title)
	fs = field(fs, "description", a.Description, b.Description)
	return fs
}

func tableFields(a, b model.Table) []FieldChange {
	var fs []FieldChange
	fs = field(fs, "kind", string(a.Kind), string(b.Kind))
	fs = field(fs, "grain", a.Grain, b.Grain)
	fs = field(fs, "updateFrequency", a.UpdateFrequency, b.UpdateFrequency)
	fs = field(fs, "layer", a.Layer, b.Layer)
	fs = field(fs, "description", a.Description, b.Description)
	fs = field(fs, "conformed", a.Conformed, b.Conformed)
	return fs
}

func sameRelationship(a, b model.Relationship) bool {
	return a.Cardinality == b.Cardinality && a.Resolution == b.Resolution && a.ToTableID == b.ToTableID
}

func sameLineage(a, b tableLineage) bool {
	return a.Notes == b.Notes && a.Derived == b.Derived
}

func relationships(m *model.Model) []model.Relationship {
	var out []model.Relationship
	for _, t := range m.Tables {
		out = append(out, t.Relationships...)
	}
	return out
}

func lineage(m *model.Model) []tableLineage {
	var out []tableLineage
	for _, t := range m.Tables {
		for _, l := range t.ColumnLineage {
			out = append(out, tableLineage{tableID: t.ID, ColumnLineage: l})
		}
	}
	return out
}

// walk visits the union of keys in sorted order, which is what keeps every
// output slice sorted. a is nil for Added, b is nil for Removed. emit returns
// whether it reported anything, so unchanged pairs are not counted.
func walk[T any](from, to map[string]T, counts *Counts, emit func(a, b *T, c Change) bool) {
	keys := make([]string, 0, len(from)+len(to))
	for k := range from {
		keys = append(keys, k)
	}
	for k := range to {
		if _, ok := from[k]; !ok {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	for _, k := range keys {
		a, inFrom := from[k]
		b, inTo := to[k]
		switch {
		case !inTo:
			if emit(&a, nil, Removed) {
				counts.Removed++
			}
		case !inFrom:
			if emit(nil, &b, Added) {
				counts.Added++
			}
		default:
			if emit(&a, &b, Changed) {
				counts.Changed++
			}
		}
	}
}

func pick[T any](a, b *T) *T {
	if b != nil {
		return b
	}
	return a
}

func field[T comparable](fs []FieldChange, name string, from, to T) []FieldChange {
	if from == to {
		return fs
	}
	return append(fs, FieldChange{Field: name, From: from, To: to})
}

func nonNil(fs []FieldChange) []FieldChange {
	if fs == nil {
		return []FieldChange{}
	}
	return fs
}
