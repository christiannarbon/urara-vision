//go:build integration

package postgres_test

import (
	"errors"
	"slices"
	"testing"

	"urara-vision/backend/internal/model"
	"urara-vision/backend/internal/store/postgres"
	"urara-vision/backend/tests/integration/harness"
)

func listedIDs(t *testing.T, pg *postgres.Store, owner, sid string) []string {
	t.Helper()
	convs, err := pg.ListConversations(harness.Context(t), owner, sid, 50)
	if err != nil {
		t.Fatalf("ListConversations(%q): %v", owner, err)
	}
	ids := []string{}
	for _, c := range convs {
		ids = append(ids, c.ID)
	}
	return ids
}

func TestConversationsAreScopedToTheirOwner(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)
	sid := harness.SavedModel(t, ctx, pg).Snapshot.ID
	a := createdUser(t, ctx, pg, "h")
	b := createdUser(t, ctx, pg, "h")

	conv, err := pg.CreateConversation(ctx, a.ID, sid, "a's")
	if err != nil {
		t.Fatal(err)
	}
	if conv.UserID != a.ID {
		t.Errorf("UserID = %q, want %q", conv.UserID, a.ID)
	}
	if _, err := pg.AppendMessage(ctx, a.ID, conv.ID, model.Message{Role: model.RoleUser, Content: "hi"}); err != nil {
		t.Fatal(err)
	}
	bConv, err := pg.CreateConversation(ctx, b.ID, sid, "b's")
	if err != nil {
		t.Fatal(err)
	}

	if got, err := pg.GetConversation(ctx, a.ID, conv.ID); err != nil || len(got.Messages) != 1 {
		t.Errorf("owner's Get = %+v, %v", got, err)
	}
	if _, err := pg.GetConversation(ctx, b.ID, conv.ID); !errors.Is(err, postgres.ErrNotFound) {
		t.Errorf("stranger's Get err = %v", err)
	}
	if _, err := pg.ListMessages(ctx, b.ID, conv.ID); !errors.Is(err, postgres.ErrNotFound) {
		t.Errorf("stranger's ListMessages err = %v", err)
	}

	if ids := listedIDs(t, pg, b.ID, sid); slices.Contains(ids, conv.ID) || !slices.Contains(ids, bConv.ID) {
		t.Errorf("b's list = %v", ids)
	}
	if ids := listedIDs(t, pg, "", sid); !slices.Contains(ids, conv.ID) || !slices.Contains(ids, bConv.ID) {
		t.Errorf("service list = %v, want both", ids)
	}
	if _, err := pg.GetConversation(ctx, "", conv.ID); err != nil {
		t.Errorf("service Get: %v", err)
	}
}

func TestStrangerCannotChangeAConversation(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)
	sid := harness.SavedModel(t, ctx, pg).Snapshot.ID
	a := createdUser(t, ctx, pg, "h")
	b := createdUser(t, ctx, pg, "h")

	conv, err := pg.CreateConversation(ctx, a.ID, sid, "original")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := pg.AppendMessage(ctx, b.ID, conv.ID, model.Message{Role: model.RoleUser, Content: "x"}); !errors.Is(err, postgres.ErrNotFound) {
		t.Errorf("append err = %v", err)
	}
	if _, err := pg.UpdateConversationTitle(ctx, b.ID, conv.ID, "hijacked"); !errors.Is(err, postgres.ErrNotFound) {
		t.Errorf("rename err = %v", err)
	}
	if err := pg.DeleteConversation(ctx, b.ID, conv.ID); !errors.Is(err, postgres.ErrNotFound) {
		t.Errorf("delete err = %v", err)
	}

	got, err := pg.GetConversation(ctx, a.ID, conv.ID)
	if err != nil {
		t.Fatalf("conversation gone: %v", err)
	}
	if got.Title != "original" || len(got.Messages) != 0 || !got.UpdatedAt.Equal(conv.UpdatedAt) {
		t.Errorf("conversation changed: %+v", got)
	}
}

func TestLegacyConversationIsServiceOnly(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)
	sid := harness.SavedModel(t, ctx, pg).Snapshot.ID
	a := createdUser(t, ctx, pg, "h")

	legacy, err := pg.CreateConversation(ctx, "", sid, "legacy")
	if err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, `SELECT count(*) FROM conversations WHERE id = $1 AND user_id IS NULL`, legacy.ID); n != 1 {
		t.Fatalf("service conversation should have a NULL owner")
	}

	if ids := listedIDs(t, pg, a.ID, sid); slices.Contains(ids, legacy.ID) {
		t.Errorf("user list includes the legacy conversation")
	}
	if _, err := pg.GetConversation(ctx, a.ID, legacy.ID); !errors.Is(err, postgres.ErrNotFound) {
		t.Errorf("user Get err = %v", err)
	}
	if ids := listedIDs(t, pg, "", sid); !slices.Contains(ids, legacy.ID) {
		t.Errorf("service list misses the legacy conversation")
	}
}

func TestDeletingAUserDeletesTheirConversations(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)
	sid := harness.SavedModel(t, ctx, pg).Snapshot.ID
	a := createdUser(t, ctx, pg, "h")

	conv, err := pg.CreateConversation(ctx, a.ID, sid, "t")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pg.AppendMessage(ctx, a.ID, conv.ID, model.Message{Role: model.RoleUser, Content: "hi"}); err != nil {
		t.Fatal(err)
	}

	if err := pg.DeleteUser(ctx, a.ID); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if _, err := pg.GetConversation(ctx, "", conv.ID); !errors.Is(err, postgres.ErrNotFound) {
		t.Errorf("conversation survived its owner: %v", err)
	}
	if n := countMessages(t, ctx, conv.ID); n != 0 {
		t.Errorf("%d messages left", n)
	}
}
