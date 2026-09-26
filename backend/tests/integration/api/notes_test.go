//go:build integration

package api_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"urara-vision/backend/internal/model"
)

// TestNotesOverHTTP: the viewer posts, the creator replies and resolves but
// cannot delete, and the admin can.
func TestNotesOverHTTP(t *testing.T) {
	m := newMatrix(t)
	_, _, sids := m.project("0.1.0")
	base := "/api/v1/snapshots/" + sids[0] + "/notes"

	post := func(as role, body map[string]string) model.Note {
		t.Helper()
		code, raw, _ := m.send(as, http.MethodPost, base, body)
		if code != http.StatusCreated {
			t.Fatalf("POST as %s = %d: %s", as, code, raw)
		}
		var n model.Note
		if err := json.Unmarshal(raw, &n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	counts := func(open, resolved int) {
		t.Helper()
		code, raw, _ := m.send(viewer, http.MethodGet, base+"/counts", nil)
		var out struct{ Counts []model.NoteCount }
		if code != http.StatusOK || json.Unmarshal(raw, &out) != nil {
			t.Fatalf("counts = %d: %s", code, raw)
		}
		var got model.NoteCount
		for _, c := range out.Counts {
			if c.AnchorKind == "table" && c.AnchorID == noteTable {
				got = c
			}
		}
		if got.Open != open || got.Resolved != resolved {
			t.Errorf("counts = %+v, want %d open, %d resolved", got, open, resolved)
		}
	}

	counts(0, 0)
	top := post(viewer, map[string]string{"anchorKind": "table", "anchorId": noteTable, "body": "why this grain?"})
	counts(1, 0)

	reply := post(creator, map[string]string{"parentId": top.ID, "body": "see the ADR"})
	if reply.AnchorID != noteTable || reply.AuthorID != m.ids[creator] {
		t.Errorf("reply = %+v", reply)
	}
	counts(1, 0)

	if code, raw, _ := m.send(creator, http.MethodPatch, "/api/v1/notes/"+top.ID, map[string]bool{"resolved": true}); code != http.StatusOK {
		t.Fatalf("resolve = %d: %s", code, raw)
	}
	counts(0, 1)

	if code, raw, _ := m.send(creator, http.MethodDelete, "/api/v1/notes/"+top.ID, nil); code != http.StatusForbidden {
		t.Errorf("creator delete = %d, want 403: %s", code, raw)
	}
	counts(0, 1)

	if code, raw, _ := m.send(admin, http.MethodDelete, "/api/v1/notes/"+top.ID, nil); code != http.StatusNoContent {
		t.Fatalf("admin delete = %d: %s", code, raw)
	}
	counts(0, 0)
	if code, _, _ := m.send(viewer, http.MethodPatch, "/api/v1/notes/"+reply.ID, map[string]string{"body": "x"}); code != http.StatusNotFound {
		t.Errorf("reply after parent delete = %d, want 404", code)
	}
}
