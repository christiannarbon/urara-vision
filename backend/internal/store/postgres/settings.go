package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// SettingChatEnabled is the key for the runtime chat switch.
const SettingChatEnabled = "chat.enabled"

// GetBoolSetting returns def when the key is unset. A non-boolean value is an
// error rather than def, so a bad row is noticed instead of silently ignored.
func (s *Store) GetBoolSetting(ctx context.Context, key string, def bool) (bool, error) {
	var raw []byte
	err := s.pool.QueryRow(ctx, `SELECT value FROM app_settings WHERE key = $1`, key).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return def, nil
	}
	if err != nil {
		return false, fmt.Errorf("get setting %q: %w", key, err)
	}
	// Decoded via any: unmarshalling null into a bool succeeds as false.
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return false, fmt.Errorf("decode setting %q: %w", key, err)
	}
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("setting %q is not a boolean: %s", key, raw)
	}
	return b, nil
}

// SetBoolSetting stores v under key, replacing any existing value.
func (s *Store) SetBoolSetting(ctx context.Context, key string, v bool) error {
	raw, _ := json.Marshal(v)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO app_settings (key, value) VALUES ($1, $2)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`,
		key, raw)
	if err != nil {
		return fmt.Errorf("set setting %q: %w", key, err)
	}
	return nil
}
