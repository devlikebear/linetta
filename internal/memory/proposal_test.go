package memory

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/internal/store"
	"github.com/devlikebear/linetta/internal/work"
)

func TestRepositoryApprovesCreateProposalIntoCanon(t *testing.T) {
	ctx := context.Background()
	repo, workID, episodeID, runID := newProposalTestRepository(t)

	proposal, err := repo.CreateProposal(ctx, CreateProposalInput{
		WorkID:     workID,
		EpisodeID:  episodeID,
		RunID:      runID,
		ChangeType: ProposalChangeCreate,
		Kind:       KindCharacter,
		Title:      "New Keeper",
		AfterBody:  "A new character proposed by the Canon Keeper.",
		Reason:     "Detected a recurring named figure in the draft.",
		Confidence: 0.74,
	})
	if err != nil {
		t.Fatalf("CreateProposal() error = %v", err)
	}
	if proposal.Status != ProposalPending {
		t.Fatalf("proposal status = %q, want pending", proposal.Status)
	}

	items, err := repo.ListItems(ctx, workID, ListFilter{Query: "New Keeper"})
	if err != nil {
		t.Fatalf("ListItems() error = %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("items len before approval = %d, want 0", len(items))
	}

	approved, err := repo.ApproveProposal(ctx, proposal.ID, "human")
	if err != nil {
		t.Fatalf("ApproveProposal() error = %v", err)
	}
	if approved.Status != ProposalApproved || approved.TargetItemID == "" {
		t.Fatalf("approved = %+v, want approved status and target item", approved)
	}

	item, err := repo.GetItem(ctx, approved.TargetItemID)
	if err != nil {
		t.Fatalf("GetItem(approved target) error = %v", err)
	}
	if item.Title != "New Keeper" || item.Status != StatusCanon {
		t.Fatalf("item = %+v, want canon New Keeper", item)
	}

	decisions, err := repo.ListDecisions(ctx, workID)
	if err != nil {
		t.Fatalf("ListDecisions() error = %v", err)
	}
	if !hasDecision(decisions, DecisionApprove) {
		t.Fatalf("decisions = %+v, want approve decision", decisions)
	}
}

func TestRepositoryRejectsProposalWithoutChangingCanon(t *testing.T) {
	ctx := context.Background()
	repo, workID, episodeID, runID := newProposalTestRepository(t)

	proposal, err := repo.CreateProposal(ctx, CreateProposalInput{
		WorkID:     workID,
		EpisodeID:  episodeID,
		RunID:      runID,
		ChangeType: ProposalChangeCreate,
		Kind:       KindWorldFact,
		Title:      "Rejected Fact",
		AfterBody:  "This should not enter canon.",
		Reason:     "Low confidence extraction.",
	})
	if err != nil {
		t.Fatalf("CreateProposal() error = %v", err)
	}
	rejected, err := repo.RejectProposal(ctx, proposal.ID, "human")
	if err != nil {
		t.Fatalf("RejectProposal() error = %v", err)
	}
	if rejected.Status != ProposalRejected {
		t.Fatalf("rejected status = %q, want rejected", rejected.Status)
	}

	items, err := repo.ListItems(ctx, workID, ListFilter{Query: "Rejected Fact"})
	if err != nil {
		t.Fatalf("ListItems() error = %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("items len = %d, want 0", len(items))
	}
}

func newProposalTestRepository(t *testing.T) (*Repository, string, string, string) {
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
	workItem, err := workRepo.CreateWork(ctx, work.CreateWorkInput{Title: "Proposal Work"})
	if err != nil {
		t.Fatalf("CreateWork() error = %v", err)
	}
	episode, err := workRepo.CreateEpisode(ctx, workItem.ID, "Episode 1")
	if err != nil {
		t.Fatalf("CreateEpisode() error = %v", err)
	}
	runID := "run_test"
	if _, err := db.Conn().ExecContext(ctx, `
		INSERT INTO agent_runs (id, work_id, episode_id, status, tessera_run_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, runID, workItem.ID, episode.ID, "closed", runID, "2026-05-24T09:00:00Z"); err != nil {
		t.Fatalf("insert run error = %v", err)
	}
	return NewRepository(db, workRepo), workItem.ID, episode.ID, runID
}

func hasDecision(decisions []Decision, decisionType DecisionType) bool {
	for _, decision := range decisions {
		if decision.Type == decisionType {
			return true
		}
	}
	return false
}
