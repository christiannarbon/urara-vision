// Reading the prose between the tables: descriptions, caveat bullets and the
// embedded diagram.
package parser

import (
	"regexp"
	"strings"
)

// prose returns the non-table, non-heading paragraphs of a body, joined.
func prose(body string) string {
	var keep []string
	for _, line := range strings.Split(body, "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "|") || strings.HasPrefix(t, "#") {
			continue
		}
		keep = append(keep, t)
	}
	return strings.TrimSpace(strings.Join(keep, "\n"))
}

var bulletRe = regexp.MustCompile(`^\s*[-*+]\s+(.*)$`)

// bullets extracts list items from a body, treating a non-list paragraph as a
// single item so prose-only "Notes" sections are not lost.
func bullets(body string) []string {
	var out []string
	var cur strings.Builder
	flush := func() {
		if v := strings.TrimSpace(cur.String()); v != "" {
			out = append(out, v)
		}
		cur.Reset()
	}
	inList := false
	for _, line := range strings.Split(body, "\n") {
		if m := bulletRe.FindStringSubmatch(line); m != nil {
			flush()
			inList = true
			cur.WriteString(strings.TrimSpace(m[1]))
			continue
		}
		t := strings.TrimSpace(line)
		if t == "" {
			flush()
			continue
		}
		if strings.HasPrefix(t, "|") || strings.HasPrefix(t, "#") {
			continue
		}
		if inList {
			// Continuation of the previous bullet.
			cur.WriteString(" " + t)
			continue
		}
		cur.WriteString(t + " ")
	}
	flush()
	return out
}

var fenceRe = regexp.MustCompile("(?s)```\\s*mermaid\\s*\n(.*?)```")

// mermaidBlock returns the first mermaid fenced block in a body.
func mermaidBlock(body string) string {
	if m := fenceRe.FindStringSubmatch(body); m != nil {
		return strings.TrimSpace(m[1])
	}
	return ""
}
