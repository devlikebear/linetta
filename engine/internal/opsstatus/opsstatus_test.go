package opsstatus

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/store"
)

func newTestRepo(t *testing.T) *Repo {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return NewRepo(st)
}

func statusFor(t *testing.T, got []Status, job string) Status {
	t.Helper()
	for _, s := range got {
		if s.JobName == job {
			return s
		}
	}
	t.Fatalf("missing status for %s in %+v", job, got)
	return Status{}
}

func TestRepoRecordAndClearError(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	if err := repo.Record(ctx, JobBackup, 100, 150, false, "disk full", map[string]any{
		"path":       "/tmp/backups/library.db",
		"backup_ran": true,
	}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	got, err := repo.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	status := statusFor(t, got, JobBackup)
	if status.LastStartedAt == nil || *status.LastStartedAt != 100 {
		t.Fatalf("last_started_at = %+v", status.LastStartedAt)
	}
	if status.LastFinishedAt == nil || *status.LastFinishedAt != 150 {
		t.Fatalf("last_finished_at = %+v", status.LastFinishedAt)
	}
	if status.LastOK {
		t.Fatal("expected LastOK=false")
	}
	if status.LastError != "disk full" {
		t.Fatalf("last_error = %q", status.LastError)
	}
	if !strings.Contains(status.MetadataJSON, `"backup_ran":true`) {
		t.Fatalf("metadata_json missing backup_ran: %s", status.MetadataJSON)
	}

	if err := repo.ClearError(ctx, JobBackup); err != nil {
		t.Fatalf("ClearError: %v", err)
	}
	got, err = repo.Get(ctx)
	if err != nil {
		t.Fatalf("Get after ClearError: %v", err)
	}
	status = statusFor(t, got, JobBackup)
	if status.LastError != "" {
		t.Fatalf("last_error after clear = %q", status.LastError)
	}
	if status.LastOK {
		t.Fatal("clearing an error should not mark the last run as successful")
	}
}

func TestRepoRecordUpsertsExistingJob(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	if err := repo.Record(ctx, JobGitSync, 100, 150, false, "git push failed", nil); err != nil {
		t.Fatalf("Record first: %v", err)
	}
	if err := repo.Record(ctx, JobGitSync, 200, 250, true, "", map[string]any{
		"files_written": 2,
		"pushed":        true,
	}); err != nil {
		t.Fatalf("Record second: %v", err)
	}

	got, err := repo.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("status count = %d, want 1: %+v", len(got), got)
	}
	status := got[0]
	if !status.LastOK || status.LastError != "" {
		t.Fatalf("expected successful latest status, got %+v", status)
	}
	if status.LastFinishedAt == nil || *status.LastFinishedAt != 250 {
		t.Fatalf("last_finished_at = %+v", status.LastFinishedAt)
	}
	if !strings.Contains(status.MetadataJSON, `"files_written":2`) {
		t.Fatalf("metadata_json missing files_written: %s", status.MetadataJSON)
	}
}
