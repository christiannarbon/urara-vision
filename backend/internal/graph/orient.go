// Deciding which side of a declared join key belongs to which table.
//
// This is the one place the documents cannot be taken at their word: the order
// a join key is written in does not reliably say which column belongs to which
// side, so the sides are matched against the tables' real column lists instead.
package graph

import (
	"fmt"

	"urara-vision/backend/internal/model"
)

// It returns the column belonging to owner first, then the column belonging to
// other, plus whether the assignment could be established from the columns.
func orientJoinKey(left, right string, owner, other *model.Table) (string, string, bool) {
	if left == "" || right == "" {
		return left, right, false
	}
	if left == right {
		// Same name on both sides: order carries no information and none is needed.
		return left, right, true
	}
	ownerCols := columnSet(owner)
	otherCols := columnSet(other)

	leftOwner, rightOther := ownerCols[left], otherCols[right]
	leftOther, rightOwner := otherCols[left], ownerCols[right]

	switch {
	case leftOwner && rightOther && !(leftOther && rightOwner):
		return left, right, true
	case leftOther && rightOwner && !(leftOwner && rightOther):
		return right, left, true
	case leftOwner && rightOther:
		// Both readings work; keep the written order.
		return left, right, true
	case leftOwner:
		return left, right, true
	case rightOwner:
		return right, left, true
	}
	return left, right, false
}

// orientRelationship normalises a relationship's join columns so FromColumn
// always names a column of the declaring table, and records a diagnostic when
// neither side can be matched to either table.
func orientRelationship(r *model.Relationship, owner, other *model.Table, diags *[]model.Diagnostic) {
	if other == nil {
		return
	}
	from, to, ok := orientJoinKey(r.FromColumn, r.ToColumn, owner, other)
	r.FromColumn, r.ToColumn = from, to
	if ok || r.JoinKeyRaw == "" {
		return
	}
	*diags = append(*diags, model.Diagnostic{
		Severity: model.SeverityWarning,
		Code:     "unmatched_join_key",
		Message: fmt.Sprintf("%s joins %s on %q, but neither side names a documented column of either table.",
			owner.Name, other.Name, r.JoinKeyRaw),
		DomainID: owner.DomainID,
		TableID:  owner.ID,
		DocPath:  owner.DocPath,
	})
}
