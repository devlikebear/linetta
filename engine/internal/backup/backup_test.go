package backup_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/devlikebear/linetta/engine/internal/backup"
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
	path, did, err := backup.RunDailyIfNeeded(context.Background(), s.DB(), home, now)
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
	if _, _, err := backup.RunDailyIfNeeded(context.Background(), s.DB(), home, now); err != nil {
		t.Fatalf("first: %v", err)
	}
	// Same day, later time.
	later := time.Date(2026, 5, 26, 18, 0, 0, 0, time.UTC)
	_, did, err := backup.RunDailyIfNeeded(context.Background(), s.DB(), home, later)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if did {
		t.Error("did=true on second same-day run")
	}
}

func TestRunDailyIfNeeded_retriesSameDayAfterFailedAttempt(t *testing.T) {
	s, home := openSeededStore(t)
	now := time.Date(2026, 5, 26, 9, 0, 0, 0, time.UTC)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, did, err := backup.RunDailyIfNeeded(cancelled, s.DB(), home, now); err == nil || did {
		t.Fatalf("cancelled backup = did %v, err %v; want failure", did, err)
	}

	path, did, err := backup.RunDailyIfNeeded(context.Background(), s.DB(), home, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if !did || path == "" {
		t.Fatalf("retry = path %q, did %v; want completed backup", path, did)
	}
}

func TestRunDailyIfNeeded_retriesWhenCompletedFileIsMissing(t *testing.T) {
	s, home := openSeededStore(t)
	now := time.Date(2026, 5, 26, 9, 0, 0, 0, time.UTC)
	path, _, err := backup.RunDailyIfNeeded(context.Background(), s.DB(), home, now)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove completed file: %v", err)
	}

	retryPath, did, err := backup.RunDailyIfNeeded(context.Background(), s.DB(), home, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if !did || retryPath == path {
		t.Fatalf("retry = %q, did %v; want a new completed backup", retryPath, did)
	}
}

func TestRunPreMigration_createsDiscoverableCompletedBackup(t *testing.T) {
	s, home := openSeededStore(t)
	now := time.Date(2026, 5, 26, 8, 30, 0, 0, time.UTC)

	path, err := backup.RunPreMigration(context.Background(), s.DB(), home, 15, now)
	if err != nil {
		t.Fatalf("RunPreMigration: %v", err)
	}
	if filepath.Base(path) != "library-pre-migration-v0015-083000.db" {
		t.Fatalf("backup path = %q", path)
	}
	marker, err := os.ReadFile(filepath.Join(filepath.Dir(path), ".complete"))
	if err != nil {
		t.Fatalf("read marker: %v", err)
	}
	if string(marker) != filepath.Base(path)+"\n" {
		t.Fatalf("marker = %q", marker)
	}
}

func TestRunManualRecovery_createsVersionedFullLibraryArchive(t *testing.T) {
	s, home := openSeededStore(t)
	now := time.Date(2026, 7, 12, 23, 50, 0, 0, time.UTC)

	result, err := backup.RunManualRecovery(context.Background(), s.DB(), home, now)
	if err != nil {
		t.Fatalf("RunManualRecovery: %v", err)
	}
	if result.FormatVersion != 1 || filepath.Ext(result.Path) != ".linetta" {
		t.Fatalf("result = %+v", result)
	}
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(result.Path), ".complete"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("manifest JSON: %v", err)
	}
	if manifest["format"] != "linetta-library-backup" || manifest["database"] != filepath.Base(result.Path) {
		t.Fatalf("manifest = %#v", manifest)
	}
	restored, err := sql.Open("sqlite", "file:"+result.Path+"?mode=ro")
	if err != nil {
		t.Fatalf("open archive read-only: %v", err)
	}
	defer restored.Close()
	var projectCount int
	if err := restored.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM projects`).Scan(&projectCount); err != nil {
		t.Fatalf("query archive: %v", err)
	}
	if projectCount != 1 {
		t.Fatalf("project count in archive = %d, want 1", projectCount)
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
	if err := backup.Prune(home, now); err != nil {
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
	onTick := func(result backup.TickResult) {
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
	waitFn := func(context.Context, time.Duration) bool {
		time.Sleep(10 * time.Millisecond)
		return true
	}
	stop := backup.Start(context.Background(), s.DB(), home, retention,
		func() time.Time { return time.Date(2026, 5, 26, 23, 59, 59, 0, time.Local) },
		waitFn,
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
	calls := make(chan backup.TickResult, 1)
	stop := backup.Start(context.Background(), s.DB(), home, nil,
		func() time.Time { return time.Date(2026, 5, 26, 9, 0, 0, 0, time.Local) },
		func(context.Context, time.Duration) bool {
			time.Sleep(10 * time.Millisecond)
			return true
		},
		func(result backup.TickResult) { calls <- result })
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

func TestStartStopCancelsLongWait(t *testing.T) {
	s, home := openSeededStore(t)
	waiting := make(chan struct{})
	stop := backup.Start(context.Background(), s.DB(), home, nil,
		func() time.Time { return time.Date(2026, 5, 26, 9, 0, 0, 0, time.Local) },
		func(ctx context.Context, _ time.Duration) bool {
			close(waiting)
			<-ctx.Done()
			return false
		},
		nil)

	select {
	case <-waiting:
	case <-time.After(2 * time.Second):
		t.Fatal("scheduler did not enter wait")
	}
	done := make(chan struct{})
	go func() {
		stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stop did not cancel scheduler wait within one second")
	}
}
