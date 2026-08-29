// Stable identifiers and diagnostic ordering.
package graph

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"sort"

	"urara-vision/backend/internal/model"
)

// edgeID gives every relationship a stable identifier derived from its
// endpoints, so re-ingesting the same documents yields the same IDs.
func edgeID(from, target, fromCol, toCol string, ordinal int) string {
	h := sha1.Sum([]byte(fmt.Sprintf("%s|%s|%s|%s|%d", from, target, fromCol, toCol, ordinal)))
	return hex.EncodeToString(h[:8])
}

// severityRank orders diagnostics worst-first.
func severityRank(s string) int {
	switch s {
	case model.SeverityError:
		return 0
	case model.SeverityWarning:
		return 1
	default:
		return 2
	}
}

func sortDiagnostics(d []model.Diagnostic) {
	sort.SliceStable(d, func(i, j int) bool {
		if a, b := severityRank(d[i].Severity), severityRank(d[j].Severity); a != b {
			return a < b
		}
		if d[i].Code != d[j].Code {
			return d[i].Code < d[j].Code
		}
		return d[i].Message < d[j].Message
	})
}
