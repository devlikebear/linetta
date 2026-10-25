package forecast

import (
	"context"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/entity"
)

func TestInputs_sameDataSameHashRegardlessOfOrder(t *testing.T) {
	ctx := context.Background()
	envA := newTestEnv(t)
	envB := newTestEnv(t)

	// Identical rows (fixed ids so the two DBs hold the same entities), but
	// inserted in opposite order so SQLite's own natural scan order (and
	// entity.Repo.ListByProject's ORDER BY updated_at DESC) differs between
	// the two environments. The hash must not care.
	insertEntity := func(env *testEnv, id, name string, createdAt int64) {
		if _, err := env.st.DB().ExecContext(ctx, `
INSERT INTO entities (id, project_id, kind, name, aliases, role, summary, attributes, created_at, updated_at)
VALUES (?, ?, 'character', ?, '[]', '', '', '{}', ?, ?)`,
			id, env.project.ID, name, createdAt, createdAt); err != nil {
			t.Fatalf("insert entity: %v", err)
		}
	}
	insertEntity(envA, "e1", "하린", 1_000)
	insertEntity(envA, "e2", "유진", 1_001)
	insertEntity(envB, "e2", "유진", 1_000) // reversed order, same final rows
	insertEntity(envB, "e1", "하린", 1_001)

	hashA, err := Inputs(ctx, envA.deps, envA.project.ID)
	if err != nil {
		t.Fatalf("Inputs A: %v", err)
	}
	hashB, err := Inputs(ctx, envB.deps, envB.project.ID)
	if err != nil {
		t.Fatalf("Inputs B: %v", err)
	}
	if hashA.Entities != hashB.Entities {
		t.Errorf("Entities hash differs by insertion order: %q vs %q", hashA.Entities, hashB.Entities)
	}
	// The two envs' own opening scenes have different (randomly generated)
	// node ids, so their Outline/Episodes groups legitimately differ —
	// only the group under test (Entities, built from fixed ids) is
	// compared above.

	// Calling Inputs again on the same project, unchanged, gives the same
	// hash — no timestamps leaking in.
	hashA2, err := Inputs(ctx, envA.deps, envA.project.ID)
	if err != nil {
		t.Fatalf("Inputs A again: %v", err)
	}
	if hashA.Fingerprint() != hashA2.Fingerprint() {
		t.Error("Fingerprint changed with no canon change")
	}
}

func TestInputs_bodyEditChangesOnlyThatEpisode(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)

	// A second scene/episode so there is something that must NOT change.
	other, err := env.nodes.CreateSibling(ctx, env.sceneID, "leaf", "2화", "", 1_000)
	if err != nil {
		t.Fatalf("CreateSibling: %v", err)
	}
	env.writeScene(t, env.sceneID, "첫 문장.", 1)
	env.writeScene(t, other.ID, "둘째 화의 문장.", 1)

	before, err := Inputs(ctx, env.deps, env.project.ID)
	if err != nil {
		t.Fatalf("Inputs before: %v", err)
	}

	env.writeScene(t, env.sceneID, "고친 첫 문장.", 2)

	after, err := Inputs(ctx, env.deps, env.project.ID)
	if err != nil {
		t.Fatalf("Inputs after: %v", err)
	}

	if before.Episodes[env.sceneID] == after.Episodes[env.sceneID] {
		t.Error("edited episode's hash did not change")
	}
	if before.Episodes[other.ID] != after.Episodes[other.ID] {
		t.Error("untouched episode's hash changed")
	}
	if before.Outline != after.Outline {
		t.Error("outline hash should not change from a body-only edit")
	}
	if before.Fingerprint() == after.Fingerprint() {
		t.Error("overall fingerprint should change when a scene body changes")
	}

	reasons := Diff(before, after)
	foundEpisodes := false
	for _, r := range reasons {
		if r.Group == "episodes" {
			foundEpisodes = true
			if r.Count != 1 {
				t.Errorf("episodes stale count = %d, want 1", r.Count)
			}
		}
	}
	if !foundEpisodes {
		t.Errorf("Diff did not report episodes group changed: %+v", reasons)
	}
}

func TestInputs_entityEditChangesEntitiesGroup(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)

	e, err := env.entities.Create(ctx, 1_000, entity.NewInput{
		ProjectID: env.project.ID, Kind: "character", Name: "하린",
	})
	if err != nil {
		t.Fatalf("create entity: %v", err)
	}

	before, err := Inputs(ctx, env.deps, env.project.ID)
	if err != nil {
		t.Fatalf("Inputs before: %v", err)
	}

	if err := env.entities.Update(ctx, 1_001, entity.UpdateInput{ID: e.ID, Summary: strPtr("수정된 요약")}); err != nil {
		t.Fatalf("update entity: %v", err)
	}

	after, err := Inputs(ctx, env.deps, env.project.ID)
	if err != nil {
		t.Fatalf("Inputs after: %v", err)
	}

	if before.Entities == after.Entities {
		t.Error("entities hash did not change after an entity edit")
	}
	if before.Outline != after.Outline || before.Plot != after.Plot {
		t.Error("unrelated groups changed from an entity-only edit")
	}
}

func TestInputs_summaryOnlyEditChangesOutlineGroup(t *testing.T) {
	// node.summary feeds the brief, so it belongs in the outline bundle —
	// and a forecast must go stale when only the summary changes, even
	// though the manuscript body (the episodes group) did not.
	ctx := context.Background()
	env := newTestEnv(t)
	env.writeScene(t, env.sceneID, "본문은 그대로.", 1)

	before, err := Inputs(ctx, env.deps, env.project.ID)
	if err != nil {
		t.Fatalf("Inputs before: %v", err)
	}

	if err := env.nodes.SetSummary(ctx, env.sceneID, "바뀐 요약", 1); err != nil {
		t.Fatalf("SetSummary: %v", err)
	}

	after, err := Inputs(ctx, env.deps, env.project.ID)
	if err != nil {
		t.Fatalf("Inputs after: %v", err)
	}

	if before.Outline == after.Outline {
		t.Error("outline hash did not change after node.summary changed")
	}
	if before.Episodes[env.sceneID] != after.Episodes[env.sceneID] {
		t.Error("episodes hash should not change from a summary-only edit (body untouched)")
	}
}

func TestInputs_writingAForecastRowDoesNotChangeHashes(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	env.writeScene(t, env.sceneID, "문장.", 1)

	before, err := Inputs(ctx, env.deps, env.project.ID)
	if err != nil {
		t.Fatalf("Inputs before: %v", err)
	}

	if _, err := env.st.DB().ExecContext(ctx, `
INSERT INTO forecasts (id, project_id, divergence, base_node_id, horizon, branches_json, fingerprint,
                        input_hashes_json, source, created_at)
VALUES ('f1', ?, '다음 전개?', ?, 3, '[]', 'x', '{}', 'agent', 9999)`,
		env.project.ID, env.sceneID); err != nil {
		t.Fatalf("insert forecast row: %v", err)
	}

	after, err := Inputs(ctx, env.deps, env.project.ID)
	if err != nil {
		t.Fatalf("Inputs after: %v", err)
	}
	if before.Fingerprint() != after.Fingerprint() {
		t.Error("writing to the forecasts table changed the fingerprint — it must never be an input")
	}
}

func TestInputs_revertingCanonRevertsHashes(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	env.writeScene(t, env.sceneID, "원래 문장.", 1)

	original, err := Inputs(ctx, env.deps, env.project.ID)
	if err != nil {
		t.Fatalf("Inputs original: %v", err)
	}

	env.writeScene(t, env.sceneID, "임시로 바꾼 문장.", 2)
	changed, err := Inputs(ctx, env.deps, env.project.ID)
	if err != nil {
		t.Fatalf("Inputs changed: %v", err)
	}
	if changed.Fingerprint() == original.Fingerprint() {
		t.Fatal("expected a change after editing the scene")
	}

	env.writeScene(t, env.sceneID, "원래 문장.", 3)
	reverted, err := Inputs(ctx, env.deps, env.project.ID)
	if err != nil {
		t.Fatalf("Inputs reverted: %v", err)
	}
	if reverted.Episodes[env.sceneID] != original.Episodes[env.sceneID] {
		t.Error("episode hash did not revert to its original value")
	}
	if reverted.Fingerprint() != original.Fingerprint() {
		t.Error("fingerprint did not revert once canon reverted")
	}
}

func strPtr(s string) *string { return &s }
