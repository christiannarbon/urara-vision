package httpapi_test

import (
	"reflect"
	"testing"

	"urara-vision/backend/internal/chat/config"
	"urara-vision/backend/internal/chat/httpapi"
)

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

func TestModelInfo(t *testing.T) {
	if got := httpapi.ModelInfo(chatSettings()); !reflect.DeepEqual(got, map[string]string{
		"provider": "vertex", "model": "gemini-2.5-flash", "location": "us-central1",
	}) {
		t.Errorf("vertex: %v", got)
	}
	claude := &config.Settings{LLMProvider: "vertex-claude", LLMModel: "m", VertexLocation: "europe-west1"}
	if got := httpapi.ModelInfo(claude); got["location"] != "europe-west1" {
		t.Errorf("any vertex provider reports its region: %v", got)
	}
	other := &config.Settings{LLMProvider: "anthropic", LLMModel: "m", VertexLocation: "us-central1"}
	if got := httpapi.ModelInfo(other); !reflect.DeepEqual(got, map[string]string{"provider": "anthropic", "model": "m"}) {
		t.Errorf("no region outside vertex: %v", got)
	}
}
