package proposals

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/beat"
	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/fact"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/linetta/engine/internal/storyops"
	"github.com/devlikebear/linetta/engine/internal/thread"
)

type testEnv struct {
	svc      *Service
	nodes    *node.Repo
	entities *entity.Repo
	threads  *thread.Repo
	beats    *beat.Repo
	projects *project.Repo
	storyops *storyops.Service
	project  project.Project
	sceneID  string
}

func newTestEnv(t *testing.T) *testEnv {
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

	p, err := projects.Create(ctx, 1_000, project.NewInput{
		Title: "제안함 테스트", Genres: []string{"fantasy"}, LengthTarget: "short", DefaultPOV: "first",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	sceneID := *p.LastOpenedNodeID
	// Give the opening scene real text so evidence can cite it.
	doc, err := storyops.PlainTextToTiptapDoc("하린은 오른손으로 검을 뽑았다.\n\n두 번째 문단이다.")
	if err != nil {
		t.Fatalf("doc: %v", err)
	}
	if err := nodes.UpdateContent(ctx, sceneID, doc, 1_001); err != nil {
		t.Fatalf("set scene content: %v", err)
	}

	svc := New(st, so, entities, threads, beats, facts, nodes, projects, nil)
	return &testEnv{
		svc: svc, nodes: nodes, entities: entities, threads: threads, beats: beats,
		projects: projects, storyops: so, project: p, sceneID: sceneID,
	}
}

func now() int64 { return 10_000 }

func TestSubmit_acceptsAllowedOpsAndStoresPending(t *testing.T) {
	env := newTestEnv(t)
	res, problems, err := env.svc.Submit(context.Background(), now(), SubmitInput{
		ProjectID: env.project.ID,
		Summary:   "새 인물",
		Items: []ItemInput{
			{Op: storyops.Op{Type: "create_entity", Ref: "e1", Name: "하린", Kind: "character"}},
		},
	}, SourceAgent)
	if err != nil {
		t.Fatalf("Submit error: %v", err)
	}
	if len(problems) != 0 {
		t.Fatalf("problems = %+v, want none", problems)
	}
	if res.ProposalID == "" || len(res.Items) != 1 {
		t.Fatalf("result = %+v", res)
	}
	if res.Items[0].Status != StatusPending {
		t.Errorf("status = %q, want pending", res.Items[0].Status)
	}
}

func TestSubmit_rejectsDisallowedOp(t *testing.T) {
	env := newTestEnv(t)
	_, problems, err := env.svc.Submit(context.Background(), now(), SubmitInput{
		ProjectID: env.project.ID,
		Items:     []ItemInput{{Op: storyops.Op{Type: "set_scene_text", NodeID: "x", Text: "hi"}}},
	}, SourceAgent)
	if err != nil {
		t.Fatalf("Submit error: %v", err)
	}
	if len(problems) == 0 {
		t.Fatal("want a problem for a disallowed op")
	}
	if !strings.Contains(problems[0].Message, "linetta_apply_story_ops") || !strings.Contains(problems[0].Message, "linetta_write_scene") {
		t.Errorf("message = %q, want guidance toward apply/write_scene", problems[0].Message)
	}
}

func TestSubmit_rejectsForecastPurposeAsUnsupported(t *testing.T) {
	env := newTestEnv(t)
	_, problems, err := env.svc.Submit(context.Background(), now(), SubmitInput{
		ProjectID: env.project.ID,
		Purpose:   PurposeForecast,
		Items:     []ItemInput{{Op: storyops.Op{Type: "create_entity", Name: "x"}}},
	}, SourceAgent)
	if err != nil {
		t.Fatalf("Submit error: %v", err)
	}
	if len(problems) == 0 || !strings.Contains(problems[0].Message, "not supported yet") {
		t.Fatalf("problems = %+v, want a 'not supported yet' refusal for forecast", problems)
	}
}

func TestSubmit_rejectsInvalidPurpose(t *testing.T) {
	env := newTestEnv(t)
	_, problems, err := env.svc.Submit(context.Background(), now(), SubmitInput{
		ProjectID: env.project.ID,
		Purpose:   "bogus",
		Items:     []ItemInput{{Op: storyops.Op{Type: "create_entity", Name: "x"}}},
	}, SourceAgent)
	if err != nil {
		t.Fatalf("Submit error: %v", err)
	}
	if len(problems) == 0 || !strings.Contains(problems[0].Message, "bogus") {
		t.Fatalf("problems = %+v, want a message naming the bad purpose", problems)
	}
}

func TestSubmit_resolvesRefDependencyWithinBatch(t *testing.T) {
	env := newTestEnv(t)
	res, problems, err := env.svc.Submit(context.Background(), now(), SubmitInput{
		ProjectID: env.project.ID,
		Items: []ItemInput{
			{Op: storyops.Op{Type: "create_entity", Ref: "e1", Name: "하린"}},
			{Op: storyops.Op{Type: "create_entity", Ref: "e2", Name: "유나"}},
			{Op: storyops.Op{Type: "create_relationship", FromRef: "e1", ToRef: "e2", Label: "스승"}},
		},
	}, SourceAgent)
	if err != nil || len(problems) != 0 {
		t.Fatalf("Submit() problems=%+v err=%v", problems, err)
	}
	rel := res.Items[2]
	if len(rel.DependsOn) != 2 {
		t.Fatalf("depends_on = %+v, want 2 entries", rel.DependsOn)
	}
	if rel.DependsOn[0] != res.Items[0].ID || rel.DependsOn[1] != res.Items[1].ID {
		t.Errorf("depends_on = %+v, want [%s %s]", rel.DependsOn, res.Items[0].ID, res.Items[1].ID)
	}
}

func TestSubmit_rejectsUndeclaredRef(t *testing.T) {
	env := newTestEnv(t)
	_, problems, err := env.svc.Submit(context.Background(), now(), SubmitInput{
		ProjectID: env.project.ID,
		Items: []ItemInput{
			{Op: storyops.Op{Type: "create_entity", Ref: "e1", Name: "하린"}},
			{Op: storyops.Op{Type: "create_relationship", FromRef: "e1", ToRef: "ghost", Label: "스승"}},
		},
	}, SourceAgent)
	if err != nil {
		t.Fatalf("Submit error: %v", err)
	}
	if len(problems) == 0 || !strings.Contains(problems[0].Message, "ghost") {
		t.Fatalf("problems = %+v, want a problem naming the undeclared ref", problems)
	}
}

func TestSubmit_rejectsUnknownEntityID(t *testing.T) {
	env := newTestEnv(t)
	_, problems, err := env.svc.Submit(context.Background(), now(), SubmitInput{
		ProjectID: env.project.ID,
		Items:     []ItemInput{{Op: storyops.Op{Type: "update_entity", EntityID: "nope", Summary: "x"}}},
	}, SourceAgent)
	if err != nil {
		t.Fatalf("Submit error: %v", err)
	}
	if len(problems) == 0 || !strings.Contains(problems[0].Message, "nope") {
		t.Fatalf("problems = %+v, want a problem naming the unknown entity id", problems)
	}
}

func TestSubmit_verifiesEvidenceQuote(t *testing.T) {
	env := newTestEnv(t)
	_, problems, err := env.svc.Submit(context.Background(), now(), SubmitInput{
		ProjectID: env.project.ID,
		Items: []ItemInput{{
			Op:       storyops.Op{Type: "create_entity", Name: "하린"},
			Evidence: []EvidenceInput{{NodeID: env.sceneID, Paragraph: 1, Quote: "이런 문장은 본문에 없다"}},
		}},
	}, SourceAgent)
	if err != nil {
		t.Fatalf("Submit error: %v", err)
	}
	if len(problems) == 0 || !strings.Contains(problems[0].Message, "quote not found") {
		t.Fatalf("problems = %+v, want a quote-not-found problem", problems)
	}
}

func TestSubmit_acceptsVerifiedEvidence(t *testing.T) {
	env := newTestEnv(t)
	res, problems, err := env.svc.Submit(context.Background(), now(), SubmitInput{
		ProjectID: env.project.ID,
		Purpose:   PurposeImportExtraction,
		Items: []ItemInput{{
			Op:          storyops.Op{Type: "create_entity", Name: "하린"},
			Evidence:    []EvidenceInput{{NodeID: env.sceneID, Paragraph: 1, Quote: "오른손으로 검을"}},
			FromNodeIDs: []string{env.sceneID},
		}},
	}, SourceAgent)
	if err != nil || len(problems) != 0 {
		t.Fatalf("Submit() problems=%+v err=%v", problems, err)
	}
	if len(res.Items[0].Evidence) != 1 || len(res.Items[0].FromNodeIDs) != 1 {
		t.Fatalf("item = %+v, evidence/from_node_ids not stored", res.Items[0])
	}
}

func TestSubmit_rejectsNonLeafFromNodeID(t *testing.T) {
	env := newTestEnv(t)
	_, problems, err := env.svc.Submit(context.Background(), now(), SubmitInput{
		ProjectID: env.project.ID,
		Items: []ItemInput{{
			Op:          storyops.Op{Type: "create_entity", Name: "하린"},
			FromNodeIDs: []string{"no-such-node"},
		}},
	}, SourceAgent)
	if err != nil {
		t.Fatalf("Submit error: %v", err)
	}
	if len(problems) == 0 || !strings.Contains(problems[0].Message, "no-such-node") {
		t.Fatalf("problems = %+v, want a problem naming the bad from_node_ids entry", problems)
	}
}

func TestSubmit_rejectsTooManyItems(t *testing.T) {
	env := newTestEnv(t)
	items := make([]ItemInput, MaxItemsPerBatch+1)
	for i := range items {
		items[i] = ItemInput{Op: storyops.Op{Type: "create_entity", Name: "x"}}
	}
	_, problems, err := env.svc.Submit(context.Background(), now(), SubmitInput{
		ProjectID: env.project.ID, Items: items,
	}, SourceAgent)
	if err != nil {
		t.Fatalf("Submit error: %v", err)
	}
	if len(problems) == 0 || !strings.Contains(problems[0].Message, "61") {
		t.Fatalf("problems = %+v, want a problem naming the over-limit count", problems)
	}
}

func TestSubmit_rejectsTooManyEvidencePerItem(t *testing.T) {
	env := newTestEnv(t)
	ev := make([]EvidenceInput, MaxEvidencePerItem+1)
	for i := range ev {
		ev[i] = EvidenceInput{NodeID: env.sceneID, Paragraph: 1, Quote: "오른손으로"}
	}
	_, problems, err := env.svc.Submit(context.Background(), now(), SubmitInput{
		ProjectID: env.project.ID,
		Items:     []ItemInput{{Op: storyops.Op{Type: "create_entity", Name: "x"}, Evidence: ev}},
	}, SourceAgent)
	if err != nil {
		t.Fatalf("Submit error: %v", err)
	}
	if len(problems) == 0 {
		t.Fatal("want a problem for too many evidence entries")
	}
}

func TestSubmit_nothingStoredWhenAnyItemHasAProblem(t *testing.T) {
	env := newTestEnv(t)
	_, problems, err := env.svc.Submit(context.Background(), now(), SubmitInput{
		ProjectID: env.project.ID,
		Items: []ItemInput{
			{Op: storyops.Op{Type: "create_entity", Ref: "e1", Name: "하린"}},
			{Op: storyops.Op{Type: "update_entity", EntityID: "nope"}},
		},
	}, SourceAgent)
	if err != nil {
		t.Fatalf("Submit error: %v", err)
	}
	if len(problems) == 0 {
		t.Fatal("want a problem")
	}
	all, err := env.svc.listProposals(context.Background(), env.project.ID, nil)
	if err != nil {
		t.Fatalf("listProposals: %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("stored %d proposals, want 0 (all-or-nothing on a bad submission)", len(all))
	}
}

func TestSubmit_rejectsNonLeafNodeIDOnFactCard(t *testing.T) {
	env := newTestEnv(t)
	_, problems, err := env.svc.Submit(context.Background(), now(), SubmitInput{
		ProjectID: env.project.ID,
		Items: []ItemInput{{Op: storyops.Op{
			Type: "create_fact_card", Claim: "x", Result: "y", NodeID: "no-such-node",
			Sources: []fact.SourceInput{{URL: "https://example.com"}},
		}}},
	}, SourceAgent)
	if err != nil {
		t.Fatalf("Submit error: %v", err)
	}
	if len(problems) == 0 || !strings.Contains(problems[0].Message, "no-such-node") {
		t.Fatalf("problems = %+v, want a problem naming the bad node_id", problems)
	}
}
