// Splitting a document into its sections.
//
// The documents are markdown written by hand, so section headings are matched
// case-insensitively and by any of several spellings.
package parser

import (
	"regexp"
	"strings"
)

// section is a heading and the raw body that follows it, up to the next
// heading of the same or higher level.
type section struct {
	level int
	title string
	body  string
}

var headingRe = regexp.MustCompile(`(?m)^(#{1,6})\s+(.*?)\s*$`)

// splitSections walks the document and returns every heading with its body.
// Bodies exclude nested headings' own text but include their content, so a
// level-2 section carries its level-3 children.
func splitSections(doc string) []section {
	locs := headingRe.FindAllStringSubmatchIndex(doc, -1)
	if len(locs) == 0 {
		return nil
	}
	out := make([]section, 0, len(locs))
	for i, loc := range locs {
		level := loc[3] - loc[2]
		title := strings.TrimSpace(doc[loc[4]:loc[5]])
		bodyStart := loc[1]
		bodyEnd := len(doc)
		// The body runs until the next heading at the same or shallower depth.
		for j := i + 1; j < len(locs); j++ {
			if locs[j][3]-locs[j][2] <= level {
				bodyEnd = locs[j][0]
				break
			}
		}
		if bodyStart > bodyEnd {
			bodyStart = bodyEnd
		}
		out = append(out, section{level: level, title: title, body: doc[bodyStart:bodyEnd]})
	}
	return out
}

// findSection returns the body of the first section whose title matches any of
// the supplied names, case-insensitively. Matching is exact after trimming, then
// falls back to a prefix match so "Notes / Caveats" also answers to "Notes".
func findSection(secs []section, names ...string) (string, bool) {
	for _, name := range names {
		want := strings.ToLower(name)
		for _, s := range secs {
			if strings.ToLower(s.title) == want {
				return s.body, true
			}
		}
	}
	for _, name := range names {
		want := strings.ToLower(name)
		for _, s := range secs {
			if strings.HasPrefix(strings.ToLower(s.title), want) {
				return s.body, true
			}
		}
	}
	return "", false
}

// firstHeading returns the text of the first level-1 heading, if any.
func firstHeading(secs []section) string {
	for _, s := range secs {
		if s.level == 1 {
			return s.title
		}
	}
	return ""
}
