package auth_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"urara-vision/backend/internal/auth"
)

func TestPasswordPolicyByteLength(t *testing.T) {
	cases := []struct {
		name string
		pw   string
		ok   bool
	}{
		{"11 bytes", strings.Repeat("a", 11), false},
		{"12 bytes", strings.Repeat("a", 12), true},
		{"72 bytes", strings.Repeat("a", 72), true},
		{"73 bytes", strings.Repeat("a", 73), false},
		{"24 Japanese runes = 72 bytes", strings.Repeat("あ", 24), true},
	}
	for _, c := range cases {
		err := auth.CheckPasswordPolicy(c.pw)
		if (err == nil) != c.ok {
			t.Errorf("%s: err = %v, want ok=%v", c.name, err, c.ok)
		}
		if err != nil && !errors.Is(err, auth.ErrPasswordPolicy) {
			t.Errorf("%s: err %v is not ErrPasswordPolicy", c.name, err)
		}
	}
}

func TestPolicyMessageNamesLimits(t *testing.T) {
	err := auth.CheckPasswordPolicy("short")
	if err == nil || !strings.Contains(err.Error(), "12") || !strings.Contains(err.Error(), "72") {
		t.Errorf("err = %v, want both limits named", err)
	}
}

func TestHashPasswordChecksPolicy(t *testing.T) {
	if _, err := auth.HashPassword("short"); !errors.Is(err, auth.ErrPasswordPolicy) {
		t.Errorf("err = %v, want ErrPasswordPolicy", err)
	}
}

func TestHashThenVerify(t *testing.T) {
	pw := "correct horse battery"
	h1, err := auth.HashPassword(pw)
	if err != nil {
		t.Fatal(err)
	}
	h2, err := auth.HashPassword(pw)
	if err != nil {
		t.Fatal(err)
	}
	if h1 == h2 {
		t.Error("two hashes of the same password are equal")
	}
	if !auth.VerifyPassword(h1, pw) {
		t.Error("right password rejected")
	}
	if auth.VerifyPassword(h1, "wrong horse battery") {
		t.Error("wrong password accepted")
	}
}

func TestVerifyAgainstDummyTakesComparableTime(t *testing.T) {
	if testing.Short() {
		t.Skip("timing check")
	}
	pw := "correct horse battery"
	h, err := auth.HashPassword(pw)
	if err != nil {
		t.Fatal(err)
	}
	auth.VerifyAgainstDummy(pw) // warm the sync.Once

	start := time.Now()
	auth.VerifyPassword(h, pw)
	real := time.Since(start)

	start = time.Now()
	auth.VerifyAgainstDummy(pw)
	dummy := time.Since(start)

	if dummy < real/2 {
		t.Errorf("dummy %v < 50%% of real %v", dummy, real)
	}
}
