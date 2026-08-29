// Turning the free text documents are written in into the canonical values the
// model uses.
//
// Documents are written by people, so a type is "Dimension (Conformed)" as
// often as "dimension", a Data Vault model says "Satellite" where a star schema
// says "Dimension", and a join key may be written either way round. Every
// tolerance the parser extends lives here.
package parser

import (
	"regexp"
	"sort"
	"strings"

	"urara-vision/backend/internal/model"
)

// nonSlugRe matches everything a role identifier may not contain.
var nonSlugRe = regexp.MustCompile(`[^a-z0-9]+`)

// splitJoinKey turns "alpha_id_1 = alpha_id" into its two sides.
func splitJoinKey(raw string) (string, string) {
	s := unticked(raw)
	parts := strings.SplitN(s, "=", 2)
	if len(parts) != 2 {
		return strings.TrimSpace(strings.Trim(s, "`")), ""
	}
	return strings.TrimSpace(strings.Trim(parts[0], "` ")), strings.TrimSpace(strings.Trim(parts[1], "` "))
}

// stripTables removes pipe-delimited rows so only prose remains.
func stripTables(body string) string {
	var keep []string
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "|") {
			continue
		}
		keep = append(keep, line)
	}
	return strings.Join(keep, "\n")
}

// normKey lowercases and collapses whitespace in a property label.
func normKey(s string) string {
	return strings.TrimSpace(whitespaceRe.ReplaceAllString(strings.ToLower(s), " "))
}

// roleAliases maps a phrase that may appear in a document's Type property onto
// a role. Matching is by whole words and longest phrase first, so "junk
// dimension" beats "dimension" and "factless fact" beats "fact".
var roleAliases = map[string]model.TableKind{
	// Kimball.
	"fact":                      model.KindFact,
	"fact table":                model.KindFact,
	"transaction fact":          model.KindFact,
	"periodic snapshot":         model.KindFact,
	"accumulating snapshot":     model.KindFact,
	"aggregate fact":            model.KindFact,
	"factless":                  model.KindFactless,
	"factless fact":             model.KindFactless,
	"coverage fact":             model.KindFactless,
	"dim":                       model.KindDimension,
	"dimension":                 model.KindDimension,
	"conformed dimension":       model.KindDimension,
	"role playing dimension":    model.KindDimension,
	"role-playing dimension":    model.KindDimension,
	"slowly changing dimension": model.KindDimension,
	"scd":                       model.KindDimension,
	"mini dimension":            model.KindDimension,
	"mini-dimension":            model.KindDimension,
	"shrunken dimension":        model.KindDimension,
	"outrigger":                 model.KindOutrigger,
	"outrigger dimension":       model.KindOutrigger,
	"sub dimension":             model.KindOutrigger,
	"sub-dimension":             model.KindOutrigger,
	"subdimension":              model.KindOutrigger,
	"snowflaked dimension":      model.KindOutrigger,
	"snowflake dimension":       model.KindOutrigger,
	"normalised dimension":      model.KindOutrigger,
	"normalized dimension":      model.KindOutrigger,
	"bridge":                    model.KindBridge,
	"bridge table":              model.KindBridge,
	"multi valued bridge":       model.KindBridge,
	"multi-valued bridge":       model.KindBridge,
	"helper table":              model.KindBridge,
	"junk":                      model.KindJunk,
	"junk dimension":            model.KindJunk,
	"degenerate":                model.KindDegenerate,
	"degenerate dimension":      model.KindDegenerate,

	// Data Vault.
	"hub":                   model.KindHub,
	"hub table":             model.KindHub,
	"link":                  model.KindLink,
	"link table":            model.KindLink,
	"same as link":          model.KindLink,
	"same-as link":          model.KindLink,
	"hierarchical link":     model.KindLink,
	"sat":                   model.KindSatellite,
	"satellite":             model.KindSatellite,
	"effectivity satellite": model.KindSatellite,
	"pit":                   model.KindPIT,
	"point in time":         model.KindPIT,
	"point-in-time":         model.KindPIT,

	// Third normal form and plain relational.
	"entity":             model.KindEntity,
	"base table":         model.KindEntity,
	"normalised entity":  model.KindEntity,
	"normalized entity":  model.KindEntity,
	"associative":        model.KindAssociative,
	"associative entity": model.KindAssociative,
	"associative table":  model.KindAssociative,
	"junction":           model.KindAssociative,
	"junction table":     model.KindAssociative,
	"join table":         model.KindAssociative,
	"cross reference":    model.KindAssociative,
	"cross-reference":    model.KindAssociative,
	"xref":               model.KindAssociative,
	"lookup":             model.KindLookup,
	"lookup table":       model.KindLookup,
	"code table":         model.KindLookup,
	"reference":          model.KindReference,
	"reference table":    model.KindReference,
	"reference data":     model.KindReference,
	"master data":        model.KindReference,
}

// aliasOrder lists every alias longest-first, which is the order they must be
// tried in: a Type of "junk dimension" contains "dimension" too, and the
// specific reading is the right one.
var aliasOrder = func() []string {
	out := make([]string, 0, len(roleAliases))
	for a := range roleAliases {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool {
		if len(out[i]) != len(out[j]) {
			return len(out[i]) > len(out[j])
		}
		return out[i] < out[j]
	})
	return out
}()

// maxSlugWords caps how much text is accepted as a role of its own. A Type cell
// holding a phrase or two is someone naming a role this tool has not heard of;
// a Type cell holding a sentence is prose that landed in the wrong column.
const maxSlugWords = 3

// normaliseKind maps a free-text type label onto a role.
//
// A label matching none of the aliases is not thrown away: if it is short
// enough to be a role name it is slugified and becomes a role in its own right.
// That is what keeps the vocabulary open -- a model built on roles nobody here
// anticipated still draws with its own names instead of a canvas full of
// "unknown".
func normaliseKind(raw string) model.TableKind {
	l := normKey(raw)
	if l == "" {
		return model.KindUnknown
	}
	if k, ok := roleAliases[l]; ok {
		return k
	}
	words := strings.Fields(l)
	for _, a := range aliasOrder {
		if containsPhrase(words, strings.Fields(a)) {
			return roleAliases[a]
		}
	}
	return slugKind(l)
}

// containsPhrase reports whether phrase appears in words as a run of whole
// words. Whole words rather than a substring: "hub" must not be found inside
// "github".
func containsPhrase(words, phrase []string) bool {
	if len(phrase) == 0 || len(phrase) > len(words) {
		return false
	}
	for i := 0; i+len(phrase) <= len(words); i++ {
		match := true
		for j, w := range phrase {
			if words[i+j] != w {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// slugKind turns an unrecognised type label into a role identifier, or gives up
// and returns KindUnknown. The identifier is restricted to lowercase letters,
// digits and underscores so it stays safe to use as a graph property and as a
// selector in the UI's stylesheet.
func slugKind(l string) model.TableKind {
	// A parenthetical qualifier describes an instance, not a role:
	// "Anchor (historised)" is still an anchor.
	if i := strings.IndexByte(l, '('); i > 0 {
		l = strings.TrimSpace(l[:i])
	}
	words := strings.Fields(nonSlugRe.ReplaceAllString(l, " "))
	if len(words) == 0 || len(words) > maxSlugWords {
		return model.KindUnknown
	}
	slug := strings.Join(words, "_")
	if slug == "" || len(slug) > 32 || !isLower(rune(slug[0])) {
		return model.KindUnknown
	}
	return model.TableKind(slug)
}

func isLower(r rune) bool { return r >= 'a' && r <= 'z' }

// namePrefixes and nameSuffixes are the naming conventions kindFromName reads,
// longest first so "factless_" is not mistaken for "fact".
var namePrefixes = []struct {
	affix string
	kind  model.TableKind
}{
	{"factless_", model.KindFactless}, {"fact_", model.KindFact}, {"fct_", model.KindFact},
	{"dimension_", model.KindDimension}, {"dim_", model.KindDimension},
	{"outrigger_", model.KindOutrigger}, {"junk_", model.KindJunk},
	{"bridge_", model.KindBridge}, {"brg_", model.KindBridge},
	{"hub_", model.KindHub}, {"link_", model.KindLink}, {"lnk_", model.KindLink},
	{"satellite_", model.KindSatellite}, {"sat_", model.KindSatellite},
	{"pit_", model.KindPIT},
	{"xref_", model.KindAssociative}, {"map_", model.KindAssociative},
	{"lookup_", model.KindLookup}, {"lkp_", model.KindLookup},
	{"ref_", model.KindReference},
}

var nameSuffixes = []struct {
	affix string
	kind  model.TableKind
}{
	{"_factless", model.KindFactless}, {"_fact", model.KindFact},
	{"_dimension", model.KindDimension}, {"_dim", model.KindDimension},
	{"_outrigger", model.KindOutrigger}, {"_bridge", model.KindBridge},
	{"_hub", model.KindHub}, {"_link", model.KindLink},
	{"_satellite", model.KindSatellite}, {"_sat", model.KindSatellite},
	{"_pit", model.KindPIT}, {"_xref", model.KindAssociative},
	{"_lookup", model.KindLookup}, {"_ref", model.KindReference},
}

// kindFromName is the fallback when a document omits or mangles its Type. Only
// the naming conventions are read here -- an explicit Type always wins, however
// oddly the table happens to be named.
func kindFromName(name string) model.TableKind {
	l := strings.ToLower(strings.TrimSpace(name))
	for _, p := range namePrefixes {
		if strings.HasPrefix(l, p.affix) {
			return p.kind
		}
	}
	for _, s := range nameSuffixes {
		if strings.HasSuffix(l, s.affix) {
			return s.kind
		}
	}
	return model.KindUnknown
}

// normaliseCardinality trims qualifiers like "One-to-one (snapshot)" down to
// the cardinality itself while keeping the original wording readable.
func normaliseCardinality(raw string) string {
	s := strings.TrimSpace(unticked(raw))
	if s == "" {
		return ""
	}
	return s
}

// humanise turns a directory slug into a display title.
func humanise(s string) string {
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == '_' || r == '-' })
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}
