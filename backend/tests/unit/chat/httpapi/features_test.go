// Ported from chat/tests/unit/test_features_guard.py.
package httpapi_test

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"testing"
	"time"

	"urara-vision/backend/internal/chat/apiclient"
	"urara-vision/backend/internal/chat/httpapi"
	"urara-vision/backend/internal/chat/reqctx"
)

const routeOK = http.StatusOK // the list route ran

func TestEnabledRunsTheRoute(t *testing.T) {
	if code := asUser(gated(t, &fakeBackend{enabled: true}, newClock()), "u"); code != routeOK {
		t.Errorf("status %d", code)
	}
}

func TestDisabledIs503(t *testing.T) {
	if code := asUser(gated(t, &fakeBackend{enabled: false}, newClock()), "u"); code != http.StatusServiceUnavailable {
		t.Errorf("status %d", code)
	}
}

func TestTheAnswerIsCachedForTheTTL(t *testing.T) {
	f, c := &fakeBackend{enabled: true}, newClock()
	h := gated(t, f, c)

	asUser(h, "u")
	c.advance(featuresTTL - time.Second)
	asUser(h, "u")
	if f.calls() != 1 {
		t.Fatalf("%d calls inside the TTL", f.calls())
	}
	c.advance(time.Second)
	asUser(h, "u")
	if f.calls() != 2 {
		t.Errorf("%d calls after the TTL", f.calls())
	}
}

func TestAChangeIsSeenAfterTheTTL(t *testing.T) {
	f, c := &fakeBackend{enabled: true}, newClock()
	h := gated(t, f, c)

	if code := asUser(h, "u"); code != routeOK {
		t.Fatalf("status %d", code)
	}
	f.set(func(f *fakeBackend) { f.enabled = false })
	if code := asUser(h, "u"); code != routeOK {
		t.Errorf("changed inside the TTL: %d", code)
	}
	c.advance(featuresTTL)
	if code := asUser(h, "u"); code != http.StatusServiceUnavailable {
		t.Errorf("after the TTL: %d", code)
	}
}

func TestABackendFailureBeforeAnyAnswerAllows(t *testing.T) {
	for name, err := range map[string]error{
		"unreachable":  &apiclient.Error{Status: 502, Message: "backend unreachable"},
		"server error": &apiclient.Error{Status: 500, Message: "internal error"},
		"malformed":    errors.New("backend response: features.chat is missing"),
	} {
		f := &fakeBackend{featuresErr: err}
		if code := asUser(gated(t, f, newClock()), "u"); code != routeOK {
			t.Errorf("%s: status %d, want allowed", name, code)
		}
	}
}

func TestAKnownOffSurvivesABackendError(t *testing.T) {
	f, c := &fakeBackend{enabled: false}, newClock()
	h := gated(t, f, c)
	if code := asUser(h, "u"); code != http.StatusServiceUnavailable {
		t.Fatalf("status %d", code)
	}
	f.set(func(f *fakeBackend) { f.featuresErr = errors.New("down") })
	c.advance(featuresTTL)
	if code := asUser(h, "u"); code != http.StatusServiceUnavailable {
		t.Errorf("status %d after a failed refresh, want still refused", code)
	}
	if f.calls() != 2 {
		t.Errorf("%d calls, want the refresh attempted", f.calls())
	}
}

func TestAFailureIsCachedForTheTTL(t *testing.T) {
	f := &fakeBackend{featuresErr: errors.New("down")}
	h := gated(t, f, newClock())
	asUser(h, "u")
	asUser(h, "u")
	if f.calls() != 1 {
		t.Errorf("%d calls", f.calls())
	}
}

func TestConcurrentCallersOnAColdCacheRefreshOnce(t *testing.T) {
	f := &fakeBackend{enabled: true, delay: 20 * time.Millisecond}
	gate := httpapi.NewFeatureGate(f, featuresTTL, newClock().now, slog.New(slog.DiscardHandler))

	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := gate.Require(context.Background()); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if f.calls() != 1 {
		t.Errorf("%d features calls", f.calls())
	}

	h := gated(t, &fakeBackend{enabled: true, delay: 20 * time.Millisecond}, newClock())
	codes := make(chan int, 2)
	for range 2 {
		go func() { codes <- asUser(h, "u") }()
	}
	if a, b := <-codes, <-codes; a != routeOK || b != routeOK {
		t.Errorf("statuses %d, %d", a, b)
	}
}

func TestTheFeaturesCallRunsAsTheService(t *testing.T) {
	f := &fakeBackend{enabled: true}
	asUser(gated(t, f, newClock()), "user-1")
	if len(f.featureUsers) != 1 || f.featureUsers[0] != "" {
		t.Errorf("features called as %q, want the service", f.featureUsers)
	}
}

func TestACancelledCallerDoesNotFailTheRefresh(t *testing.T) {
	f := &fakeBackend{enabled: false}
	gate := httpapi.NewFeatureGate(f, featuresTTL, newClock().now, slog.New(slog.DiscardHandler))
	ctx, cancel := context.WithCancel(reqctx.WithUserID(context.Background(), "u"))
	cancel()
	if err := gate.Require(ctx); !errors.Is(err, httpapi.ErrChatDisabled) || f.cancelled {
		t.Errorf("Require = %v, cancelled context reached the backend: %v", err, f.cancelled)
	}
}

func TestProbesAreNotGated(t *testing.T) {
	f := &fakeBackend{enabled: false}
	h := gated(t, f, newClock())
	if rec := get(h, "/healthz", ""); rec.Code != http.StatusOK {
		t.Errorf("status %d", rec.Code)
	}
	if f.calls() != 0 {
		t.Errorf("%d features calls from a probe", f.calls())
	}
}
