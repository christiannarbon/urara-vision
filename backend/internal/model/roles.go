// The vocabulary of roles a table can play in a model.
//
// A role is deliberately not a closed set. Documents are written by people
// modelling in whatever style their warehouse uses -- Kimball star and
// snowflake schemas, Data Vault, plain third normal form -- and a role this
// file has never heard of still has to survive the trip to the canvas. So
// TableKind is an open string: the constants below are the roles that get a
// label and a family, and anything else keeps a slug of whatever the document
// called it rather than collapsing to "unknown".
package model

import "strings"

// TableKind is the role a table plays in its model. Open by design: see above.
type TableKind string

// RoleFamily is the modelling style a role belongs to. It exists so the UI can
// offer a coherent vocabulary instead of one long undifferentiated list.
type RoleFamily string

const (
	FamilyKimball    RoleFamily = "kimball"
	FamilyVault      RoleFamily = "vault"
	FamilyRelational RoleFamily = "relational"
	// FamilyOther covers a role read straight from a document that matches
	// none of the vocabularies above.
	FamilyOther RoleFamily = "other"
)

// Kimball: star and snowflake schemas.
const (
	KindFact       TableKind = "fact"
	KindFactless   TableKind = "factless"
	KindDimension  TableKind = "dimension"
	KindOutrigger  TableKind = "outrigger"
	KindBridge     TableKind = "bridge"
	KindJunk       TableKind = "junk"
	KindDegenerate TableKind = "degenerate"
)

// Data Vault.
const (
	KindHub       TableKind = "hub"
	KindLink      TableKind = "link"
	KindSatellite TableKind = "satellite"
	KindPIT       TableKind = "pit"
)

// Third normal form and plain relational models.
const (
	KindEntity      TableKind = "entity"
	KindAssociative TableKind = "associative"
	KindLookup      TableKind = "lookup"
	KindReference   TableKind = "reference"
)

// KindUnknown is a table whose document names no role and whose name follows no
// convention this tool recognises.
const KindUnknown TableKind = "unknown"

// Role describes one role in the known vocabulary.
type Role struct {
	ID     TableKind  `json:"id"`
	Label  string     `json:"label"`
	Family RoleFamily `json:"family"`
	// Connective marks a role whose entire purpose is to join other tables: a
	// fact, a Data Vault link, a junction table. One of these with no
	// resolvable relationship is a documentation gap, and diagnostics say so.
	// A dimension is not connective -- a conformed dimension nothing in this
	// directory joins to is perfectly ordinary.
	Connective bool `json:"connective"`
}

// knownRoles is in display order: the order the UI lists filters and legends
// in, grouped by family with the most common role of each family first.
var knownRoles = []Role{
	{KindFact, "Fact", FamilyKimball, true},
	{KindFactless, "Factless fact", FamilyKimball, true},
	{KindDimension, "Dimension", FamilyKimball, false},
	{KindOutrigger, "Outrigger", FamilyKimball, false},
	{KindBridge, "Bridge", FamilyKimball, true},
	{KindJunk, "Junk dimension", FamilyKimball, false},
	{KindDegenerate, "Degenerate dimension", FamilyKimball, false},

	{KindHub, "Hub", FamilyVault, false},
	{KindLink, "Link", FamilyVault, true},
	{KindSatellite, "Satellite", FamilyVault, false},
	{KindPIT, "Point-in-time", FamilyVault, false},

	{KindEntity, "Entity", FamilyRelational, false},
	{KindAssociative, "Associative", FamilyRelational, true},
	{KindLookup, "Lookup", FamilyRelational, false},
	{KindReference, "Reference", FamilyRelational, false},

	{KindUnknown, "Unknown", FamilyOther, false},
}

var roleByID = func() map[TableKind]Role {
	m := make(map[TableKind]Role, len(knownRoles))
	for _, r := range knownRoles {
		m[r.ID] = r
	}
	return m
}()

// KnownRoles returns the built-in vocabulary in display order.
func KnownRoles() []Role {
	out := make([]Role, len(knownRoles))
	copy(out, knownRoles)
	return out
}

// RoleOf describes a kind. A kind outside the built-in vocabulary is not an
// error: it gets its own role, labelled from its slug and filed under
// FamilyOther, so a model built on a vocabulary this tool has never seen still
// reads with its own names.
func RoleOf(k TableKind) Role {
	if r, ok := roleByID[k]; ok {
		return r
	}
	if k == "" {
		return roleByID[KindUnknown]
	}
	return Role{ID: k, Label: RoleLabel(k), Family: FamilyOther}
}

// IsConnective reports whether a role exists to join other tables.
func IsConnective(k TableKind) bool { return RoleOf(k).Connective }

// RoleLabel turns a role slug into a display label: "point_in_time" reads as
// "Point in time".
//
// Sentence case, not title case. The built-in labels above are written that way
// -- "Junk dimension", "Factless fact" -- and a role read from a document has
// to sit in the same list without looking like a different kind of thing.
func RoleLabel(k TableKind) string {
	parts := strings.FieldsFunc(string(k), func(r rune) bool { return r == '_' || r == '-' })
	if len(parts) == 0 {
		return string(k)
	}
	parts[0] = strings.ToUpper(parts[0][:1]) + parts[0][1:]
	return strings.Join(parts, " ")
}
