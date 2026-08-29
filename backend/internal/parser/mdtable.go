// Reading markdown pipe tables, which is where all the structured data in a
// model document lives.
package parser

import (
	"regexp"
	"strings"
)

var (
	dividerRe = regexp.MustCompile(`^\s*\|?\s*:?-{2,}:?\s*(\|\s*:?-{2,}:?\s*)*\|?\s*$`)
	tickRe    = regexp.MustCompile("`([^`]*)`")
	boldRe    = regexp.MustCompile(`\*\*(.*?)\*\*`)
)

// mdTable is a parsed GitHub-flavoured markdown table.
type mdTable struct {
	Header []string
	Rows   [][]string
}

// parseTables extracts every markdown table found in a block of text. A table
// is a run of pipe-delimited lines; the divider row is consumed, not returned.
func parseTables(body string) []mdTable {
	var tables []mdTable
	var cur *mdTable
	flush := func() {
		if cur != nil && len(cur.Header) > 0 {
			tables = append(tables, *cur)
		}
		cur = nil
	}
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "|") {
			flush()
			continue
		}
		if dividerRe.MatchString(trimmed) {
			// Divider confirms the preceding line was a header; skip it.
			continue
		}
		cells := splitRow(trimmed)
		if cur == nil {
			cur = &mdTable{Header: cells}
			continue
		}
		cur.Rows = append(cur.Rows, cells)
	}
	flush()
	return tables
}

// splitRow splits one markdown table row into trimmed cells, honouring
// backslash-escaped pipes.
func splitRow(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	var cells []string
	var sb strings.Builder
	escaped := false
	for _, r := range line {
		switch {
		case escaped:
			sb.WriteRune(r)
			escaped = false
		case r == '\\':
			escaped = true
		case r == '|':
			cells = append(cells, strings.TrimSpace(sb.String()))
			sb.Reset()
		default:
			sb.WriteRune(r)
		}
	}
	cells = append(cells, strings.TrimSpace(sb.String()))
	return cells
}

// cell returns column i of a row, or "" when the row is short.
func cell(row []string, i int) string {
	if i < 0 || i >= len(row) {
		return ""
	}
	return row[i]
}

// unticked strips surrounding markdown code ticks and bold markers, returning
// the plain text a human would read.
func unticked(s string) string {
	s = strings.TrimSpace(s)
	if m := tickRe.FindStringSubmatch(s); m != nil && strings.HasPrefix(s, "`") {
		return strings.TrimSpace(m[1])
	}
	s = boldRe.ReplaceAllString(s, "$1")
	return strings.TrimSpace(strings.Trim(s, "`"))
}

// allTicked returns every backtick-quoted token in a string, in order.
func allTicked(s string) []string {
	ms := tickRe.FindAllStringSubmatch(s, -1)
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		if v := strings.TrimSpace(m[1]); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// headerIndex finds the position of a column whose header contains want.
func headerIndex(header []string, want ...string) int {
	for _, w := range want {
		lw := strings.ToLower(w)
		for i, h := range header {
			if strings.Contains(strings.ToLower(unticked(h)), lw) {
				return i
			}
		}
	}
	return -1
}
