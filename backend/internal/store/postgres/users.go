// Users and their login identities. Hashes are computed by internal/auth.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"urara-vision/backend/internal/model"
)

const userColumns = `u.id, u.username, u.display_name, u.role, u.created_at, u.updated_at`

func scanUser(row pgx.Row, extra ...any) (*model.User, error) {
	var u model.User
	dest := append([]any{&u.ID, &u.Username, &u.DisplayName, &u.Role, &u.CreatedAt, &u.UpdatedAt}, extra...)
	if err := row.Scan(dest...); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

// CreatePasswordUser inserts the user and its password identity in one transaction.
func (s *Store) CreatePasswordUser(ctx context.Context, username, displayName, role, passwordHash string) (*model.User, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	u, err := createPasswordUser(ctx, tx, username, displayName, role, passwordHash)
	if err != nil {
		return nil, err
	}
	return u, tx.Commit(ctx)
}

func createPasswordUser(ctx context.Context, tx pgx.Tx, username, displayName, role, passwordHash string) (*model.User, error) {
	u, err := scanUser(tx.QueryRow(ctx,
		`INSERT INTO users AS u (id, username, display_name, role)
		 VALUES ($1, $2, $3, $4)
		 RETURNING `+userColumns,
		uuid.NewString(), username, displayName, role))
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrConflict
		}
		return nil, fmt.Errorf("insert user: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO user_identities (id, user_id, provider, subject, password_hash)
		 VALUES ($1, $2, 'password', $3, $4)`,
		uuid.NewString(), u.ID, username, passwordHash); err != nil {
		if isUniqueViolation(err) {
			return nil, ErrConflict
		}
		return nil, fmt.Errorf("insert identity: %w", err)
	}
	return u, nil
}

func (s *Store) GetUser(ctx context.Context, id string) (*model.User, error) {
	return scanUser(s.pool.QueryRow(ctx, `SELECT `+userColumns+` FROM users u WHERE u.id = $1`, id))
}

// PasswordIdentity returns the user and hash for a password login.
func (s *Store) PasswordIdentity(ctx context.Context, username string) (*model.User, string, error) {
	var hash string
	u, err := scanUser(s.pool.QueryRow(ctx,
		`SELECT `+userColumns+`, i.password_hash
		   FROM user_identities i JOIN users u ON u.id = i.user_id
		  WHERE i.provider = 'password' AND i.subject = $1`, username), &hash)
	if err != nil {
		return nil, "", err
	}
	return u, hash, nil
}

func (s *Store) SetPasswordHash(ctx context.Context, userID, hash string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE user_identities SET password_hash = $2
		  WHERE user_id = $1 AND provider = 'password'`, userID, hash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&n)
	return n, err
}

// bootstrapLock is distinct from migrateLock so bootstrap never waits on a migration.
const bootstrapLock int64 = 0x75726175 // "urau"

// BootstrapAdmin creates the first admin when there are no users. It reports whether it created one.
// The lock serialises replicas starting together, so only one creates the admin.
func (s *Store) BootstrapAdmin(ctx context.Context, username, passwordHash string) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, bootstrapLock); err != nil {
		return false, fmt.Errorf("take bootstrap lock: %w", err)
	}
	var n int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&n); err != nil {
		return false, err
	}
	if n > 0 {
		return false, nil
	}
	if _, err := createPasswordUser(ctx, tx, username, "", "admin", passwordHash); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}
