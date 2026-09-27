package tools_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"urara-vision/backend/internal/chat/tools"
)

var pythonOrder = []string{
	"list_domains", "list_tables", "get_tables", "search_model", "get_neighbourhood",
	"find_join_paths", "get_lineage", "list_diagnostics", "list_source_models",
}

type goldenTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Schema      map[string]any `json:"schema"`
}

func goldenTools(t *testing.T) map[string]goldenTool {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "testdata", "golden", "debug", "tools.json"))
	if err != nil {
		t.Fatal(err)
	}
	var g struct {
		Body []goldenTool `json:"body"`
	}
	if err := json.Unmarshal(raw, &g); err != nil {
		t.Fatal(err)
	}
	out := map[string]goldenTool{}
	for _, tool := range g.Body {
		out[tool.Name] = tool
	}
	return out
}

// pythonSchema is the golden schema as Go sends it: no title keys, and a
// nullable optional ({"anyOf": [X, null], "default": null}) as plain X.
func pythonSchema(v any) any {
	switch v := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, x := range v {
			if k != "title" {
				out[k] = pythonSchema(x)
			}
		}
		if anyOf, ok := out["anyOf"].([]any); ok && len(anyOf) == 2 && reflect.DeepEqual(anyOf[1], map[string]any{"type": "null"}) {
			delete(out, "anyOf")
			if out["default"] == nil {
				delete(out, "default")
			}
			for k, x := range anyOf[0].(map[string]any) {
				out[k] = x
			}
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, x := range v {
			out[i] = pythonSchema(x)
		}
		return out
	}
	return v
}

func decoded(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("%s: %v", raw, err)
	}
	return m
}

func TestThereAreNineInPythonOrder(t *testing.T) {
	if got := tools.Names(); !slices.Equal(got, pythonOrder) {
		t.Errorf("Names() = %v", got)
	}
	var built []string
	for _, s := range tools.Build(&fakeBackend{}, "snap-1") {
		built = append(built, s.Name)
	}
	if !slices.Equal(built, pythonOrder) {
		t.Errorf("Build order = %v", built)
	}
}

func TestSchemasAndDescriptionsMatchPython(t *testing.T) {
	golden := goldenTools(t)
	for _, s := range tools.Build(&fakeBackend{}, "snap-1") {
		g, ok := golden[s.Name]
		if !ok {
			t.Errorf("%s is not in the golden file", s.Name)
			continue
		}
		if s.Description != g.Description {
			t.Errorf("%s description differs:\n got %q\nwant %q", s.Name, s.Description, g.Description)
		}
		if got, want := decoded(t, s.Schema), pythonSchema(g.Schema); !reflect.DeepEqual(got, want) {
			t.Errorf("%s schema differs:\n got %v\nwant %v", s.Name, got, want)
		}
	}
}

func TestDescriptionsAreEnoughToChooseBy(t *testing.T) {
	for _, s := range tools.Build(&fakeBackend{}, "snap-1") {
		if len(s.Description) <= 80 {
			t.Errorf("%s description is too thin", s.Name)
		}
	}
	// The model has to be told IDs are 'domain/table' or it will guess a name.
	for _, s := range tools.Build(&fakeBackend{}, "snap-1") {
		switch s.Name {
		case "get_tables", "get_neighbourhood", "find_join_paths", "get_lineage":
			if !strings.Contains(s.Description, "domain/table") {
				t.Errorf("%s does not spell out the ID format", s.Name)
			}
		}
	}
}

func TestEveryArgumentIsDescribed(t *testing.T) {
	for _, s := range tools.Build(&fakeBackend{}, "snap-1") {
		props, _ := decoded(t, s.Schema)["properties"].(map[string]any)
		for name, p := range props {
			if d, _ := p.(map[string]any)["description"].(string); d == "" {
				t.Errorf("%s.%s has no description", s.Name, name)
			}
		}
	}
}

func TestNoSchemaMentionsASnapshot(t *testing.T) {
	for _, s := range tools.Build(&fakeBackend{}, "snap-1") {
		if lower := strings.ToLower(string(s.Schema)); strings.Contains(lower, "snapshot") || strings.Contains(lower, `"sid"`) {
			t.Errorf("%s exposes a snapshot to the model", s.Name)
		}
	}
}

// Two agents on two snapshots must not read each other's.
func TestEachToolSetUsesItsOwnSnapshot(t *testing.T) {
	a, b := &fakeBackend{}, &fakeBackend{}
	first, second := tools.Build(a, "snap-1"), tools.Build(b, "snap-2")
	ctx := context.Background()
	for i := range first {
		_, _ = first[i].Run(ctx, nil)
		_, _ = second[i].Run(ctx, nil)
	}
	for _, sid := range a.snapshots {
		if sid != "snap-1" {
			t.Errorf("first set called snapshot %q", sid)
		}
	}
	for _, sid := range b.snapshots {
		if sid != "snap-2" {
			t.Errorf("second set called snapshot %q", sid)
		}
	}
	if len(a.snapshots) != 9 || len(b.snapshots) != 9 {
		t.Errorf("%d and %d backend calls, want 9 each", len(a.snapshots), len(b.snapshots))
	}
}
