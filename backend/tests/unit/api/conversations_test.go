// The conversation endpoints: what the handlers themselves decide, which is
// parameter validation and how a store outcome becomes a status code.
//
// The case that matters most is the "latest" alias. It is resolved when a
// thread is created, so what reaches the store is a concrete snapshot ID and a
// transcript cannot change subject under a later ingest.
package api_test

import (
	"net/http"
	"strings"
	"testing"

	"urara-vision/backend/internal/model"
	"urara-vision/backend/internal/store/postgres"
)

// postJSON issues a POST with a JSON body.
func postJSON(t *testing.T, h http.Handler, target, body string) *http.Response {
	t.Helper()
	return do(t, h, http.MethodPost, target, strings.NewReader(body), "application/json").Result()
}

func TestCreateConversationResolvesLatest(t *testing.T) {
	meta := &fakeMeta{latest: "real-snapshot-id"}
	h := newServer(t, meta, &fakeGraphs{})

	res := postJSON(t, h, "/api/v1/conversations", `{"snapshotId":"latest","title":"why"}`)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", res.StatusCode)
	}
	// The whole point: the store must never be handed the literal alias.
	if meta.createdFor != "real-snapshot-id" {
		t.Errorf("stored snapshot = %q, want the resolved ID", meta.createdFor)
	}
	if meta.createdTitle != "why" {
		t.Errorf("title = %q", meta.createdTitle)
	}
}

func TestCreateConversationRejectsBadInput(t *testing.T) {
	t.Run("unknown snapshot", func(t *testing.T) {
		meta := &fakeMeta{errGetSnapshot: postgres.ErrNotFound}
		h := newServer(t, meta, &fakeGraphs{})

		res := postJSON(t, h, "/api/v1/conversations", `{"snapshotId":"nope"}`)
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", res.StatusCode)
		}
		if meta.createdFor != "" {
			t.Error("a conversation was created against a snapshot that does not exist")
		}
	})

	t.Run("missing snapshotId", func(t *testing.T) {
		h := newServer(t, &fakeMeta{}, &fakeGraphs{})
		res := postJSON(t, h, "/api/v1/conversations", `{"title":"orphan"}`)
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", res.StatusCode)
		}
	})

	// A misspelled field must be refused rather than silently dropped: a client
	// that sends "snapshot_id" should hear about it, not watch its value vanish.
	t.Run("unknown field", func(t *testing.T) {
		h := newServer(t, &fakeMeta{}, &fakeGraphs{})
		res := postJSON(t, h, "/api/v1/conversations", `{"snapshotId":"s1","snapshot_id":"s1"}`)
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", res.StatusCode)
		}
	})
}

func TestListConversationsRequiresSnapshot(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		h := newServer(t, &fakeMeta{}, &fakeGraphs{})
		rec := do(t, h, http.MethodGet, "/api/v1/conversations", nil, "")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
	})

	t.Run("latest is accepted and resolved", func(t *testing.T) {
		meta := &fakeMeta{
			latest:        "real-snapshot-id",
			conversations: []model.Conversation{{ID: "conv-1"}},
		}
		h := newServer(t, meta, &fakeGraphs{})

		rec := do(t, h, http.MethodGet, "/api/v1/conversations?snapshot=latest", nil, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		if meta.listedFor != "real-snapshot-id" {
			t.Errorf("listed for %q, want the resolved ID", meta.listedFor)
		}
		if convs, _ := decode(t, rec.Body.Bytes())["conversations"].([]any); len(convs) != 1 {
			t.Errorf("conversations = %v, want 1", convs)
		}
	})
}

func TestGetConversationUnknownIs404(t *testing.T) {
	h := newServer(t, &fakeMeta{}, &fakeGraphs{})
	rec := do(t, h, http.MethodGet, "/api/v1/conversations/nope", nil, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestDeleteConversation(t *testing.T) {
	t.Run("success is 204", func(t *testing.T) {
		meta := &fakeMeta{}
		h := newServer(t, meta, &fakeGraphs{})
		rec := do(t, h, http.MethodDelete, "/api/v1/conversations/conv-1", nil, "")
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want 204", rec.Code)
		}
		if meta.convID != "conv-1" {
			t.Errorf("deleted %q", meta.convID)
		}
	})

	t.Run("unknown is 404", func(t *testing.T) {
		h := newServer(t, &fakeMeta{errConversation: postgres.ErrNotFound}, &fakeGraphs{})
		rec := do(t, h, http.MethodDelete, "/api/v1/conversations/nope", nil, "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", rec.Code)
		}
	})
}

// TestAppendMessageReturnsStoredOrdinal: the ordinal is the database's to
// assign, so the handler must return what came back rather than what was sent.
func TestAppendMessageReturnsStoredOrdinal(t *testing.T) {
	meta := &fakeMeta{appended: &model.Message{Ordinal: 7, Role: model.RoleUser, Content: "hi"}}
	h := newServer(t, meta, &fakeGraphs{})

	rec := do(t, h, http.MethodPost, "/api/v1/conversations/conv-1/messages",
		strings.NewReader(`{"role":"user","content":"hi","citations":["d/t"]}`), "application/json")
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body)
	}
	if got := decode(t, rec.Body.Bytes())["ordinal"]; got != float64(7) {
		t.Errorf("ordinal = %v, want the store's 7", got)
	}
	if meta.appendedTo != "conv-1" {
		t.Errorf("appended to %q", meta.appendedTo)
	}
	if len(meta.appendedMsg.Citations) != 1 {
		t.Errorf("citations = %v, did not reach the store", meta.appendedMsg.Citations)
	}
}

func TestAppendMessageRejectsBadInput(t *testing.T) {
	t.Run("invalid role", func(t *testing.T) {
		meta := &fakeMeta{}
		h := newServer(t, meta, &fakeGraphs{})
		rec := do(t, h, http.MethodPost, "/api/v1/conversations/conv-1/messages",
			strings.NewReader(`{"role":"robot","content":"hi"}`), "application/json")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
		// The message has to name what is acceptable, or the caller is guessing.
		msg, _ := decode(t, rec.Body.Bytes())["error"].(string)
		for _, role := range []string{model.RoleUser, model.RoleAssistant, model.RoleSystem} {
			if !strings.Contains(msg, role) {
				t.Errorf("error %q does not name the valid role %q", msg, role)
			}
		}
		if meta.appendedTo != "" {
			t.Error("an invalid role reached the store")
		}
	})

	t.Run("whitespace-only content", func(t *testing.T) {
		meta := &fakeMeta{}
		h := newServer(t, meta, &fakeGraphs{})
		rec := do(t, h, http.MethodPost, "/api/v1/conversations/conv-1/messages",
			strings.NewReader(`{"role":"user","content":"   "}`), "application/json")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
		if meta.appendedTo != "" {
			t.Error("an empty message reached the store")
		}
	})

	t.Run("unknown field", func(t *testing.T) {
		h := newServer(t, &fakeMeta{}, &fakeGraphs{})
		rec := do(t, h, http.MethodPost, "/api/v1/conversations/conv-1/messages",
			strings.NewReader(`{"role":"user","content":"hi","citation":["d/t"]}`), "application/json")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
	})

	t.Run("unknown conversation", func(t *testing.T) {
		h := newServer(t, &fakeMeta{errAppend: postgres.ErrNotFound}, &fakeGraphs{})
		rec := do(t, h, http.MethodPost, "/api/v1/conversations/nope/messages",
			strings.NewReader(`{"role":"user","content":"hi"}`), "application/json")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", rec.Code)
		}
	})
}
