package handlers

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/export"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/snapshot"
	"github.com/devlikebear/linetta/engine/internal/store"
)

func TestSmokeCreateSaveSnapshotExport(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	projects := project.NewRepo(st)
	nodes := node.NewRepo(st)
	snaps := snapshot.NewRepo(st)
	entities := entity.NewRepo(st)

	createdRaw, err := CreateProject(projects, func() int64 { return 1000 })(ctx, json.RawMessage(`{
		"title": "Smoke Story",
		"genres": ["literary"],
		"length_target": "short",
		"default_pov": "first"
	}`))
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	var created project.Project
	if err := json.Unmarshal(createdRaw, &created); err != nil {
		t.Fatalf("unmarshal project: %v", err)
	}
	if created.LastOpenedNodeID == nil {
		t.Fatal("created project has no first leaf")
	}

	doc := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"첫 문장을 저장한다."}]}]}`
	saveParams, _ := json.Marshal(map[string]string{"id": *created.LastOpenedNodeID, "doc": doc})
	if _, err := UpdateNodeContent(nodes, snaps, func() int64 { return 2000 }, nil)(ctx, saveParams); err != nil {
		t.Fatalf("save content: %v", err)
	}

	snapshotParams, _ := json.Marshal(map[string]string{"node_id": *created.LastOpenedNodeID, "doc": doc})
	if _, err := CreateManualSnapshot(snaps, func() int64 { return 3000 })(ctx, snapshotParams); err != nil {
		t.Fatalf("manual snapshot: %v", err)
	}

	exportParams, _ := json.Marshal(map[string]string{"project_id": created.ID})
	exportedRaw, err := ExportProject(projects, nodes, entities)(ctx, exportParams)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	var payload export.Payload
	if err := json.Unmarshal(exportedRaw, &payload); err != nil {
		t.Fatalf("unmarshal export: %v", err)
	}
	if !strings.Contains(payload.Markdown, "첫 문장을 저장한다.") {
		t.Fatalf("exported markdown missing saved text: %q", payload.Markdown)
	}
	if !strings.HasSuffix(payload.SuggestedFilename, ".md") {
		t.Fatalf("suggested filename = %q", payload.SuggestedFilename)
	}
}
