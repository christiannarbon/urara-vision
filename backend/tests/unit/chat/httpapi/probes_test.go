package httpapi_test

import "testing"

func TestHealthzMatchesTheGoldenAndNeverTouchesTheBackend(t *testing.T) {
	f := &fakeBackend{unhealthy: true}
	assertGolden(t, "probes/healthz.json", get(gated(t, f, newClock()), "/healthz", ""))
	if n := f.health(); n != 0 {
		t.Errorf("%d backend health calls", n)
	}
}

func TestReadyzMatchesTheGolden(t *testing.T) {
	assertGolden(t, "probes/readyz.json", get(gated(t, &fakeBackend{}, newClock()), "/readyz", ""))
	assertGolden(t, "probes/readyz-backend-down.json",
		get(gated(t, &fakeBackend{unhealthy: true}, newClock()), "/readyz", ""))
}
