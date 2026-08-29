// The table registry: what exists, under which name, in which domain.
//
// Resolution asks the same two questions repeatedly -- "is there a table with
// this ID?" and "what else is called this?" -- so both are indexed once up
// front.
package graph

import (
	"sort"

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

// markConformed flags every table whose name is documented in more than one
// domain, recording where the other instances are, and returns how many were
// flagged.
//
// A repeated name is the documentation's own way of saying "conformed
// dimension"; nothing declares it explicitly.
func (reg *registry) markConformed(tables []model.Table) int {
	count := 0
	for i := range tables {
		t := &tables[i]
		ids := reg.byName[t.Name]
		if len(ids) < 2 {
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
