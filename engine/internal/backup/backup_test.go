package backup

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
)

func openSeededStore(t *testing.T) (*store.Store, string) {
	t.Helper()
	home := t.TempDir()
	dbPath := filepath.Join(home, "library.db")
	s, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	pr := project.NewRepo(s)
	if _, err := pr.Create(context.Background(), 1, project.NewInput{
		Title: "T", Genres: []string{"SF"}, LengthTarget: "short", DefaultPOV: "first",
	}); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	return s, home
}

func TestRunDailyIfNeeded_createsBackupFileFirstTime(t *testing.T) {
	s, home := openSeededStore(t)
	now := time.Date(2026, 5, 26, 9, 15, 30, 0, time.UTC)
	path, did, err := RunDailyIfNeeded(context.Background(), s.DB(), home, now)
	if err != nil {
		t.Fatalf("RunDaily: %v", err)
	}
	if !did {
		t.Error("did=false on first run")
	}
	if filepath.Base(filepath.Dir(path)) != "2026-05-26" {
		t.Errorf("dir = %s, want 2026-05-26", filepath.Dir(path))
	}
	if filepath.Base(path) != "library-091530.db" {
		t.Errorf("file = %s, want library-091530.db", filepath.Base(path))
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Size() == 0 {
		t.Error("backup is empty")
	}
}

func TestRunDailyIfNeeded_skipsWhenTodayDirExists(t *testing.T) {
	s, home := openSeededStore(t)
	now := time.Date(2026, 5, 26, 9, 0, 0, 0, time.UTC)
	if _, _, err := RunDailyIfNeeded(context.Background(), s.DB(), home, now); err != nil {
		t.Fatalf("first: %v", err)
	}
	// Same day, later time.
	later := time.Date(2026, 5, 26, 18, 0, 0, 0, time.UTC)
	_, did, err := RunDailyIfNeeded(context.Background(), s.DB(), home, later)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if did {
		t.Error("did=true on second same-day run")
	}
}

func TestPrune_removesDirsOlderThan14Days(t *testing.T) {
	_, home := openSeededStore(t)
	root := filepath.Join(home, "backups")
	for _, d := range []string{"2026-05-26", "2026-05-12", "2026-05-11", "not-a-date"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o700); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	now := time.Date(2026, 5, 26, 0, 0, 0, 0, time.UTC)
	if err := Prune(home, now); err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "2026-05-26")); err != nil {
		t.Error("today removed")
	}
	if _, err := os.Stat(filepath.Join(root, "2026-05-12")); err != nil {
		t.Error("14-day window inner edge removed")
	}
	if _, err := os.Stat(filepath.Join(root, "2026-05-11")); !os.IsNotExist(err) {
		t.Error("15-day-old dir not removed")
	}
	if _, err := os.Stat(filepath.Join(root, "not-a-date")); err != nil {
		t.Error("non-date dir incorrectly removed")
	}
}

func TestStart_runsImmediatelyAndCallsRetention(t *testing.T) {
	s, home := openSeededStore(t)
	calls := make(chan struct{}, 4)
	retentionRan := make(chan struct{}, 4)
	retention := func(ctx context.Context) error {
		select {
		case retentionRan <- struct{}{}:
		default:
		}
		return nil
	}
	onTick := func(result TickResult) {
		if !result.OK() {
			t.Errorf("unexpected tick error: %+v", result)
		}
		if result.StartedAt == 0 || result.FinishedAt == 0 {
			t.Errorf("tick timestamps not populated: %+v", result)
		}
		select {
		case calls <- struct{}{}:
		default:
		}
	}
	sleepFn := func(d time.Duration) { time.Sleep(10 * time.Millisecond) }
	stop := Start(context.Background(), s.DB(), home, retention,
		func() time.Time { return time.Date(2026, 5, 26, 23, 59, 59, 0, time.Local) },
		sleepFn,
		onTick)
	defer stop()

	select {
	case <-calls:
	case <-time.After(2 * time.Second):
		t.Fatal("immediate run did not invoke onTick")
	}
	select {
	case <-retentionRan:
	case <-time.After(2 * time.Second):
		t.Fatal("retention callback not invoked")
	}
	// Confirm backup file actually landed.
	root := filepath.Join(home, "backups", "2026-05-26")
	matches, _ := filepath.Glob(filepath.Join(root, "library-*.db"))
	if len(matches) != 1 {
		t.Errorf("expected 1 backup file, got %d", len(matches))
	}
}

func TestStartReportsBackupErrorsToOnTick(t *testing.T) {
	s, home := openSeededStore(t)
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	calls := make(chan TickResult, 1)
	stop := Start(context.Background(), s.DB(), home, nil,
		func() time.Time { return time.Date(2026, 5, 26, 9, 0, 0, 0, time.Local) },
		func(time.Duration) { time.Sleep(10 * time.Millisecond) },
		func(result TickResult) { calls <- result })
	defer stop()

	select {
	case result := <-calls:
		if result.OK() {
			t.Fatalf("expected failed tick, got %+v", result)
		}
		if result.BackupError == "" {
			t.Fatalf("expected backup error, got %+v", result)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("immediate run did not report tick result")
	}
}
