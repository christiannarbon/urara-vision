//go:build integration

// Conversations against a real database: the SQL, the two cascades, and the
// ordinal race.
//
// These are the parts no fake can stand in for. In particular the concurrent
// append is the only place the retry on a unique violation is actually
// exercised -- in-process fakes hand out ordinals sequentially and would pass
// whether or not the store got that right.
package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"urara-vision/backend/internal/model"
	"urara-vision/backend/internal/store/postgres"
	"urara-vision/backend/tests/integration/harness"
)

// countMessages reads conversation_messages directly, so a cascade is proven by
// the rows being gone rather than by a store method declining to find them.
func countMessages(t *testing.T, ctx context.Context, conversationID string) int {
	t.Helper()
	conn, err := pgx.Connect(ctx, harness.PostgresDSN(t))
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	defer func() { _ = conn.Close(context.Background()) }()

	var n int
	if err := conn.QueryRow(ctx,
		`SELECT count(*) FROM conversation_messages WHERE conversation_id = $1`,
		conversationID).Scan(&n); err != nil {
		t.Fatalf("count messages: %v", err)
	}
	return n
}

func TestConversationRoundTrip(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)
	m := harness.SavedModel(t, ctx, pg)

	conv, err := pg.CreateConversation(ctx, m.Snapshot.ID, "why is fact_primary conformed")
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}

	citations := []string{"domain_one/fact_primary", "domain_one/dim_alpha"}
	meta := map[string]any{
		"model": "claude-opus-5",
		"usage": map[string]any{"inputTokens": float64(1200)},
	}
	for _, in := range []model.Message{
		{Role: model.RoleUser, Content: "which tables are conformed?"},
		{Role: model.RoleAssistant, Content: "two of them", Citations: citations, Meta: meta},
		{Role: model.RoleUser, Content: "why?"},
	} {
		if _, err := pg.AppendMessage(ctx, conv.ID, in); err != nil {
			t.Fatalf("AppendMessage(%s): %v", in.Role, err)
		}
	}

	got, err := pg.GetConversation(ctx, conv.ID)
	if err != nil {
		t.Fatalf("GetConversation: %v", err)
	}
	if len(got.Messages) != 3 {
		t.Fatalf("messages = %d, want 3", len(got.Messages))
	}

	wantRoles := []string{model.RoleUser, model.RoleAssistant, model.RoleUser}
	for i, msg := range got.Messages {
		if msg.Ordinal != i {
			t.Errorf("message %d has ordinal %d", i, msg.Ordinal)
		}
		if msg.Role != wantRoles[i] {
			t.Errorf("message %d role = %q, want %q", i, msg.Role, wantRoles[i])
		}
		if msg.CreatedAt.IsZero() {
			t.Errorf("message %d has no created_at", i)
		}
	}

	answer := got.Messages[1]
	if len(answer.Citations) != len(citations) {
		t.Fatalf("citations = %v, want %v", answer.Citations, citations)
	}
	for i, c := range citations {
		if answer.Citations[i] != c {
			t.Errorf("citation %d = %q, want %q", i, answer.Citations[i], c)
		}
	}
	if answer.Meta["model"] != "claude-opus-5" {
		t.Errorf("meta model = %v", answer.Meta["model"])
	}
	// The nested value is the one that would be lost by a shallow round trip.
	usage, ok := answer.Meta["usage"].(map[string]any)
	if !ok {
		t.Fatalf("meta usage = %v (%T), want a nested object", answer.Meta["usage"], answer.Meta["usage"])
	}
	if usage["inputTokens"] != float64(1200) {
		t.Errorf("meta usage.inputTokens = %v, want 1200", usage["inputTokens"])
	}

	// A turn that cited nothing must come back as an empty list, not null:
	// "drew on no tables" is an answer, and it has to survive storage.
	if got.Messages[0].Citations == nil {
		t.Error("a message with no citations came back nil rather than an empty list")
	}
}

func TestConversationUpdatedAtAdvances(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)
	m := harness.SavedModel(t, ctx, pg)

	conv, err := pg.CreateConversation(ctx, m.Snapshot.ID, "t")
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	before := conv.UpdatedAt

	if _, err := pg.AppendMessage(ctx, conv.ID, model.Message{Role: model.RoleUser, Content: "hi"}); err != nil {
		t.Fatalf("AppendMessage: %v", err)
	}

	got, err := pg.GetConversation(ctx, conv.ID)
	if err != nil {
		t.Fatalf("GetConversation: %v", err)
	}
	if !got.UpdatedAt.After(before) {
		t.Errorf("updated_at = %v, want it after the creation time %v", got.UpdatedAt, before)
	}
	if len(got.Messages) != 1 {
		t.Errorf("messages = %d, want 1", len(got.Messages))
	}
}

func TestConversationCascadeFromConversation(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)
	m := harness.SavedModel(t, ctx, pg)

	conv, err := pg.CreateConversation(ctx, m.Snapshot.ID, "t")
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	for i := 0; i < 2; i++ {
		if _, err := pg.AppendMessage(ctx, conv.ID, model.Message{Role: model.RoleUser, Content: "hi"}); err != nil {
			t.Fatalf("AppendMessage: %v", err)
		}
	}
	if n := countMessages(t, ctx, conv.ID); n != 2 {
		t.Fatalf("messages before delete = %d, want 2", n)
	}

	if err := pg.DeleteConversation(ctx, conv.ID); err != nil {
		t.Fatalf("DeleteConversation: %v", err)
	}
	if n := countMessages(t, ctx, conv.ID); n != 0 {
		t.Errorf("messages after deleting the conversation = %d, want 0", n)
	}
}

// TestConversationCascadeFromSnapshot is what justifies hanging conversations
// off the snapshot. Every citation a transcript carries is a table ID, so once
// the snapshot is gone those point at nothing and the transcript goes with it.
func TestConversationCascadeFromSnapshot(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)
	m := harness.SavedModel(t, ctx, pg)

	conv, err := pg.CreateConversation(ctx, m.Snapshot.ID, "t")
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	if _, err := pg.AppendMessage(ctx, conv.ID, model.Message{
		Role:      model.RoleAssistant,
		Content:   "fact_primary joins dim_alpha",
		Citations: []string{"domain_one/fact_primary"},
	}); err != nil {
		t.Fatalf("AppendMessage: %v", err)
	}

	if err := pg.DeleteSnapshot(ctx, m.Snapshot.ID); err != nil {
		t.Fatalf("DeleteSnapshot: %v", err)
	}

	if _, err := pg.GetConversation(ctx, conv.ID); !errors.Is(err, postgres.ErrNotFound) {
		t.Errorf("GetConversation after the snapshot went = %v, want ErrNotFound", err)
	}
	if n := countMessages(t, ctx, conv.ID); n != 0 {
		t.Errorf("messages after deleting the snapshot = %d, want 0", n)
	}
}

// TestConversationConcurrentAppendOrdinals is what justifies the retry in
// AppendMessage. Two writers computing the next ordinal at the same time see
// the same maximum; the primary key turns that into a unique violation, and the
// loser has to succeed on its retry. A gap or a duplicate here means a
// transcript that cannot be replayed in order.
func TestConversationConcurrentAppendOrdinals(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)
	m := harness.SavedModel(t, ctx, pg)

	conv, err := pg.CreateConversation(ctx, m.Snapshot.ID, "t")
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}

	const (
		writers = 2
		each    = 20
		total   = writers * each
	)
	errs := make(chan error, total)
	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < each; i++ {
				_, err := pg.AppendMessage(ctx, conv.ID, model.Message{
					Role:    model.RoleUser,
					Content: fmt.Sprintf("writer %d message %d", w, i),
				})
				if err != nil {
					errs <- fmt.Errorf("writer %d message %d: %w", w, i, err)
				}
			}
		}(w)
	}
	wg.Wait()
	close(errs)

	// Every failure, not just the first: one lost append and one duplicate
	// ordinal are different bugs and the run should say which happened.
	for err := range errs {
		t.Errorf("append failed: %v", err)
	}

	msgs, err := pg.ListMessages(ctx, conv.ID)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(msgs) != total {
		t.Fatalf("messages = %d, want %d", len(msgs), total)
	}

	seen := make(map[int]bool, total)
	for _, msg := range msgs {
		if seen[msg.Ordinal] {
			t.Errorf("ordinal %d appears more than once", msg.Ordinal)
		}
		seen[msg.Ordinal] = true
	}
	for i := 0; i < total; i++ {
		if !seen[i] {
			t.Errorf("ordinal %d is missing: the sequence has a gap", i)
		}
	}
}

func TestConversationNotFound(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)

	const unknown = "no-such-conversation"

	if _, err := pg.GetConversation(ctx, unknown); !errors.Is(err, postgres.ErrNotFound) {
		t.Errorf("GetConversation = %v, want ErrNotFound", err)
	}
	if err := pg.DeleteConversation(ctx, unknown); !errors.Is(err, postgres.ErrNotFound) {
		t.Errorf("DeleteConversation = %v, want ErrNotFound", err)
	}
	if _, err := pg.AppendMessage(ctx, unknown, model.Message{
		Role: model.RoleUser, Content: "hi",
	}); !errors.Is(err, postgres.ErrNotFound) {
		t.Errorf("AppendMessage = %v, want ErrNotFound", err)
	}
}
