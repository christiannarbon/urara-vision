package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"urara-vision/backend/internal/model"
)

func diffModel(sid, version, grain string) *model.Model {
	return &model.Model{
		Snapshot: model.Snapshot{ID: sid, Project: model.ProjectMeta{Project: model.Project{Version: version}}},
		Domains:  []model.Domain{{ID: "sales"}},
		Tables: []model.Table{{ID: "sales/fact_orders", DomainID: "sales", Grain: grain,
			Columns: []model.Column{{Name: "order_id", Type: "STRING"}}}},
	}
}

func diffMeta() *fakeMeta {
	return &fakeMeta{
		project: &model.ProjectSummary{Slug: "shop"},
		versionsByLabel: map[string]*model.Snapshot{
			"1.0.0":  {ID: "s1"},
			"2.0.0":  {ID: "s2"},
			"latest": {ID: "s2"},
		},
		models: map[string]*model.Model{
			"s1": diffModel("s1", "1.0.0", "one row per order"),
			"s2": diffModel("s2", "2.0.0", "one row per order line"),
		},
	}
}

func TestDiffMissingParamIs400(t *testing.T) {
	for query, want := range map[string]string{
		"?to=2.0.0":   "from is required",
		"?from=1.0.0": "to is required",
		"":            "from is required",
	} {
		rec := do(t, newServer(t, diffMeta(), &fakeGraphs{}), http.MethodGet, "/api/v1/projects/shop/diff"+query, nil, "")
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), want) {
			t.Errorf("%q: status = %d, body %s", query, rec.Code, rec.Body)
		}
	}
}

func TestDiffUnknownVersionIs404NamingIt(t *testing.T) {
	rec := do(t, newServer(t, diffMeta(), &fakeGraphs{}), http.MethodGet, "/api/v1/projects/shop/diff?from=1.0.0&to=9.9.9", nil, "")
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), `"version 9.9.9 not found"`) {
		t.Errorf("status = %d, body %s", rec.Code, rec.Body)
	}
}

func TestDiffUnknownProjectIs404(t *testing.T) {
	meta := diffMeta()
	meta.project = nil
	meta.versionsByLabel = map[string]*model.Snapshot{}
	rec := do(t, newServer(t, meta, &fakeGraphs{}), http.MethodGet, "/api/v1/projects/nope/diff?from=1.0.0&to=2.0.0", nil, "")
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "project not found") {
		t.Errorf("status = %d, body %s", rec.Code, rec.Body)
	}
}

func TestDiffSameVersionIsEmpty(t *testing.T) {
	rec := do(t, newServer(t, diffMeta(), &fakeGraphs{}), http.MethodGet, "/api/v1/projects/shop/diff?from=1.0.0&to=1.0.0", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var body struct {
		Summary map[string]map[string]int `json:"summary"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	for thing, counts := range body.Summary {
		for k, n := range counts {
			if n != 0 {
				t.Errorf("summary.%s.%s = %d, want 0", thing, k, n)
			}
		}
	}
}

func TestDiffBodyShape(t *testing.T) {
	rec := do(t, newServer(t, diffMeta(), &fakeGraphs{}), http.MethodGet, "/api/v1/projects/shop/diff?from=1.0.0&to=latest", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var body struct {
		Project string `json:"project"`
		From    struct {
			Version    string `json:"version"`
			SnapshotID string `json:"snapshotId"`
		} `json:"from"`
		To struct {
			Version    string `json:"version"`
			SnapshotID string `json:"snapshotId"`
		} `json:"to"`
		Summary struct {
			Tables struct {
				Changed int `json:"changed"`
			} `json:"tables"`
		} `json:"summary"`
		Tables []struct {
			ID     string `json:"id"`
			Fields []struct {
				Field string `json:"field"`
			} `json:"fields"`
		} `json:"tables"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Project != "shop" || body.From.Version != "1.0.0" || body.To.Version != "2.0.0" || body.To.SnapshotID != "s2" {
		t.Errorf("header = %+v", body)
	}
	if body.Summary.Tables.Changed != 1 || len(body.Tables) != 1 || body.Tables[0].Fields[0].Field != "grain" {
		t.Errorf("diff = %+v", body)
	}
}

func TestDiffSameSnapshotLoadsOnce(t *testing.T) {
	meta := diffMeta()
	rec := do(t, newServer(t, meta, &fakeGraphs{}), http.MethodGet, "/api/v1/projects/shop/diff?from=latest&to=2.0.0", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if meta.loadModelCalls != 1 {
		t.Errorf("LoadModel called %d times, want 1", meta.loadModelCalls)
	}
}
