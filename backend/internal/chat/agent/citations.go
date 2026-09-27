package agent

import (
	"regexp"
	"slices"
	"sort"
	"strings"
)

// MaxCitations caps the chips shown under an answer.
const MaxCitations = 20

// maxDepth guards against a malformed result hanging a turn; results are a few levels deep.
const maxDepth = 12

var tableID = regexp.MustCompile(`(?i)^[a-z0-9_][a-z0-9_.-]*/[a-z0-9_][a-z0-9_.-]*$`)

var plainWord = regexp.MustCompile(`^[a-z]+$`)

// Keys whose string value is an identifier, not prose; "tables" holds a list of them.
var idKeys = map[string]bool{"id": true, "tableId": true, "from": true, "to": true, "source": true, "target": true}

// ExtractCitations returns the table IDs the answer drew on: retrieved this
// turn and mentioned in the answer, in order of first mention.
func ExtractCitations(results []any, answer string) []string {
	cited := []string{}
	if answer == "" {
		return cited
	}
	lowered := strings.ToLower(answer)
	candidates := collect(results)

	// A full ID names one table, so its bare name must not cite a namesake too.
	unpinned := lowered
	for _, c := range candidates {
		needle := strings.ToLower(c)
		for _, at := range allBounded(unpinned, needle) {
			unpinned = unpinned[:at] + strings.Repeat(" ", len(needle)) + unpinned[at+len(needle):]
		}
	}

	type mention struct {
		pos, order int
		id         string
	}
	var mentions []mention
	for order, c := range candidates {
		if pos := firstMention(c, lowered, unpinned); pos >= 0 {
			mentions = append(mentions, mention{pos, order, c})
		}
	}
	// Discovery order breaks ties, so namesakes come back in a stable order.
	sort.Slice(mentions, func(i, j int) bool {
		if mentions[i].pos != mentions[j].pos {
			return mentions[i].pos < mentions[j].pos
		}
		return mentions[i].order < mentions[j].order
	})
	for _, m := range mentions[:min(len(mentions), MaxCitations)] {
		cited = append(cited, m.id)
	}
	return cited
}

// collect returns every table ID in the results, deduplicated
// case-insensitively, in discovery order. Map keys are walked sorted, as Go
// maps have no insertion order.
func collect(results []any) []string {
	var ids []string
	seen := map[string]bool{}
	add := func(v string) {
		v = strings.TrimSpace(v)
		if tableID.MatchString(v) && !seen[strings.ToLower(v)] {
			seen[strings.ToLower(v)] = true
			ids = append(ids, v)
		}
	}
	var walk func(node any, depth int)
	walk = func(node any, depth int) {
		if depth > maxDepth {
			return
		}
		switch n := node.(type) {
		case map[string]any:
			keys := make([]string, 0, len(n))
			for k := range n {
				keys = append(keys, k)
			}
			slices.Sort(keys)
			for _, k := range keys {
				v := n[k]
				switch {
				case idKeys[k]:
					if s, ok := v.(string); ok {
						add(s)
					}
				case k == "tables":
					for _, e := range stringsOf(v) {
						add(e)
					}
				}
				walk(v, depth+1)
			}
		case []any:
			for _, e := range n {
				walk(e, depth+1)
			}
		}
	}
	for _, r := range results {
		walk(r, 0)
	}
	return ids
}

func stringsOf(v any) []string {
	switch l := v.(type) {
	case []string:
		return l
	case []any:
		var out []string
		for _, e := range l {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// firstMention is where the answer first names the candidate, or -1.
func firstMention(candidate, lowered, unpinned string) int {
	pos := indexBounded(lowered, strings.ToLower(candidate))

	_, bare, _ := strings.Cut(candidate, "/")
	var b int
	if plainWord.MatchString(bare) {
		// A plain word such as film is also prose, so only a code span names it.
		b = indexCodeSpan(unpinned, bare)
	} else {
		b = indexBounded(unpinned, strings.ToLower(bare))
	}
	if pos < 0 || (b >= 0 && b < pos) {
		pos = b
	}
	return pos
}

// isWordByte is [0-9a-z_]: the text is lower-cased, and non-ASCII is never a word byte.
func isWordByte(c byte) bool {
	return c == '_' || ('0' <= c && c <= '9') || ('a' <= c && c <= 'z')
}

// indexBounded returns the first index of needle in hay where the characters
// immediately before and after are not [0-9a-z_], or -1.
func indexBounded(hay, needle string) int {
	return boundedFrom(hay, needle, 0)
}

// boundedFrom is indexBounded starting at from, with bounds checked against all of hay.
func boundedFrom(hay, needle string, from int) int {
	for from <= len(hay) {
		i := strings.Index(hay[from:], needle)
		if i < 0 {
			return -1
		}
		at, end := from+i, from+i+len(needle)
		if (at == 0 || !isWordByte(hay[at-1])) && (end == len(hay) || !isWordByte(hay[end])) {
			return at
		}
		from = at + 1
	}
	return -1
}

// allBounded is every non-overlapping bounded occurrence, left to right, as re.sub finds them.
func allBounded(hay, needle string) []int {
	var out []int
	for at := boundedFrom(hay, needle, 0); at >= 0; at = boundedFrom(hay, needle, at+len(needle)) {
		out = append(out, at)
	}
	return out
}

// indexCodeSpan finds "`name" followed by "`" or ".", or -1.
func indexCodeSpan(hay, name string) int {
	needle := "`" + name
	for from := 0; from < len(hay); {
		i := strings.Index(hay[from:], needle)
		if i < 0 {
			return -1
		}
		at := from + i
		if end := at + len(needle); end < len(hay) && (hay[end] == '`' || hay[end] == '.') {
			return at
		}
		from = at + 1
	}
	return -1
}
