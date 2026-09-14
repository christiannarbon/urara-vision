package projectmeta_test

import (
	"strings"
	"testing"

	"urara-vision/backend/internal/projectmeta"
)

func TestSlug(t *testing.T) {
	cases := []struct {
		name, want string
	}{
		{"jaffle-shop-ddd", "jaffle-shop-ddd"},
		{"Jaffle Shop", "jaffle-shop"},
		{"  --Fintech__BI--  ", "fintech-bi"},
		{"a..b", "a-b"},
		{"日本語", ""},
		{"Café 2", "caf-2"},
		{strings.Repeat("a", 70), strings.Repeat("a", 64)},
		// The cut lands on the "-", which the second trim removes.
		{strings.Repeat("a", 63) + " b", strings.Repeat("a", 63)},
	}
	for _, c := range cases {
		if got := projectmeta.Slug(c.name); got != c.want {
			t.Errorf("Slug(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestLegacySlug(t *testing.T) {
	const id = "0b7c6a4e-1f2d-4c3b-9a8e-5d6f7a8b9c0d"
	got := projectmeta.LegacySlug(id)
	if got != projectmeta.LegacySlug(id) {
		t.Error("LegacySlug is not stable")
	}
	if !strings.HasPrefix(got, "legacy-") || len(got) != 19 {
		t.Errorf("LegacySlug = %q, want legacy- plus 12 hex characters", got)
	}
	if got == projectmeta.LegacySlug(id+"x") {
		t.Error("LegacySlug gave two snapshot IDs the same slug")
	}
}
