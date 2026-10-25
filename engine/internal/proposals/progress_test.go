package proposals

import (
	"context"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/storyops"
)

func TestExtractionProgress_tracksCoverageAndNextEpisode(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	before, err := env.svc.ExtractionProgress(ctx, env.project.ID)
	if err != nil {
		t.Fatalf("ExtractionProgress: %v", err)
	}
	if before.TotalEpisodes != 1 {
		t.Fatalf("total episodes = %d, want 1 (a fresh project with no chapter structure is one 'whole' episode)", before.TotalEpisodes)
	}
	if before.CoveredEpisodes != 0 {
		t.Fatalf("covered = %d, want 0 before any extraction", before.CoveredEpisodes)
	}
	if before.NextEpisode == nil {
		t.Fatal("want a next episode before any coverage")
	}

	mustSubmit(t, env, SubmitInput{
		ProjectID: env.project.ID,
		Purpose:   PurposeImportExtraction,
		Items: []ItemInput{{
			Op:          storyops.Op{Type: "create_entity", Name: "하린"},
			FromNodeIDs: []string{env.sceneID},
		}},
	})

	after, err := env.svc.ExtractionProgress(ctx, env.project.ID)
	if err != nil {
		t.Fatalf("ExtractionProgress: %v", err)
	}
	if after.CoveredEpisodes != 1 {
		t.Fatalf("covered = %d, want 1 after an import_extraction item cites that episode's scene", after.CoveredEpisodes)
	}
	if after.NextEpisode != nil {
		t.Fatalf("next episode = %+v, want nil (every episode covered)", after.NextEpisode)
	}
	if after.PendingItems != 1 {
		t.Errorf("pending items = %d, want 1", after.PendingItems)
	}
}

func TestExtractionProgress_ignoresGeneralPurposeItems(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	// A purpose=general item citing the same scene must NOT count toward
	// import_extraction's episode coverage — the two purposes track
	// different things.
	mustSubmit(t, env, SubmitInput{
		ProjectID: env.project.ID,
		Items: []ItemInput{{
			Op:          storyops.Op{Type: "create_entity", Name: "하린"},
			FromNodeIDs: []string{env.sceneID},
		}},
	})
	progress, err := env.svc.ExtractionProgress(ctx, env.project.ID)
	if err != nil {
		t.Fatalf("ExtractionProgress: %v", err)
	}
	if progress.CoveredEpisodes != 0 {
		t.Fatalf("covered = %d, want 0 — a general-purpose item must not count", progress.CoveredEpisodes)
	}
}

func TestExtractionEstimate_sizesTheWholeManuscript(t *testing.T) {
	env := newTestEnv(t)
	est, err := env.svc.ExtractionEstimate(context.Background(), env.project.ID)
	if err != nil {
		t.Fatalf("ExtractionEstimate: %v", err)
	}
	if est.Episodes != 1 {
		t.Errorf("episodes = %d, want 1", est.Episodes)
	}
	if est.Chars == 0 {
		t.Error("chars = 0, want the fixture scene's text counted")
	}
	if est.InputTokens == 0 {
		t.Error("input_tokens = 0, want a nonzero estimate")
	}
	// InputTokens must be strictly more than a bare token count for the same
	// text — the 1.5x allowance for the brief and tool round trips.
	bareChars := est.Chars
	if est.InputTokens <= bareChars/4 {
		t.Errorf("input_tokens = %d looks too small for %d chars with the 1.5x allowance applied", est.InputTokens, bareChars)
	}
}

func TestExtractionEstimate_growsWithMoreManuscriptText(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	before, err := env.svc.ExtractionEstimate(ctx, env.project.ID)
	if err != nil {
		t.Fatalf("ExtractionEstimate: %v", err)
	}
	doc, err := storyops.PlainTextToTiptapDoc("새로 추가된 두 번째 씬의 본문이다. 꽤 길게 써 본다.")
	if err != nil {
		t.Fatalf("doc: %v", err)
	}
	if _, err := env.nodes.CreateSibling(ctx, env.sceneID, node.KindLeaf, "두 번째 씬", "", now()); err != nil {
		t.Fatalf("create sibling: %v", err)
	}
	siblings, err := env.nodes.ListByProject(ctx, env.project.ID)
	if err != nil {
		t.Fatalf("list nodes: %v", err)
	}
	var newLeaf string
	for _, n := range siblings {
		if n.Kind == node.KindLeaf && n.ID != env.sceneID {
			newLeaf = n.ID
		}
	}
	if newLeaf == "" {
		t.Fatal("new leaf not found")
	}
	if err := env.nodes.UpdateContent(ctx, newLeaf, doc, now()+1); err != nil {
		t.Fatalf("set content: %v", err)
	}

	after, err := env.svc.ExtractionEstimate(ctx, env.project.ID)
	if err != nil {
		t.Fatalf("ExtractionEstimate: %v", err)
	}
	if after.Chars <= before.Chars {
		t.Fatalf("chars = %d, want more than before (%d) after adding a scene", after.Chars, before.Chars)
	}
}

// Review fixup (f5): an item that only cites evidence from a scene — no
// from_node_ids — must still mark that scene's episode covered, or
// NextEpisode keeps naming the same episode forever even though the agent
// already read it and found something worth citing.
func TestExtractionProgress_evidenceAloneCountsAsCoverage(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	mustSubmit(t, env, SubmitInput{
		ProjectID: env.project.ID,
		Purpose:   PurposeImportExtraction,
		Items: []ItemInput{{
			Op:       storyops.Op{Type: "create_entity", Name: "하린"},
			Evidence: []EvidenceInput{{NodeID: env.sceneID, Paragraph: 1, Quote: "오른손으로 검을"}},
			// Deliberately no FromNodeIDs: the agent cited evidence but
			// did not also fill the progress hint.
		}},
	})

	progress, err := env.svc.ExtractionProgress(ctx, env.project.ID)
	if err != nil {
		t.Fatalf("ExtractionProgress: %v", err)
	}
	if progress.CoveredEpisodes != 1 {
		t.Fatalf("covered = %d, want 1 (evidence alone should count as coverage)", progress.CoveredEpisodes)
	}
	if progress.NextEpisode != nil {
		t.Fatalf("next episode = %+v, want nil (every episode covered via evidence)", progress.NextEpisode)
	}
}
