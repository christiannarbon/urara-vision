// Domain-level and whole-model observations: how many tables a domain actually
// documents, and connective tables left joined to nothing.
package graph

import (
	"urara-vision/backend/internal/model"
)

// countDomainTables fills in each domain's table count and warns about a domain
// index whose directory documents nothing.
func countDomainTables(m *model.Model, snapshotID string) {
	counts := map[string]int{}
	for i := range m.Tables {
		counts[m.Tables[i].DomainID]++
	}
	for i := range m.Domains {
		d := &m.Domains[i]
		d.SnapshotID = snapshotID
		d.TableCount = counts[d.ID]
		if d.TableCount == 0 {
			m.Diagnostics = append(m.Diagnostics, model.Diagnostic{
				Severity: model.SeverityWarning,
				Code:     "empty_domain",
				Message:  "Domain index exists but no table documents were found in its directory.",
				DomainID: d.ID,
				DocPath:  d.DocPath,
			})
		}
	}
}

// flagIsolatedTables warns about a table whose whole purpose is to join others
// but which resolved no relationship at all: a fact joined to no dimension, a
// Data Vault link with no hubs, a junction table joining nothing. That is
// almost always a documentation gap rather than a real standalone table.
//
// Roles that are legitimately standalone are left alone. A conformed dimension
// nothing in this directory happens to join to is ordinary, not a problem.
func flagIsolatedTables(m *model.Model) {
	for i := range m.Tables {
		t := &m.Tables[i]
		role := model.RoleOf(t.Kind)
		if !role.Connective {
			continue
		}
		linked := 0
		for _, r := range t.Relationships {
			if r.ToTableID != "" {
				linked++
			}
		}
		if linked == 0 {
			m.Diagnostics = append(m.Diagnostics, model.Diagnostic{
				Severity: model.SeverityWarning,
				Code:     isolationCode(t.Kind),
				Message:  role.Label + " table has no resolvable relationship to any other table.",
				DomainID: t.DomainID,
				TableID:  t.ID,
				DocPath:  t.DocPath,
			})
		}
	}
}

// isolationCode keeps the long-standing code for the star schema case, which is
// the one readers and tests already know by name, and files the roles this tool
// learned later under a general one.
func isolationCode(k model.TableKind) string {
	if k == model.KindFact || k == model.KindFactless {
		return "isolated_fact"
	}
	return "isolated_table"
}
