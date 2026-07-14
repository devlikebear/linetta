//go:build !mas && !mobile

package gitsync

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/export"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/opsstatus"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/linetta/engine/internal/store"
)

// recordingRunner stubs CmdRunner: it remembers every git invocation and
// returns a canned (stdout, err) per command keyed by the first arg
// ("add"/"status"/"commit"/"push").
type recordingRunner struct {
	mu        sync.Mutex
	calls     [][]string
	responses map[string]stub
}

type stub struct {
	stdout string
	err    error
}

func newRecordingRunner() *recordingRunner {
	return &recordingRunner{responses: map[string]stub{}}
}

func (r *recordingRunner) run(ctx context.Context, dir string, args ...string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, append([]string{}, args...))
	if len(args) == 0 {
		return "", nil
	}
	if s, ok := r.responses[args[0]]; ok {
		return s.stdout, s.err
	}
	return "", nil
}

func (r *recordingRunner) cmdNames() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	names := make([]string, 0, len(r.calls))
	for _, c := range r.calls {
		if len(c) > 0 {
			names = append(names, c[0])
		}
	}
	return names
}

// newFixture wires a real settings.Store + store + project/node/entity repos
// against a fresh LINETTA_HOME, then seeds one non-archived project.
func newFixture(t *testing.T) (*Syncer, *recordingRunner, string, *opsstatus.Repo) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("LINETTA_HOME", home)

	st, err := settings.New()
	if err != nil {
		t.Fatalf("settings.New: %v", err)
	}

	dbPath := filepath.Join(t.TempDir(), "test.db")
	storeDB, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = storeDB.Close() })

	projects := project.NewRepo(storeDB)
	nodes := node.NewRepo(storeDB)
	entities := entity.NewRepo(storeDB)

	if _, err := projects.Create(context.Background(), 1, project.NewInput{
		Title:        "Quiet City",
		Genres:       []string{"literary"},
		LengthTarget: "short",
		DefaultPOV:   "first",
	}); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	// Fake git repo dir.
	repoDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoDir, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}

	runner := newRecordingRunner()
	fixed := time.Date(2026, 5, 27, 9, 15, 0, 0, time.UTC)
	s := &Syncer{
		Settings: st, Projects: projects, Nodes: nodes, Entities: entities,
		Run: runner.run,
		Now: func() time.Time { return fixed },
		Ops: opsstatus.NewRepo(storeDB),
	}
	return s, runner, repoDir, s.Ops
}

func TestRunOnce_skippedWhenDirEmpty(t *testing.T) {
	s, runner, _, _ := newFixture(t)
	// Leave GitSyncDir empty (default).
	res, err := s.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if !res.Skipped {
		t.Errorf("expected Skipped=true, got %+v", res)
	}
	if names := runner.cmdNames(); len(names) != 0 {
		t.Errorf("expected no git calls, got %v", names)
	}
}

func TestRunOnce_errorWhenNotAGitRepo(t *testing.T) {
	s, runner, _, _ := newFixture(t)
	// Point at a dir that exists but has no .git.
	bare := t.TempDir()
	if _, err := s.Settings.Set(context.Background(), settings.Patch{GitSyncDir: strPtr(bare)}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	res, err := s.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if !strings.Contains(res.Error, "not a git repo") {
		t.Errorf("expected Error to contain 'not a git repo', got %q", res.Error)
	}
	if names := runner.cmdNames(); len(names) != 0 {
		t.Errorf("expected no git calls, got %v", names)
	}
}

func TestRunOnce_writesFilesAndCommitsAndPushes(t *testing.T) {
	s, runner, repoDir, _ := newFixture(t)
	runner.responses["status"] = stub{stdout: " M quiet-city.md\n"}
	if _, err := s.Settings.Set(context.Background(), settings.Patch{GitSyncDir: strPtr(repoDir)}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	res, err := s.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if res.FilesWritten != 1 {
		t.Errorf("FilesWritten = %d, want 1", res.FilesWritten)
	}
	if !res.Committed || !res.Pushed {
		t.Errorf("expected Committed && Pushed, got %+v", res)
	}
	want := "Linetta sync 2026-05-27 09:15"
	if res.Message != want {
		t.Errorf("Message = %q, want %q", res.Message, want)
	}
	if res.Error != "" {
		t.Errorf("unexpected Error: %q", res.Error)
	}
	// Verify a markdown file landed in the repo dir.
	entries, _ := os.ReadDir(repoDir)
	foundMD := false
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".md") {
			foundMD = true
			break
		}
	}
	if !foundMD {
		t.Errorf("expected a .md file in %s, got entries=%v", repoDir, entries)
	}
	manifest, err := os.ReadFile(filepath.Join(repoDir, export.SyncManifestFilename))
	if err != nil || !strings.Contains(string(manifest), `"format_version": 1`) {
		t.Fatalf("sync manifest missing or invalid: %s err=%v", manifest, err)
	}
	// Verify git calls in order.
	gotNames := runner.cmdNames()
	wantNames := []string{"add", "status", "commit", "push"}
	if len(gotNames) != len(wantNames) {
		t.Fatalf("git call count = %v, want %v", gotNames, wantNames)
	}
	for i, n := range wantNames {
		if gotNames[i] != n {
			t.Errorf("git call[%d] = %q, want %q (all=%v)", i, gotNames[i], n, gotNames)
		}
	}
}

func TestRunOnce_stopsBeforeGitWhenProjectWriteFails(t *testing.T) {
	s, runner, repoDir, _ := newFixture(t)
	ctx := context.Background()
	projs, err := s.Projects.List(ctx, project.ListFilter{Limit: 10})
	if err != nil || len(projs) != 1 {
		t.Fatalf("projects = %v, err = %v", projs, err)
	}
	blocked := filepath.Join(repoDir, export.SyncFilename(projs[0].Title, projs[0].ID))
	if err := os.Mkdir(blocked, 0o755); err != nil {
		t.Fatalf("block target: %v", err)
	}
	if _, err := s.Settings.Set(ctx, settings.Patch{GitSyncDir: strPtr(repoDir)}); err != nil {
		t.Fatalf("Set: %v", err)
	}

	res, err := s.RunOnce(ctx)
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if res.Error == "" || res.FilesWritten != 0 {
		t.Fatalf("write failure reported as success: %+v", res)
	}
	if calls := runner.cmdNames(); len(calls) != 0 {
		t.Fatalf("git ran after partial export failure: %v", calls)
	}
}

func TestRunOnce_writesOutlinePresetMetadata(t *testing.T) {
	s, runner, repoDir, _ := newFixture(t)
	ctx := context.Background()
	runner.responses["status"] = stub{stdout: " M quiet-city.md\n"}
	projs, err := s.Projects.List(ctx, project.ListFilter{Limit: 10})
	if err != nil {
		t.Fatalf("projects.List: %v", err)
	}
	if len(projs) != 1 {
		t.Fatalf("projects=%d want 1", len(projs))
	}
	preset := project.OutlinePresetWebNovel
	if _, err := s.Projects.Update(ctx, 2, project.UpdateInput{ID: projs[0].ID, OutlinePreset: &preset}); err != nil {
		t.Fatalf("set outline preset: %v", err)
	}
	if _, err := s.Settings.Set(ctx, settings.Patch{GitSyncDir: strPtr(repoDir)}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if _, err := s.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(repoDir, "quiet-city--*.md"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("find exported markdown: matches=%v err=%v", matches, err)
	}
	body, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read exported markdown: %v", err)
	}
	if !strings.Contains(string(body), "outline_preset: webnovel\n") {
		t.Fatalf("missing outline preset metadata; doc=\n%s", string(body))
	}
}

func TestRunOnce_noopWhenStatusEmpty(t *testing.T) {
	s, runner, repoDir, _ := newFixture(t)
	runner.responses["status"] = stub{stdout: ""}
	if _, err := s.Settings.Set(context.Background(), settings.Patch{GitSyncDir: strPtr(repoDir)}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	res, err := s.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if res.Committed || res.Pushed {
		t.Errorf("expected no commit/push when status empty, got %+v", res)
	}
	if res.Error != "" {
		t.Errorf("unexpected Error: %q", res.Error)
	}
	names := runner.cmdNames()
	for _, n := range names {
		if n == "commit" || n == "push" {
			t.Errorf("unexpected git call %q when status empty (all=%v)", n, names)
		}
	}
}

func TestRunOnce_pushFailureCapturedInSummary(t *testing.T) {
	s, runner, repoDir, _ := newFixture(t)
	runner.responses["status"] = stub{stdout: " M quiet-city.md\n"}
	runner.responses["push"] = stub{err: errors.New("fatal: no upstream")}
	if _, err := s.Settings.Set(context.Background(), settings.Patch{GitSyncDir: strPtr(repoDir)}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	res, err := s.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce returned hard error: %v", err)
	}
	if !res.Committed {
		t.Errorf("expected Committed=true, got %+v", res)
	}
	if res.Pushed {
		t.Errorf("expected Pushed=false, got %+v", res)
	}
	if !strings.Contains(strings.ToLower(res.Error), "push") {
		t.Errorf("expected Error to mention 'push', got %q", res.Error)
	}
}

func TestRunOnce_defaultTemplateWhenEmpty(t *testing.T) {
	s, runner, repoDir, _ := newFixture(t)
	runner.responses["status"] = stub{stdout: " M quiet-city.md\n"}
	empty := ""
	if _, err := s.Settings.Set(context.Background(), settings.Patch{
		GitSyncDir:            strPtr(repoDir),
		GitSyncCommitTemplate: &empty,
	}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	res, err := s.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if !strings.HasPrefix(res.Message, "Linetta sync ") {
		t.Errorf("expected Message to start with 'Linetta sync ', got %q", res.Message)
	}
}

func TestRunOnce_recordsOpsStatusOnPushFailure(t *testing.T) {
	s, runner, repoDir, ops := newFixture(t)
	runner.responses["status"] = stub{stdout: " M quiet-city.md\n"}
	runner.responses["push"] = stub{err: errors.New("fatal: no upstream")}
	if _, err := s.Settings.Set(context.Background(), settings.Patch{GitSyncDir: strPtr(repoDir)}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if _, err := s.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	statuses, err := ops.Get(context.Background())
	if err != nil {
		t.Fatalf("ops.Get: %v", err)
	}
	var status opsstatus.Status
	for _, s := range statuses {
		if s.JobName == opsstatus.JobGitSync {
			status = s
			break
		}
	}
	if status.JobName == "" {
		t.Fatalf("missing git sync status: %+v", statuses)
	}
	if status.LastOK {
		t.Fatalf("expected failed status, got %+v", status)
	}
	if !strings.Contains(status.LastError, "git push") {
		t.Fatalf("expected git push error, got %+v", status)
	}
	if !strings.Contains(status.MetadataJSON, `"files_written":1`) {
		t.Fatalf("metadata_json missing files_written: %s", status.MetadataJSON)
	}
}

func strPtr(v string) *string { return &v }
