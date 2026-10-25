package proposals

import (
	"context"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/storyops"
)

func TestNormalizeForDedupe(t *testing.T) {
	tests := []struct{ a, b string }{
		{"김민호", "김 민호"},
		{"Ha-rin", "harin"},
		{"하린", "하 린"},
	}
	for _, tc := range tests {
		if normalizeForDedupe(tc.a) != normalizeForDedupe(tc.b) {
			t.Errorf("normalizeForDedupe(%q) = %q, normalizeForDedupe(%q) = %q; want equal",
				tc.a, normalizeForDedupe(tc.a), tc.b, normalizeForDedupe(tc.b))
		}
	}
}

func TestFindDuplicates_exactNameMatch(t *testing.T) {
	dups := findDuplicates("김 민호", "character", []dedupeCandidate{
		{EntityID: "e1", Name: "김민호", Kind: "character"},
	})
	if len(dups) != 1 || dups[0].Matched != "exact" || dups[0].EntityID != "e1" {
		t.Fatalf("dups = %+v", dups)
	}
}

func TestFindDuplicates_aliasMatch(t *testing.T) {
	dups := findDuplicates("harin", "character", []dedupeCandidate{
		{EntityID: "e1", Name: "하린", Kind: "character", Aliases: []string{"Ha-rin"}},
	})
	if len(dups) != 1 || dups[0].EntityID != "e1" {
		t.Fatalf("dups = %+v, want a match via alias", dups)
	}
}

func TestFindDuplicates_kindMismatchStillReportedButFlagged(t *testing.T) {
	dups := findDuplicates("하린", "place", []dedupeCandidate{
		{EntityID: "e1", Name: "하린", Kind: "character"},
	})
	if len(dups) != 1 || dups[0].Matched != "exact_kind_mismatch" {
		t.Fatalf("dups = %+v, want exact_kind_mismatch", dups)
	}
}

func TestFindDuplicates_pendingItemMatch(t *testing.T) {
	dups := findDuplicates("하린", "character", []dedupeCandidate{
		{ItemID: "item-1", Name: "하린", Kind: "character"},
	})
	if len(dups) != 1 || dups[0].ItemID != "item-1" || dups[0].EntityID != "" {
		t.Fatalf("dups = %+v, want a pending-item match", dups)
	}
}

func TestFindDuplicates_noMatch(t *testing.T) {
	dups := findDuplicates("전혀 다른 이름", "character", []dedupeCandidate{
		{EntityID: "e1", Name: "하린", Kind: "character"},
	})
	if len(dups) != 0 {
		t.Fatalf("dups = %+v, want none", dups)
	}
}

func TestSubmit_flagsDuplicateAgainstExistingEntity(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	if _, err := env.entities.Create(ctx, now(), entity.NewInput{ProjectID: env.project.ID, Name: "김민호", Kind: "character"}); err != nil {
		t.Fatalf("seed entity: %v", err)
	}
	res, problems, err := env.svc.Submit(ctx, now(), SubmitInput{
		ProjectID: env.project.ID,
		Items:     []ItemInput{{Op: storyops.Op{Type: "create_entity", Name: "김 민호", Kind: "character"}}},
	}, SourceAgent)
	if err != nil || len(problems) != 0 {
		t.Fatalf("Submit() problems=%+v err=%v", problems, err)
	}
	if len(res.Items[0].Duplicates) != 1 || res.Items[0].Duplicates[0].Matched != "exact" {
		t.Fatalf("duplicates = %+v, want one exact match", res.Items[0].Duplicates)
	}
	if res.PossibleDuplicates != 1 {
		t.Errorf("PossibleDuplicates = %d, want 1", res.PossibleDuplicates)
	}
}

func TestSubmit_flagsDuplicateAgainstAnotherPendingItem(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	// A first submission leaves a pending create_entity item...
	if _, problems, err := env.svc.Submit(ctx, now(), SubmitInput{
		ProjectID: env.project.ID,
		Items:     []ItemInput{{Op: storyops.Op{Type: "create_entity", Name: "하린", Kind: "character"}}},
	}, SourceAgent); err != nil || len(problems) != 0 {
		t.Fatalf("first Submit() problems=%+v err=%v", problems, err)
	}
	// ...and a second submission's own create_entity must flag against it.
	res, problems, err := env.svc.Submit(ctx, now(), SubmitInput{
		ProjectID: env.project.ID,
		Items:     []ItemInput{{Op: storyops.Op{Type: "create_entity", Name: "Ha-rin", Kind: "character"}}},
	}, SourceAgent)
	if err != nil || len(problems) != 0 {
		t.Fatalf("second Submit() problems=%+v err=%v", problems, err)
	}
	if len(res.Items[0].Duplicates) != 0 {
		t.Fatalf("duplicates = %+v, want none ('Ha-rin' does not normalize to '하린')", res.Items[0].Duplicates)
	}
}

func TestSubmit_flagsDuplicateWithinSameBatch(t *testing.T) {
	env := newTestEnv(t)
	res, problems, err := env.svc.Submit(context.Background(), now(), SubmitInput{
		ProjectID: env.project.ID,
		Items: []ItemInput{
			{Op: storyops.Op{Type: "create_entity", Name: "하린", Kind: "character"}},
			{Op: storyops.Op{Type: "create_entity", Name: "하 린", Kind: "character"}},
		},
	}, SourceAgent)
	if err != nil || len(problems) != 0 {
		t.Fatalf("Submit() problems=%+v err=%v", problems, err)
	}
	if len(res.Items[0].Duplicates) != 1 || res.Items[0].Duplicates[0].ItemID != res.Items[1].ID {
		t.Fatalf("item 0 duplicates = %+v, want a match against item 1", res.Items[0].Duplicates)
	}
	if len(res.Items[1].Duplicates) != 1 || res.Items[1].Duplicates[0].ItemID != res.Items[0].ID {
		t.Fatalf("item 1 duplicates = %+v, want a match against item 0", res.Items[1].Duplicates)
	}
}
