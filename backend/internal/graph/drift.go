// Conformed drift: the same table name documented differently in different
// domains.
//
// A conformed dimension is a promise that every domain means the same thing by
// it. Comparing the column sets is how that promise gets checked.
package graph

import (
	"fmt"
	"sort"
	"strings"

	"urara-vision/backend/internal/model"
)

// detectConformedDrift compares every instance of a conformed table against the
// authority instance and reports structural disagreement. Two documents both
// called dim_date but carrying different columns is a real modelling problem,
// and it is exactly what a reader of a single domain document cannot see.
func detectConformedDrift(byName map[string][]string, byID map[string]*model.Table) []model.Diagnostic {
	var out []model.Diagnostic
	names := make([]string, 0, len(byName))
	for n, ids := range byName {
		if len(ids) > 1 {
			names = append(names, n)
		}
	}
	sort.Strings(names)

	for _, name := range names {
		ids := byName[name]
		authority := pickConformed(ids, byID)
		base, ok := byID[authority]
		if !ok {
			continue
		}
		baseCols := columnSet(base)
		for _, id := range ids {
			if id == authority {
				continue
			}
			t, ok := byID[id]
			if !ok {
				continue
			}
			missing, extra := diffColumns(baseCols, columnSet(t))
			if len(missing) == 0 && len(extra) == 0 {
				continue
			}
			out = append(out, model.Diagnostic{
				Severity: model.SeverityWarning,
				Code:     "conformed_drift",
				Message: fmt.Sprintf("%s differs from the conformed definition in %s: %s.",
					id, base.DomainID, describeDrift(missing, extra)),
				DomainID: t.DomainID,
				TableID:  t.ID,
				DocPath:  t.DocPath,
			})
		}
	}
	return out
}

func columnSet(t *model.Table) map[string]bool {
	s := make(map[string]bool, len(t.Columns))
	for _, c := range t.Columns {
		s[c.Name] = true
	}
	return s
}

// diffColumns returns the columns present in base but absent from other
// (missing), and those present in other but absent from base (extra).
func diffColumns(base, other map[string]bool) (missing, extra []string) {
	for c := range base {
		if !other[c] {
			missing = append(missing, c)
		}
	}
	for c := range other {
		if !base[c] {
			extra = append(extra, c)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	return missing, extra
}

func describeDrift(missing, extra []string) string {
	var parts []string
	if len(missing) > 0 {
		parts = append(parts, fmt.Sprintf("missing %d (%s)", len(missing), preview(missing)))
	}
	if len(extra) > 0 {
		parts = append(parts, fmt.Sprintf("adds %d (%s)", len(extra), preview(extra)))
	}
	return strings.Join(parts, ", ")
}

// preview renders at most three names, summarising the rest.
func preview(v []string) string {
	if len(v) <= 3 {
		return strings.Join(v, ", ")
	}
	return fmt.Sprintf("%s, +%d more", strings.Join(v[:3], ", "), len(v)-3)
}
