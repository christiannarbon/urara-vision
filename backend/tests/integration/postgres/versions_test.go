//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"urara-vision/backend/internal/model"
	"urara-vision/backend/internal/projectmeta"
	"urara-vision/backend/internal/store/postgres"
	"urara-vision/backend/tests/fixtures"
	"urara-vision/backend/tests/integration/harness"
)

func versionsOf(sns []model.Snapshot) []string {
	out := make([]string, len(sns))
	for i, sn := range sns {
		out[i] = sn.Project.Project.Version
	}
	return out
}

func TestSaveSameVersionConflicts(t *testing.T) {
	t.Parallel()
	ctx := harness.Context(t)
	pg := harness.Postgres(t)
	name := uniqueProject()
	savedUnder(t, ctx, pg, name, "1.0.0", past(0))

	m := fixtures.BuildAs(harness.SnapshotID(), fixtures.StarSchema())
	m.Snapshot.Project.Project.Name = name
	m.Snapshot.Project.Project.Version = "1.0.0"
	if err := pg.SaveSnapshot(ctx, m); !errors.Is(err, postgres.ErrConflict) {
		t.Fatalf("second save err = %v, want ErrConflict", err)
	}
}

// Not parallel: the unique index is missing for everyone until Migrate runs.
func TestMigrateRenamesDuplicateVersions(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)
	slug := projectmeta.Slug(uniqueProject())
	projectID := uuid.NewString()
	// Oldest first. The unlabelled one is newest, so it keeps "legacy".
	rows := []string{"v", "v", "v", "legacy", ""}
	t.Cleanup(func() {
		execSQL(t, `DELETE FROM projects WHERE id = $1`, projectID)
		// Put the index back if the test failed before Migrate did.
		_ = pg.Migrate(context.Background())
	})

	conn, err := pgx.Connect(ctx, harness.PostgresDSN(t))
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	defer func() { _ = conn.Close(context.Background()) }()
	// One transaction, so a concurrent Migrate cannot restore the index mid-insert.
	err = pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DROP INDEX snapshots_project_version_key`); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO projects (id, slug, name) VALUES ($1, $2, $2)`,
			projectID, slug); err != nil {
			return err
		}
		for i, v := range rows {
			if _, err := tx.Exec(ctx,
				`INSERT INTO snapshots (id, name, created_at, project_version, project_id)
				 VALUES ($1, 'dup', $2, $3, $4)`,
				harness.SnapshotID(), past(time.Duration(i)*time.Minute), v, projectID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("insert duplicates: %v", err)
	}

	if err := pg.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	want := []string{"legacy", "legacy+legacy.1", "v", "v+legacy.1", "v+legacy.2"}
	sns, err := pg.ListVersions(ctx, slug)
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if got := versionsOf(sns); !reflect.DeepEqual(got, want) {
		t.Fatalf("versions = %v, want %v", got, want)
	}

	if err := pg.Migrate(ctx); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	if sns, err = pg.ListVersions(ctx, slug); err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if got := versionsOf(sns); !reflect.DeepEqual(got, want) {
		t.Errorf("versions after re-run = %v, want %v", got, want)
	}
}

func TestListVersionsNewestFirst(t *testing.T) {
	t.Parallel()
	ctx := harness.Context(t)
	pg := harness.Postgres(t)
	name := uniqueProject()
	m := savedUnder(t, ctx, pg, name, "1.0.0", past(0))
	savedUnder(t, ctx, pg, name, "3.0.0", past(2*time.Minute))
	savedUnder(t, ctx, pg, name, "2.0.0", past(time.Minute))

	sns, err := pg.ListVersions(ctx, m.Snapshot.ProjectSlug)
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if got, want := versionsOf(sns), []string{"3.0.0", "2.0.0", "1.0.0"}; !reflect.DeepEqual(got, want) {
		t.Errorf("versions = %v, want %v", got, want)
	}

	if _, err := pg.ListVersions(ctx, "no-such-project-"+uuid.NewString()); !errors.Is(err, postgres.ErrNotFound) {
		t.Errorf("unknown project err = %v, want ErrNotFound", err)
	}
}

func TestGetVersion(t *testing.T) {
	t.Parallel()
	ctx := harness.Context(t)
	pg := harness.Postgres(t)
	name := uniqueProject()
	savedUnder(t, ctx, pg, name, "1.0.0+build.7", past(0))
	newest := savedUnder(t, ctx, pg, name, "2024 Q1", past(time.Minute))
	slug := newest.Snapshot.ProjectSlug

	sn, err := pg.GetVersion(ctx, slug, "latest")
	if err != nil {
		t.Fatalf("GetVersion(latest): %v", err)
	}
	if sn.ID != newest.Snapshot.ID {
		t.Errorf("latest = %s, want %s", sn.ID, newest.Snapshot.ID)
	}

	for _, v := range []string{"1.0.0+build.7", "2024 Q1"} {
		sn, err := pg.GetVersion(ctx, slug, v)
		if err != nil {
			t.Fatalf("GetVersion(%q): %v", v, err)
		}
		if sn.Project.Project.Version != v || sn.ProjectSlug != slug {
			t.Errorf("GetVersion(%q) = %q in %q", v, sn.Project.Project.Version, sn.ProjectSlug)
		}
	}

	if _, err := pg.GetVersion(ctx, slug, "9.9.9"); !errors.Is(err, postgres.ErrNotFound) {
		t.Errorf("unknown version err = %v, want ErrNotFound", err)
	}
	if _, err := pg.GetVersion(ctx, "no-such-project-"+uuid.NewString(), "latest"); !errors.Is(err, postgres.ErrNotFound) {
		t.Errorf("unknown project err = %v, want ErrNotFound", err)
	}
}

func TestDeleteVersion(t *testing.T) {
	t.Parallel()
	ctx := harness.Context(t)
	pg := harness.Postgres(t)
	name := uniqueProject()
	first := savedUnder(t, ctx, pg, name, "1.0.0", past(0))
	second := savedUnder(t, ctx, pg, name, "2.0.0", past(time.Minute))
	slug := first.Snapshot.ProjectSlug

	sid, projectDeleted, err := pg.DeleteVersion(ctx, slug, "1.0.0")
	if err != nil {
		t.Fatalf("DeleteVersion(1.0.0): %v", err)
	}
	if sid != first.Snapshot.ID || projectDeleted {
		t.Errorf("DeleteVersion(1.0.0) = %s, %v; want %s, false", sid, projectDeleted, first.Snapshot.ID)
	}
	if _, _, err := pg.DeleteVersion(ctx, slug, "1.0.0"); !errors.Is(err, postgres.ErrNotFound) {
		t.Errorf("deleting it again err = %v, want ErrNotFound", err)
	}

	sid, projectDeleted, err = pg.DeleteVersion(ctx, slug, "2.0.0")
	if err != nil {
		t.Fatalf("DeleteVersion(2.0.0): %v", err)
	}
	if sid != second.Snapshot.ID || !projectDeleted {
		t.Errorf("DeleteVersion(2.0.0) = %s, %v; want %s, true", sid, projectDeleted, second.Snapshot.ID)
	}
	if _, err := pg.GetProject(ctx, slug); !errors.Is(err, postgres.ErrNotFound) {
		t.Errorf("GetProject after last delete err = %v, want ErrNotFound", err)
	}
}
