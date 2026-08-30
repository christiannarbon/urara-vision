// What the upload accepts and refuses.
//
// The paths come from whatever the browser handed over, so this is where the
// filtering is pinned down: non-markdown files, editor cruft, dependency
// directories, and paths that try to climb out of the selected directory.
package api_test

import (
	"net/http"
	"strings"
	"testing"

	"urara-vision/backend/internal/projectmeta"
	"urara-vision/backend/tests/fixtures"
)

// TestIngestWithoutFilesIs400 with an actionable message, since an empty
// upload is almost always a directory picked one level too high.
func TestIngestWithoutFilesIs400(t *testing.T) {
	meta := &fakeMeta{}
	h := newServer(t, meta, &fakeGraphs{})

	rec := do(t, h, http.MethodPost, "/api/v1/ingest",
		ingestBody(t, "", "", nil), "application/json")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if meta.saved != nil {
		t.Error("an empty upload still created a snapshot")
	}
	if msg, _ := decode(t, rec.Body.Bytes())["error"].(string); !strings.Contains(msg, "directory") {
		t.Errorf("error = %q; it should say what to do about it", msg)
	}
}

// TestIngestRejectsInvalidJSON rather than treating it as an empty upload.
func TestIngestRejectsInvalidJSON(t *testing.T) {
	h := newServer(t, &fakeMeta{}, &fakeGraphs{})
	rec := do(t, h, http.MethodPost, "/api/v1/ingest",
		strings.NewReader("{not json"), "application/json")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// TestIngestEnforcesFileLimit: the cap exists so one mis-picked directory
// cannot tie the server up parsing a whole home folder.
func TestIngestEnforcesFileLimit(t *testing.T) {
	meta := &fakeMeta{}
	h := newServerWithMaxFiles(t, meta, &fakeGraphs{}, 2)

	files := map[string]string{}
	for _, n := range []string{"a", "b", "c"} {
		files["d/fact_"+n+".md"] = fixtures.Doc("fact_"+n, "Fact", "D", []string{"id"}, "")
	}

	rec := do(t, h, http.MethodPost, "/api/v1/ingest",
		ingestBody(t, "", "", files), "application/json")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if meta.saved != nil {
		t.Error("an over-limit upload still created a snapshot")
	}
}

// TestIngestFiltersUploadedPaths covers everything the browser might hand over
// that must not become a table: non-markdown files, dotfiles and dependency
// directories. Paths that climb out of the selected directory are flattened
// rather than dropped -- see TestIngestClimbingPathIsFlattened.
func TestIngestFiltersUploadedPaths(t *testing.T) {
	doc := fixtures.Doc("fact_keep", "Fact", "D", []string{"id"}, "")

	meta := &fakeMeta{}
	h := newServer(t, meta, &fakeGraphs{})
	rec := do(t, h, http.MethodPost, "/api/v1/ingest", ingestBody(t, "", "", map[string]string{
		"d/fact_keep.md":               doc,
		"d/README.txt":                 "not markdown",
		"d/.DS_Store":                  "cruft",
		"d/.hidden.md":                 doc,
		"node_modules/pkg/fact_dep.md": doc,
		"d/.git/fact_git.md":           doc,
		"d/image.png":                  "binary",
	}), "application/json")

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body)
	}
	if meta.saved == nil {
		t.Fatal("nothing was saved")
	}

	var ids []string
	for _, tb := range meta.saved.Tables {
		ids = append(ids, tb.ID)
	}
	if len(ids) != 1 || ids[0] != "d/fact_keep" {
		t.Errorf("tables = %v, want only d/fact_keep", ids)
	}
	for _, tb := range meta.saved.Tables {
		if strings.HasPrefix(tb.DocPath, "..") || strings.HasPrefix(tb.DocPath, "/") {
			t.Errorf("doc path %q escaped the selected directory", tb.DocPath)
		}
	}
}

// TestIngestClimbingPathIsFlattened: an uploaded path is an identifier, never
// something opened on disk, so one that climbs is stripped back to a relative
// path and the document is kept rather than silently discarded. What matters is
// that nothing absolute or climbing survives into a table ID.
func TestIngestClimbingPathIsFlattened(t *testing.T) {
	cases := map[string]string{
		"../domain_one/fact_primary.md":       "domain_one/fact_primary.md",
		"../../../domain_one/fact_primary.md": "domain_one/fact_primary.md",
		"/abs/domain_one/fact_primary.md":     "abs/domain_one/fact_primary.md",
		"domain_one\\fact_primary.md":         "domain_one/fact_primary.md",
	}
	for in, want := range cases {
		meta := &fakeMeta{}
		h := newServer(t, meta, &fakeGraphs{})
		rec := do(t, h, http.MethodPost, "/api/v1/ingest", ingestBody(t, "", "", map[string]string{
			in: fixtures.Doc("fact_primary", "Fact", "Domain One", []string{"primary_id"}, ""),
		}), "application/json")
		if rec.Code != http.StatusCreated {
			t.Fatalf("%q: status = %d: %s", in, rec.Code, rec.Body)
		}
		if len(meta.saved.Tables) != 1 {
			t.Fatalf("%q: tables = %d", in, len(meta.saved.Tables))
		}
		got := meta.saved.Tables[0].DocPath
		if got != want {
			t.Errorf("%q -> doc path %q, want %q", in, got, want)
		}
		if strings.HasPrefix(got, "..") || strings.HasPrefix(got, "/") {
			t.Errorf("%q -> %q still climbs or is absolute", in, got)
		}
	}
}

// TestIngestDefaultsTheNameFromTheDirectory, so an unnamed ingest is still
// tellable apart from the last one in the snapshot list.
func TestIngestDefaultsTheNameFromTheDirectory(t *testing.T) {
	meta := &fakeMeta{}
	h := newServer(t, meta, &fakeGraphs{})

	rec := do(t, h, http.MethodPost, "/api/v1/ingest",
		ingestBody(t, "   ", "/Users/someone/data-modelling/star-schema", map[string]string{
			"d/fact_x.md": fixtures.Doc("fact_x", "Fact", "D", []string{"id"}, ""),
		}), "application/json")
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	name := meta.saved.Snapshot.Name
	if !strings.HasPrefix(name, "star-schema") {
		t.Errorf("name = %q, want it to start with the directory name", name)
	}
	if name == "star-schema" {
		t.Error("name carries no timestamp, so repeated ingests are indistinguishable")
	}
}

// TestIngestWithoutAManifestIs400: the directory has to say what project it is
// before any of it is read, and the message has to name the file to write.
func TestIngestWithoutAManifestIs400(t *testing.T) {
	meta := &fakeMeta{}
	h := newServer(t, meta, &fakeGraphs{})

	rec := do(t, h, http.MethodPost, "/api/v1/ingest",
		rawIngestBody(t, "", "", map[string]string{
			"d/fact_x.md": fixtures.Doc("fact_x", "Fact", "D", []string{"id"}, ""),
		}), "application/json")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body)
	}
	if msg, _ := decode(t, rec.Body.Bytes())["error"].(string); !strings.Contains(msg, projectmeta.FileName) {
		t.Errorf("error = %q; it should name the file that is missing", msg)
	}
	if meta.saved != nil {
		t.Error("an upload with no manifest still created a snapshot")
	}
}

// TestIngestRejectsAManifestBelowTheRoot with a different message: the file
// exists and the fix is to move it, not to write it.
func TestIngestRejectsAManifestBelowTheRoot(t *testing.T) {
	meta := &fakeMeta{}
	h := newServer(t, meta, &fakeGraphs{})

	rec := do(t, h, http.MethodPost, "/api/v1/ingest",
		rawIngestBody(t, "", "", map[string]string{
			"d/fact_x.md":               fixtures.Doc("fact_x", "Fact", "D", []string{"id"}, ""),
			"d/" + projectmeta.FileName: fixtures.ProjectMetaTOML,
		}), "application/json")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body)
	}
	msg, _ := decode(t, rec.Body.Bytes())["error"].(string)
	if !strings.Contains(msg, "d/"+projectmeta.FileName) {
		t.Errorf("error = %q; it should say where the manifest actually is", msg)
	}
	if meta.saved != nil {
		t.Error("a misplaced manifest still created a snapshot")
	}
}

// TestIngestRejectsAnInvalidManifest, reporting what is wrong with it rather
// than that something is.
func TestIngestRejectsAnInvalidManifest(t *testing.T) {
	meta := &fakeMeta{}
	h := newServer(t, meta, &fakeGraphs{})

	rec := do(t, h, http.MethodPost, "/api/v1/ingest",
		rawIngestBody(t, "", "", map[string]string{
			"d/fact_x.md":        fixtures.Doc("fact_x", "Fact", "D", []string{"id"}, ""),
			projectmeta.FileName: "[project]\nname = \"p\"\n",
		}), "application/json")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body)
	}
	msg, _ := decode(t, rec.Body.Bytes())["error"].(string)
	if !strings.Contains(msg, "project.version is required") {
		t.Errorf("error = %q; it should say which field is wrong", msg)
	}
	if meta.saved != nil {
		t.Error("an invalid manifest still created a snapshot")
	}
}

// TestManifestIsNotADocument: it is read as metadata, so it neither becomes a
// table nor counts towards the file limit that protects the parser.
func TestManifestIsNotADocument(t *testing.T) {
	meta := &fakeMeta{}
	h := newServerWithMaxFiles(t, meta, &fakeGraphs{}, 1)

	rec := do(t, h, http.MethodPost, "/api/v1/ingest",
		ingestBody(t, "", "", map[string]string{
			"d/fact_x.md": fixtures.Doc("fact_x", "Fact", "D", []string{"id"}, ""),
		}), "application/json")
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body)
	}
	for _, tb := range meta.saved.Tables {
		if strings.Contains(tb.DocPath, projectmeta.FileName) {
			t.Errorf("the manifest was parsed as a document: %q", tb.DocPath)
		}
	}
	if n := meta.saved.Snapshot.Stats.FilesParsed; n != 1 {
		t.Errorf("files parsed = %d, want 1", n)
	}
}
