//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"urara-vision/backend/tests/integration/harness"
)

// settingKey returns a key no other test uses and deletes its row afterwards.
func settingKey(t *testing.T) string {
	t.Helper()
	key := "test." + uuid.NewString()
	t.Cleanup(func() { execSQL(t, `DELETE FROM app_settings WHERE key = $1`, key) })
	return key
}

func execSQL(t *testing.T, sql string, args ...any) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, harness.PostgresDSN(t))
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	defer func() { _ = conn.Close(ctx) }()
	if _, err := conn.Exec(ctx, sql, args...); err != nil {
		t.Fatalf("exec: %v", err)
	}
}

func TestBoolSettingMissingReturnsDefault(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)
	key := settingKey(t)

	for _, def := range []bool{true, false} {
		got, err := pg.GetBoolSetting(ctx, key, def)
		if err != nil {
			t.Fatalf("GetBoolSetting: %v", err)
		}
		if got != def {
			t.Errorf("GetBoolSetting(def=%v) = %v", def, got)
		}
	}
}

func TestBoolSettingUpsert(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)
	key := settingKey(t)

	for _, want := range []bool{false, true} {
		if err := pg.SetBoolSetting(ctx, key, want); err != nil {
			t.Fatalf("SetBoolSetting(%v): %v", want, err)
		}
		// The default is the opposite, so a missed write cannot pass.
		got, err := pg.GetBoolSetting(ctx, key, !want)
		if err != nil {
			t.Fatalf("GetBoolSetting: %v", err)
		}
		if got != want {
			t.Errorf("after set %v, got %v", want, got)
		}
	}
}

func TestBoolSettingRejectsNonBoolean(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)

	for _, raw := range []string{`"yes"`, `1`, `null`, `{"on": true}`} {
		key := settingKey(t)
		execSQL(t, `INSERT INTO app_settings (key, value) VALUES ($1, $2::jsonb)`, key, raw)
		if _, err := pg.GetBoolSetting(ctx, key, true); err == nil {
			t.Errorf("GetBoolSetting on %s: want an error", raw)
		}
	}
}
