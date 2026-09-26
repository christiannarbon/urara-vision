// Sessions, keyed by the SHA-256 of the token.
package postgres

import (
	"context"
	"time"

	"urara-vision/backend/internal/model"
)

func (s *Store) CreateSession(ctx context.Context, tokenHash, userID string, expiresAt time.Time) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO sessions (token_hash, user_id, expires_at) VALUES ($1, $2, $3)`,
		tokenHash, userID, expiresAt)
	return err
}

// SessionUser returns the user for a live session, joining users so the role is current.
func (s *Store) SessionUser(ctx context.Context, tokenHash string) (*model.User, error) {
	return scanUser(s.pool.QueryRow(ctx,
		`SELECT `+userColumns+`
		   FROM sessions s JOIN users u ON u.id = s.user_id
		  WHERE s.token_hash = $1 AND s.expires_at > now()`, tokenHash))
}

func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1`, tokenHash)
	return err
}

// DeleteUserSessions removes a user's sessions, except keepHash when it is not empty.
func (s *Store) DeleteUserSessions(ctx context.Context, userID, keepHash string) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM sessions WHERE user_id = $1 AND token_hash <> $2`, userID, keepHash)
	return err
}

func (s *Store) DeleteExpiredSessions(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE expires_at <= now()`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
