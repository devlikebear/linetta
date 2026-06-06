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
	h := ExportProject(f.proj, f.nodes, er, rr)
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
