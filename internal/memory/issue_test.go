package memory

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/internal/store"
	"github.com/devlikebear/linetta/internal/work"
)

func TestRepositoryCreatesAndUpdatesContinuityIssues(t *testing.T) {
	ctx := context.Background()
	repo, workID, episodeID, runID := newIssueTestRepository(t)

	issue, err := repo.CreateIssue(ctx, CreateIssueInput{
		WorkID:    workID,
		EpisodeID: episodeID,
		RunID:     runID,
		Severity:  IssueWarning,
		Title:     "Timeline ambiguity",
		Body:      "The draft mentions an old disaster without anchoring it in the timeline.",
	})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	if issue.Status != IssueOpen {
		t.Fatalf("status = %q, want open", issue.Status)
	}

	issues, err := repo.ListIssues(ctx, workID, episodeID)
	if err != nil {
		t.Fatalf("ListIssues() error = %v", err)
	}
	if len(issues) != 1 || issues[0].ID != issue.ID {
		t.Fatalf("issues = %+v, want created issue", issues)
	}

	resolved, err := repo.UpdateIssueStatus(ctx, issue.ID, IssueResolved)
	if err != nil {
		t.Fatalf("UpdateIssueStatus() error = %v", err)
	}
	if resolved.Status != IssueResolved {
		t.Fatalf("resolved status = %q, want resolved", resolved.Status)
	}
}

func newIssueTestRepository(t *testing.T) (*Repository, string, string, string) {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "linetta.db"))
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	workRepo := work.NewRepository(db)
	workItem, err := workRepo.CreateWork(ctx, work.CreateWorkInput{Title: "Issue Work"})
	if err != nil {
		t.Fatalf("CreateWork() error = %v", err)
	}
	episode, err := workRepo.CreateEpisode(ctx, workItem.ID, "Episode 1")
	if err != nil {
		t.Fatalf("CreateEpisode() error = %v", err)
	}
	runID := "run_issue_test"
	if _, err := db.Conn().ExecContext(ctx, `
		INSERT INTO agent_runs (id, work_id, episode_id, status, tessera_run_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, runID, workItem.ID, episode.ID, "closed", runID, "2026-05-24T09:00:00Z"); err != nil {
		t.Fatalf("insert run error = %v", err)
	}
	return NewRepository(db, workRepo), workItem.ID, episode.ID, runID
}
