// Package auth holds identity logic that needs no database or HTTP.
package auth

import (
	"errors"
	"fmt"
	"sync"

	"golang.org/x/crypto/bcrypt"
)

const BcryptCost = 12

// Limits are in bytes: bcrypt reads only the first 72.
const MinPasswordBytes, MaxPasswordBytes = 12, 72

var ErrPasswordPolicy = errors.New("password policy")

func CheckPasswordPolicy(pw string) error {
	if n := len(pw); n < MinPasswordBytes || n > MaxPasswordBytes {
		return fmt.Errorf("%w: password must be %d to %d bytes, got %d",
			ErrPasswordPolicy, MinPasswordBytes, MaxPasswordBytes, n)
	}
	return nil
}

func HashPassword(pw string) (string, error) {
	if err := CheckPasswordPolicy(pw); err != nil {
		return "", err
	}
	h, err := bcrypt.GenerateFromPassword([]byte(pw), BcryptCost)
	if err != nil {
		return "", err
	}
	return string(h), nil
}

func VerifyPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

var (
	dummyOnce sync.Once
	dummyHash []byte
)

// VerifyAgainstDummy spends the same time as a real check when the user does
// not exist, so login timing does not reveal which usernames exist.
func VerifyAgainstDummy(pw string) {
	dummyOnce.Do(func() {
		dummyHash, _ = bcrypt.GenerateFromPassword([]byte("dummy-password-for-timing"), BcryptCost)
	})
	_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(pw))
}
