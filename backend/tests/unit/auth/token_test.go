package auth_test

import (
	"regexp"
	"testing"

	"urara-vision/backend/internal/auth"
)

func TestSessionTokens(t *testing.T) {
	urlSafe := regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)
	seen := map[string]bool{}
	for range 1000 {
		tok, hash, err := auth.NewSessionToken()
		if err != nil {
			t.Fatal(err)
		}
		if !urlSafe.MatchString(tok) {
			t.Fatalf("token %q is not 43 URL-safe characters", tok)
		}
		if seen[tok] {
			t.Fatalf("duplicate token %q", tok)
		}
		seen[tok] = true
		if auth.HashToken(tok) != hash {
			t.Fatalf("HashToken(%q) != returned hash", tok)
		}
	}
}
