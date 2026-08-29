// Reading a table document's Overview block: the property table that carries
// the table's name, type, grain and update frequency.
package parser

import "strings"

// parseOverview reads the Overview section, returning its property table as a
// lookup keyed by normalised property name, along with the section's raw body
// for the prose description that sits outside the table.
//
// The block is written as a two-column table, but the header is not reliably
// "Property | Value" -- some documents put the first property on the header row
// instead, so a two-column table with an unrecognised header is read as data
// rather than skipped.
func parseOverview(secs []section) (map[string]string, string) {
	overview, _ := findSection(secs, "Overview")
	props := map[string]string{}

	for _, tbl := range parseTables(overview) {
		ki := headerIndex(tbl.Header, "property")
		vi := headerIndex(tbl.Header, "value")
		if ki < 0 || vi < 0 {
			if len(tbl.Header) != 2 {
				continue
			}
			ki, vi = 0, 1
			props[normKey(unticked(tbl.Header[0]))] = unticked(tbl.Header[1])
		}
		for _, row := range tbl.Rows {
			k := normKey(unticked(cell(row, ki)))
			if k == "" {
				continue
			}
			props[k] = strings.TrimSpace(cell(row, vi))
		}
	}
	return props, overview
}
