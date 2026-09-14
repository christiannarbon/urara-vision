package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"urara-vision/backend/internal/model"
	"urara-vision/backend/internal/store/postgres"
	"urara-vision/backend/tests/fixtures"
)

func TestListProjectsKeepsTheStoreOrder(t *testing.T) {
	meta := &fakeMeta{projects: []model.ProjectSummary{
		{Slug: "newer", VersionCount: 2, Latest: &model.ProjectVersionRef{SnapshotID: "s2", Version: "2"}},
		{Slug: "older", VersionCount: 1},
	}}
	rec := do(t, newServer(t, meta, &fakeGraphs{}), http.MethodGet, "/api/v1/projects", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var body struct {
		Projects []model.ProjectSummary `json:"projects"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Projects) != 2 || body.Projects[0].Slug != "newer" || body.Projects[1].Slug != "older" {
		t.Errorf("projects = %+v, want newer then older", body.Projects)
	}
	if body.Projects[0].Latest == nil || body.Projects[0].Latest.SnapshotID != "s2" {
		t.Errorf("latest = %+v", body.Projects[0].Latest)
	}
}

func TestListProjectsIsAListWhenEmpty(t *testing.T) {
	meta := &fakeMeta{projects: []model.ProjectSummary{}}
	rec := do(t, newServer(t, meta, &fakeGraphs{}), http.MethodGet, "/api/v1/projects", nil, "")
	if !strings.Contains(rec.Body.String(), `"projects":[]`) {
		t.Errorf("body = %s, want an empty list", rec.Body)
	}
}

func TestGetProject(t *testing.T) {
	meta := &fakeMeta{project: &model.ProjectSummary{Slug: "jaffle-shop", Name: "Jaffle Shop"}}
	rec := do(t, newServer(t, meta, &fakeGraphs{}), http.MethodGet, "/api/v1/projects/jaffle-shop", nil, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"name":"Jaffle Shop"`) {
		t.Errorf("status = %d, body %s", rec.Code, rec.Body)
	}
	if meta.gotProjectSlug != "jaffle-shop" {
		t.Errorf("store asked for %q", meta.gotProjectSlug)
	}
}

func TestGetUnknownProjectIs404(t *testing.T) {
	rec := do(t, newServer(t, &fakeMeta{}, &fakeGraphs{}), http.MethodGet, "/api/v1/projects/nope", nil, "")
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "project not found") {
		t.Errorf("status = %d, body %s", rec.Code, rec.Body)
	}
}

func TestDeleteProjectClearsEachSnapshotGraph(t *testing.T) {
	meta := &fakeMeta{projectSnapshots: []string{"s1", "s2"}}
	graphs := &fakeGraphs{}
	rec := do(t, newServer(t, meta, graphs), http.MethodDelete, "/api/v1/projects/jaffle-shop", nil, "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if len(meta.deletedProjects) != 1 || meta.deletedProjects[0] != "jaffle-shop" {
		t.Errorf("postgres deletes = %v", meta.deletedProjects)
	}
	if strings.Join(graphs.deleted, ",") != "s1,s2" {
		t.Errorf("graph deletes = %v, want s1,s2", graphs.deleted)
	}
}

func TestDeleteProjectSurvivesAGraphFailure(t *testing.T) {
	meta := &fakeMeta{projectSnapshots: []string{"s1", "s2", "s3"}}
	graphs := &fakeGraphs{errDelete: map[string]error{"s2": errBoom}}
	rec := do(t, newServer(t, meta, graphs), http.MethodDelete, "/api/v1/projects/jaffle-shop", nil, "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if strings.Join(graphs.deleted, ",") != "s1,s2,s3" {
		t.Errorf("graph deletes = %v, want every snapshot tried", graphs.deleted)
	}
}

func TestDeleteUnknownProjectIs404(t *testing.T) {
	graphs := &fakeGraphs{}
	meta := &fakeMeta{errProject: postgres.ErrNotFound}
	rec := do(t, newServer(t, meta, graphs), http.MethodDelete, "/api/v1/projects/nope", nil, "")
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "project not found") {
		t.Errorf("status = %d, body %s", rec.Code, rec.Body)
	}
	if len(graphs.deleted) != 0 {
		t.Errorf("graph deletes = %v, want none", graphs.deleted)
	}
}

func TestProjectStoreErrorIs500(t *testing.T) {
	h := newServer(t, &fakeMeta{errProject: errBoom}, &fakeGraphs{})
	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		rec := do(t, h, method, "/api/v1/projects/jaffle-shop", nil, "")
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("%s status = %d, want 500", method, rec.Code)
		}
	}
}

func TestIngestResponseCarriesTheProject(t *testing.T) {
	meta := &fakeMeta{savedProjectID: "p-1", savedProjectSlug: "sample-data-modelling-project"}
	h := newServer(t, meta, &fakeGraphs{})
	files := map[string]string{}
	for _, f := range fixtures.StarSchema() {
		files[f.Path] = f.Content
	}
	body := ingestBody(t, "demo", "test", files)
	rec := do(t, h, http.MethodPost, "/api/v1/ingest", body, "application/json")
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var out struct {
		Project struct {
			ID   string `json:"id"`
			Slug string `json:"slug"`
		} `json:"project"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Project.ID != "p-1" || out.Project.Slug != "sample-data-modelling-project" {
		t.Errorf("project = %+v", out.Project)
	}
}
