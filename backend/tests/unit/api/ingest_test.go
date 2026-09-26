// POST /api/v1/ingest: what a successful upload produces, and how a failure
// part-way through is undone.
package api_test

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"urara-vision/backend/internal/auth"
	"urara-vision/backend/internal/model"
	"urara-vision/backend/internal/projectmeta"
	"urara-vision/backend/internal/store/postgres"
	"urara-vision/backend/tests/fixtures"
)

// ingestBody renders a JSON ingest request, adding the manifest every upload
// has to carry. A suite testing the manifest itself passes its own under
// projectmeta.FileName, or none at all.
func ingestBody(t *testing.T, name, sourceLabel string, files map[string]string) *bytes.Reader {
	t.Helper()
	withMeta := make(map[string]string, len(files)+1)
	for p, c := range files {
		withMeta[p] = c
	}
	if _, ok := withMeta[projectmeta.FileName]; !ok {
		withMeta[projectmeta.FileName] = fixtures.ProjectMetaTOML
	}
	return rawIngestBody(t, name, sourceLabel, withMeta)
}

// rawIngestBody renders exactly the files it is given, manifest or not.
func rawIngestBody(t *testing.T, name, sourceLabel string, files map[string]string) *bytes.Reader {
	t.Helper()
	type f struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	body := struct {
		Name        string `json:"name"`
		SourceLabel string `json:"sourceLabel"`
		Files       []f    `json:"files"`
	}{Name: name, SourceLabel: sourceLabel}
	for p, c := range files {
		body.Files = append(body.Files, f{Path: p, Content: c})
	}
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(b)
}

func TestIngestJSONCreatesASnapshot(t *testing.T) {
	meta := &fakeMeta{}
	graphs := &fakeGraphs{}
	h := newServer(t, meta, graphs)

	files := map[string]string{}
	for _, f := range fixtures.StarSchema() {
		files[f.Path] = f.Content
	}

	rec := do(t, h, http.MethodPost, "/api/v1/ingest",
		ingestBody(t, "my snapshot", "star-schema", files), "application/json")
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body)
	}

	if meta.saved == nil {
		t.Fatal("snapshot was never saved to the record of truth")
	}
	if graphs.projected == nil {
		t.Fatal("model was never projected into the graph")
	}
	if meta.saved.Snapshot.Name != "my snapshot" {
		t.Errorf("name = %q", meta.saved.Snapshot.Name)
	}
	if meta.saved.Snapshot.CreatedAt.IsZero() {
		t.Error("CreatedAt was left unset")
	}
	if got := meta.saved.Snapshot.Stats.Tables; got != 3 {
		t.Errorf("tables = %d, want 3", got)
	}
	// The manifest is metadata about the ingest, so it travels with the
	// snapshot rather than with any document in it.
	if got, want := meta.saved.Snapshot.Project, fixtures.ProjectMeta(); !reflect.DeepEqual(got, want) {
		t.Errorf("project = %+v, want %+v", got, want)
	}
	if graphs.edgeCount == 0 {
		t.Error("no edges were projected for a model with two declared joins")
	}

	body := decode(t, rec.Body.Bytes())
	for _, key := range []string{"snapshot", "edges", "diagnostics"} {
		if _, ok := body[key]; !ok {
			t.Errorf("response is missing %q", key)
		}
	}
}

// TestIngestSnapshotIDIsUnique: two ingests must not collide, or the second
// overwrites the first.
func TestIngestSnapshotIDIsUnique(t *testing.T) {
	files := map[string]string{"d/fact_x.md": fixtures.Doc("fact_x", "Fact", "D", []string{"id"}, "")}

	ids := map[string]bool{}
	for i := 0; i < 2; i++ {
		meta := &fakeMeta{}
		h := newServer(t, meta, &fakeGraphs{})
		if rec := do(t, h, http.MethodPost, "/api/v1/ingest",
			ingestBody(t, "", "", files), "application/json"); rec.Code != http.StatusCreated {
			t.Fatalf("status = %d", rec.Code)
		}
		id := meta.saved.Snapshot.ID
		if id == "" {
			t.Fatal("snapshot ID was empty")
		}
		if ids[id] {
			t.Fatalf("snapshot ID %q was reused", id)
		}
		ids[id] = true
	}
}

// TestIngestMultipart covers the other accepted body shape: each part's field
// name carries the file's relative path.
func TestIngestMultipart(t *testing.T) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := mw.WriteField("name", "multipart snapshot"); err != nil {
		t.Fatal(err)
	}
	for path, content := range map[string]string{
		"d/fact_x.md":        fixtures.Doc("fact_x", "Fact", "D", []string{"id"}, ""),
		projectmeta.FileName: fixtures.ProjectMetaTOML,
	} {
		part, err := mw.CreateFormFile(path, path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}

	meta := &fakeMeta{}
	h := newServer(t, meta, &fakeGraphs{})
	rec := do(t, h, http.MethodPost, "/api/v1/ingest", bytes.NewReader(buf.Bytes()), mw.FormDataContentType())
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body)
	}
	if meta.saved == nil || len(meta.saved.Tables) != 1 {
		t.Fatalf("saved = %+v", meta.saved)
	}
	if meta.saved.Snapshot.Name != "multipart snapshot" {
		t.Errorf("name = %q", meta.saved.Snapshot.Name)
	}
}

// TestIngestReportsDiagnostics: a document that could not be parsed has to
// come back in the response, since the reader has no other way to learn the
// model is incomplete.
func TestIngestReportsDiagnostics(t *testing.T) {
	meta := &fakeMeta{}
	h := newServer(t, meta, &fakeGraphs{})

	rec := do(t, h, http.MethodPost, "/api/v1/ingest",
		ingestBody(t, "", "", map[string]string{
			"d/fact_x.md": fixtures.Doc("fact_x", "Fact", "D", []string{"id"}, ""),
			"d/blank.md":  "  \n",
		}), "application/json")
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	raw, _ := json.Marshal(decode(t, rec.Body.Bytes())["diagnostics"])
	if !strings.Contains(string(raw), "empty_document") {
		t.Errorf("diagnostics did not mention the skipped document: %s", raw)
	}
}

// TestIngestRollsBackWhenProjectionFails: the snapshot would otherwise be
// listed but unqueryable as a graph, which looks like a working entry with an
// empty diagram.
func TestIngestRollsBackWhenProjectionFails(t *testing.T) {
	meta := &fakeMeta{}
	graphs := &fakeGraphs{errProject: errBoom}
	h := newServer(t, meta, graphs)

	rec := do(t, h, http.MethodPost, "/api/v1/ingest",
		ingestBody(t, "", "", map[string]string{
			"d/fact_x.md": fixtures.Doc("fact_x", "Fact", "D", []string{"id"}, ""),
		}), "application/json")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if meta.saved == nil {
		t.Fatal("the test needs the save to have happened before the projection failed")
	}
	if len(meta.deleted) != 1 || meta.deleted[0] != meta.saved.Snapshot.ID {
		t.Errorf("rollback deleted %v, want the snapshot just saved (%q)",
			meta.deleted, meta.saved.Snapshot.ID)
	}
}

// TestIngestFailsWhenTheSaveFails, without projecting a model that is not
// stored anywhere.
func TestIngestFailsWhenTheSaveFails(t *testing.T) {
	meta := &fakeMeta{errSave: errBoom}
	graphs := &fakeGraphs{}
	h := newServer(t, meta, graphs)

	rec := do(t, h, http.MethodPost, "/api/v1/ingest",
		ingestBody(t, "", "", map[string]string{
			"d/fact_x.md": fixtures.Doc("fact_x", "Fact", "D", []string{"id"}, ""),
		}), "application/json")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if graphs.projected != nil {
		t.Error("the graph was projected even though the snapshot was never stored")
	}
}

// fixtureSlug and fixtureVersion are what fixtures.ProjectMetaTOML declares.
var fixtureSlug, fixtureVersion = projectmeta.Slug(fixtures.ProjectMeta().Project.Name), fixtures.ProjectMeta().Project.Version

func oneDoc() map[string]string {
	return map[string]string{"d/fact_x.md": fixtures.Doc("fact_x", "Fact", "D", []string{"id"}, "")}
}

func assertVersionConflict(t *testing.T, code int, body []byte) {
	t.Helper()
	if code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %s", code, body)
	}
	got := decode(t, body)
	if got["project"] != fixtureSlug || got["version"] != fixtureVersion {
		t.Errorf("body = %v, want project %q and version %q", got, fixtureSlug, fixtureVersion)
	}
	want := "version " + fixtureVersion + " of " + fixtureSlug + " already exists; delete it first to re-import"
	if got["error"] != want {
		t.Errorf("error = %q, want %q", got["error"], want)
	}
}

func TestIngestExistingVersionIs409BeforeParsing(t *testing.T) {
	meta := &fakeMeta{version: &model.Snapshot{ID: "existing"}}
	graphs := &fakeGraphs{}
	h := newServer(t, meta, graphs)

	rec := do(t, h, http.MethodPost, "/api/v1/ingest", ingestBody(t, "", "", oneDoc()), "application/json")
	assertVersionConflict(t, rec.Code, rec.Body.Bytes())
	if meta.gotVersionArgs != [2]string{fixtureSlug, fixtureVersion} {
		t.Errorf("GetVersion(%v), want (%s, %s)", meta.gotVersionArgs, fixtureSlug, fixtureVersion)
	}
	if meta.saved != nil || graphs.projected != nil {
		t.Error("an existing version was parsed and saved anyway")
	}
}

// Two imports raced past the early check; the unique index caught the second.
func TestIngestSaveConflictIs409(t *testing.T) {
	meta := &fakeMeta{errSave: postgres.ErrConflict}
	graphs := &fakeGraphs{}
	h := newServer(t, meta, graphs)

	rec := do(t, h, http.MethodPost, "/api/v1/ingest", ingestBody(t, "", "", oneDoc()), "application/json")
	assertVersionConflict(t, rec.Code, rec.Body.Bytes())
	if graphs.projected != nil {
		t.Error("a conflicting save was still projected")
	}
}

func TestIngestVersionCheckErrorIs500(t *testing.T) {
	meta := &fakeMeta{errVersion: errBoom}
	h := newServer(t, meta, &fakeGraphs{})

	rec := do(t, h, http.MethodPost, "/api/v1/ingest", ingestBody(t, "", "", oneDoc()), "application/json")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500: %s", rec.Code, rec.Body)
	}
	if meta.saved != nil {
		t.Error("saved despite a failed version check")
	}
}

func TestIngestPermissionCreatorNewProject(t *testing.T) {
	meta := &fakeMeta{}
	h := roleServer(t, meta)

	rec := asRole(t, h, auth.RoleCreator, http.MethodPost, "/api/v1/ingest", ingestBody(t, "", "", oneDoc()))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body)
	}
}

// No markdown would be a 400; a 403 shows the check runs before the upload is parsed.
func TestIngestPermissionViewerRefusedBeforeParsing(t *testing.T) {
	for _, existing := range []bool{false, true} {
		meta := &fakeMeta{}
		if existing {
			meta.project = &model.ProjectSummary{}
		}
		h := roleServer(t, meta)

		rec := asRole(t, h, auth.RoleViewer, http.MethodPost, "/api/v1/ingest", ingestBody(t, "", "", nil))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("existing=%v: status = %d, want 403: %s", existing, rec.Code, rec.Body)
		}
		if meta.saved != nil {
			t.Error("saved despite 403")
		}
	}
}

func TestIngestPermissionService(t *testing.T) {
	h := roleServer(t, &fakeMeta{project: &model.ProjectSummary{}})

	rec := asService(h, http.MethodPost, "/api/v1/ingest", ingestBody(t, "", "", oneDoc()))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body)
	}
}

func TestIngestPermissionProjectCheckErrorIs500(t *testing.T) {
	h := newServer(t, &fakeMeta{errProject: errBoom}, &fakeGraphs{})

	rec := do(t, h, http.MethodPost, "/api/v1/ingest", ingestBody(t, "", "", oneDoc()), "application/json")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500: %s", rec.Code, rec.Body)
	}
}

// The body is not read for a caller who can import nothing.
func TestIngestPermissionCheckedBeforeReadingTheBody(t *testing.T) {
	h := roleServer(t, &fakeMeta{})

	rec := asRole(t, h, auth.RoleViewer, http.MethodPost, "/api/v1/ingest", strings.NewReader(`{not json`))
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "not allowed") {
		t.Errorf("viewer: status = %d, want 403: %s", rec.Code, rec.Body)
	}
	if rec := asRole(t, h, auth.RoleCreator, http.MethodPost, "/api/v1/ingest", strings.NewReader(`{not json`)); rec.Code != http.StatusBadRequest {
		t.Errorf("creator: status = %d, want 400: %s", rec.Code, rec.Body)
	}
}
