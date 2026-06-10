package stats

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
)

func newStatsRepoOnTemp(t *testing.T, now time.Time) (*store.Store, *Repo, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	proj, err := project.NewRepo(s).Create(context.Background(), now.UnixMilli(), project.NewInput{
		Title: "Stats", Genres: []string{"SF"}, LengthTarget: "series", DefaultPOV: "first",
	})
	if err != nil {
		t.Fatalf("project.Create: %v", err)
	}
	repo := NewRepoWithClock(s, time.UTC, func() time.Time { return now })
	return s, repo, proj.ID
}

func seedStat(t *testing.T, s *store.Store, projectID string, day string, chars int) {
	t.Helper()
	if _, err := s.DB().ExecContext(context.Background(), `
INSERT INTO writing_stats (project_id, day, chars_added)
VALUES (?, ?, ?)`, projectID, day, chars); err != nil {
		t.Fatalf("seed stat: %v", err)
	}
}

func TestRepo_Range_includesBlankDays(t *testing.T) {
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	s, repo, projectID := newStatsRepoOnTemp(t, now)
	seedStat(t, s, projectID, "2026-06-08", 1200)
	seedStat(t, s, projectID, "2026-06-10", 800)

	got, err := repo.Range(context.Background(), projectID, "2026-06-08", "2026-06-10")
	if err != nil {
		t.Fatalf("Range: %v", err)
	}
	want := []DayStat{
		{Day: "2026-06-08", CharsAdded: 1200},
		{Day: "2026-06-09", CharsAdded: 0},
		{Day: "2026-06-10", CharsAdded: 800},
	}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestRepo_Range_emptyWhenFromAfterTo(t *testing.T) {
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	_, repo, projectID := newStatsRepoOnTemp(t, now)

	got, err := repo.Range(context.Background(), projectID, "2026-06-11", "2026-06-10")
	if err != nil {
		t.Fatalf("Range: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d rows, want empty", len(got))
	}
}

func TestRepo_Summary_recentSevenDayAverageAndTotalDays(t *testing.T) {
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	s, repo, projectID := newStatsRepoOnTemp(t, now)
	seedStat(t, s, projectID, "2026-06-01", 1000)
	seedStat(t, s, projectID, "2026-06-04", 140)
	seedStat(t, s, projectID, "2026-06-10", 700)

	got, err := repo.Summary(context.Background(), projectID)
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if got.Today != 700 {
		t.Fatalf("today = %d, want 700", got.Today)
	}
	if got.WeekAvg != 120 {
		t.Fatalf("week_avg = %d, want 120", got.WeekAvg)
	}
	if got.TotalDays != 3 {
		t.Fatalf("total_days = %d, want 3", got.TotalDays)
	}
}
