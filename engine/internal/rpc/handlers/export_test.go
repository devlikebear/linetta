package handlers

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/export"
	"github.com/devlikebear/linetta/engine/internal/relationship"
)

func TestExportProjectHandler(t *testing.T) {
	f := newNodeFixture(t)
	er := entity.NewRepo(f.store)
	rr := relationship.NewRepo(f.store)
	h := ExportProject(export.Sources{Projects: f.proj, Nodes: f.nodes, Entities: er, Relationships: rr}, nil)
	res, err := h(context.Background(), json.RawMessage(`{"project_id":"`+f.pID+`"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var p export.Payload
	if err := json.Unmarshal(res, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !strings.Contains(p.Markdown, "\n# T\n\n") {
		t.Errorf("missing project H1: %q", p.Markdown)
	}
	if !strings.HasSuffix(p.SuggestedFilename, ".md") {
		t.Errorf("filename = %q", p.SuggestedFilename)
	}
}

func TestExportNodeHandler(t *testing.T) {
	f := newNodeFixture(t)
	// seed content
	_ = f.nodes
	h := ExportNode(f.nodes)
	res, err := h(context.Background(), json.RawMessage(`{"node_id":"`+f.nID+`"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var p export.Payload
	_ = json.Unmarshal(res, &p)
	if strings.Contains(p.Markdown, "#") {
		t.Errorf("node export should not have headings: %q", p.Markdown)
	}
}

func TestExportNodeTextHandler(t *testing.T) {
	f := newNodeFixture(t)
	if err := f.nodes.UpdateContent(context.Background(), f.nID,
		`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"플랫폼 복사"}]}]}`, 100); err != nil {
		t.Fatalf("UpdateContent: %v", err)
	}
	h := ExportNodeText(f.nodes)
	res, err := h(context.Background(), json.RawMessage(`{"node_id":"`+f.nID+`"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var p export.TextPayload
	if err := json.Unmarshal(res, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.Text != "플랫폼 복사" {
		t.Fatalf("text = %q", p.Text)
	}
	if p.CharCount != 6 {
		t.Fatalf("char_count = %d, want 6", p.CharCount)
	}
}
