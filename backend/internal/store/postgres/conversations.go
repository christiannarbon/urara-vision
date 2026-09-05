// Chat conversations and their turns.
//
// Message ordinals are assigned by the database inside the INSERT itself rather
// than read into Go first. Reading the maximum and then writing it back is a
// race: two appends to the same conversation would both see the same number and
// one would be lost or would collide. Here the next ordinal is computed in the
// same statement that stores the row, and the primary key on
// (conversation_id, ordinal) is what makes a collision impossible rather than
// merely unlikely.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"urara-vision/backend/internal/model"
)

// uniqueViolation is the SQLSTATE for a duplicate key. It is compared as a code
// rather than matched in the error text, which is localised and free to change.
const uniqueViolation = "23505"

// conversationColumns is every field a conversation is rebuilt from, shared by
// the reads below so a new one cannot be added to only some of them.
const conversationColumns = `id, snapshot_id, title, created_at, updated_at`

// scanConversation reads one row of conversationColumns.
func scanConversation(row pgx.Row) (model.Conversation, error) {
	var c model.Conversation
	err := row.Scan(&c.ID, &c.SnapshotID, &c.Title, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

// CreateConversation starts a thread about one snapshot. The stored row comes
// back from the INSERT rather than from a second read, so the timestamps are
// the ones the database actually wrote.
func (s *Store) CreateConversation(ctx context.Context, snapshotID, title string) (*model.Conversation, error) {
	c, err := scanConversation(s.pool.QueryRow(ctx,
		`INSERT INTO conversations (id, snapshot_id, title)
		 VALUES ($1, $2, $3)
		 RETURNING `+conversationColumns,
		uuid.NewString(), snapshotID, title))
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// ListConversations returns the threads about one snapshot, newest first. It
// leaves Messages empty: a list is a menu, and loading every transcript to
// render it would grow without bound.
func (s *Store) ListConversations(ctx context.Context, snapshotID string) ([]model.Conversation, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+conversationColumns+`
		   FROM conversations WHERE snapshot_id = $1 ORDER BY created_at DESC`, snapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []model.Conversation{}
	for rows.Next() {
		c, err := scanConversation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetConversation returns one thread with its full transcript.
func (s *Store) GetConversation(ctx context.Context, id string) (*model.Conversation, error) {
	c, err := scanConversation(s.pool.QueryRow(ctx,
		`SELECT `+conversationColumns+` FROM conversations WHERE id = $1`, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	msgs, err := s.ListMessages(ctx, id)
	if err != nil {
		return nil, err
	}
	c.Messages = msgs
	return &c, nil
}

// DeleteConversation removes a thread and its messages.
func (s *Store) DeleteConversation(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM conversations WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// AppendMessage adds one turn to a conversation and returns it as stored, with
// the ordinal the database assigned.
//
// A concurrent append can compute the same ordinal: under READ COMMITTED both
// statements can see the same maximum before either commits. The primary key
// turns that into a unique violation rather than a duplicate ordinal, and the
// loser succeeds on a retry because by then the winner's row is visible. One
// retry is enough for two writers; a conversation with more than two
// simultaneous writers is not a case this API can produce.
func (s *Store) AppendMessage(ctx context.Context, conversationID string, m model.Message) (*model.Message, error) {
	const attempts = 2
	for attempt := 1; ; attempt++ {
		stored, err := s.appendMessageOnce(ctx, conversationID, m)
		if err == nil {
			return stored, nil
		}
		var pgErr *pgconn.PgError
		if attempt < attempts && errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			continue
		}
		return nil, err
	}
}

// appendMessageOnce is one attempt at AppendMessage: the insert and the parent
// row's updated_at in a single transaction, so a stored turn always leaves the
// conversation looking touched.
func (s *Store) appendMessageOnce(ctx context.Context, conversationID string, m model.Message) (*model.Message, error) {
	citations, err := json.Marshal(orEmptyStrings(m.Citations))
	if err != nil {
		return nil, err
	}
	meta, err := json.Marshal(orEmptyMeta(m.Meta))
	if err != nil {
		return nil, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// The parent is touched first: if it is not there, the transaction rolls
	// back before a message can be written under a conversation that does not
	// exist.
	tag, err := tx.Exec(ctx, `UPDATE conversations SET updated_at = now() WHERE id = $1`, conversationID)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}

	stored, err := scanMessage(tx.QueryRow(ctx,
		`INSERT INTO conversation_messages
		        (conversation_id, ordinal, role, content, citations, meta)
		 SELECT $1, coalesce(max(ordinal), -1) + 1, $2, $3, $4, $5
		   FROM conversation_messages WHERE conversation_id = $1
		 RETURNING `+messageColumns,
		conversationID, m.Role, m.Content, citations, meta))
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &stored, nil
}

// messageColumns is every field a message is rebuilt from.
const messageColumns = `ordinal, role, content, citations, meta, created_at`

// scanMessage reads one row of messageColumns. Citations always comes back as a
// list: "cited nothing" is a real answer, and it must not arrive as null.
func scanMessage(row pgx.Row) (model.Message, error) {
	var m model.Message
	var citations, meta []byte
	if err := row.Scan(&m.Ordinal, &m.Role, &m.Content, &citations, &meta, &m.CreatedAt); err != nil {
		return m, err
	}
	if err := json.Unmarshal(citations, &m.Citations); err != nil {
		return m, err
	}
	m.Citations = orEmptyStrings(m.Citations)
	if err := json.Unmarshal(meta, &m.Meta); err != nil {
		return m, err
	}
	return m, nil
}

// ListMessages returns a conversation's turns in the order they were written.
func (s *Store) ListMessages(ctx context.Context, conversationID string) ([]model.Message, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+messageColumns+`
		   FROM conversation_messages WHERE conversation_id = $1 ORDER BY ordinal ASC`,
		conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []model.Message{}
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// orEmptyMeta keeps a nil meta map out of the database as {} rather than null,
// matching the column's default.
func orEmptyMeta(v map[string]any) map[string]any {
	if v == nil {
		return map[string]any{}
	}
	return v
}
