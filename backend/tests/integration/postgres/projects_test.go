//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"urara-vision/backend/internal/model"
	"urara-vision/backend/internal/projectmeta"
	"urara-vision/backend/internal/store/postgres"
	"urara-vision/backend/tests/fixtures"
	"urara-vision/backend/tests/integration/harness"
)

// savedUnder saves the star-schema fixture under its own project name, so a
// delete here cannot reach another test's snapshots, and removes the project
// when the test ends.
func savedUnder(t *testing.T, ctx context.Context, pg *postgres.Store, projectName, version string, createdAt time.Time) *model.Model {
	t.Helper()
	m := fixtures.BuildAs(harness.SnapshotID(), fixtures.StarSchema())
	m.Snapshot.Project.Project.Name = projectName
	m.Snapshot.Project.Project.Version = version
	m.Snapshot.Project.Project.Description = "about " + version
	m.Snapshot.CreatedAt = createdAt
	if err := pg.SaveSnapshot(ctx, m); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}
	t.Cleanup(func() {
		_, err := pg.DeleteProject(context.Background(), m.Snapshot.ProjectSlug)
		if err != nil && !errors.Is(err, postgres.ErrNotFound) {
			t.Errorf("cleanup: delete project %s: %v", m.Snapshot.ProjectSlug, err)
		}
	})
	return m
}

func uniqueProject() string { return "Test Project " + uuid.NewString()[:8] }

// past dates these snapshots so they never become another test's "latest".
func past(offset time.Duration) time.Time {
	return time.Now().Add(-24*time.Hour + offset).UTC().Truncate(time.Microsecond)
}

func TestSaveCreatesTheProject(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)
	name := uniqueProject()

	m := savedUnder(t, ctx, pg, name, "1.0.0", past(0))

	if m.Snapshot.ProjectSlug != projectmeta.Slug(name) || m.Snapshot.ProjectID == "" {
		t.Errorf("model project = %q / %q", m.Snapshot.ProjectID, m.Snapshot.ProjectSlug)
	}
	sn, err := pg.GetSnapshot(ctx, m.Snapshot.ID)
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}
	if sn.ProjectSlug != projectmeta.Slug(name) || sn.ProjectID != m.Snapshot.ProjectID {
		t.Errorf("GetSnapshot project = %q / %q, want %q / %q",
			sn.ProjectID, sn.ProjectSlug, m.Snapshot.ProjectID, projectmeta.Slug(name))
	}
}

func TestSaveReusesTheProject(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)
	name := uniqueProject()
	older := past(0)
	newer := past(30 * time.Minute)

	first := savedUnder(t, ctx, pg, name, "1.0.0", older)
	// Differently cased, same slug.
	second := savedUnder(t, ctx, pg, "test project "+name[len("Test Project "):], "2.0.0", newer)

	if first.Snapshot.ProjectID != second.Snapshot.ProjectID {
		t.Fatalf("saves landed in projects %s and %s, want one", first.Snapshot.ProjectID, second.Snapshot.ProjectID)
	}
	p, err := pg.GetProject(ctx, first.Snapshot.ProjectSlug)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if p.Name != second.Snapshot.Project.Project.Name || p.Description != "about 2.0.0" {
		t.Errorf("project = %q / %q, want the second save's name and description", p.Name, p.Description)
	}
	if p.VersionCount != 2 {
		t.Errorf("VersionCount = %d, want 2", p.VersionCount)
	}
	if p.Latest == nil || p.Latest.SnapshotID != second.Snapshot.ID || p.Latest.Version != "2.0.0" ||
		!p.Latest.CreatedAt.Equal(newer) {
		t.Errorf("Latest = %+v, want snapshot %s, version 2.0.0 at %s", p.Latest, second.Snapshot.ID, newer)
	}
}

func TestSaveWithoutAProjectNameIsLegacy(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)

	m := savedUnder(t, ctx, pg, "", "1.0.0", past(0))

	if want := projectmeta.LegacySlug(m.Snapshot.ID); m.Snapshot.ProjectSlug != want {
		t.Errorf("slug = %q, want %q", m.Snapshot.ProjectSlug, want)
	}
	p, err := pg.GetProject(ctx, m.Snapshot.ProjectSlug)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if p.Name != m.Snapshot.Name {
		t.Errorf("name = %q, want the snapshot's name %q", p.Name, m.Snapshot.Name)
	}
}

func TestListProjectsByUpdatedAt(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)
	a, b := uniqueProject(), uniqueProject()

	savedUnder(t, ctx, pg, a, "1", past(0))
	savedUnder(t, ctx, pg, b, "1", past(0))
	// A new version moves a back to the top.
	aSlug := savedUnder(t, ctx, pg, a, "2", past(time.Second)).Snapshot.ProjectSlug
	bSlug := projectmeta.Slug(b)

	projects, err := pg.ListProjects(ctx)
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	at := map[string]int{}
	for i, p := range projects {
		at[p.Slug] = i
		if i > 0 && p.UpdatedAt.After(projects[i-1].UpdatedAt) {
			t.Errorf("%s is listed after an older project", p.Slug)
		}
	}
	ai, aok := at[aSlug]
	bi, bok := at[bSlug]
	if !aok || !bok || ai > bi {
		t.Errorf("positions a=%d (%v) b=%d (%v), want a before b", ai, aok, bi, bok)
	}
	for _, p := range projects {
		if p.Slug == aSlug && p.VersionCount != 2 {
			t.Errorf("a VersionCount = %d, want 2", p.VersionCount)
		}
	}
}

func TestDeleteProjectRemovesEverything(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)
	name := uniqueProject()

	first := savedUnder(t, ctx, pg, name, "1", past(0))
	second := savedUnder(t, ctx, pg, name, "2", past(time.Minute))
	conv, err := pg.CreateConversation(ctx, first.Snapshot.ID, "about the project")
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}

	ids, err := pg.DeleteProject(ctx, first.Snapshot.ProjectSlug)
	if err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}
	got := map[string]bool{}
	for _, id := range ids {
		got[id] = true
	}
	if len(ids) != 2 || !got[first.Snapshot.ID] || !got[second.Snapshot.ID] {
		t.Errorf("DeleteProject returned %v, want both snapshot IDs", ids)
	}

	for _, m := range []*model.Model{first, second} {
		sid := m.Snapshot.ID
		if _, err := pg.GetSnapshot(ctx, sid); !errors.Is(err, postgres.ErrNotFound) {
			t.Errorf("GetSnapshot(%s) = %v, want ErrNotFound", sid, err)
		}
		if _, err := pg.GetTable(ctx, sid, m.Tables[0].ID); !errors.Is(err, postgres.ErrNotFound) {
			t.Errorf("GetTable(%s) = %v, want ErrNotFound", sid, err)
		}
	}
	if _, err := pg.GetConversation(ctx, conv.ID); !errors.Is(err, postgres.ErrNotFound) {
		t.Errorf("GetConversation = %v, want ErrNotFound", err)
	}
	if _, err := pg.GetProject(ctx, first.Snapshot.ProjectSlug); !errors.Is(err, postgres.ErrNotFound) {
		t.Errorf("GetProject after delete = %v, want ErrNotFound", err)
	}
}

func TestDeleteUnknownProject(t *testing.T) {
	ctx := harness.Context(t)
	pg := harness.Postgres(t)
	if _, err := pg.DeleteProject(ctx, "no-such-project-"+uuid.NewString()[:8]); !errors.Is(err, postgres.ErrNotFound) {
		t.Errorf("DeleteProject = %v, want ErrNotFound", err)
	}
	if _, err := pg.GetProject(ctx, "no-such-project"); !errors.Is(err, postgres.ErrNotFound) {
		t.Errorf("GetProject = %v, want ErrNotFound", err)
	}
}
