// Notes handlers: validation, replies and authorship.
package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"urara-vision/backend/internal/auth"
	"urara-vision/backend/internal/model"
	"urara-vision/backend/internal/store/postgres"
)

const notesPath = "/api/v1/snapshots/s1/notes"

// notesMeta has a table anchor t1 and a viewer's note n0 on it, with a reply r0.
func notesMeta() *fakeMeta {
	meta := &fakeMeta{}
	meta.fakeNotes.anchors = map[string]bool{"table:d/t1": true}
	meta.addNote(model.Note{ID: "n0", SnapshotID: "s1", AnchorKind: "table", AnchorID: "d/t1", Body: "hi", AuthorID: "viewer", AuthorName: "viewer"})
	meta.addNote(model.Note{ID: "r0", SnapshotID: "s1", ParentID: "n0", AnchorKind: "table", AnchorID: "d/t1", Body: "re", AuthorID: "viewer", AuthorName: "viewer"})
	return meta
}

func noteReq(t *testing.T, h http.Handler, role auth.Role, method, target, body string) (int, string) {
	t.Helper()
	rec := asRole(t, h, role, method, target, strings.NewReader(body))
	return rec.Code, rec.Body.String()
}

func TestNotesListNeedsAnchor(t *testing.T) {
	h := roleServer(t, notesMeta())
	for _, q := range []string{"", "?anchorKind=table", "?anchorId=d/t1"} {
		if code, body := noteReq(t, h, auth.RoleViewer, http.MethodGet, notesPath+q, ""); code != http.StatusBadRequest {
			t.Errorf("GET %s = %d: %s", q, code, body)
		}
	}
	if code, body := noteReq(t, h, auth.RoleViewer, http.MethodGet, notesPath+"?anchorKind=nope&anchorId=x", ""); code != http.StatusBadRequest {
		t.Errorf("unknown kind = %d: %s", code, body)
	}
	code, body := noteReq(t, h, auth.RoleViewer, http.MethodGet, notesPath+"?anchorKind=table&anchorId=d/t1", "")
	if code != http.StatusOK || !strings.Contains(body, `"id":"n0"`) {
		t.Errorf("list = %d: %s", code, body)
	}
}

func TestNotesCreate(t *testing.T) {
	h := roleServer(t, notesMeta())
	code, body := noteReq(t, h, auth.RoleViewer, http.MethodPost, notesPath, `{"anchorKind":"table","anchorId":"d/t1","body":"  new  "}`)
	if code != http.StatusCreated {
		t.Fatalf("create = %d: %s", code, body)
	}
	var n model.Note
	_ = json.Unmarshal([]byte(body), &n)
	if n.Body != "new" || n.AuthorID != "viewer" || n.AuthorName != "viewer" {
		t.Errorf("created = %+v", n)
	}
}

func TestNotesCreateRejects(t *testing.T) {
	h := roleServer(t, notesMeta())
	cases := []struct{ name, body, want string }{
		{"missing anchor", `{"anchorKind":"table","anchorId":"d/nope","body":"x"}`, "table d/nope does not exist in this version"},
		{"bad kind", `{"anchorKind":"row","anchorId":"d/t1","body":"x"}`, "anchor kind"},
		{"no anchor id", `{"anchorKind":"table","body":"x"}`, "anchorId"},
		{"empty body", `{"anchorKind":"table","anchorId":"d/t1","body":"  "}`, "empty"},
		{"too long", `{"anchorKind":"table","anchorId":"d/t1","body":"` + strings.Repeat("é", 4001) + `"}`, "longer"},
		{"reply to reply", `{"parentId":"r0","body":"x"}`, "replies cannot be replied to"},
	}
	for _, c := range cases {
		code, body := noteReq(t, h, auth.RoleViewer, http.MethodPost, notesPath, c.body)
		if code != http.StatusBadRequest || !strings.Contains(body, c.want) {
			t.Errorf("%s = %d: %s", c.name, code, body)
		}
	}
	if code, body := noteReq(t, h, auth.RoleViewer, http.MethodPost, notesPath, `{"parentId":"missing","body":"x"}`); code != http.StatusNotFound {
		t.Errorf("unknown parent = %d: %s", code, body)
	}
}

func TestNotesReplyUsesParentAnchor(t *testing.T) {
	h := roleServer(t, notesMeta())
	code, body := noteReq(t, h, auth.RoleCreator, http.MethodPost, notesPath, `{"parentId":"n0","anchorKind":"domain","anchorId":"elsewhere","body":"re"}`)
	if code != http.StatusCreated {
		t.Fatalf("reply = %d: %s", code, body)
	}
	var n model.Note
	_ = json.Unmarshal([]byte(body), &n)
	if n.ParentID != "n0" || n.AnchorKind != "table" || n.AnchorID != "d/t1" {
		t.Errorf("reply = %+v", n)
	}
}

func TestNotesCreateNeedsAUser(t *testing.T) {
	body := `{"anchorKind":"table","anchorId":"d/t1","body":"x"}`
	wantCode(t, "service", asService(roleServer(t, notesMeta()), http.MethodPost, notesPath, strings.NewReader(body)), http.StatusForbidden)
	anon := newServer(t, notesMeta(), &fakeGraphs{})
	wantCode(t, "anonymous", do(t, anon, http.MethodPost, notesPath, strings.NewReader(body), "application/json"), http.StatusForbidden)
}

func TestNotesPatchNeedsOneField(t *testing.T) {
	h := roleServer(t, notesMeta())
	for _, b := range []string{`{}`, `{"body":"x","resolved":true}`} {
		if code, body := noteReq(t, h, auth.RoleViewer, http.MethodPatch, "/api/v1/notes/n0", b); code != http.StatusBadRequest {
			t.Errorf("PATCH %s = %d: %s", b, code, body)
		}
	}
}

func TestNotesEditBodyAuthorship(t *testing.T) {
	h := roleServer(t, notesMeta())
	edit := `{"body":"edited"}`
	wantCode(t, "creator", asRole(t, h, auth.RoleCreator, http.MethodPatch, "/api/v1/notes/n0", strings.NewReader(edit)), http.StatusForbidden)
	wantCode(t, "author", asRole(t, h, auth.RoleViewer, http.MethodPatch, "/api/v1/notes/n0", strings.NewReader(edit)), http.StatusOK)
	wantCode(t, "moderator", asRole(t, h, auth.RoleAdmin, http.MethodPatch, "/api/v1/notes/n0", strings.NewReader(edit)), http.StatusOK)
	wantCode(t, "unknown", asRole(t, h, auth.RoleAdmin, http.MethodPatch, "/api/v1/notes/nx", strings.NewReader(edit)), http.StatusNotFound)
}

func TestNotesResolve(t *testing.T) {
	meta := notesMeta()
	h := roleServer(t, meta)
	code, body := noteReq(t, h, auth.RoleCreator, http.MethodPatch, "/api/v1/notes/n0", `{"resolved":true}`)
	if code != http.StatusOK || meta.notes["n0"].ResolvedByName != "creator" {
		t.Errorf("resolve = %d: %s", code, body)
	}
	if code, body := noteReq(t, h, auth.RoleCreator, http.MethodPatch, "/api/v1/notes/r0", `{"resolved":true}`); code != http.StatusBadRequest {
		t.Errorf("resolve reply = %d: %s", code, body)
	}
}

func TestNotesDeleteAuthorship(t *testing.T) {
	meta := notesMeta()
	h := roleServer(t, meta)
	wantCode(t, "creator", asRole(t, h, auth.RoleCreator, http.MethodDelete, "/api/v1/notes/n0", nil), http.StatusForbidden)
	wantCode(t, "moderator", asRole(t, h, auth.RoleAdmin, http.MethodDelete, "/api/v1/notes/n0", nil), http.StatusNoContent)
	if _, ok := meta.notes["n0"]; ok {
		t.Error("n0 still stored")
	}
	wantCode(t, "author", asRole(t, h, auth.RoleViewer, http.MethodDelete, "/api/v1/notes/r0", nil), http.StatusNoContent)
	wantCode(t, "unknown", asRole(t, h, auth.RoleViewer, http.MethodDelete, "/api/v1/notes/r0", nil), http.StatusNotFound)
}

func TestNotesResolveRaceSaysResolved(t *testing.T) {
	meta := notesMeta()
	meta.errResolve = postgres.ErrReplyDepth
	h := roleServer(t, meta)
	code, body := noteReq(t, h, auth.RoleCreator, http.MethodPatch, "/api/v1/notes/n0", `{"resolved":true}`)
	if code != http.StatusBadRequest || !strings.Contains(body, "replies cannot be resolved") {
		t.Errorf("resolve race = %d: %s", code, body)
	}
}
