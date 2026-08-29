// Matching a declared relationship target to a real table.
//
// A target is written as a bare table name, which may name a table in the same
// domain, a conformed dimension borrowed from another domain, a table nobody
// documented, or prose that was never a table name at all. Each outcome is a
// different resolution and, for the last three, a diagnostic.
package graph

import (
	"fmt"
	"sort"
	"strings"

	"urara-vision/backend/internal/model"
)

// resolveRelationships binds every declared relationship to a table where it
// can, and returns how many relationships were declared in total.
func resolveRelationships(m *model.Model, reg *registry) int {
	relCount := 0
	for i := range m.Tables {
		t := &m.Tables[i]
		for j := range t.Relationships {
			r := &t.Relationships[j]
			r.ID = edgeID(t.ID, r.TargetRef, r.FromColumn, r.ToColumn, j)
			relCount++

			if !isIdentifier(r.TargetRef) {
				r.Resolution = model.ResolvedNarrative
				m.Diagnostics = append(m.Diagnostics, model.Diagnostic{
					Severity: model.SeverityInfo,
					Code:     "narrative_reference",
					Message: fmt.Sprintf("%s references %q, which is prose rather than a table document.",
						t.Name, r.TargetRef),
					DomainID: t.DomainID,
					TableID:  t.ID,
					DocPath:  t.DocPath,
				})
				continue
			}

			// Prefer a table in the same domain.
			local := t.DomainID + "/" + r.TargetRef
			if target, ok := reg.byID[local]; ok {
				r.ToTableID = local
				r.Resolution = model.ResolvedLocal
				orientRelationship(r, t, target, &m.Diagnostics)
				continue
			}

			candidates := reg.byName[r.TargetRef]
			if len(candidates) == 0 {
				r.Resolution = model.ResolvedUnresolved
				m.Diagnostics = append(m.Diagnostics, model.Diagnostic{
					Severity: model.SeverityError,
					Code:     "unresolved_reference",
					Message: fmt.Sprintf("%s declares a relationship to %q but no document defines that table.",
						t.Name, r.TargetRef),
					DomainID: t.DomainID,
					TableID:  t.ID,
					DocPath:  t.DocPath,
				})
				continue
			}

			// The target exists, but only in other domains: a conformed
			// dimension being borrowed. Bind to a deterministic instance and
			// record the alternatives.
			r.ToTableID = pickConformed(candidates, reg.byID)
			r.Resolution = model.ResolvedConformed
			r.Candidates = candidates
			orientRelationship(r, t, reg.byID[r.ToTableID], &m.Diagnostics)
			m.Diagnostics = append(m.Diagnostics, model.Diagnostic{
				Severity: model.SeverityWarning,
				Code:     "cross_domain_reference",
				Message: fmt.Sprintf("%s references %s, which has no document in domain %q; bound to %s.",
					t.Name, r.TargetRef, t.DomainID, r.ToTableID),
				DomainID: t.DomainID,
				TableID:  t.ID,
				DocPath:  t.DocPath,
			})
		}
	}
	return relCount
}

// orientJoinKey decides which side of a declared join key belongs to which
// table. Documents are not consistent about the order: a dimension may write
// "alpha_id_1 = alpha_id" on its own One-to-many row, naming the fact's
// column first. Trusting the written order would silently reverse those joins,
// so the sides are matched against the tables' real column lists instead.
//
// pickConformed chooses which instance of a cross-domain table a reference
// binds to. Preference order: a document that explicitly labels its domain
// "Conformed", then the richest definition (most columns), then alphabetical.
// Matching requires the word "conformed" specifically -- a domain merely named
// something like "Domain Four & Cross-Domain Reporting" is not claiming to be the
// conformed authority.
func pickConformed(candidates []string, byID map[string]*model.Table) string {
	type scored struct {
		id       string
		declared bool
		columns  int
	}
	ranked := make([]scored, 0, len(candidates))
	for _, id := range candidates {
		t, ok := byID[id]
		if !ok {
			continue
		}
		label := strings.ToLower(t.DomainLabel + " " + t.KindRaw)
		ranked = append(ranked, scored{
			id:       id,
			declared: strings.Contains(label, "conformed"),
			columns:  len(t.Columns),
		})
	}
	if len(ranked) == 0 {
		return candidates[0]
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].declared != ranked[j].declared {
			return ranked[i].declared
		}
		if ranked[i].columns != ranked[j].columns {
			return ranked[i].columns > ranked[j].columns
		}
		return ranked[i].id < ranked[j].id
	})
	return ranked[0].id
}

// identRe-equivalent check: a reference is a table identifier only if it looks
// like a snake_case token. Anything else is prose in the cell, e.g.
// "Various Fact Tables".
func isIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '.' {
			continue
		}
		return false
	}
	return true
}
