package storyops

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/fact"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/thread"
)

// worldEntity creates one entity in projectID with a paired relationship to a
// second one, and returns both.
func worldPair(t *testing.T, svc *Service, projectID string) (entity.Entity, entity.Entity, []relationship.Relationship) {
	t.Helper()
	ctx := context.Background()
	a, err := svc.entities.Create(ctx, 1, entity.NewInput{ProjectID: projectID, Kind: "character", Name: "서린"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	b, err := svc.entities.Create(ctx, 1, entity.NewInput{ProjectID: projectID, Kind: "character", Name: "마충"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	rels, err := svc.relationships.CreatePair(ctx, relationship.NewPairInput{
		ProjectID: projectID, FromID: a.ID, ToID: b.ID, Label: "스승", InverseLabel: "제자",
	})
	if err != nil {
		t.Fatalf("pair: %v", err)
	}
	return a, b, rels
}

func applyWorld(t *testing.T, svc *Service, projectID string, ops ...Op) ApplyOpsResult {
	t.Helper()
	return svc.ApplyOps(context.Background(), projectID, "", Proposal{Summary: "정리", Ops: ops}, now())
}

func TestDeleteEntityTakesItsRelationshipsAndUndoBringsThemBack(t *testing.T) {
	ctx := context.Background()
	svc, _, _, projectID, _ := newTestService(t)
	a, _, _ := worldPair(t, svc, projectID)

	res := applyWorld(t, svc, projectID, Op{Type: "delete_entity", EntityID: a.ID})
	if res.IsError() {
		t.Fatalf("delete: %+v", res.Failures)
	}
	if len(res.Deleted) != 1 || res.Deleted[0].Label != "서린" || res.Deleted[0].Relationships != 2 {
		t.Fatalf("Deleted = %+v", res.Deleted)
	}
	if _, err := svc.entities.Get(ctx, a.ID); err == nil {
		t.Fatal("entity survived the delete")
	}
	if res.UndoBatchID == "" {
		t.Fatal("a delete must return an undo batch")
	}

	if err := svc.UndoApply(ctx, res.UndoBatchID, now()); err != nil {
		t.Fatalf("undo: %v", err)
	}
	if _, err := svc.entities.Get(ctx, a.ID); err != nil {
		t.Fatalf("entity did not come back: %v", err)
	}
	rels, err := svc.relationships.ListByProject(ctx, projectID)
	if err != nil || len(rels) != 2 {
		t.Fatalf("relationships after undo = %d (%v), want 2", len(rels), err)
	}
}

// Both ends deleted in one batch: the restore runs last-first, so the pair
// that went with the first delete finds both of its records.
func TestUndoOfADeleteBatchRestoresARelationshipBetweenTwoDeletedRecords(t *testing.T) {
	ctx := context.Background()
	svc, _, _, projectID, _ := newTestService(t)
	a, b, _ := worldPair(t, svc, projectID)

	res := applyWorld(t, svc, projectID,
		Op{Type: "delete_entity", EntityID: a.ID}, Op{Type: "delete_entity", EntityID: b.ID})
	if res.IsError() {
		t.Fatalf("delete: %+v", res.Failures)
	}
	if err := svc.UndoApply(ctx, res.UndoBatchID, now()); err != nil {
		t.Fatalf("undo: %v", err)
	}
	rels, err := svc.relationships.ListByProject(ctx, projectID)
	if err != nil || len(rels) != 2 {
		t.Fatalf("relationships after undo = %d (%v), want 2", len(rels), err)
	}
}

func TestWorldDeletesStayInsideTheNamedWork(t *testing.T) {
	ctx := context.Background()
	svc, _, _, projectID, _ := newTestService(t)
	a, _, rels := worldPair(t, svc, projectID)
	card, err := svc.facts.Create(ctx, 1, fact.NewInput{
		ProjectID: projectID, Claim: "c", Result: "r", Status: fact.StatusUncertain,
		Sources: []fact.SourceInput{{URL: "https://example.com"}},
	})
	if err != nil {
		t.Fatalf("fact: %v", err)
	}
	th, err := svc.threads.Create(ctx, thread.NewInput{ProjectID: projectID, Name: "복수"})
	if err != nil {
		t.Fatalf("thread: %v", err)
	}
	other, err := svc.projects.Create(ctx, 2, project.NewInput{
		Title: "다른 작품", Genres: []string{}, LengthTarget: "short", DefaultPOV: "first",
	})
	if err != nil {
		t.Fatalf("project: %v", err)
	}

	for _, op := range []Op{
		{Type: "delete_entity", EntityID: a.ID},
		{Type: "delete_relationship", RelationshipID: rels[0].ID},
		{Type: "delete_fact_card", FactID: card.ID},
		{Type: "delete_thread", ThreadID: th.ID},
	} {
		if res := applyWorld(t, svc, other.ID, op); !res.IsError() {
			t.Errorf("%s reached across works", op.Type)
		}
	}
	if _, err := svc.entities.Get(ctx, a.ID); err != nil {
		t.Errorf("entity gone: %v", err)
	}
}

func TestDeleteThreadTakesItsBeatsAndUndoBringsThemBack(t *testing.T) {
	ctx := context.Background()
	svc, _, _, projectID, _ := newTestService(t)
	th, err := svc.threads.Create(ctx, thread.NewInput{ProjectID: projectID, Name: "복수"})
	if err != nil {
		t.Fatalf("thread: %v", err)
	}
	if res := applyWorld(t, svc, projectID, Op{Type: "add_beat", ThreadID: th.ID, Label: "맹세"}); res.IsError() {
		t.Fatalf("beat: %+v", res.Failures)
	}

	res := applyWorld(t, svc, projectID, Op{Type: "delete_thread", ThreadID: th.ID})
	if res.IsError() || len(res.Deleted) != 1 || res.Deleted[0].Beats != 1 {
		t.Fatalf("delete: %+v", res)
	}
	if err := svc.UndoApply(ctx, res.UndoBatchID, now()); err != nil {
		t.Fatalf("undo: %v", err)
	}
	beats, err := svc.beats.ListByThread(ctx, th.ID)
	if err != nil || len(beats) != 1 {
		t.Fatalf("beats after undo = %d (%v), want 1", len(beats), err)
	}
}

func TestDeleteFactCardUndoKeepsItsSources(t *testing.T) {
	ctx := context.Background()
	svc, _, _, projectID, _ := newTestService(t)
	card, err := svc.facts.Create(ctx, 1, fact.NewInput{
		ProjectID: projectID, Claim: "c", Result: "r", Status: fact.StatusUncertain,
		Sources: []fact.SourceInput{{URL: "https://example.com"}},
	})
	if err != nil {
		t.Fatalf("fact: %v", err)
	}
	res := applyWorld(t, svc, projectID, Op{Type: "delete_fact_card", FactID: card.ID})
	if res.IsError() {
		t.Fatalf("delete: %+v", res.Failures)
	}
	if err := svc.UndoApply(ctx, res.UndoBatchID, now()); err != nil {
		t.Fatalf("undo: %v", err)
	}
	got, err := svc.facts.Get(ctx, card.ID)
	if err != nil || len(got.Sources) != 1 {
		t.Fatalf("card after undo = %+v (%v)", got, err)
	}
}

func TestDeleteBeatIsUndoable(t *testing.T) {
	ctx := context.Background()
	svc, _, _, projectID, _ := newTestService(t)
	th, err := svc.threads.Create(ctx, thread.NewInput{ProjectID: projectID, Name: "복수"})
	if err != nil {
		t.Fatalf("thread: %v", err)
	}
	if res := applyWorld(t, svc, projectID, Op{Type: "add_beat", ThreadID: th.ID, Label: "맹세"}); res.IsError() {
		t.Fatalf("beat: %+v", res.Failures)
	}
	beats, err := svc.beats.ListByThread(ctx, th.ID)
	if err != nil || len(beats) != 1 {
		t.Fatalf("beats: %v", err)
	}
	res := applyWorld(t, svc, projectID, Op{Type: "delete_beat", BeatID: beats[0].ID})
	if res.IsError() || res.UndoBatchID == "" {
		t.Fatalf("delete: %+v", res)
	}
	if err := svc.UndoApply(ctx, res.UndoBatchID, now()); err != nil {
		t.Fatalf("undo: %v", err)
	}
	if beats, _ := svc.beats.ListByThread(ctx, th.ID); len(beats) != 1 {
		t.Fatalf("beats after undo = %d, want 1", len(beats))
	}
}

// A failing op in a plain world batch keeps what the others did, so what they
// deleted must still be one undo away.
func TestAPartlyFailedBatchKeepsItsDeletesUndoable(t *testing.T) {
	ctx := context.Background()
	svc, _, _, projectID, _ := newTestService(t)
	a, _, _ := worldPair(t, svc, projectID)
	res := applyWorld(t, svc, projectID,
		Op{Type: "delete_entity", EntityID: a.ID},
		Op{Type: "delete_entity", EntityID: "없는 이름"})
	if !res.IsError() || res.Applied != 1 || res.UndoBatchID == "" {
		t.Fatalf("result = %+v", res)
	}
	if err := svc.UndoApply(ctx, res.UndoBatchID, now()); err != nil {
		t.Fatalf("undo: %v", err)
	}
	if _, err := svc.entities.Get(ctx, a.ID); err != nil {
		t.Fatalf("entity did not come back: %v", err)
	}
}

// A structural batch reports "nothing was applied" when it rolls back, so a
// delete inside it has to go back with the outline.
func TestARolledBackStructuralBatchPutsItsDeletesBack(t *testing.T) {
	ctx := context.Background()
	svc, _, _, projectID, nodeID := newTestService(t)
	a, _, _ := worldPair(t, svc, projectID)
	res := svc.ApplyOps(ctx, projectID, nodeID, Proposal{Summary: "s", Ops: []Op{
		{Type: "delete_entity", EntityID: a.ID},
		{Type: "create_scene", Label: "새 씬"},
		{Type: "rename_outline_node", NodeID: "no-such-node", Label: "x"},
	}}, now())
	if !res.RolledBack {
		t.Fatalf("result = %+v", res)
	}
	if len(res.Deleted) != 0 || res.UndoBatchID != "" {
		t.Fatalf("a rolled-back batch reported %+v / %q", res.Deleted, res.UndoBatchID)
	}
	if _, err := svc.entities.Get(ctx, a.ID); err != nil {
		t.Fatalf("entity not put back: %v", err)
	}
}

func TestUndoOfAStructuralBatchAlsoRestoresItsDeletes(t *testing.T) {
	ctx := context.Background()
	svc, nodes, _, projectID, nodeID := newTestService(t)
	a, _, _ := worldPair(t, svc, projectID)
	before, err := nodes.ListByProject(ctx, projectID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	res := svc.ApplyOps(ctx, projectID, nodeID, Proposal{Summary: "s", Ops: []Op{
		{Type: "delete_entity", EntityID: a.ID},
		{Type: "create_scene", Label: "새 씬"},
	}}, now())
	if res.IsError() || res.UndoBatchID == "" {
		t.Fatalf("result = %+v", res)
	}
	if err := svc.UndoApply(ctx, res.UndoBatchID, now()); err != nil {
		t.Fatalf("undo: %v", err)
	}
	after, err := nodes.ListByProject(ctx, projectID)
	if err != nil || len(after) != len(before) {
		t.Fatalf("outline after undo = %d nodes (%v), want %d", len(after), err, len(before))
	}
	if _, err := svc.entities.Get(ctx, a.ID); err != nil {
		t.Fatalf("entity did not come back: %v", err)
	}
}

func TestUpdateEntityRemovesOnlyWhatItNames(t *testing.T) {
	ctx := context.Background()
	svc, _, _, projectID, _ := newTestService(t)
	a, _, _ := worldPair(t, svc, projectID)
	if res := applyWorld(t, svc, projectID, Op{
		Type: "update_entity", EntityID: a.ID,
		Aliases: []string{"명패 장수", "영감"}, Attributes: map[string]string{"나이": "60", "고향": "서촌"},
	}); res.IsError() {
		t.Fatalf("seed: %+v", res.Failures)
	}
	if res := applyWorld(t, svc, projectID, Op{
		Type: "update_entity", EntityID: a.ID,
		Aliases: []string{"노인"}, RemoveAliases: []string{"영감"},
		Attributes: map[string]string{"직업": "장수"}, RemoveAttributes: []string{"고향"},
	}); res.IsError() {
		t.Fatalf("update: %+v", res.Failures)
	}
	got, err := svc.entities.Get(ctx, a.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if slices.Contains(got.Aliases, "영감") || !slices.Contains(got.Aliases, "명패 장수") || !slices.Contains(got.Aliases, "노인") {
		t.Errorf("aliases = %v", got.Aliases)
	}
	if _, ok := got.Attributes["고향"]; ok || got.Attributes["나이"] != "60" || got.Attributes["직업"] != "장수" {
		t.Errorf("attributes = %v", got.Attributes)
	}
}

func TestUpdateRelationshipKeepsWhatItDoesNotName(t *testing.T) {
	ctx := context.Background()
	svc, _, _, projectID, _ := newTestService(t)
	_, _, rels := worldPair(t, svc, projectID)
	if res := applyWorld(t, svc, projectID, Op{Type: "update_relationship", RelationshipID: rels[0].ID, Notes: "오래된 사이"}); res.IsError() {
		t.Fatalf("update: %+v", res.Failures)
	}
	got, err := svc.relationships.Get(ctx, rels[0].ID)
	if err != nil || got.Label != "스승" || got.Notes != "오래된 사이" {
		t.Fatalf("relationship = %+v (%v)", got, err)
	}
}

func TestDeleteRelationshipTakesItsPairAndUndoBringsBothBack(t *testing.T) {
	ctx := context.Background()
	svc, _, _, projectID, _ := newTestService(t)
	_, _, rels := worldPair(t, svc, projectID)
	res := applyWorld(t, svc, projectID, Op{Type: "delete_relationship", RelationshipID: rels[0].ID})
	if res.IsError() {
		t.Fatalf("delete: %+v", res.Failures)
	}
	if left, _ := svc.relationships.ListByProject(ctx, projectID); len(left) != 0 {
		t.Fatalf("relationships left = %d, want 0", len(left))
	}
	if err := svc.UndoApply(ctx, res.UndoBatchID, now()); err != nil {
		t.Fatalf("undo: %v", err)
	}
	if left, _ := svc.relationships.ListByProject(ctx, projectID); len(left) != 2 {
		t.Fatalf("relationships after undo = %d, want 2", len(left))
	}
}

func TestUpdateFactCardChangesOnlyWhatItNames(t *testing.T) {
	ctx := context.Background()
	svc, _, _, projectID, _ := newTestService(t)
	card, err := svc.facts.Create(ctx, 1, fact.NewInput{
		ProjectID: projectID, Claim: "c", Result: "r", Status: fact.StatusUncertain,
		Sources: []fact.SourceInput{{URL: "https://example.com"}},
	})
	if err != nil {
		t.Fatalf("fact: %v", err)
	}
	if res := applyWorld(t, svc, projectID, Op{Type: "update_fact_card", FactID: card.ID, Status: fact.StatusVerified}); res.IsError() {
		t.Fatalf("update: %+v", res.Failures)
	}
	got, err := svc.facts.Get(ctx, card.ID)
	if err != nil || got.Status != fact.StatusVerified || got.Claim != "c" {
		t.Fatalf("card = %+v (%v)", got, err)
	}
	res := applyWorld(t, svc, projectID, Op{Type: "update_fact_card", FactID: card.ID, Status: "maybe"})
	if !res.IsError() || !strings.Contains(res.Failures[0].Error, "status") {
		t.Fatalf("bad status = %+v", res)
	}
}

func TestCloseAndReopenThread(t *testing.T) {
	ctx := context.Background()
	svc, _, _, projectID, _ := newTestService(t)
	th, err := svc.threads.Create(ctx, thread.NewInput{ProjectID: projectID, Name: "복수"})
	if err != nil {
		t.Fatalf("thread: %v", err)
	}
	if res := applyWorld(t, svc, projectID, Op{Type: "close_thread", ThreadID: "복수"}); res.IsError() {
		t.Fatalf("close: %+v", res.Failures)
	}
	if got, _ := svc.threads.Get(ctx, th.ID); got.ClosedAt == nil {
		t.Fatal("not closed")
	}
	if res := applyWorld(t, svc, projectID, Op{Type: "reopen_thread", ThreadID: th.ID}); res.IsError() {
		t.Fatalf("reopen: %+v", res.Failures)
	}
	if got, _ := svc.threads.Get(ctx, th.ID); got.ClosedAt != nil {
		t.Fatal("not reopened")
	}
}
