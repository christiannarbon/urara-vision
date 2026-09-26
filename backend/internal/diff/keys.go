package diff

import (
	"strconv"
	"strings"

	"urara-vision/backend/internal/model"
)

func domainKey(d model.Domain) string { return d.ID }

func tableKey(t model.Table) string { return t.ID }

func columnKey(c model.Column) string { return c.Name }

// relationshipKey ignores Relationship.ID, which embeds an ordinal, and keys on
// TargetRef so a join keeps its identity when its resolution changes.
func relationshipKey(r model.Relationship) string {
	return joinKey(r.FromTableID, r.TargetRef, r.FromColumn, r.ToColumn)
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
		out[occurrence(k, seen[k])] = it
	}
	return out
}

// pairIndex keys both sides so equal repeats pair first, then the rest by
// position. Leftovers can only remain on one side, so plain numbering is safe.
func pairIndex[T any](from, to []T, key func(T) string, same func(a, b T) bool) (map[string]T, map[string]T) {
	fromGroups, toGroups := group(from, key), group(to, key)
	outFrom := make(map[string]T, len(from))
	outTo := make(map[string]T, len(to))
	bases := make(map[string]bool, len(fromGroups)+len(toGroups))
	for b := range fromGroups {
		bases[b] = true
	}
	for b := range toGroups {
		bases[b] = true
	}

	for base := range bases {
		fs, ts := fromGroups[base], toGroups[base]
		match := make([]int, len(fs))
		used := make([]bool, len(ts))
		for i, f := range fs {
			match[i] = -1
			for j, t := range ts {
				if !used[j] && same(f, t) {
					match[i], used[j] = j, true
					break
				}
			}
		}
		j := 0
		for i := range fs {
			if match[i] >= 0 {
				continue
			}
			for j < len(ts) && used[j] {
				j++
			}
			if j < len(ts) {
				match[i], used[j] = j, true
			}
		}

		n := 0
		for i, m := range match {
			if m >= 0 {
				n++
				k := occurrence(base, n)
				outFrom[k], outTo[k] = fs[i], ts[m]
			}
		}
		for i, m := range match {
			if m < 0 {
				n++
				outFrom[occurrence(base, n)] = fs[i]
			}
		}
		for j, u := range used {
			if !u {
				n++
				outTo[occurrence(base, n)] = ts[j]
			}
		}
	}
	return outFrom, outTo
}

func group[T any](items []T, key func(T) string) map[string][]T {
	out := make(map[string][]T, len(items))
	for _, it := range items {
		k := key(it)
		out[k] = append(out[k], it)
	}
	return out
}

func occurrence(base string, n int) string {
	if n == 1 {
		return base
	}
	return base + "#" + strconv.Itoa(n)
}
