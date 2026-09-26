package auth_test

import (
	"context"
	"testing"

	"urara-vision/backend/internal/auth"
)

func TestPrincipalContextRoundTrip(t *testing.T) {
	want := auth.Principal{Kind: auth.KindUser, UserID: "u1", Username: "alice", Role: auth.RoleCreator}
	got, ok := auth.PrincipalFrom(auth.WithPrincipal(context.Background(), want))
	if !ok || got != want {
		t.Errorf("got %+v, %v; want %+v", got, ok, want)
	}
}

func TestMissingPrincipal(t *testing.T) {
	if _, ok := auth.PrincipalFrom(context.Background()); ok {
		t.Error("PrincipalFrom on empty context returned true")
	}
}
