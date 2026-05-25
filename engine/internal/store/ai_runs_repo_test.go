package store

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestAIRunsRepo_InsertUpdateList(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	r := NewAIRunsRepo(s)
	ctx := context.Background()

	// Need a project row to satisfy the FK.
	if _, err := s.DB().ExecContext(ctx, `
INSERT INTO projects (id, title, genres, length_target, default_pov, created_at, updated_at)
VALUES ('p1', 'T', '["SF"]', 'novel', 'first', 0, 0)`); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	// Seed a node row so the FK on ai_runs.node_id is satisfied.
	if _, err := s.DB().ExecContext(ctx, `
INSERT INTO nodes (id, project_id, ordinal, kind, label, created_at, updated_at)
VALUES ('n1', 'p1', 1, 'scene', 'Scene 1', 0, 0)`); err != nil {
		t.Fatalf("seed node: %v", err)
	}

	nid := "n1"
	if err := r.Insert(ctx, AIRun{
		ID: "r1", ProjectID: "p1", NodeID: &nid,
		Provider: "claude-code-cli", Prompt: "다시 써줘",
		ContextJSON: json.RawMessage(`{"entities":["해진"]}`),
		Status:      AIRunStreaming,
		StartedAt:   1000,
	}); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	if err := r.UpdateStatus(ctx, "r1", AIRunDone, "결과 본문", "", 1500); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	got, err := r.ListRecent(ctx, "p1", 10)
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0].Status != AIRunDone || got[0].Output != "결과 본문" || got[0].EndedAt == nil || *got[0].EndedAt != 1500 {
		t.Errorf("row = %+v", got[0])
	}
}
