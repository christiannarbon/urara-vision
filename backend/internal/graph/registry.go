// The table registry: what exists, under which name, in which domain.
//
// Resolution asks the same two questions repeatedly -- "is there a table with
// this ID?" and "what else is called this?" -- so both are indexed once up
// front.
package graph

import (
	"sort"
	"strings"

	"urara-vision/backend/internal/model"
)

// registry indexes a snapshot's tables by ID and by name.
type registry struct {
	// byID maps a table ID to the table itself.
	byID map[string]*model.Table
	// byName maps a bare table name to every ID carrying it, sorted, so a name
	// documented in several domains resolves deterministically.
	byName map[string][]string
}

// newRegistry indexes the tables and stamps each with the snapshot it belongs
// to.
func newRegistry(tables []model.Table, snapshotID string) *registry {
	reg := &registry{
		byID:   map[string]*model.Table{},
		byName: map[string][]string{},
	}
	for i := range tables {
		t := &tables[i]
		t.SnapshotID = snapshotID
		reg.byID[t.ID] = t
		reg.byName[t.Name] = append(reg.byName[t.Name], t.ID)
	}
	for k := range reg.byName {
		sort.Strings(reg.byName[k])
	}
	return reg
}

// markConformed flags every conformed table, recording where its other
// instances are, and returns how many were flagged.
//
// A table is conformed either because its name is documented in more than one
// domain -- the documentation's own way of saying "conformed dimension" -- or
// because the document declares it, the way a shared kernel writes
// "Shared Kernel (Conformed Dimensions)" in its Domain cell. The declaration
// counts on its own: a kernel dimension no other directory has got round to
// documenting is still a conformed dimension, and the detail pane should say
// so. declaresConformed is the same test pickConformed uses to choose the
// authority among several instances.
func (reg *registry) markConformed(tables []model.Table) int {
	count := 0
	for i := range tables {
		t := &tables[i]
		ids := reg.byName[t.Name]
		if len(ids) < 2 && !declaresConformed(t) {
			continue
		}
		t.Conformed = true
		count++
		for _, id := range ids {
			if id == t.ID {
				continue
			}
			t.ConformedIn = append(t.ConformedIn, id)
		}
	}
	return count
}

// declaresConformed reports whether a document claims conformed status for
// itself, in its Domain cell or its Type. Matching requires the word
// "conformed" specifically, so a domain merely named something like
// "Cross-Domain Reporting" is not claiming it.
func declaresConformed(t *model.Table) bool {
	return strings.Contains(strings.ToLower(t.DomainLabel+" "+t.KindRaw), "conformed")
}
