package api

import (
	"strings"
	"sync"
	"time"
)

const (
	loginWindow       = 15 * time.Minute
	loginUserFailures = 5
	loginIPFailures   = 20
)

// RateLimiter counts failures per key in a sliding window. It is per replica,
// so the real limit is N × replicas.
type RateLimiter struct {
	mu       sync.Mutex
	now      func() time.Time
	window   time.Duration
	failures map[string][]time.Time
}

// NewLoginLimiter limits "user:" keys to 5 failures and "ip:" keys to 20 per 15 minutes.
func NewLoginLimiter(now func() time.Time) *RateLimiter {
	return &RateLimiter{now: now, window: loginWindow, failures: map[string][]time.Time{}}
}

func limitFor(key string) int {
	if strings.HasPrefix(key, "ip:") {
		return loginIPFailures
	}
	return loginUserFailures
}

// Allow reports whether key may try again, and if not, how long until it may.
func (l *RateLimiter) Allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	fs := l.prune(key)
	if len(fs) < limitFor(key) {
		return true, 0
	}
	return false, fs[0].Add(l.window).Sub(l.now())
}

func (l *RateLimiter) Fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.failures[key] = append(l.prune(key), l.now())
	// Bound memory against many one-off keys.
	if len(l.failures) > 10000 {
		for k := range l.failures {
			l.prune(k)
		}
	}
}

func (l *RateLimiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.failures, key)
}

// prune drops failures outside the window. Callers hold mu.
func (l *RateLimiter) prune(key string) []time.Time {
	cutoff := l.now().Add(-l.window)
	fs := l.failures[key]
	i := 0
	for i < len(fs) && !fs[i].After(cutoff) {
		i++
	}
	fs = fs[i:]
	if len(fs) == 0 {
		delete(l.failures, key)
		return nil
	}
	l.failures[key] = fs
	return fs
}
