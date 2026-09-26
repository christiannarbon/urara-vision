package auth

import (
	"context"
	"slices"
)

type Kind string

const (
	KindUser      Kind = "user"
	KindService   Kind = "service"
	KindAnonymous Kind = "anonymous"
)

type Principal struct {
	Kind        Kind
	UserID      string
	Username    string
	DisplayName string
	Role        Role // for KindUser; empty otherwise
}

// Permissions: anonymous only exists with AUTH_DISABLED, so it gets admin's.
func (p Principal) Permissions() []Permission {
	switch p.Kind {
	case KindUser:
		return Permissions(p.Role)
	case KindService:
		return sorted(servicePerms)
	case KindAnonymous:
		return Permissions(RoleAdmin)
	}
	return nil
}

func (p Principal) Can(perm Permission) bool {
	return slices.Contains(p.Permissions(), perm)
}

type principalKey struct{}

func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

func PrincipalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}
