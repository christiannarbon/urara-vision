//go:build integration

package chat_test

import "testing"

func TestProbes(t *testing.T) {
	assertGolden(t, "probes/healthz.json", chatDo(t, "GET", "/healthz", "", nil))

	ready := chatDo(t, "GET", "/readyz", "", nil)
	assertGolden(t, "probes/readyz.json", ready)
	if body := ready.json(t); body["backend"] != "ok" {
		t.Errorf("readyz %v", body)
	}
}
