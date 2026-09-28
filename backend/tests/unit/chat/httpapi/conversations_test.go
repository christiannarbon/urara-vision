// Ported from the CRUD cases in chat/tests/unit/test_chat_routes.py.
package httpapi_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"urara-vision/backend/internal/chat/apiclient"
	"urara-vision/backend/internal/model"
)

// upstream is the backend's own message, which a caller must never see.
const upstream = "conversations table is on fire at db-internal-7"

func conversationCall(t *testing.T, f *fakeBackend, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, r)
	req.Header.Set("X-User-Id", "user-1")
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	return serve(gated(t, f, newClock()), req)
}

func decoded(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body %q: %v", rec.Body, err)
	}
	return body
}

func firstField(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	fields, _ := decoded(t, rec)["fields"].([]any)
	if len(fields) == 0 {
		t.Fatalf("no fields in %s", rec.Body)
	}
	return fields[0].(map[string]any)
}

func TestConversationGoldens(t *testing.T) {
	cases := []struct{ golden, method, path, body string }{
		{"conversations/create.json", "POST", "/api/chat/conversations", `{"snapshotId":"latest","title":"golden"}`},
		{"conversations/list.json", "GET", "/api/chat/conversations?snapshot=latest", ""},
		{"conversations/get.json", "GET", "/api/chat/conversations/0d3c5e6f-7a8b-4c9d-8e1f-2a3b4c5d6e7f", ""},
		{"conversations/delete.json", "DELETE", "/api/chat/conversations/0d3c5e6f-7a8b-4c9d-8e1f-2a3b4c5d6e7f", ""},
	}
	for _, c := range cases {
		t.Run(c.golden, func(t *testing.T) {
			assertGolden(t, c.golden, conversationCall(t, &fakeBackend{enabled: true}, c.method, c.path, c.body))
		})
	}
}

func TestConversationUnknownGolden(t *testing.T) {
	f := &fakeBackend{enabled: true, convErr: &apiclient.Error{Status: 404, Message: upstream}}
	for _, method := range []string{"GET", "DELETE"} {
		rec := conversationCall(t, f, method, "/api/chat/conversations/nope", "")
		assertGolden(t, "errors/conversation-unknown.json", rec)
		if strings.Contains(rec.Body.String(), upstream) {
			t.Errorf("%s leaks the backend's message", method)
		}
	}
}

func TestConversationActingUserReachesTheBackend(t *testing.T) {
	f := &fakeBackend{enabled: true}
	conversationCall(t, f, "POST", "/api/chat/conversations", `{"snapshotId":"latest"}`)
	conversationCall(t, f, "GET", "/api/chat/conversations?snapshot=latest", "")
	conversationCall(t, f, "GET", "/api/chat/conversations/c1", "")
	conversationCall(t, f, "DELETE", "/api/chat/conversations/c1", "")
	if want := []string{"user-1", "user-1", "user-1", "user-1"}; !reflect.DeepEqual(f.convUsers, want) {
		t.Errorf("users %v, want %v", f.convUsers, want)
	}
}

func TestConversationCreateLatestReachesTheBackendUntouched(t *testing.T) {
	f := &fakeBackend{enabled: true}
	rec := conversationCall(t, f, "POST", "/api/chat/conversations", `{"snapshotId":"latest","title":"why"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d", rec.Code)
	}
	if want := []createCall{{"latest", "why"}}; !reflect.DeepEqual(f.created, want) {
		t.Errorf("created %v", f.created)
	}
	if sid := decoded(t, rec)["snapshotId"]; sid == "latest" {
		t.Error("the alias came back, not the backend's concrete ID")
	}
}

func TestConversationCreateAcceptsSnakeCase(t *testing.T) {
	f := &fakeBackend{enabled: true}
	if rec := conversationCall(t, f, "POST", "/api/chat/conversations", `{"snapshot_id":"latest"}`); rec.Code != http.StatusCreated {
		t.Fatalf("status %d", rec.Code)
	}
	if want := []createCall{{"latest", ""}}; !reflect.DeepEqual(f.created, want) {
		t.Errorf("created %v", f.created)
	}
}

func TestConversationCreateRequestShape(t *testing.T) {
	cases := []struct{ name, body, field string }{
		{"missing snapshotId", `{"title":"orphan"}`, "snapshotId"},
		{"unknown field", `{"snapshotId":"latest","colour":"red"}`, "colour"},
		{"wrong type", `{"snapshotId":7}`, "snapshotId"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := &fakeBackend{enabled: true}
			rec := conversationCall(t, f, "POST", "/api/chat/conversations", c.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status %d", rec.Code)
			}
			if got := firstField(t, rec); got["field"] != c.field || got["location"] != "body" {
				t.Errorf("field %v", got)
			}
			if len(f.created) != 0 {
				t.Error("the backend was called")
			}
		})
	}
}

func TestConversationListPassesSnapshotAndLimit(t *testing.T) {
	f := &fakeBackend{enabled: true}
	conversationCall(t, f, "GET", "/api/chat/conversations?snapshot=latest&limit=5", "")
	conversationCall(t, f, "GET", "/api/chat/conversations?snapshot=latest", "")
	if want := []listCall{{"latest", 5}, {"latest", 0}}; !reflect.DeepEqual(f.listed, want) {
		t.Errorf("listed %v, want %v", f.listed, want)
	}
}

func TestConversationListQueryValidation(t *testing.T) {
	cases := []struct{ name, query, field string }{
		{"missing snapshot", "", "snapshot"},
		{"empty snapshot", "?snapshot=", "snapshot"},
		{"limit 0", "?snapshot=latest&limit=0", "limit"},
		{"limit abc", "?snapshot=latest&limit=abc", "limit"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := &fakeBackend{enabled: true}
			rec := conversationCall(t, f, "GET", "/api/chat/conversations"+c.query, "")
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status %d", rec.Code)
			}
			if got := firstField(t, rec); got["field"] != c.field || got["location"] != "query" {
				t.Errorf("field %v", got)
			}
			if len(f.listed) != 0 {
				t.Error("the backend was called")
			}
		})
	}
}

func TestConversationMissingSnapshotIsFieldRequired(t *testing.T) {
	rec := conversationCall(t, &fakeBackend{enabled: true}, "GET", "/api/chat/conversations", "")
	want := map[string]any{"field": "snapshot", "location": "query", "reason": "Field required"}
	if got := firstField(t, rec); !reflect.DeepEqual(got, want) {
		t.Errorf("field %v", got)
	}
}

func TestConversationGetReturnsTheTranscript(t *testing.T) {
	f := &fakeBackend{enabled: true, messages: []model.Message{
		{Ordinal: 1, Role: "user", Content: "hi"},
		{Ordinal: 2, Role: "assistant", Content: "hello", Citations: []string{"ordering/fact_orders"}, Meta: map[string]any{"model": "m"}},
	}}
	rec := conversationCall(t, f, "GET", "/api/chat/conversations/conv-9", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := decoded(t, rec)
	if body["id"] != "conv-9" || !reflect.DeepEqual(f.fetched, []string{"conv-9"}) {
		t.Errorf("id %v, fetched %v", body["id"], f.fetched)
	}
	messages := body["messages"].([]any)
	if len(messages) != 2 {
		t.Fatalf("%d messages", len(messages))
	}
	second := messages[1].(map[string]any)
	if second["role"] != "assistant" || !reflect.DeepEqual(second["citations"], []any{"ordering/fact_orders"}) {
		t.Errorf("message %v", second)
	}
}

func TestConversationMessagesHaveNoNulls(t *testing.T) {
	f := &fakeBackend{enabled: true, messages: []model.Message{{Role: "user", Content: "hi", Citations: nil, Meta: nil}}}
	rec := conversationCall(t, f, "GET", "/api/chat/conversations/c1", "")
	msg := decoded(t, rec)["messages"].([]any)[0].(map[string]any)
	for _, key := range []string{"ordinal", "role", "content", "citations", "meta", "createdAt"} {
		if _, ok := msg[key]; !ok {
			t.Errorf("no %s", key)
		}
	}
	if !reflect.DeepEqual(msg["citations"], []any{}) || !reflect.DeepEqual(msg["meta"], map[string]any{}) {
		t.Errorf("citations %v, meta %v", msg["citations"], msg["meta"])
	}
}

func TestConversationDeleteIs204WithNoBody(t *testing.T) {
	f := &fakeBackend{enabled: true}
	rec := conversationCall(t, f, "DELETE", "/api/chat/conversations/conv-1", "")
	if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
		t.Errorf("status %d, body %q", rec.Code, rec.Body)
	}
	if !reflect.DeepEqual(f.deleted, []string{"conv-1"}) {
		t.Errorf("deleted %v", f.deleted)
	}
}

func TestConversationBackendErrors(t *testing.T) {
	cases := []struct {
		status int
		want   int
		msg    string
	}{
		{404, http.StatusNotFound, "not found"},
		{403, http.StatusForbidden, "not allowed"},
		{500, http.StatusBadGateway, "the model store is unavailable"},
		{422, http.StatusInternalServerError, "internal error"},
	}
	routes := []struct{ method, path, body string }{
		{"POST", "/api/chat/conversations", `{"snapshotId":"nope"}`},
		{"GET", "/api/chat/conversations?snapshot=latest", ""},
		{"GET", "/api/chat/conversations/c1", ""},
		{"DELETE", "/api/chat/conversations/c1", ""},
	}
	for _, c := range cases {
		for _, route := range routes {
			f := &fakeBackend{enabled: true, convErr: &apiclient.Error{Status: c.status, Message: upstream}}
			rec := conversationCall(t, f, route.method, route.path, route.body)
			body := decoded(t, rec)
			if rec.Code != c.want || body["error"] != c.msg {
				t.Errorf("%s %s on backend %d: %d %v", route.method, route.path, c.status, rec.Code, body["error"])
			}
			if strings.Contains(rec.Body.String(), upstream) {
				t.Errorf("%s %s leaks the backend's message", route.method, route.path)
			}
			if body["requestId"] != rec.Header().Get("X-Request-Id") || body["requestId"] == "" {
				t.Errorf("request IDs: body %v, header %q", body["requestId"], rec.Header().Get("X-Request-Id"))
			}
		}
	}
}
