package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"urara-vision/backend/internal/model"
	"urara-vision/backend/internal/store/postgres"
)

func TestListVersions(t *testing.T) {
	meta := &fakeMeta{versions: []model.Snapshot{{ID: "s2"}, {ID: "s1"}}}
	rec := do(t, newServer(t, meta, &fakeGraphs{}), http.MethodGet, "/api/v1/projects/jaffle/versions", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var body struct {
		Versions []model.Snapshot `json:"versions"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Versions) != 2 || body.Versions[0].ID != "s2" || body.Versions[1].ID != "s1" {
		t.Errorf("versions = %+v, want the store's order", body.Versions)
	}
	if meta.gotProjectSlug != "jaffle" {
		t.Errorf("store asked for %q", meta.gotProjectSlug)
	}
}

func TestListVersionsUnknownProjectIs404(t *testing.T) {
	meta := &fakeMeta{errVersions: postgres.ErrNotFound}
	rec := do(t, newServer(t, meta, &fakeGraphs{}), http.MethodGet, "/api/v1/projects/nope/versions", nil, "")
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "project not found") {
		t.Errorf("status = %d, body %s", rec.Code, rec.Body)
	}
}

func TestGetVersionDecodesTheLabel(t *testing.T) {
	for path, want := range map[string]string{
		"1.0.0%2Bbuild.7": "1.0.0+build.7",
		"2024%20Q1":       "2024 Q1",
		"latest":          "latest",
		"100%25":          "100%",
	} {
		meta := &fakeMeta{version: &model.Snapshot{ID: "s1"}}
		rec := do(t, newServer(t, meta, &fakeGraphs{}), http.MethodGet, "/api/v1/projects/jaffle/versions/"+path, nil, "")
		if rec.Code != http.StatusOK {
			t.Errorf("%s: status = %d, body %s", path, rec.Code, rec.Body)
		}
		if meta.gotVersionArgs != [2]string{"jaffle", want} {
			t.Errorf("%s: store asked for %v, want %q", path, meta.gotVersionArgs, want)
		}
	}
}

func TestGetVersionMalformedEscapeIs400(t *testing.T) {
	// httptest.NewRequest refuses %ZZ, so the URL is set as a client could send it.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/jaffle/versions/x", nil)
	req.URL.Path = "/api/v1/projects/jaffle/versions/%ZZ"
	req.URL.RawPath = req.URL.Path
	rec := httptest.NewRecorder()
	meta := &fakeMeta{}
	newServer(t, meta, &fakeGraphs{}).ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400: %s", rec.Code, rec.Body)
	}
	if meta.gotVersionArgs != [2]string{} {
		t.Errorf("store was called with %v", meta.gotVersionArgs)
	}
}

func TestGetVersionNotFoundNamesWhatIsMissing(t *testing.T) {
	cases := []struct {
		name    string
		project *model.ProjectSummary
		want    string
	}{
		{"unknown project", nil, "project not found"},
		{"unknown version", &model.ProjectSummary{Slug: "jaffle"}, "version not found"},
	}
	for _, c := range cases {
		meta := &fakeMeta{project: c.project}
		rec := do(t, newServer(t, meta, &fakeGraphs{}), http.MethodGet, "/api/v1/projects/jaffle/versions/9.9.9", nil, "")
		if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), c.want) {
			t.Errorf("%s: status = %d, body %s; want 404 %q", c.name, rec.Code, rec.Body, c.want)
		}
	}
}

func TestDeleteVersionClearsTheGraph(t *testing.T) {
	meta := &fakeMeta{deletedSID: "s1"}
	graphs := &fakeGraphs{}
	rec := do(t, newServer(t, meta, graphs), http.MethodDelete, "/api/v1/projects/jaffle/versions/1.0.0%2Bbuild.7", nil, "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if len(meta.deletedVersions) != 1 || meta.deletedVersions[0] != [2]string{"jaffle", "1.0.0+build.7"} {
		t.Errorf("postgres deletes = %v", meta.deletedVersions)
	}
	if strings.Join(graphs.deleted, ",") != "s1" {
		t.Errorf("graph deletes = %v, want s1", graphs.deleted)
	}
}

func TestDeleteVersionSurvivesAGraphFailure(t *testing.T) {
	meta := &fakeMeta{deletedSID: "s1"}
	graphs := &fakeGraphs{errDelete: map[string]error{"s1": errBoom}}
	rec := do(t, newServer(t, meta, graphs), http.MethodDelete, "/api/v1/projects/jaffle/versions/1.0.0", nil, "")
	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204 despite the graph failure", rec.Code)
	}
}

func TestDeleteLatestVersionIs400(t *testing.T) {
	meta := &fakeMeta{}
	graphs := &fakeGraphs{}
	rec := do(t, newServer(t, meta, graphs), http.MethodDelete, "/api/v1/projects/jaffle/versions/latest", nil, "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400: %s", rec.Code, rec.Body)
	}
	if len(meta.deletedVersions) != 0 || len(graphs.deleted) != 0 {
		t.Errorf("stores were called: %v / %v", meta.deletedVersions, graphs.deleted)
	}
}

func TestDeleteUnknownVersionIs404(t *testing.T) {
	meta := &fakeMeta{errDelVersion: postgres.ErrNotFound, project: &model.ProjectSummary{Slug: "jaffle"}}
	graphs := &fakeGraphs{}
	rec := do(t, newServer(t, meta, graphs), http.MethodDelete, "/api/v1/projects/jaffle/versions/9.9.9", nil, "")
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "version not found") {
		t.Errorf("status = %d, body %s", rec.Code, rec.Body)
	}
	if len(graphs.deleted) != 0 {
		t.Errorf("graph deletes = %v, want none", graphs.deleted)
	}
}
