//go:build integration

// Loading a whole snapshot back as one model.
package postgres_test

import (
	"errors"
	"reflect"
	"testing"

	"urara-vision/backend/internal/store/postgres"
	"urara-vision/backend/tests/integration/harness"
)

func TestLoadModelHasEveryTable(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)
	m := harness.SavedModel(t, ctx, pg)

	got, err := pg.LoadModel(ctx, m.Snapshot.ID)
	if err != nil {
		t.Fatalf("LoadModel: %v", err)
	}
	if got.Snapshot.ID != m.Snapshot.ID {
		t.Errorf("snapshot = %q, want %q", got.Snapshot.ID, m.Snapshot.ID)
	}
	if len(got.Domains) != len(m.Domains) {
		t.Errorf("domains = %d, want %d", len(got.Domains), len(m.Domains))
	}
	if len(got.Tables) != len(m.Tables) {
		t.Fatalf("tables = %d, want %d", len(got.Tables), len(m.Tables))
	}

	loaded := map[string]int{}
	for i, tb := range got.Tables {
		loaded[tb.ID] = i
	}
	for _, want := range m.Tables {
		i, ok := loaded[want.ID]
		if !ok {
			t.Errorf("%s missing", want.ID)
			continue
		}
		tb := got.Tables[i]
		if len(tb.Columns) != len(want.Columns) ||
			len(tb.ColumnLineage) != len(want.ColumnLineage) ||
			len(tb.Relationships) != len(want.Relationships) {
			t.Errorf("%s: columns/lineage/relationships = %d/%d/%d, want %d/%d/%d", want.ID,
				len(tb.Columns), len(tb.ColumnLineage), len(tb.Relationships),
				len(want.Columns), len(want.ColumnLineage), len(want.Relationships))
		}
	}
}

func TestLoadModelMatchesGetTable(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)
	m := harness.SavedModel(t, ctx, pg)

	got, err := pg.LoadModel(ctx, m.Snapshot.ID)
	if err != nil {
		t.Fatalf("LoadModel: %v", err)
	}
	if len(got.Tables) < 3 {
		t.Fatalf("fixture has %d tables, want at least 3", len(got.Tables))
	}
	for _, tb := range got.Tables {
		want, err := pg.GetTable(ctx, m.Snapshot.ID, tb.ID)
		if err != nil {
			t.Fatalf("GetTable %s: %v", tb.ID, err)
		}
		if !reflect.DeepEqual(tb, *want) {
			t.Errorf("%s:\nLoadModel %+v\nGetTable  %+v", tb.ID, tb, *want)
		}
	}
}

func TestLoadModelUnknownSnapshot(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)

	if _, err := pg.LoadModel(ctx, harness.SnapshotID()); !errors.Is(err, postgres.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}
