package graph

import (
	"sort"
	"strings"

	"urara-vision/backend/internal/model"
)

// Edge is a resolved, direction-normalised join between two tables. Documents
// declare relationships from both sides ("fact_primary -> dim_alpha,
// Many-to-one" and "dim_alpha -> fact_primary, One-to-many"); both collapse to a
// single edge pointing from the many side to the one side.
//
// Nothing here reads a table's role, which is why a snowflake's dimension-to-
// dimension joins and a Data Vault's link-to-hub joins need no special case:
// cardinality alone decides the direction.
type Edge struct {
	ID          string           `json:"id"`
	From        string           `json:"from"`
	To          string           `json:"to"`
	FromColumn  string           `json:"fromColumn"`
	ToColumn    string           `json:"toColumn"`
	Cardinality string           `json:"cardinality"`
	Resolution  model.Resolution `json:"resolution"`
	// DeclaredBy lists the table documents that asserted this join, so the UI
	// can show when only one side documents a relationship.
	DeclaredBy  []string `json:"declaredBy"`
	CrossDomain bool     `json:"crossDomain"`
}

// isOneToMany reports whether a cardinality points from the one side to the
// many side, meaning the edge must be reversed to normalise it.
func isOneToMany(c string) bool {
	l := strings.ToLower(strings.TrimSpace(c))
	return strings.HasPrefix(l, "one-to-many") || strings.HasPrefix(l, "one to many") || strings.HasPrefix(l, "1:n")
}

// Edges projects a resolved model into deduplicated graph edges.
func Edges(m *model.Model) []Edge {
	domainOf := map[string]string{}
	for i := range m.Tables {
		domainOf[m.Tables[i].ID] = m.Tables[i].DomainID
	}

	merged := map[string]*Edge{}
	var order []string

	for i := range m.Tables {
		t := &m.Tables[i]
		for _, r := range t.Relationships {
			if r.ToTableID == "" {
				continue // narrative or unresolved: not a drawable edge
			}
			from, to := r.FromTableID, r.ToTableID
			fromCol, toCol := r.FromColumn, r.ToColumn
			card := r.Cardinality
			if isOneToMany(card) {
				// Reverse the edge and its join columns together, so the key
				// still names the many side's column first.
				from, to = to, from
				fromCol, toCol = toCol, fromCol
				card = "Many-to-one"
			}
			key := from + "\x00" + to + "\x00" + fromCol + "\x00" + toCol
			e, ok := merged[key]
			if !ok {
				e = &Edge{
					ID:          edgeID(from, to, fromCol, toCol, 0),
					From:        from,
					To:          to,
					FromColumn:  fromCol,
					ToColumn:    toCol,
					Cardinality: card,
					Resolution:  r.Resolution,
					CrossDomain: domainOf[from] != domainOf[to],
				}
				merged[key] = e
				order = append(order, key)
			}
			// A conformed binding is the more interesting fact about an edge,
			// so it survives a merge with a local declaration.
			if r.Resolution == model.ResolvedConformed {
				e.Resolution = model.ResolvedConformed
			}
			if !contains(e.DeclaredBy, t.ID) {
				e.DeclaredBy = append(e.DeclaredBy, t.ID)
			}
		}
	}

	out := make([]Edge, 0, len(order))
	for _, k := range order {
		e := merged[k]
		sort.Strings(e.DeclaredBy)
		out = append(out, *e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].From != out[j].From {
			return out[i].From < out[j].From
		}
		if out[i].To != out[j].To {
			return out[i].To < out[j].To
		}
		return out[i].FromColumn < out[j].FromColumn
	})
	return out
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
