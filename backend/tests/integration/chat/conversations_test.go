//go:build integration

// Ported from chat/tests/integration/test_conversations.py, minus the model.
package chat_test

import (
	"net/http"
	"net/url"
	"testing"
)

func TestConversationCRUD(t *testing.T) {
	sid := fixture(t)

	created := chatPost(t, "/api/chat/conversations", map[string]string{"snapshotId": sid, "title": "golden"})
	assertGolden(t, "conversations/create.json", created)
	cid, _ := created.json(t)["id"].(string)
	if cid == "" {
		t.Fatalf("no id in %s", created.raw)
	}
	deleted := false
	t.Cleanup(func() {
		if !deleted {
			chatDelete(t, "/api/chat/conversations/"+cid)
		}
	})

	listed := chatGet(t, "/api/chat/conversations?snapshot="+url.QueryEscape(sid))
	assertGolden(t, "conversations/list.json", listed)
	convs, _ := listed.json(t)["conversations"].([]any)
	if len(convs) != 1 || convs[0].(map[string]any)["id"] != cid {
		t.Errorf("listing %s", listed.raw)
	}

	fetched := chatGet(t, "/api/chat/conversations/"+cid)
	assertGolden(t, "conversations/get.json", fetched)

	assertGolden(t, "conversations/delete.json", chatDelete(t, "/api/chat/conversations/"+cid))
	deleted = true
	assertGolden(t, "errors/conversation-unknown.json", chatGet(t, "/api/chat/conversations/"+cid))
}

func TestConversationCreateWithLatestStoresAConcreteID(t *testing.T) {
	fixture(t)
	created := chatPost(t, "/api/chat/conversations", map[string]string{"snapshotId": "latest"})
	if created.status != http.StatusCreated {
		t.Fatalf("%d %s", created.status, created.raw)
	}
	body := created.json(t)
	t.Cleanup(func() { chatDelete(t, "/api/chat/conversations/"+body["id"].(string)) })
	if sid, _ := body["snapshotId"].(string); sid == "latest" || len(sid) != 36 {
		t.Errorf("snapshotId %q is not a concrete UUID", sid)
	}
	if msgs, ok := body["messages"].([]any); !ok || len(msgs) != 0 {
		t.Errorf("messages %v", body["messages"])
	}
}

func TestConversationAnonymousIs401(t *testing.T) {
	assertGolden(t, "errors/anonymous.json", chatDo(t, "GET", "/api/chat/conversations?snapshot=latest", "", nil))
}

func TestConversationCascadeFromProjectDelete(t *testing.T) {
	sid := fixture(t)
	created := chatPost(t, "/api/chat/conversations", map[string]string{"snapshotId": sid})
	cid, _ := created.json(t)["id"].(string)

	// A message, so the cascade has something to cascade through.
	res := backendDo(t, "POST", "/api/v1/conversations/"+cid+"/messages",
		map[string]any{"role": "user", "content": "does this survive?", "citations": []string{}}, true)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusCreated && res.StatusCode != http.StatusOK {
		t.Fatalf("append: %d", res.StatusCode)
	}

	res = backendDo(t, "DELETE", "/api/v1/snapshots/"+sid, nil, true)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("delete snapshot: %d", res.StatusCode)
	}
	if got := chatGet(t, "/api/chat/conversations/"+cid); got.status != http.StatusNotFound {
		t.Errorf("after the snapshot went: %d", got.status)
	}
}

func TestStatsEmptySnapshot(t *testing.T) {
	sid := fixture(t)
	assertGolden(t, "stats/empty.json", chatGet(t, "/api/chat/stats?snapshot="+url.QueryEscape(sid)))
}
