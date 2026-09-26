package auth_test

import (
	"slices"
	"strings"
	"testing"

	"urara-vision/backend/internal/auth"
)

var allPerms = []auth.Permission{
	auth.PermProjectView, auth.PermChatUse, auth.PermNoteWrite,
	auth.PermProjectImport, auth.PermVersionImport, auth.PermProjectDelete,
	auth.PermVersionDelete, auth.PermNoteModerate, auth.PermSettingsManage,
	auth.PermUserManage, auth.PermUserDelete,
}

// Rows of the "Permissions" table in wave-2-architecture.md.
var matrix = []struct {
	perm                            auth.Permission
	viewer, creator, admin, service bool
}{
	{auth.PermProjectView, true, true, true, true},
	{auth.PermChatUse, true, true, true, true},
	{auth.PermNoteWrite, true, true, true, false},
	{auth.PermProjectImport, false, true, true, true},
	{auth.PermVersionImport, false, true, true, true},
	{auth.PermProjectDelete, false, false, true, false},
	{auth.PermVersionDelete, false, false, true, false},
	{auth.PermNoteModerate, false, false, true, false},
	{auth.PermSettingsManage, false, false, true, false},
	{auth.PermUserManage, false, false, true, false},
	{auth.PermUserDelete, false, false, true, false},
}

func TestMatrixCoversEveryPermission(t *testing.T) {
	if len(matrix) != len(allPerms) {
		t.Fatalf("matrix has %d rows, want %d", len(matrix), len(allPerms))
	}
}

func TestPermissionMatrix(t *testing.T) {
	principals := map[string]auth.Principal{
		"viewer":  {Kind: auth.KindUser, Role: auth.RoleViewer},
		"creator": {Kind: auth.KindUser, Role: auth.RoleCreator},
		"admin":   {Kind: auth.KindUser, Role: auth.RoleAdmin},
		"service": {Kind: auth.KindService},
	}
	for name, p := range principals {
		var want []auth.Permission
		for _, row := range matrix {
			allowed := map[string]bool{
				"viewer": row.viewer, "creator": row.creator,
				"admin": row.admin, "service": row.service,
			}[name]
			if allowed {
				want = append(want, row.perm)
			}
			if got := p.Can(row.perm); got != allowed {
				t.Errorf("%s.Can(%s) = %v, want %v", name, row.perm, got, allowed)
			}
			if p.Kind == auth.KindUser {
				if got := auth.Can(p.Role, row.perm); got != allowed {
					t.Errorf("Can(%s, %s) = %v, want %v", p.Role, row.perm, got, allowed)
				}
			}
		}
		slices.Sort(want)
		if got := p.Permissions(); !slices.Equal(got, want) {
			t.Errorf("%s permissions = %v, want %v", name, got, want)
		}
	}
}

func TestAnonymousGetsAdminPermissions(t *testing.T) {
	anon := auth.Principal{Kind: auth.KindAnonymous}
	if !slices.Equal(anon.Permissions(), auth.Permissions(auth.RoleAdmin)) {
		t.Errorf("anonymous = %v", anon.Permissions())
	}
}

func TestUnknownRoleHasNoPermissions(t *testing.T) {
	if ps := auth.Permissions("owner"); len(ps) != 0 {
		t.Errorf("permissions = %v", ps)
	}
	if (auth.Principal{Kind: auth.KindUser}).Can(auth.PermProjectView) {
		t.Error("user without role can view")
	}
}

func TestParseRole(t *testing.T) {
	for _, s := range []string{"viewer", "creator", "admin"} {
		if r, err := auth.ParseRole(s); err != nil || string(r) != s {
			t.Errorf("ParseRole(%q) = %q, %v", s, r, err)
		}
	}
	for _, s := range []string{"Admin", "owner", ""} {
		if _, err := auth.ParseRole(s); err == nil {
			t.Errorf("ParseRole(%q) accepted", s)
		}
	}
}

func TestNormaliseUsername(t *testing.T) {
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"Alice", "alice", true},
		{"a.b_c-1", "a.b_c-1", true},
		{"ab", "", false},
		{"a b", "", false},
		{strings.Repeat("a", 64), strings.Repeat("a", 64), true},
		{strings.Repeat("a", 65), "", false},
	}
	for _, c := range cases {
		got, err := auth.NormaliseUsername(c.in)
		if (err == nil) != c.ok || got != c.want {
			t.Errorf("NormaliseUsername(%q) = %q, %v; want %q ok=%v", c.in, got, err, c.want, c.ok)
		}
	}
}
