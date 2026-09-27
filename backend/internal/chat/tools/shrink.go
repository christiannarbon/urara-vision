package tools

import (
	"bytes"
	"encoding/json"

	"urara-vision/backend/internal/chat/apiclient"
)

// MaxItems bounds every list result, so a long list cannot crowd out the question.
const MaxItems = 50

// MaxColumnDescriptionRunes keeps enough column prose to tell columns apart.
const MaxColumnDescriptionRunes = 300

// Capped is the one shape every list result takes.
func Capped(items []any) map[string]any {
	total := len(items)
	if items == nil {
		items = []any{}
	}
	return map[string]any{"items": items[:min(total, MaxItems)], "truncated": total > MaxItems, "total": total}
}

// TruncateRunes shortens text to limit characters, marking the cut with "…".
func TruncateRunes(text string, limit int) string {
	r := []rune(text)
	if len(r) <= limit {
		return text
	}
	return string(r[:limit]) + "…"
}

// Prune drops what carries no information, recursively. Lists keep every
// element, as in Python.
func Prune(v any) any {
	switch v := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, x := range v {
			if x = Prune(x); worthKeeping(x) {
				out[k] = x
			}
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, x := range v {
			out[i] = Prune(x)
		}
		return out
	}
	return v
}

// worthKeeping keeps every number, including 0, and drops false and empties.
func worthKeeping(v any) bool {
	switch v := v.(type) {
	case nil:
		return false
	case bool:
		return v
	case string:
		return v != ""
	case []any:
		return len(v) > 0
	case map[string]any:
		return len(v) > 0
	}
	return true
}

func strs(s []string) []any {
	out := make([]any, len(s))
	for i, x := range s {
		out[i] = x
	}
	return out
}

// asMap renders v through its JSON tags, keeping numbers exact.
func asMap(v any) (map[string]any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var m map[string]any
	return m, dec.Decode(&m)
}

func shrinkTable(d apiclient.TableDetail) (map[string]any, error) {
	table, err := asMap(d.Table)
	if err != nil {
		return nil, err
	}
	delete(table, "docPath")
	delete(table, "snapshotId")
	if cols, ok := table["columns"].([]any); ok {
		for _, c := range cols {
			if col, ok := c.(map[string]any); ok {
				desc, _ := col["description"].(string)
				col["description"] = TruncateRunes(desc, MaxColumnDescriptionRunes)
			}
		}
	}

	shrunk := map[string]any{"table": table}
	if len(d.Incoming) > 0 {
		incoming := make([]any, len(d.Incoming))
		for i, r := range d.Incoming {
			m, err := asMap(r)
			if err != nil {
				return nil, err
			}
			delete(m, "docPath")
			incoming[i] = m
		}
		shrunk["incoming"] = incoming
	}
	if len(d.Upstream) > 0 {
		shrunk["upstream"] = entryIDs(d.Upstream)
	}
	if len(d.Siblings) > 0 {
		shrunk["siblings"] = entryIDs(d.Siblings)
	}
	return Prune(shrunk).(map[string]any), nil
}

func entryIDs(entries []apiclient.LineageEntry) []any {
	ids := make([]any, len(entries))
	for i, e := range entries {
		ids[i] = e.ID
	}
	return ids
}

// shrinkGraph keeps IDs and joins, not drawing data, and only links between kept nodes.
func shrinkGraph(g apiclient.Graph) map[string]any {
	nodes := make([]any, len(g.Nodes))
	kept := map[string]bool{}
	for i, n := range g.Nodes {
		nodes[i] = Prune(map[string]any{
			"id": n.ID, "label": n.Label, "kind": n.Kind, "domainId": n.DomainID, "type": n.Type,
		})
		if i < MaxItems {
			kept[n.ID] = true
		}
	}
	links := []any{}
	for _, l := range g.Links {
		if kept[l.Source] && kept[l.Target] {
			links = append(links, Prune(map[string]any{
				"source": l.Source, "target": l.Target, "type": l.Type,
				"fromColumn": l.FromColumn, "toColumn": l.ToColumn, "cardinality": l.Cardinality,
			}))
		}
	}
	result := Capped(nodes)
	result["links"] = links[:min(len(links), MaxItems)]
	return result
}

func shrinkPath(p apiclient.JoinPath) map[string]any {
	hops := make([]any, len(p.Hops))
	for i, h := range p.Hops {
		hops[i] = map[string]any{
			"from": h.From, "to": h.To, "fromColumn": h.FromColumn, "toColumn": h.ToColumn, "cardinality": h.Cardinality,
		}
	}
	return map[string]any{"length": p.Length, "tables": strs(p.Tables), "hops": hops}
}

func shrinkBatch(b apiclient.TablesDetail) (map[string]any, error) {
	tables := make([]any, len(b.Tables))
	for i, d := range b.Tables {
		t, err := shrinkTable(d)
		if err != nil {
			return nil, err
		}
		tables[i] = t
	}
	result := Capped(tables)
	// Kept when empty: an absent key reads as though every ID was found.
	result["missing"] = strs(b.Missing)
	return result, nil
}
