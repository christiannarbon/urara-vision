// Sticky notes on parts of a snapshot. Bodies are checked by internal/notes.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"urara-vision/backend/internal/model"
	"urara-vision/backend/internal/notes"
)

// ErrReplyDepth is returned for a reply to a reply, or for resolving a reply.
var ErrReplyDepth = errors.New("replies are one level deep")

const noteColumns = `id, snapshot_id, COALESCE(parent_id, ''), anchor_kind, anchor_id, body,
	COALESCE(author_id, ''), author_name, resolved_at, resolved_by_name, created_at, updated_at`

func scanNote(row pgx.Row) (*model.Note, error) {
	var n model.Note
	err := row.Scan(&n.ID, &n.SnapshotID, &n.ParentID, &n.AnchorKind, &n.AnchorID, &n.Body,
		&n.AuthorID, &n.AuthorName, &n.ResolvedAt, &n.ResolvedByName, &n.CreatedAt, &n.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &n, nil
}

func collectNotes(rows pgx.Rows) ([]model.Note, error) {
	defer rows.Close()
	out := []model.Note{}
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *n)
	}
	return out, rows.Err()
}

// AnchorExists reports whether the anchor names something in the snapshot.
// A malformed column or lineage anchor names nothing, so it is false, not an error.
func (s *Store) AnchorExists(ctx context.Context, sid string, kind notes.Kind, id string) (bool, error) {
	var query string
	args := []any{sid}
	switch kind {
	case notes.KindDomain:
		query, args = `SELECT 1 FROM domains WHERE snapshot_id = $1 AND id = $2`, append(args, id)
	case notes.KindTable:
		query, args = `SELECT 1 FROM tables WHERE snapshot_id = $1 AND id = $2`, append(args, id)
	case notes.KindRelationship:
		query, args = `SELECT 1 FROM relationships WHERE snapshot_id = $1 AND id = $2`, append(args, id)
	case notes.KindColumn, notes.KindLineage:
		table, column, err := notes.SplitColumnAnchor(id)
		if err != nil {
			return false, nil
		}
		query = `SELECT 1 FROM columns WHERE snapshot_id = $1 AND table_id = $2 AND name = $3`
		if kind == notes.KindLineage {
			query = `SELECT 1 FROM column_lineage WHERE snapshot_id = $1 AND table_id = $2 AND column_name = $3`
		}
		args = append(args, table, column)
	default:
		return false, fmt.Errorf("unknown anchor kind %q", kind)
	}
	var exists bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS (`+query+`)`, args...).Scan(&exists)
	return exists, err
}

// CreateNote stores a note. A reply takes its anchor from the parent.
// ErrNotFound for a missing snapshot or a parent in another snapshot.
func (s *Store) CreateNote(ctx context.Context, n model.Note) (*model.Note, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Snapshot locked first, as in CreateConversation, so a concurrent Migrate cannot deadlock us.
	var one int
	if err := tx.QueryRow(ctx,
		`SELECT 1 FROM snapshots WHERE id = $1 FOR KEY SHARE`, n.SnapshotID).Scan(&one); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if n.ParentID != "" {
		var parentSnapshot string
		var isReply bool
		err := tx.QueryRow(ctx,
			`SELECT snapshot_id, parent_id IS NOT NULL, anchor_kind, anchor_id
			   FROM notes WHERE id = $1 FOR KEY SHARE`, n.ParentID).
			Scan(&parentSnapshot, &isReply, &n.AnchorKind, &n.AnchorID)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && parentSnapshot != n.SnapshotID) {
			return nil, ErrNotFound
		}
		if err != nil {
			return nil, err
		}
		if isReply {
			return nil, ErrReplyDepth
		}
	}

	out, err := scanNote(tx.QueryRow(ctx,
		`INSERT INTO notes (id, snapshot_id, parent_id, anchor_kind, anchor_id, body, author_id, author_name)
		 VALUES ($1, $2, NULLIF($3, ''), $4, $5, $6, NULLIF($7, ''), $8)
		 RETURNING `+noteColumns,
		uuid.NewString(), n.SnapshotID, n.ParentID, n.AnchorKind, n.AnchorID, n.Body, n.AuthorID, n.AuthorName))
	if err != nil {
		return nil, fmt.Errorf("insert note: %w", err)
	}
	return out, tx.Commit(ctx)
}

// GetNote returns one note without its replies.
func (s *Store) GetNote(ctx context.Context, id string) (*model.Note, error) {
	return scanNote(s.pool.QueryRow(ctx, `SELECT `+noteColumns+` FROM notes WHERE id = $1`, id))
}

// ListNotes returns top-level notes for an anchor with their replies, oldest first, in two queries.
func (s *Store) ListNotes(ctx context.Context, sid string, kind notes.Kind, anchorID string) ([]model.Note, error) {
	const where = ` FROM notes WHERE snapshot_id = $1 AND anchor_kind = $2 AND anchor_id = $3`
	rows, err := s.pool.Query(ctx,
		`SELECT `+noteColumns+where+` AND parent_id IS NULL ORDER BY created_at, id`, sid, string(kind), anchorID)
	if err != nil {
		return nil, err
	}
	top, err := collectNotes(rows)
	if err != nil || len(top) == 0 {
		return top, err
	}

	// Replies share their parent's anchor, so the same index finds them.
	rows, err = s.pool.Query(ctx,
		`SELECT `+noteColumns+where+` AND parent_id IS NOT NULL ORDER BY created_at, id`, sid, string(kind), anchorID)
	if err != nil {
		return nil, err
	}
	replies, err := collectNotes(rows)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]int, len(top))
	for i := range top {
		byID[top[i].ID] = i
	}
	for _, r := range replies {
		if i, ok := byID[r.ParentID]; ok {
			top[i].Replies = append(top[i].Replies, r)
		}
	}
	return top, nil
}

// CountNotes counts open and resolved top-level notes per anchor.
func (s *Store) CountNotes(ctx context.Context, sid string) ([]model.NoteCount, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT anchor_kind, anchor_id,
		        count(*) FILTER (WHERE resolved_at IS NULL),
		        count(*) FILTER (WHERE resolved_at IS NOT NULL)
		   FROM notes
		  WHERE snapshot_id = $1 AND parent_id IS NULL
		  GROUP BY anchor_kind, anchor_id
		  ORDER BY anchor_kind, anchor_id`, sid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.NoteCount{}
	for rows.Next() {
		var c model.NoteCount
		if err := rows.Scan(&c.AnchorKind, &c.AnchorID, &c.Open, &c.Resolved); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) UpdateNoteBody(ctx context.Context, id, body string) (*model.Note, error) {
	return scanNote(s.pool.QueryRow(ctx,
		`UPDATE notes SET body = $2, updated_at = now() WHERE id = $1 RETURNING `+noteColumns, id, body))
}

// SetNoteResolved resolves or reopens a top-level note. It leaves updated_at alone,
// which tracks body edits. ErrReplyDepth for a reply.
func (s *Store) SetNoteResolved(ctx context.Context, id string, resolved bool, byName string) (*model.Note, error) {
	n, err := scanNote(s.pool.QueryRow(ctx,
		`UPDATE notes
		    SET resolved_at      = CASE WHEN $2 THEN now() END,
		        resolved_by_name = CASE WHEN $2 THEN $3 ELSE '' END
		  WHERE id = $1 AND parent_id IS NULL
		 RETURNING `+noteColumns, id, resolved, byName))
	if !errors.Is(err, ErrNotFound) {
		return n, err
	}
	var isReply bool
	if err := s.pool.QueryRow(ctx,
		`SELECT parent_id IS NOT NULL FROM notes WHERE id = $1`, id).Scan(&isReply); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if isReply {
		return nil, ErrReplyDepth
	}
	return nil, ErrNotFound
}

// DeleteNote removes a note; its replies go by cascade.
func (s *Store) DeleteNote(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM notes WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
