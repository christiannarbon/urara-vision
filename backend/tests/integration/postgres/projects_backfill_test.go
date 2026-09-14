//go:build integration

package postgres_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"urara-vision/backend/internal/projectmeta"
	"urara-vision/backend/internal/store/postgres"
	"urara-vision/backend/tests/integration/harness"
)

// backfill inserts snapshots with no project and cleans up whatever the
// backfill attaches to them. Other packages migrate the same database
// concurrently, so assertions only look at these rows.
type backfill struct {
	t   *testing.T
	ctx context.Context
	pg  *postgres.Store
	ids []string
}

func newBackfill(t *testing.T) *backfill {
	b := &backfill{t: t, ctx: harness.Context(t), pg: harness.Postgres(t)}
	t.Cleanup(func() {
		execSQL(t, `DELETE FROM projects WHERE id IN (SELECT project_id FROM snapshots WHERE id = ANY($1))`, b.ids)
		execSQL(t, `DELETE FROM snapshots WHERE id = ANY($1)`, b.ids)
	})
	return b
}

func (b *backfill) snapshot(name, projectName string, createdAt time.Time) string {
	b.t.Helper()
	id := harness.SnapshotID()
	b.ids = append(b.ids, id)
	execSQL(b.t, `INSERT INTO snapshots (id, name, project_name, project_description, created_at)
		VALUES ($1, $2, $3, $4, $5)`, id, name, projectName, "about "+name, createdAt)
	return id
}

func (b *backfill) migrate() {
	b.t.Helper()
	if err := b.pg.Migrate(b.ctx); err != nil {
		b.t.Fatalf("Migrate: %v", err)
	}
}

type backfilledProject struct {
	id, slug, name, description string
	createdAt, updatedAt        time.Time
}

// projectOf reads the project a snapshot was attached to.
func (b *backfill) projectOf(snapshotID string) backfilledProject {
	b.t.Helper()
	var p backfilledProject
	queryRow(b.t, `SELECT p.id, p.slug, p.name, p.description, p.created_at, p.updated_at
		FROM snapshots s JOIN projects p ON p.id = s.project_id WHERE s.id = $1`,
		[]any{snapshotID}, &p.id, &p.slug, &p.name, &p.description, &p.createdAt, &p.updatedAt)
	return p
}

// counts is how many of this test's snapshots are orphaned, and how many
// distinct projects they belong to.
func (b *backfill) counts() (orphans, projects int) {
	b.t.Helper()
	queryRow(b.t, `SELECT count(*) FILTER (WHERE project_id IS NULL), count(DISTINCT project_id)
		FROM snapshots WHERE id = ANY($1)`, []any{b.ids}, &orphans, &projects)
	return orphans, projects
}

func queryRow(t *testing.T, sql string, args []any, dest ...any) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, harness.PostgresDSN(t))
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	defer func() { _ = conn.Close(ctx) }()
	if err := conn.QueryRow(ctx, sql, args...).Scan(dest...); err != nil {
		t.Fatalf("query: %v", err)
	}
}

func TestBackfillGroupsSnapshotsByName(t *testing.T) {
	b := newBackfill(t)
	suffix := uuid.NewString()[:8]
	older := time.Now().Add(-2 * time.Hour).UTC().Truncate(time.Microsecond)
	newer := older.Add(time.Hour)

	first := b.snapshot("old ingest", "Test Backfill "+suffix, older)
	second := b.snapshot("new ingest", "test backfill "+suffix, newer)
	b.migrate()

	p := b.projectOf(first)
	if got := b.projectOf(second).id; got != p.id {
		t.Fatalf("snapshots landed in projects %s and %s, want one", p.id, got)
	}
	if want := projectmeta.Slug("Test Backfill " + suffix); p.slug != want {
		t.Errorf("slug = %q, want %q", p.slug, want)
	}
	if p.name != "test backfill "+suffix || p.description != "about new ingest" {
		t.Errorf("project = %q / %q, want the newer snapshot's name and description", p.name, p.description)
	}
	if !p.createdAt.Equal(older) || !p.updatedAt.Equal(newer) {
		t.Errorf("created/updated = %s / %s, want %s / %s", p.createdAt, p.updatedAt, older, newer)
	}
}

func TestBackfillGivesALegacySnapshotItsOwnProject(t *testing.T) {
	b := newBackfill(t)
	id := b.snapshot("pre-manifest ingest", "", time.Now())
	nameless := b.snapshot("unslugged ingest", "日本語", time.Now())
	b.migrate()

	p := b.projectOf(id)
	if want := projectmeta.LegacySlug(id); p.slug != want {
		t.Errorf("slug = %q, want %q", p.slug, want)
	}
	if p.name != "pre-manifest ingest" {
		t.Errorf("name = %q, want the snapshot's name", p.name)
	}
	if got := b.projectOf(nameless).slug; got != projectmeta.LegacySlug(nameless) {
		t.Errorf("a name with an empty slug got %q, want its legacy slug", got)
	}
}

func TestBackfillSlugMatchesGo(t *testing.T) {
	b := newBackfill(t)
	// 10.1's slug table, each tagged so it cannot share a project with real
	// data. The tag is 8 hex characters, and the long cases are shortened by
	// that much so the 64-byte cut lands where 10.1's table has it.
	tag := uuid.NewString()[:8]
	names := []string{
		"jaffle-shop-ddd " + tag,
		"Jaffle Shop " + tag,
		"  --Fintech__BI-- " + tag + "  ",
		"a..b " + tag,
		"日本語",
		"Café 2 " + tag,
		tag + strings.Repeat("a", 62),
		tag + strings.Repeat("a", 55) + " b",
	}
	ids := map[string]string{}
	for _, name := range names {
		ids[name] = b.snapshot("slug case", name, time.Now())
	}
	b.migrate()

	for name, id := range ids {
		want := projectmeta.Slug(name)
		if want == "" {
			continue
		}
		if got := b.projectOf(id).slug; got != want {
			t.Errorf("SQL slug of %q = %q, Go slug = %q", name, got, want)
		}
	}
	if got, want := projectmeta.Slug(names[7]), tag+strings.Repeat("a", 55); got != want {
		t.Errorf("the cut case no longer lands on a dash: %q", got)
	}
}

func TestBackfillDoesNothingOnReRun(t *testing.T) {
	b := newBackfill(t)
	suffix := uuid.NewString()[:8]
	b.snapshot("one", "Rerun "+suffix, time.Now().Add(-time.Minute))
	b.snapshot("two", "Rerun "+suffix, time.Now())
	b.snapshot("legacy", "", time.Now())
	b.migrate()

	orphans, projects := b.counts()
	if orphans != 0 || projects != 2 {
		t.Fatalf("after backfill: %d orphans, %d projects, want 0 and 2", orphans, projects)
	}
	var before int
	queryRow(t, `SELECT count(*) FROM projects`, nil, &before)

	b.migrate()

	if o, p := b.counts(); o != orphans || p != projects {
		t.Errorf("re-run changed this test's rows: %d orphans, %d projects", o, p)
	}
	var after int
	queryRow(t, `SELECT count(*) FROM projects`, nil, &after)
	// Other packages may add snapshots between the two runs, never remove projects.
	if after < before {
		t.Errorf("projects went from %d to %d on a re-run", before, after)
	}
}
