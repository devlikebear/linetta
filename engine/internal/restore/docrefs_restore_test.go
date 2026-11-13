package restore

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/mention"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/note"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/linetta/engine/internal/store/storetest"
)

// docIDs lists the ids a scene document names: mentioned characters and
// note markers.
func docIDs(t *testing.T, raw string) (mentions, notes []string) {
	t.Helper()
	var doc any
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("parse %s: %v", raw, err)
	}
	var walk func(v any)
	walk = func(v any) {
		switch n := v.(type) {
		case map[string]any:
			attrs, _ := n["attrs"].(map[string]any)
			switch n["type"] {
			case "mention":
				mentions = append(mentions, attrs["id"].(string))
			case "noteMarker":
				notes = append(notes, attrs["noteId"].(string))
			}
			walk(n["content"])
		case []any:
			for _, c := range n {
				walk(c)
			}
		}
	}
	walk(doc)
	return mentions, notes
}

func sceneDoc(entityID, noteID string) string {
	return fmt.Sprintf(`{"type":"doc","content":[{"type":"paragraph","content":[`+
		`{"type":"mention","attrs":{"id":%q,"label":"해진"}},`+
		`{"type":"text","text":"이 문을 열었다."},`+
		`{"type":"noteMarker","attrs":{"noteId":%q}}]}]}`, entityID, noteID)
}

// A work merged back into the same library it came from must name its own
// characters and notes inside its scenes, not the original's: the mention
// table is rebuilt from those ids on every save, and a note marker opens
// whatever note its id names (#108).
func TestMergeProjectPointsSceneMentionsAndNotesAtTheCopies(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	st, err := storetest.Open(ctx, filepath.Join(home, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	nodes := node.NewRepo(st)
	mentions := mention.NewRepo(st)
	nodes.SetMentionResyncer(func(ctx context.Context, nodeID, doc string) error {
		return mentions.ResyncForNode(ctx, nodeID, mention.Collect([]byte(doc)))
	})
	notes := note.NewRepo(st)

	p, err := project.NewRepo(st).Create(ctx, 1, project.NewInput{Title: "검은 탑", LengthTarget: "short", DefaultPOV: "first"})
	if err != nil {
		t.Fatal(err)
	}
	scene, err := nodes.Get(ctx, *p.LastOpenedNodeID)
	if err != nil {
		t.Fatal(err)
	}
	hero, err := entity.NewRepo(st).Create(ctx, 2, entity.NewInput{ProjectID: p.ID, Kind: "character", Name: "해진"})
	if err != nil {
		t.Fatal(err)
	}
	memo, err := notes.Create(ctx, note.NewInput{NodeID: scene.ID, Anchor: 1, Body: "문 색깔 확인"}, 3)
	if err != nil {
		t.Fatal(err)
	}
	doc := sceneDoc(hero.ID, memo.ID)
	if err := nodes.UpdateContent(ctx, scene.ID, doc, 4); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB().ExecContext(ctx,
		`INSERT INTO node_snapshots (id, node_id, content_doc, reason, created_at) VALUES ('s1', ?, ?, 'manual', 5)`,
		scene.ID, doc); err != nil {
		t.Fatal(err)
	}
	backupPath := filepath.Join(home, "backup.db")
	if _, err := st.DB().ExecContext(ctx, "VACUUM INTO ?", backupPath); err != nil {
		t.Fatal(err)
	}

	merged, err := MergeProject(ctx, st, backupPath, home, p.ID, " (복원)", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	copyScene := onlyID(ctx, t, st, `SELECT last_opened_node_id FROM projects WHERE id = ?`, merged.ProjectID)
	copyHero := onlyID(ctx, t, st, `SELECT id FROM entities WHERE project_id = ?`, merged.ProjectID)
	copyMemo := onlyID(ctx, t, st, `SELECT id FROM notes WHERE node_id = ?`, copyScene)
	if copyHero == hero.ID || copyMemo == memo.ID {
		t.Fatalf("copies kept the original ids: hero %s memo %s", copyHero, copyMemo)
	}

	wantCopies := func(where, raw string) {
		t.Helper()
		m, n := docIDs(t, raw)
		if len(m) != 1 || m[0] != copyHero || len(n) != 1 || n[0] != copyMemo {
			t.Fatalf("%s names mentions %v notes %v, want [%s] [%s]", where, m, n, copyHero, copyMemo)
		}
	}
	copied, err := nodes.Get(ctx, copyScene)
	if err != nil {
		t.Fatal(err)
	}
	wantCopies("copied scene", *copied.ContentDoc)
	var snapDoc string
	if err := st.DB().QueryRowContext(ctx, `SELECT content_doc FROM node_snapshots WHERE node_id = ?`, copyScene).Scan(&snapDoc); err != nil {
		t.Fatal(err)
	}
	wantCopies("copied version", snapDoc)

	// Saving the copy (and the startup rebuild) keeps its mention on its own
	// character.
	if err := nodes.UpdateContent(ctx, copyScene, *copied.ContentDoc, 6); err != nil {
		t.Fatal(err)
	}
	if err := mentions.RebuildAll(ctx); err != nil {
		t.Fatal(err)
	}
	assertMentions(ctx, t, mentions, copyScene, copyHero)
	assertMentions(ctx, t, mentions, scene.ID, hero.ID)

	// Putting the old version back does not bring the original's ids back.
	if err := nodes.UpdateContent(ctx, copyScene, snapDoc, 7); err != nil {
		t.Fatal(err)
	}
	assertMentions(ctx, t, mentions, copyScene, copyHero)

	// Editing the copy's note leaves the original's alone.
	if err := notes.Update(ctx, note.UpdateInput{ID: copyMemo, Body: "복원본에서 고침"}); err != nil {
		t.Fatal(err)
	}
	if got, err := notes.Get(ctx, memo.ID); err != nil || got.Body != "문 색깔 확인" {
		t.Fatalf("original note = %+v, %v", got, err)
	}

	// Deleting the original leaves the copy whole.
	if err := project.NewRepo(st).Delete(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if err := mentions.RebuildAll(ctx); err != nil {
		t.Fatal(err)
	}
	assertMentions(ctx, t, mentions, copyScene, copyHero)
	if got, err := notes.Get(ctx, copyMemo); err != nil || got.Body != "복원본에서 고침" {
		t.Fatalf("copied note after deleting the original = %+v, %v", got, err)
	}
}

func onlyID(ctx context.Context, t *testing.T, st *store.Store, query string, arg string) string {
	t.Helper()
	rows, err := st.DB().QueryContext(ctx, query, arg)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if len(ids) != 1 {
		t.Fatalf("%s %s = %v, want one", query, arg, ids)
	}
	return ids[0]
}

func assertMentions(ctx context.Context, t *testing.T, mentions *mention.Repo, nodeID, entityID string) {
	t.Helper()
	got, err := mentions.ListForNode(ctx, nodeID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].EntityID != entityID {
		t.Fatalf("mentions of %s = %+v, want one of %s", nodeID, got, entityID)
	}
}
