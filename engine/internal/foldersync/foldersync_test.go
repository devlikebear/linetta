package foldersync

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/opsstatus"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/linetta/engine/internal/store"
)

func newFixture(t *testing.T) (*Syncer, *settings.Store, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("LINETTA_HOME", home)

	st, err := settings.New()
	if err != nil {
		t.Fatalf("settings.New: %v", err)
	}
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	projects := project.NewRepo(db)
	if _, err := projects.Create(context.Background(), 1, project.NewInput{
		Title: "Quiet City", Genres: []string{"literary"}, LengthTarget: "short", DefaultPOV: "first",
	}); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	fixed := time.Date(2026, 6, 19, 9, 0, 0, 0, time.UTC)
	s := &Syncer{
		Settings: st, Projects: projects, Nodes: node.NewRepo(db),
		Entities: entity.NewRepo(db), Relationships: relationship.NewRepo(db),
		Now: func() time.Time { return fixed }, Ops: opsstatus.NewRepo(db),
	}
	return s, st, home
}

func TestRunOnceWritesMarkdown(t *testing.T) {
	s, st, _ := newFixture(t)
	target := t.TempDir()
	enabled := true
	if _, err := st.Set(context.Background(), settings.Patch{FolderSyncDir: &target, FolderSyncEnabled: &enabled}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	res, err := s.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("unexpected error: %s", res.Error)
	}
	if res.FilesWritten != 1 {
		t.Fatalf("FilesWritten = %d, want 1", res.FilesWritten)
	}
	entries, _ := os.ReadDir(target)
	if len(entries) != 1 {
		t.Fatalf("target has %d files, want 1", len(entries))
	}
}

func TestRunOnceSkipsWhenDisabled(t *testing.T) {
	s, st, _ := newFixture(t)
	target := t.TempDir()
	if _, err := st.Set(context.Background(), settings.Patch{FolderSyncDir: &target}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	res, err := s.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if !res.Skipped {
		t.Fatalf("expected skipped when disabled")
	}
}

func TestStageWritesToContainer(t *testing.T) {
	s, st, home := newFixture(t)
	target := t.TempDir()
	enabled := true
	if _, err := st.Set(context.Background(), settings.Patch{FolderSyncDir: &target, FolderSyncEnabled: &enabled}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	res, err := s.Stage(context.Background())
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if len(res.Files) != 1 {
		t.Fatalf("Files = %d, want 1", len(res.Files))
	}
	if filepath.Dir(res.StagingDir) != home {
		t.Fatalf("staging dir %q not under home %q", res.StagingDir, home)
	}
	if _, err := os.Stat(filepath.Join(res.StagingDir, res.Files[0])); err != nil {
		t.Fatalf("staged file missing: %v", err)
	}
}
