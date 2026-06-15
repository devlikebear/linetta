package companion

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/tars/pkg/session"
)

func TestHistoryRepoListScopesMessagesByScene(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	projects := project.NewRepo(st)
	nodes := node.NewRepo(st)
	p, err := projects.Create(ctx, 1, project.NewInput{Title: "작품", Genres: []string{"fantasy"}, LengthTarget: "novel", DefaultPOV: "first"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	sceneA, err := nodes.CreateRoot(ctx, p.ID, node.KindLeaf, "씬 1", "식탁 위 고지서", 10)
	if err != nil {
		t.Fatalf("create scene A: %v", err)
	}
	sceneB, err := nodes.CreateRoot(ctx, p.ID, node.KindLeaf, "씬 2", "퇴근 선언", 20)
	if err != nil {
		t.Fatalf("create scene B: %v", err)
	}

	repo := NewHistoryRepo(st.DB())
	if err := repo.Append(ctx, HistoryMessage{
		ID: "m1", ProjectID: p.ID, NodeID: sceneA.ID, RunID: "r1",
		Role: "user", Scope: HistoryScopeScene, Intent: "scene_write", Status: HistoryStatusDone,
		Content: "이 씬 작성해줘", CreatedAt: 100,
	}); err != nil {
		t.Fatalf("append scene A user: %v", err)
	}
	if err := repo.Append(ctx, HistoryMessage{
		ID: "m2", ProjectID: p.ID, NodeID: sceneA.ID, RunID: "r1",
		Role: "assistant", Scope: HistoryScopeScene, Intent: "scene_write", Status: HistoryStatusApplied,
		Content: "현재 씬 본문을 반영했습니다.", CreatedAt: 110,
	}); err != nil {
		t.Fatalf("append scene A assistant: %v", err)
	}
	if err := repo.Append(ctx, HistoryMessage{
		ID: "m3", ProjectID: p.ID, NodeID: sceneB.ID, RunID: "r2",
		Role: "user", Scope: HistoryScopeScene, Intent: "scene_write", Status: HistoryStatusDone,
		Content: "다음 씬 작성해줘", CreatedAt: 120,
	}); err != nil {
		t.Fatalf("append scene B user: %v", err)
	}

	sceneMsgs, err := repo.List(ctx, HistoryQuery{ProjectID: p.ID, NodeID: sceneA.ID, Scope: HistoryViewScene, Limit: 20})
	if err != nil {
		t.Fatalf("list scene: %v", err)
	}
	if len(sceneMsgs) != 2 {
		t.Fatalf("scene message count = %d, want 2: %+v", len(sceneMsgs), sceneMsgs)
	}
	for _, msg := range sceneMsgs {
		if msg.NodeID != sceneA.ID {
			t.Fatalf("scene list included node %q, want %q", msg.NodeID, sceneA.ID)
		}
		if msg.NodeLabel != "식탁 위 고지서" {
			t.Fatalf("node label = %q, want title", msg.NodeLabel)
		}
	}

	projectMsgs, err := repo.List(ctx, HistoryQuery{ProjectID: p.ID, Scope: HistoryViewProject, Limit: 20})
	if err != nil {
		t.Fatalf("list project: %v", err)
	}
	if len(projectMsgs) != 3 {
		t.Fatalf("project message count = %d, want 3: %+v", len(projectMsgs), projectMsgs)
	}
	if projectMsgs[2].NodeID != sceneB.ID || projectMsgs[2].NodeLabel != "퇴근 선언" {
		t.Fatalf("project list missing scene B metadata: %+v", projectMsgs[2])
	}
}

func TestHistoryRepoImportLegacyDoesNotMarkEmptyTranscriptImported(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	projects := project.NewRepo(st)
	p, err := projects.Create(ctx, 1, project.NewInput{Title: "작품", Genres: []string{"fantasy"}, LengthTarget: "novel", DefaultPOV: "first"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	repo := NewHistoryRepo(st.DB())

	if err := repo.ImportLegacy(ctx, p.ID, nil, 100); err != nil {
		t.Fatalf("import empty legacy: %v", err)
	}

	var imports int
	if err := st.DB().QueryRowContext(ctx,
		`SELECT COUNT(1) FROM companion_history_imports WHERE project_id = ?`, p.ID).Scan(&imports); err != nil {
		t.Fatalf("count imports: %v", err)
	}
	if imports != 0 {
		t.Fatalf("empty transcript marked imported, count=%d", imports)
	}
}

func TestHistoryRepoImportLegacyRecoversFromStaleEmptyImportMarker(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	projects := project.NewRepo(st)
	p, err := projects.Create(ctx, 1, project.NewInput{Title: "작품", Genres: []string{"fantasy"}, LengthTarget: "novel", DefaultPOV: "first"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	repo := NewHistoryRepo(st.DB())
	if _, err := st.DB().ExecContext(ctx,
		`INSERT INTO companion_history_imports(project_id, imported_at) VALUES(?, ?)`, p.ID, int64(100)); err != nil {
		t.Fatalf("insert stale marker: %v", err)
	}

	msgs := []session.Message{{
		Role:      "user",
		Content:   "앱 종료 전 질문",
		Timestamp: time.UnixMilli(200).UTC(),
	}, {
		Role:      "assistant",
		Content:   "앱 재시작 후 복원될 답",
		Timestamp: time.UnixMilli(210).UTC(),
	}}
	if err := repo.ImportLegacy(ctx, p.ID, msgs, 300); err != nil {
		t.Fatalf("import legacy after stale marker: %v", err)
	}

	got, err := repo.List(ctx, HistoryQuery{ProjectID: p.ID, Scope: HistoryViewProject, Limit: 10})
	if err != nil {
		t.Fatalf("list imported: %v", err)
	}
	if len(got) != 2 || got[0].Content != "앱 종료 전 질문" || got[1].Content != "앱 재시작 후 복원될 답" {
		t.Fatalf("legacy messages not recovered: %+v", got)
	}
}
