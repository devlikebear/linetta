package restore

import (
	"bytes"
	"context"
	"fmt"
	"github.com/devlikebear/linetta/engine/internal/visual"
	"path/filepath"
	"testing"
	"time"

	"github.com/devlikebear/linetta/engine/internal/agentmemory"
	"github.com/devlikebear/linetta/engine/internal/backup"
	"github.com/devlikebear/linetta/engine/internal/beat"
	"github.com/devlikebear/linetta/engine/internal/continuity"
	"github.com/devlikebear/linetta/engine/internal/direction"
	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/fact"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/note"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/snapshot"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/linetta/engine/internal/style"
	"github.com/devlikebear/linetta/engine/internal/thread"
)

// TestMergeProjectFromBackup builds a work carrying every copied table, snapshots
// the library, then merges the work back in and verifies the copy is complete
// and purely additive.
func TestMergeProjectFromBackup(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	st, err := store.Open(ctx, filepath.Join(home, "library.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	now := int64(1000)
	projects := project.NewRepo(st)
	nodes := node.NewRepo(st)
	entities := entity.NewRepo(st)
	relationships := relationship.NewRepo(st)
	threads := thread.NewRepo(st)
	beats := beat.NewRepo(st)
	notes := note.NewRepo(st)
	facts := fact.NewRepo(st)
	snaps := snapshot.NewRepo(st)
	memory := agentmemory.NewRepo(st.DB())

	p, err := projects.Create(ctx, now, project.NewInput{
		Title: "원본", Genres: []string{"sf"}, LengthTarget: "short", DefaultPOV: "first",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	leafID := *p.LastOpenedNodeID
	doc := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"본문"}]}]}`
	if err := nodes.UpdateContent(ctx, leafID, doc, now); err != nil {
		t.Fatalf("content: %v", err)
	}
	child, err := nodes.CreateSibling(ctx, leafID, "leaf", "씬 2", "", now)
	if err != nil {
		t.Fatalf("sibling: %v", err)
	}
	hero, err := entities.Create(ctx, now, entity.NewInput{ProjectID: p.ID, Kind: "character", Name: "주인공"})
	if err != nil {
		t.Fatalf("entity: %v", err)
	}
	villain, err := entities.Create(ctx, now, entity.NewInput{ProjectID: p.ID, Kind: "character", Name: "적수"})
	if err != nil {
		t.Fatalf("entity2: %v", err)
	}
	if _, err := relationships.CreatePair(ctx, relationship.NewPairInput{
		ProjectID: p.ID, FromID: hero.ID, ToID: villain.ID, Label: "맞선다", InverseLabel: "몰아붙인다",
	}); err != nil {
		t.Fatalf("relationship pair: %v", err)
	}
	th, err := threads.Create(ctx, thread.NewInput{ProjectID: p.ID, Name: "주 갈등", Color: "#333"})
	if err != nil {
		t.Fatalf("thread: %v", err)
	}
	if _, err := beats.Create(ctx, beat.NewInput{ThreadID: th.ID, NodeID: &child.ID, Label: "격돌", Intensity: 3}); err != nil {
		t.Fatalf("beat: %v", err)
	}
	if _, err := notes.Create(ctx, note.NewInput{NodeID: leafID, Anchor: 1, Body: "메모"}, now); err != nil {
		t.Fatalf("note: %v", err)
	}
	if _, err := facts.Create(ctx, now, fact.NewInput{
		ProjectID: p.ID, Claim: "사실", Result: "확인됨", Status: fact.StatusVerified,
		Sources: []fact.SourceInput{{URL: "https://example.com", AccessedAt: now}},
	}); err != nil {
		t.Fatalf("fact: %v", err)
	}
	if _, err := snaps.Create(ctx, leafID, doc, "manual", now); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if _, err := memory.Save(ctx, agentmemory.ScopeWorkNotes, p.ID, "민준은 3화부터 존댓말", now); err != nil {
		t.Fatalf("work notes: %v", err)
	}
	if _, err := memory.Save(ctx, agentmemory.ScopeWriterProfile, "", "짧은 문장을 선호", now); err != nil {
		t.Fatalf("writer profile: %v", err)
	}

	res, err := backup.RunManualRecovery(ctx, st.DB(), home, time.UnixMilli(2000))
	if err != nil {
		t.Fatalf("backup: %v", err)
	}

	list, err := ListBackups(home)
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(list) == 0 {
		t.Fatal("no backups listed")
	}
	if err := ValidateBackupPath(home, res.Path); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if err := ValidateBackupPath(home, filepath.Join(home, "library.db")); err == nil {
		t.Fatal("path outside backups folder accepted")
	}

	peeked, err := PeekProjects(ctx, res.Path, "")
	if err != nil {
		t.Fatalf("peek: %v", err)
	}
	if len(peeked) != 1 || peeked[0].Title != "원본" {
		t.Fatalf("peeked = %+v", peeked)
	}

	merged, err := MergeProject(ctx, st, res.Path, "", p.ID, " (복원)", time.UnixMilli(3000))
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if merged.Title != "원본 (복원)" {
		t.Errorf("merged title = %q", merged.Title)
	}
	if merged.ProjectID == p.ID {
		t.Fatal("merge reused the original project id")
	}

	// Original untouched.
	orig, err := projects.Get(ctx, p.ID)
	if err != nil || orig.Title != "원본" {
		t.Fatalf("original mutated: %+v err=%v", orig, err)
	}

	got, err := projects.Get(ctx, merged.ProjectID)
	if err != nil {
		t.Fatalf("get merged: %v", err)
	}
	if len(got.Genres) != 1 || got.Genres[0] != "sf" {
		t.Errorf("genres = %v", got.Genres)
	}
	if got.LastOpenedNodeID == nil {
		t.Error("last_opened_node_id not remapped")
	}

	mergedNodes, err := nodes.ListByProject(ctx, merged.ProjectID)
	if err != nil {
		t.Fatalf("list nodes: %v", err)
	}
	if len(mergedNodes) != 2 {
		t.Fatalf("nodes = %d, want 2", len(mergedNodes))
	}
	var mergedLeafID, mergedChildID string
	for _, n := range mergedNodes {
		if n.ID == leafID || n.ID == child.ID {
			t.Fatalf("node id %s reused", n.ID)
		}
		switch n.Label {
		case "씬 2":
			mergedChildID = n.ID
		default:
			mergedLeafID = n.ID
			if n.ContentDoc == nil || *n.ContentDoc != doc {
				t.Errorf("content lost on %q", n.Label)
			}
		}
	}

	ents, err := entities.ListByProject(ctx, merged.ProjectID)
	if err != nil || len(ents) != 2 {
		t.Fatalf("entities = %d err=%v", len(ents), err)
	}
	rels, err := relationships.ListByProject(ctx, merged.ProjectID)
	if err != nil || len(rels) != 2 {
		t.Fatalf("relationships = %d err=%v", len(rels), err)
	}
	if rels[0].PairID == nil || rels[1].PairID == nil || *rels[0].PairID != *rels[1].PairID {
		t.Errorf("pair_id not preserved as a pair: %+v", rels)
	}

	ths, err := threads.ListByProject(ctx, merged.ProjectID, true)
	if err != nil || len(ths) != 1 {
		t.Fatalf("threads = %d err=%v", len(ths), err)
	}
	bts, err := beats.ListByThread(ctx, ths[0].ID)
	if err != nil || len(bts) != 1 {
		t.Fatalf("beats = %d err=%v", len(bts), err)
	}
	if bts[0].NodeID == nil || *bts[0].NodeID != mergedChildID {
		t.Errorf("beat node = %v, want %s", bts[0].NodeID, mergedChildID)
	}

	nts, err := notes.ListForNode(ctx, mergedLeafID)
	if err != nil || len(nts) != 1 || nts[0].Body != "메모" {
		t.Fatalf("notes = %+v err=%v", nts, err)
	}

	cards, err := facts.List(ctx, fact.ListFilter{ProjectID: merged.ProjectID})
	if err != nil || len(cards) != 1 || len(cards[0].Sources) != 1 {
		t.Fatalf("cards = %+v err=%v", cards, err)
	}

	snapEntries, err := snaps.ListForNode(ctx, mergedLeafID)
	if err != nil || len(snapEntries) == 0 {
		t.Fatalf("snapshots = %d err=%v", len(snapEntries), err)
	}

	mergedNotes, err := memory.Load(ctx, agentmemory.ScopeWorkNotes, merged.ProjectID)
	if err != nil || mergedNotes.Body != "민준은 3화부터 존댓말" {
		t.Fatalf("merged work notes = %+v err=%v", mergedNotes, err)
	}
	origNotes, err := memory.Load(ctx, agentmemory.ScopeWorkNotes, p.ID)
	if err != nil || origNotes.Body != "민준은 3화부터 존댓말" {
		t.Fatalf("original work notes disturbed: %+v err=%v", origNotes, err)
	}

	var profileCount int
	if err := st.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_memory WHERE scope = 'writer_profile'`).Scan(&profileCount); err != nil {
		t.Fatalf("count writer_profile rows: %v", err)
	}
	if profileCount != 1 {
		t.Fatalf("writer_profile rows = %d, want 1 (merge must not duplicate the global profile)", profileCount)
	}
	profile, err := memory.Load(ctx, agentmemory.ScopeWriterProfile, "")
	if err != nil || profile.Body != "짧은 문장을 선호" {
		t.Fatalf("writer profile = %+v err=%v", profile, err)
	}
}

func TestMergeVisualsAndOlderBackup(t *testing.T) {
	for _, old := range []bool{false, true} {
		t.Run(fmt.Sprint(old), func(t *testing.T) {
			ctx := context.Background()
			home := t.TempDir()
			st, err := store.Open(ctx, filepath.Join(home, "library.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer st.Close()
			p, err := project.NewRepo(st).Create(ctx, 1, project.NewInput{Title: "Visual", LengthTarget: "short", DefaultPOV: "first"})
			if err != nil {
				t.Fatal(err)
			}
			e, err := entity.NewRepo(st).Create(ctx, 1, entity.NewInput{ProjectID: p.ID, Name: "Hero"})
			if err != nil {
				t.Fatal(err)
			}
			r := visual.NewRepo(st)
			data := []byte("\x89PNG\r\n\x1a\nimage")
			if _, err := r.SetSheet(ctx, 1, visual.Sheet{EntityID: e.ID, Hair: "red"}); err != nil {
				t.Fatal(err)
			}
			if _, err := r.SetArtStyle(ctx, 1, visual.ArtStyle{ProjectID: p.ID, Style: "ink"}); err != nil {
				t.Fatal(err)
			}
			image, err := r.AddImage(ctx, 1, e.ID, "image/png", "ref", data)
			if err != nil {
				t.Fatal(err)
			}
			if old {
				if _, err := st.DB().ExecContext(ctx, `DROP TABLE entity_visuals; DROP TABLE entity_reference_images; DROP TABLE project_art_style; DELETE FROM schema_migrations WHERE version = 20`); err != nil {
					t.Fatal(err)
				}
			}
			backupPath := filepath.Join(home, "backup.db")
			if _, err := st.DB().ExecContext(ctx, "VACUUM INTO ?", backupPath); err != nil {
				t.Fatal(err)
			}
			live, err := store.Open(ctx, filepath.Join(home, "live.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer live.Close()
			merged, err := MergeProject(ctx, live, backupPath, home, p.ID, " copy", time.Now())
			if err != nil {
				t.Fatal(err)
			}
			entities, err := entity.NewRepo(live).ListByProject(ctx, merged.ProjectID)
			if err != nil || len(entities) != 1 {
				t.Fatalf("entities: %v %v", entities, err)
			}
			repo := visual.NewRepo(live)
			sheet, err := repo.GetSheet(ctx, entities[0].ID)
			if err != nil {
				t.Fatal(err)
			}
			if old {
				if !sheet.Empty() {
					t.Fatal(sheet)
				}
				return
			}
			if sheet.Hair != "red" || sheet.EntityID == e.ID {
				t.Fatal(sheet)
			}
			images, err := repo.ListImages(ctx, entities[0].ID)
			if err != nil || len(images) != 1 || images[0].ID == image.ID {
				t.Fatalf("images: %v %v", images, err)
			}
			got, err := repo.GetImage(ctx, images[0].ID)
			if err != nil || !bytes.Equal(got.Data, data) {
				t.Fatalf("bytes: %v", err)
			}
			var typ string
			if err := live.DB().QueryRowContext(ctx, "SELECT typeof(data) FROM entity_reference_images").Scan(&typ); err != nil || typ != "blob" {
				t.Fatalf("storage type: %s %v", typ, err)
			}
			style, err := repo.GetArtStyle(ctx, merged.ProjectID)
			if err != nil || style.Style != "ink" {
				t.Fatalf("style: %+v %v", style, err)
			}
		})
	}
}

// A merged work brings its style rules with it (#162), and a backup taken
// before the rules existed merges cleanly with none.
func TestMergeProjectCopiesStyleRules(t *testing.T) {
	for _, old := range []bool{false, true} {
		name := "current backup"
		if old {
			name = "backup from before style rules"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			home := t.TempDir()
			st, err := store.Open(ctx, filepath.Join(home, "library.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer st.Close()
			p, err := project.NewRepo(st).Create(ctx, 1, project.NewInput{Title: "Styled", LengthTarget: "short", DefaultPOV: "first"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := style.NewRepo(st).SetRules(ctx, 1, style.Rules{
				ProjectID: p.ID, AvoidPhrases: []string{"그것은 마치", "수 있었다"}, MaxSentenceChars: 80,
			}); err != nil {
				t.Fatal(err)
			}
			if old {
				if _, err := st.DB().ExecContext(ctx, `DROP TABLE project_style_rules; DELETE FROM schema_migrations WHERE version = 21`); err != nil {
					t.Fatal(err)
				}
			}
			backupPath := filepath.Join(home, "backup.db")
			if _, err := st.DB().ExecContext(ctx, "VACUUM INTO ?", backupPath); err != nil {
				t.Fatal(err)
			}
			live, err := store.Open(ctx, filepath.Join(home, "live.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer live.Close()
			merged, err := MergeProject(ctx, live, backupPath, home, p.ID, " copy", time.Now())
			if err != nil {
				t.Fatal(err)
			}
			got, err := style.NewRepo(live).GetRules(ctx, merged.ProjectID)
			if err != nil {
				t.Fatal(err)
			}
			if old {
				if !got.Empty() {
					t.Fatalf("rules from a backup that had none: %+v", got)
				}
				return
			}
			if len(got.AvoidPhrases) != 2 || got.AvoidPhrases[0] != "그것은 마치" || got.MaxSentenceChars != 80 || merged.ProjectID == p.ID {
				t.Fatalf("merged rules = %+v", got)
			}
		})
	}
}

// Story-assist Phase 1: project_focus and scene_intents must follow a work
// through a backup and restore, with anchor_node_id and node_id remapped to
// the copy's own node ids.
func TestMergeProjectCopiesDirection(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	st, err := store.Open(ctx, filepath.Join(home, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	projects := project.NewRepo(st)
	nodes := node.NewRepo(st)
	p, err := projects.Create(ctx, 1, project.NewInput{Title: "Directed", LengthTarget: "short", DefaultPOV: "first"})
	if err != nil {
		t.Fatal(err)
	}
	firstLeaf := *p.LastOpenedNodeID
	if err := nodes.UpdateContent(ctx, firstLeaf,
		`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"글"}]}]}`, 1000); err != nil {
		t.Fatal(err)
	}

	dirRepo := direction.NewRepo(st, nodes, projects)
	if _, err := dirRepo.SetFocus(ctx, 1000, p.ID, "둘의 오해를 아직 풀지 않는다"); err != nil {
		t.Fatal(err)
	}
	if _, err := dirRepo.SetSceneIntent(ctx, 1000, direction.SceneIntent{
		NodeID: firstLeaf, Goal: "거리를 좁힌다", Keep: []string{"침묵"}, Avoid: []string{"고백"},
	}); err != nil {
		t.Fatal(err)
	}

	backupPath := filepath.Join(home, "backup.db")
	if _, err := st.DB().ExecContext(ctx, "VACUUM INTO ?", backupPath); err != nil {
		t.Fatal(err)
	}
	live, err := store.Open(ctx, filepath.Join(home, "live.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	merged, err := MergeProject(ctx, live, backupPath, home, p.ID, " copy", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if merged.ProjectID == p.ID {
		t.Fatal("merge must create a new project id")
	}

	mergedProjects := project.NewRepo(live)
	mergedNodes := node.NewRepo(live)
	mergedRepo := direction.NewRepo(live, mergedNodes, mergedProjects)

	focus, err := mergedRepo.GetFocus(ctx, merged.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if !focus.Present || focus.Body != "둘의 오해를 아직 풀지 않는다" {
		t.Fatalf("merged focus = %+v", focus)
	}
	if focus.AnchorNodeID == "" || focus.AnchorNodeID == firstLeaf {
		t.Fatalf("anchor must be remapped to the copy's own node id, got %q (original %q)", focus.AnchorNodeID, firstLeaf)
	}

	mergedP, err := mergedProjects.Get(ctx, merged.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	mergedLeaf := *mergedP.LastOpenedNodeID
	if focus.AnchorNodeID != mergedLeaf {
		t.Fatalf("anchor = %q, want the copy's first leaf %q", focus.AnchorNodeID, mergedLeaf)
	}

	intent, present, err := mergedRepo.GetSceneIntent(ctx, mergedLeaf)
	if err != nil {
		t.Fatal(err)
	}
	if !present || intent.Goal != "거리를 좁힌다" || len(intent.Keep) != 1 || intent.Keep[0] != "침묵" ||
		len(intent.Avoid) != 1 || intent.Avoid[0] != "고백" {
		t.Fatalf("merged scene intent = %+v, present=%v", intent, present)
	}
}

// Story-assist Phase 2, 2.C.2: continuity checks and observations follow a
// work through a backup and restore. The policy is deliberate and
// user-confirmed (2026-10-24): an observation's writer judgement (dismissed,
// resolved) is work that must not vanish on restore, unlike Phase 3/4's
// evidence packages and forecasts.
func TestMergeProjectCopiesContinuity(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	st, err := store.Open(ctx, filepath.Join(home, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	projects := project.NewRepo(st)
	nodes := node.NewRepo(st)
	p, err := projects.Create(ctx, 1, project.NewInput{Title: "연속성", LengthTarget: "short", DefaultPOV: "first"})
	if err != nil {
		t.Fatal(err)
	}
	leaf := *p.LastOpenedNodeID
	if err := nodes.UpdateContent(ctx, leaf,
		`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"하린은 왼손으로 검을 뽑았다."}]}]}`, 1000); err != nil {
		t.Fatal(err)
	}
	n, err := nodes.Get(ctx, leaf)
	if err != nil {
		t.Fatal(err)
	}

	entities := entity.NewRepo(st)
	harin, err := entities.Create(ctx, 1000, entity.NewInput{ProjectID: p.ID, Kind: "character", Name: "하린"})
	if err != nil {
		t.Fatal(err)
	}
	threads := thread.NewRepo(st)
	th, err := threads.Create(ctx, thread.NewInput{ProjectID: p.ID, Name: "줄기"})
	if err != nil {
		t.Fatal(err)
	}
	beats := beat.NewRepo(st)
	bt, err := beats.Create(ctx, beat.NewInput{ThreadID: th.ID, Label: "첫 전투"})
	if err != nil {
		t.Fatal(err)
	}

	contRepo := continuity.NewRepo(st)
	check, err := contRepo.Record(ctx, 1500, continuity.ReportInput{
		ProjectID: p.ID, NodeID: leaf, SceneVersion: n.ContentVersion,
		Note: "한 번 확인함",
		Observations: []continuity.ObservationInput{
			{
				Category: continuity.CategoryCharacter, Kind: continuity.KindContradiction,
				Severity: continuity.SeverityMedium, Summary: "왼손/오른손 불일치",
				ParagraphFrom: 1, ParagraphTo: 1, Quote: "하린은 왼손으로 검을 뽑았다.",
				Refs: []continuity.Ref{
					{Type: continuity.RefTypeEntity, ID: harin.ID},
					{Type: continuity.RefTypeBeat, ID: bt.ID},
					// A ref to an id that is not actually in this work —
					// remapContinuityRefs must drop it rather than leave a
					// dangling pointer across the restore.
					{Type: continuity.RefTypeEntity, ID: "no-such-entity"},
				},
			},
			{
				Category: continuity.CategoryOther, Kind: continuity.KindQuestion,
				Severity: continuity.SeverityLow, Summary: "확인이 필요한 대목",
				ParagraphFrom: 1, ParagraphTo: 1, Quote: "하린은 왼손으로 검을 뽑았다.",
			},
		},
	}, continuity.SourceAgent)
	if err != nil {
		t.Fatal(err)
	}
	checks, err := contRepo.List(ctx, continuity.ListFilter{ProjectID: p.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(checks) != 1 || len(checks[0].Observations) != 2 {
		t.Fatalf("test setup: checks = %+v", checks)
	}
	dismissedID := checks[0].Observations[1].ID
	if _, err := contRepo.SetStatus(ctx, 1600, dismissedID, continuity.StatusDismissed); err != nil {
		t.Fatal(err)
	}

	backupPath := filepath.Join(home, "backup.db")
	if _, err := st.DB().ExecContext(ctx, "VACUUM INTO ?", backupPath); err != nil {
		t.Fatal(err)
	}
	live, err := store.Open(ctx, filepath.Join(home, "live.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	merged, err := MergeProject(ctx, live, backupPath, home, p.ID, " copy", time.Now())
	if err != nil {
		t.Fatal(err)
	}

	mergedRepo := continuity.NewRepo(live)
	mergedChecks, err := mergedRepo.List(ctx, continuity.ListFilter{ProjectID: merged.ProjectID})
	if err != nil {
		t.Fatal(err)
	}
	if len(mergedChecks) != 1 {
		t.Fatalf("merged checks = %+v, want 1", mergedChecks)
	}
	mc := mergedChecks[0]
	if mc.ID == check.ID {
		t.Fatal("the merged check must get a fresh id")
	}
	if mc.Note != "한 번 확인함" || mc.ContentVersion != n.ContentVersion || mc.Source != continuity.SourceAgent {
		t.Fatalf("merged check = %+v", mc)
	}
	if mc.Stale {
		t.Fatal("a scene restored at the same content_version must not read as stale")
	}
	if len(mc.Observations) != 2 {
		t.Fatalf("merged observations = %d, want 2", len(mc.Observations))
	}

	var contradiction, question continuity.Observation
	for _, o := range mc.Observations {
		switch o.Kind {
		case continuity.KindContradiction:
			contradiction = o
		case continuity.KindQuestion:
			question = o
		}
	}
	if question.Status != continuity.StatusDismissed {
		t.Errorf("the writer's dismiss must survive the restore: status = %q", question.Status)
	}
	if len(contradiction.Refs) != 2 {
		t.Fatalf("contradiction refs = %+v, want 2 (the dangling entity ref must be dropped)", contradiction.Refs)
	}
	var sawEntity, sawBeat bool
	for _, ref := range contradiction.Refs {
		switch ref.Type {
		case continuity.RefTypeEntity:
			sawEntity = true
			if ref.ID == harin.ID {
				t.Error("entity ref must be remapped to the copy's own entity id")
			}
		case continuity.RefTypeBeat:
			sawBeat = true
			if ref.ID == bt.ID {
				t.Error("beat ref must be remapped to the copy's own beat id")
			}
		}
	}
	if !sawEntity || !sawBeat {
		t.Fatalf("expected one entity ref and one beat ref, got %+v", contradiction.Refs)
	}
}
