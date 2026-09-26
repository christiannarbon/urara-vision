package diff

import (
	"strconv"
	"strings"

	"urara-vision/backend/internal/model"
)

func domainKey(d model.Domain) string { return d.ID }

func tableKey(t model.Table) string { return t.ID }

func columnKey(c model.Column) string { return c.Name }

// relationshipKey ignores Relationship.ID: it embeds an ordinal.
func relationshipKey(r model.Relationship) string {
	to := r.ToTableID
	if to == "" {
		to = "ref:" + r.TargetRef
	}
	return joinKey(r.FromTableID, to, r.FromColumn, r.ToColumn)
}

func lineageKey(tableID string, l model.ColumnLineage) string {
	return joinKey(tableID, l.Column, l.SourceTable, l.SourceColumn)
}

func joinKey(parts ...string) string { return strings.Join(parts, "|") }

// index maps each item by key; repeats within one model get "#2", "#3", …
// so none is dropped.
func index[T any](items []T, key func(T) string) map[string]T {
	out := make(map[string]T, len(items))
	seen := make(map[string]int, len(items))
	for _, it := range items {
		k := key(it)
		seen[k]++
		if n := seen[k]; n > 1 {
			k += "#" + strconv.Itoa(n)
		}
		out[k] = it
	}
	return out
}
