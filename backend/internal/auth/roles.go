package auth

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

type Role string

const (
	RoleViewer  Role = "viewer"
	RoleCreator Role = "creator"
	RoleAdmin   Role = "admin"
)

// ParseRole is case-sensitive.
func ParseRole(s string) (Role, error) {
	switch r := Role(s); r {
	case RoleViewer, RoleCreator, RoleAdmin:
		return r, nil
	}
	return "", fmt.Errorf("unknown role %q (want viewer, creator or admin)", s)
}

type Permission string

const (
	PermProjectView    Permission = "project.view"
	PermChatUse        Permission = "chat.use"
	PermNoteWrite      Permission = "note.write"
	PermProjectImport  Permission = "project.import"
	PermVersionImport  Permission = "version.import"
	PermProjectDelete  Permission = "project.delete"
	PermVersionDelete  Permission = "version.delete"
	PermNoteModerate   Permission = "note.moderate"
	PermSettingsManage Permission = "settings.manage"
	PermUserManage     Permission = "user.manage"
	PermUserDelete     Permission = "user.delete"
)

// The one permission table; mirrors "Permissions" in wave-2-architecture.md.
var (
	viewerPerms  = []Permission{PermProjectView, PermChatUse, PermNoteWrite}
	creatorPerms = append(slices.Clone(viewerPerms), PermProjectImport, PermVersionImport)
	adminPerms   = append(slices.Clone(creatorPerms),
		PermProjectDelete, PermVersionDelete, PermNoteModerate,
		PermSettingsManage, PermUserManage, PermUserDelete)
	servicePerms = []Permission{PermProjectView, PermChatUse, PermProjectImport, PermVersionImport}
)

func sorted(ps []Permission) []Permission {
	out := slices.Clone(ps)
	slices.Sort(out)
	return out
}

// Permissions returns the role's permissions, sorted; nil for an unknown role.
func Permissions(role Role) []Permission {
	switch role {
	case RoleViewer:
		return sorted(viewerPerms)
	case RoleCreator:
		return sorted(creatorPerms)
	case RoleAdmin:
		return sorted(adminPerms)
	}
	return nil
}

func Can(role Role, p Permission) bool {
	return slices.Contains(Permissions(role), p)
}

var usernameRe = regexp.MustCompile(`^[a-z0-9._-]{3,64}$`)

// NormaliseUsername lower-cases and validates: 3–64 chars of [a-z0-9._-].
func NormaliseUsername(s string) (string, error) {
	u := strings.ToLower(s)
	if !usernameRe.MatchString(u) {
		return "", fmt.Errorf("username must be 3 to 64 characters of a-z, 0-9, '.', '_' or '-'")
	}
	return u, nil
}
