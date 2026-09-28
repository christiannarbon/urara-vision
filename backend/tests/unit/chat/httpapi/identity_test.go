// Ported from chat/tests/unit/test_identity.py, for what exists before the chat routes.
package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMissingIdentityIs401(t *testing.T) {
	h := gated(t, &fakeBackend{enabled: true}, newClock())
	rec := serve(h, httptest.NewRequest("GET", "/api/chat/conversations?snapshot=latest", nil))
	assertGolden(t, "errors/anonymous.json", rec)
	if seen(t, rec) != rec.Header().Get("X-Request-Id") {
		t.Error("body and header request IDs differ")
	}
}

func TestABlankIdentityIs401(t *testing.T) {
	if code := asUser(gated(t, &fakeBackend{enabled: true}, newClock()), "   "); code != http.StatusUnauthorized {
		t.Errorf("status %d", code)
	}
}

func TestIdentityIsCheckedBeforeTheFeatureGate(t *testing.T) {
	f := &fakeBackend{enabled: false}
	h := gated(t, f, newClock())

	if code := asUser(h, ""); code != http.StatusUnauthorized {
		t.Errorf("anonymous with chat off: %d, want 401", code)
	}
	if f.calls() != 0 {
		t.Errorf("%d features calls for an anonymous caller", f.calls())
	}

	req := httptest.NewRequest("GET", "/api/chat/conversations?snapshot=latest", nil)
	req.Header.Set("X-User-Id", "user-1")
	assertGolden(t, "errors/chat-off.json", serve(h, req))
}

func TestAnIdentifiedCallerReachesTheRoute(t *testing.T) {
	if code := asUser(gated(t, &fakeBackend{enabled: true}, newClock()), "user-1"); code != http.StatusOK {
		t.Errorf("status %d, want 200", code)
	}
}

func TestProbesNeedNoIdentity(t *testing.T) {
	h := gated(t, &fakeBackend{enabled: true}, newClock())
	for _, path := range []string{"/healthz", "/readyz"} {
		if rec := get(h, path, ""); rec.Code != http.StatusOK {
			t.Errorf("%s: %d", path, rec.Code)
		}
	}
}
