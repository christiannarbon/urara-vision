package api_test

import (
	"testing"
	"time"

	"urara-vision/backend/internal/api"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }

func TestUsernameLimitAndWindow(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	l := api.NewLoginLimiter(clock.now)
	for range 5 {
		if ok, _ := l.Allow("user:alice"); !ok {
			t.Fatal("refused before the limit")
		}
		l.Fail("user:alice")
		clock.t = clock.t.Add(time.Minute)
	}
	ok, wait := l.Allow("user:alice")
	if ok || wait != 10*time.Minute {
		t.Fatalf("6th attempt: ok=%v wait=%v, want refused for 10m", ok, wait)
	}
	clock.t = clock.t.Add(10 * time.Minute)
	if ok, _ := l.Allow("user:alice"); !ok {
		t.Error("still refused after the oldest failure left the window")
	}
}

func TestIPLimitIsTwenty(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	l := api.NewLoginLimiter(clock.now)
	for range 19 {
		l.Fail("ip:192.0.2.1")
	}
	if ok, _ := l.Allow("ip:192.0.2.1"); !ok {
		t.Fatal("refused at 19")
	}
	l.Fail("ip:192.0.2.1")
	if ok, _ := l.Allow("ip:192.0.2.1"); ok {
		t.Error("allowed at 20")
	}
}

func TestResetClearsKey(t *testing.T) {
	l := api.NewLoginLimiter(time.Now)
	for range 5 {
		l.Fail("user:alice")
	}
	l.Reset("user:alice")
	if ok, _ := l.Allow("user:alice"); !ok {
		t.Error("refused after reset")
	}
}
