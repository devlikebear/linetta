package forecast

import (
	"context"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/project"
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
	if !mapsEqual(hashA.Entities, hashB.Entities) {
		t.Errorf("Entities hash differs by insertion order: %v vs %v", hashA.Entities, hashB.Entities)
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

	if mapsEqual(before.Entities, after.Entities) {
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

// Review finding f4: storycontext/builder.go renders Genres/DefaultPOV/
// LengthTarget into the Project section of the brief right alongside
// Synopsis/Outline/StyleNotes, so a forecast drawn from one genre/POV must
// go stale once the writer changes it — the same way it already does for a
// Synopsis edit (covered by TestInputs_entityEditChangesEntitiesGroup's
// sibling tests above, none of which touch these three fields).
func TestInputs_projectGenresPOVOrLengthTargetChangeStalesTheProjectGroup(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)

	before, err := Inputs(ctx, env.deps, env.project.ID)
	if err != nil {
		t.Fatalf("Inputs before: %v", err)
	}

	if _, err := env.projects.Update(ctx, 1_000, project.UpdateInput{
		ID:           env.project.ID,
		Genres:       &[]string{"스릴러"},
		DefaultPOV:   strPtr("first"),
		LengthTarget: strPtr("novel"),
	}); err != nil {
		t.Fatalf("update project: %v", err)
	}

	after, err := Inputs(ctx, env.deps, env.project.ID)
	if err != nil {
		t.Fatalf("Inputs after: %v", err)
	}
	if before.Project == after.Project {
		t.Error("project hash did not change after genres/default_pov/length_target changed")
	}
	reasons := Diff(before, after)
	found := false
	for _, r := range reasons {
		if r.Group == "project" {
			found = true
		}
	}
	if !found {
		t.Errorf("Diff did not report the project group changed: %+v", reasons)
	}
}

// Review finding f5: Diff used to hardcode Count:1 for every non-episode
// group regardless of how many rows actually changed, so the writer-facing
// "인물 {count}개" banner always said 1 character even when several did.
// Entities/Relationships/Facts are now per-row maps precisely so this count
// is real — pinned here with three entities and two edits.
func TestDiff_entitiesCountIsTheNumberOfRowsThatActuallyChanged(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)

	e1, err := env.entities.Create(ctx, 1_000, entity.NewInput{ProjectID: env.project.ID, Kind: "character", Name: "하린"})
	if err != nil {
		t.Fatalf("create entity 1: %v", err)
	}
	e2, err := env.entities.Create(ctx, 1_001, entity.NewInput{ProjectID: env.project.ID, Kind: "character", Name: "유진"})
	if err != nil {
		t.Fatalf("create entity 2: %v", err)
	}
	if _, err := env.entities.Create(ctx, 1_002, entity.NewInput{ProjectID: env.project.ID, Kind: "character", Name: "untouched"}); err != nil {
		t.Fatalf("create entity 3: %v", err)
	}

	before, err := Inputs(ctx, env.deps, env.project.ID)
	if err != nil {
		t.Fatalf("Inputs before: %v", err)
	}

	if err := env.entities.Update(ctx, 1_003, entity.UpdateInput{ID: e1.ID, Summary: strPtr("수정 1")}); err != nil {
		t.Fatalf("update entity 1: %v", err)
	}
	if err := env.entities.Update(ctx, 1_004, entity.UpdateInput{ID: e2.ID, Summary: strPtr("수정 2")}); err != nil {
		t.Fatalf("update entity 2: %v", err)
	}

	after, err := Inputs(ctx, env.deps, env.project.ID)
	if err != nil {
		t.Fatalf("Inputs after: %v", err)
	}

	reasons := Diff(before, after)
	for _, r := range reasons {
		if r.Group == "entities" {
			if r.Count != 2 {
				t.Errorf("entities stale count = %d, want 2 (the untouched third entity must not be counted)", r.Count)
			}
			return
		}
	}
	t.Fatalf("Diff did not report the entities group changed: %+v", reasons)
}

// Review finding f6: hashAttrs and the comma-joined alias/Keep/Avoid lists
// used to collide a single attribute whose value itself contains a comma
// with a later split of that same attribute into two — both produced the
// byte-identical "a=1,b=2" line, so the entities hash (and Diff's count)
// missed the change entirely. listSep (a control character, not a comma)
// closes that gap.
func TestInputs_attributeContainingACommaDoesNotCollideWithASplitAttribute(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)

	e, err := env.entities.Create(ctx, 1_000, entity.NewInput{ProjectID: env.project.ID, Kind: "character", Name: "하린"})
	if err != nil {
		t.Fatalf("create entity: %v", err)
	}
	if err := env.entities.Update(ctx, 1_001, entity.UpdateInput{
		ID: e.ID, Attributes: &map[string]string{"a": "1,b=2"},
	}); err != nil {
		t.Fatalf("set initial attribute: %v", err)
	}

	before, err := Inputs(ctx, env.deps, env.project.ID)
	if err != nil {
		t.Fatalf("Inputs before: %v", err)
	}

	if err := env.entities.Update(ctx, 1_002, entity.UpdateInput{
		ID: e.ID, Attributes: &map[string]string{"a": "1", "b": "2"},
	}); err != nil {
		t.Fatalf("update entity attributes: %v", err)
	}

	after, err := Inputs(ctx, env.deps, env.project.ID)
	if err != nil {
		t.Fatalf("Inputs after: %v", err)
	}
	if mapsEqual(before.Entities, after.Entities) {
		t.Error("splitting one comma-containing attribute into two did not change the entities hash — a real change collided with the old state")
	}
}

func strPtr(s string) *string { return &s }

// mapsEqual compares two per-row hash maps (Entities/Relationships/Facts) —
// Go has no == for maps, and reflect.DeepEqual would also report two nil
// maps as equal to two empty-but-non-nil ones differently from what these
// tests mean by "unchanged", so a small explicit comparison is clearer here.
func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
