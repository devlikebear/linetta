package handlers

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/beat"
	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/fact"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/proposals"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/linetta/engine/internal/storyops"
	"github.com/devlikebear/linetta/engine/internal/thread"
)

type proposalsFix struct {
	svc *proposals.Service
	pID string
	now Clock
}

func newProposalsHandlerFixture(t *testing.T) proposalsFix {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	projects := project.NewRepo(st)
	nodes := node.NewRepo(st)
	threads := thread.NewRepo(st)
	beats := beat.NewRepo(st)
	entities := entity.NewRepo(st)
	relationships := relationship.NewRepo(st)
	facts := fact.NewRepo(st)
	so := storyops.New(projects, nodes, threads, beats, entities, relationships).WithFacts(facts)

	p, err := projects.Create(ctx, 1, project.NewInput{
		Title: "T", Genres: []string{}, LengthTarget: "short", DefaultPOV: "first",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	svc := proposals.New(st, so, entities, threads, beats, facts, nodes, projects, nil)
	return proposalsFix{svc: svc, pID: p.ID, now: func() int64 { return 2000 }}
}

func mustSubmitRPC(t *testing.T, f proposalsFix, in proposals.SubmitInput) proposals.SubmitResult {
	t.Helper()
	res, problems, err := f.svc.Submit(context.Background(), f.now(), in, proposals.SourceAgent)
	if err != nil || len(problems) != 0 {
		t.Fatalf("Submit() problems=%+v err=%v", problems, err)
	}
	return res
}

func TestListProposals_summaryAndStructure(t *testing.T) {
	f := newProposalsHandlerFixture(t)
	mustSubmitRPC(t, f, proposals.SubmitInput{
		ProjectID: f.pID,
		Items: []proposals.ItemInput{
			{Op: storyops.Op{Type: "create_entity", Name: "하린", Kind: "character", Role: "검술 사범"}},
		},
	})

	h := ListProposals(f.svc, nil, nil, nil)
	params, _ := json.Marshal(map[string]any{"project_id": f.pID})
	raw, err := h(context.Background(), params)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	var out struct {
		Proposals []proposalOut `json:"proposals"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Proposals) != 1 || len(out.Proposals[0].Items) != 1 {
		t.Fatalf("out = %+v", out)
	}
	item := out.Proposals[0].Items[0]
	if !strings.Contains(item.Summary, "하린") || !strings.Contains(item.Summary, "검술 사범") {
		t.Errorf("summary = %q, want it to name the entity and role", item.Summary)
	}
}

func TestListProposals_flagsBlockedByRejected(t *testing.T) {
	f := newProposalsHandlerFixture(t)
	sub := mustSubmitRPC(t, f, proposals.SubmitInput{
		ProjectID: f.pID,
		Items: []proposals.ItemInput{
			{Op: storyops.Op{Type: "create_entity", Ref: "e1", Name: "하린"}},
			{Op: storyops.Op{Type: "create_relationship", FromRef: "e1", ToRef: "e1", Label: "자기자신"}},
		},
	})
	if err := f.svc.Reject(context.Background(), f.now(), f.pID, []string{sub.Items[0].ID}); err != nil {
		t.Fatalf("Reject: %v", err)
	}

	h := ListProposals(f.svc, nil, nil, nil)
	params, _ := json.Marshal(map[string]any{"project_id": f.pID})
	raw, err := h(context.Background(), params)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	var out struct {
		Proposals []proposalOut `json:"proposals"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	var dependent proposalItemOut
	for _, it := range out.Proposals[0].Items {
		if it.ID == sub.Items[1].ID {
			dependent = it
		}
	}
	if !dependent.BlockedByRejected {
		t.Fatalf("dependent item = %+v, want blocked_by_rejected", dependent)
	}
}

func TestAcceptProposals_appliesAndReturnsCreatedID(t *testing.T) {
	f := newProposalsHandlerFixture(t)
	sub := mustSubmitRPC(t, f, proposals.SubmitInput{
		ProjectID: f.pID,
		Items:     []proposals.ItemInput{{Op: storyops.Op{Type: "create_entity", Name: "하린"}}},
	})

	h := AcceptProposals(f.svc, f.now)
	params, _ := json.Marshal(map[string]any{"project_id": f.pID, "item_ids": []string{sub.Items[0].ID}})
	raw, err := h(context.Background(), params)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	var out proposals.AcceptResult
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Items) != 1 || out.Items[0].Status != proposals.StatusAccepted || out.Items[0].Result.CreatedID == "" {
		t.Fatalf("out = %+v", out)
	}
}

func TestRejectProposals_marksRejected(t *testing.T) {
	f := newProposalsHandlerFixture(t)
	sub := mustSubmitRPC(t, f, proposals.SubmitInput{
		ProjectID: f.pID,
		Items:     []proposals.ItemInput{{Op: storyops.Op{Type: "create_entity", Name: "하린"}}},
	})

	h := RejectProposals(f.svc, f.now)
	params, _ := json.Marshal(map[string]any{"project_id": f.pID, "item_ids": []string{sub.Items[0].ID}})
	if _, err := h(context.Background(), params); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	list, err := f.svc.List(context.Background(), f.pID, nil)
	if err != nil || len(list) != 1 || list[0].Items[0].Status != proposals.StatusRejected {
		t.Fatalf("list = %+v, err = %v", list, err)
	}
}

func TestProposalsPendingCount(t *testing.T) {
	f := newProposalsHandlerFixture(t)
	mustSubmitRPC(t, f, proposals.SubmitInput{
		ProjectID: f.pID,
		Items:     []proposals.ItemInput{{Op: storyops.Op{Type: "create_entity", Name: "하린"}}},
	})

	h := ProposalsPendingCount(f.svc)
	params, _ := json.Marshal(map[string]any{"project_id": f.pID})
	raw, err := h(context.Background(), params)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	var out struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Count != 1 {
		t.Fatalf("count = %d, want 1", out.Count)
	}
}

func TestExtractionEstimateAndProgressHandlers(t *testing.T) {
	f := newProposalsHandlerFixture(t)

	eh := ExtractionEstimate(f.svc)
	params, _ := json.Marshal(map[string]any{"project_id": f.pID})
	raw, err := eh(context.Background(), params)
	if err != nil {
		t.Fatalf("estimate handler error: %v", err)
	}
	var est proposals.Estimate
	if err := json.Unmarshal(raw, &est); err != nil {
		t.Fatalf("unmarshal estimate: %v", err)
	}
	if est.Episodes != 1 {
		t.Errorf("episodes = %d, want 1", est.Episodes)
	}

	ph := ExtractionProgress(f.svc)
	raw, err = ph(context.Background(), params)
	if err != nil {
		t.Fatalf("progress handler error: %v", err)
	}
	var progress proposals.Progress
	if err := json.Unmarshal(raw, &progress); err != nil {
		t.Fatalf("unmarshal progress: %v", err)
	}
	if progress.TotalEpisodes != 1 {
		t.Errorf("total episodes = %d, want 1", progress.TotalEpisodes)
	}
}

func TestProposalsHandlers_requireProjectID(t *testing.T) {
	f := newProposalsHandlerFixture(t)
	handlers := []struct {
		name string
		h    func(context.Context, json.RawMessage) (json.RawMessage, error)
	}{
		{"list", ListProposals(f.svc, nil, nil, nil)},
		{"accept", AcceptProposals(f.svc, f.now)},
		{"reject", RejectProposals(f.svc, f.now)},
		{"pending_count", ProposalsPendingCount(f.svc)},
		{"extraction_estimate", ExtractionEstimate(f.svc)},
		{"extraction_progress", ExtractionProgress(f.svc)},
	}
	for _, tc := range handlers {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := tc.h(context.Background(), json.RawMessage(`{}`)); err == nil {
				t.Errorf("%s: want an error when project_id is missing", tc.name)
			}
		})
	}
}
