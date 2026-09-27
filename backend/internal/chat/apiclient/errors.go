package apiclient

import (
	"errors"
	"fmt"
)

// Sentinels for errors.Is against an *Error.
var (
	ErrNotFound    = errors.New("backend: not found")
	ErrForbidden   = errors.New("backend: forbidden")
	ErrRejected    = errors.New("backend: rejected")
	ErrUnavailable = errors.New("backend: unavailable")
)

// Error is a failed backend call. A 5xx matches no sentinel.
type Error struct {
	Status  int
	Message string

	unreachable bool
	cause       error
}

func (e *Error) Error() string {
	return fmt.Sprintf("backend returned %d: %s", e.Status, e.Message)
}

func (e *Error) Unwrap() error { return e.cause }

func (e *Error) Is(target error) bool {
	switch target {
	case ErrNotFound:
		return e.Status == 404
	case ErrForbidden:
		return e.Status == 403
	case ErrRejected:
		return e.Status >= 400 && e.Status < 500 && e.Status != 404 && e.Status != 403
	case ErrUnavailable:
		return e.unreachable
	}
	return false
}

// A transport failure is reported as a 502, as Python's BackendUnavailable is.
func unreachable(err error) *Error {
	return &Error{Status: 502, Message: "backend unreachable: " + err.Error(), unreachable: true, cause: err}
}
