//go:build !mobile

package mcphost

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/linetta/engine/internal/visual"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestVisualToolsOverTransport(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	d := ToolDeps{Projects: project.NewRepo(st), Entities: entity.NewRepo(st), Nodes: node.NewRepo(st), Visuals: visual.NewRepo(st)}
	p, err := d.Projects.Create(ctx, 1, project.NewInput{Title: "T", LengthTarget: "short", DefaultPOV: "first"})
	if err != nil {
		t.Fatal(err)
	}
	e, err := d.Entities.Create(ctx, 1, entity.NewInput{ProjectID: p.ID, Name: "해진", Role: "주인공"})
	if err != nil {
		t.Fatal(err)
	}
	aliases := []string{"HaEJin"}
	if err := d.Entities.Update(ctx, 1, entity.UpdateInput{ID: e.ID, Aliases: &aliases}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Visuals.SetSheet(ctx, 1, visual.Sheet{EntityID: e.ID, Hair: "검은 머리"}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Visuals.SetArtStyle(ctx, 1, visual.ArtStyle{ProjectID: p.ID, Style: "수채화", NegativePrompt: "글자"}); err != nil {
		t.Fatal(err)
	}
	img, err := d.Visuals.AddImage(ctx, 1, e.ID, "image/png", "ref", []byte("\x89PNG\r\n\x1a\nimage"))
	if err != nil {
		t.Fatal(err)
	}
	cs := connectedServer(t, d, settings.MCPModeReadOnly, AllToolGroups())
	call := func(name string, args map[string]any) *mcp.CallToolResult {
		t.Helper()
		res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	for _, name := range []string{"해진", " haejin "} {
		res := call("linetta_get_character_visuals", map[string]any{"project_id": p.ID, "names": []string{name}, "include_images": true})
		if res.IsError || len(res.Content) != 2 {
			t.Fatalf("visuals: %+v", res)
		}
		text, ok := res.Content[0].(*mcp.TextContent)
		if !ok {
			t.Fatal("missing JSON text before image")
		}
		var out characterVisualsOutput
		if err := json.Unmarshal([]byte(text.Text), &out); err != nil {
			t.Fatal(err)
		}
		if out.ArtStyle.Style != "수채화" || len(out.Characters) != 1 || out.Characters[0].Hair != "검은 머리" || !out.Characters[0].HasSheet {
			t.Fatalf("output: %+v", out)
		}
		if _, ok := res.Content[1].(*mcp.ImageContent); !ok || res.StructuredContent == nil {
			t.Fatal("missing image or structured output")
		}
	}
	res := call("linetta_get_character_visuals", map[string]any{"project_id": p.ID, "names": []string{"unknown"}})
	if !res.IsError || !strings.Contains(res.Content[0].(*mcp.TextContent).Text, "해진") {
		t.Fatalf("unknown: %+v", res)
	}
	res = call("linetta_build_illustration_prompt", map[string]any{"project_id": p.ID, "scene": "scene"})
	if !res.IsError || !strings.Contains(res.Content[0].(*mcp.TextContent).Text, "name the characters") {
		t.Fatalf("a prompt with no characters named must be refused, not filled with the whole cast: %+v", res)
	}
	res = call("linetta_build_illustration_prompt", map[string]any{"project_id": p.ID, "names": []string{"해진"}, "scene": "1컷: 웃는다", "format": "four_panel", "node_id": *p.LastOpenedNodeID})
	var out illustrationPromptOutput
	if res.IsError {
		t.Fatalf("prompt: %+v", res)
	}
	if err := json.Unmarshal([]byte(res.Content[0].(*mcp.TextContent).Text), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.Prompt, "4컷만화") || !strings.Contains(out.Prompt, "1컷: 웃는다") || out.NegativePrompt != "글자" || len(out.ReferenceImageIDs) != 1 || out.ReferenceImageIDs[0] != img.ID || out.SceneLabel == "" {
		t.Fatalf("prompt: %+v", out)
	}

	// Three maximum-size references exceed the per-call binary budget; metadata still lists all of them.
	large := make([]byte, visual.MaxImageBytes)
	copy(large, []byte("\x89PNG\r\n\x1a\n"))
	for i := 0; i < 3; i++ {
		if _, err := d.Visuals.AddImage(ctx, 2, e.ID, "image/png", "large", large); err != nil {
			t.Fatal(err)
		}
	}
	capped, summary, err := d.getCharacterVisuals(ctx, nil, characterVisualsInput{ProjectID: p.ID, IncludeImages: true})
	if err != nil || capped.IsError || len(summary.SkippedImageIDs) != 1 || len(summary.Characters[0].ReferenceImages) != 4 {
		t.Fatalf("cap: %+v %v", summary, err)
	}
	total := 0
	for _, content := range capped.Content {
		if img, ok := content.(*mcp.ImageContent); ok {
			total += len(img.Data)
		}
	}
	if total > 12*1024*1024 {
		t.Fatalf("binary budget exceeded: %d", total)
	}
	empty, err := d.Entities.Create(ctx, 1, entity.NewInput{ProjectID: p.ID, Name: "Empty"})
	if err != nil {
		t.Fatal(err)
	}
	_, missing, err := d.buildIllustrationPrompt(ctx, nil, illustrationPromptInput{ProjectID: p.ID, EntityIDs: []string{empty.ID}, Scene: "scene"})
	if err != nil || len(missing.MissingSheets) != 1 || missing.MissingSheets[0] != "Empty" || missing.Format != "single" {
		t.Fatalf("missing: %+v %v", missing, err)
	}
	other, err := d.Projects.Create(ctx, 1, project.NewInput{Title: "Other", LengthTarget: "short", DefaultPOV: "first"})
	if err != nil {
		t.Fatal(err)
	}
	res = call("linetta_build_illustration_prompt", map[string]any{"project_id": p.ID, "names": []string{"해진"}, "scene": "scene", "node_id": *other.LastOpenedNodeID})
	if !res.IsError {
		t.Fatal("cross-work scene accepted")
	}
	res = call("linetta_get_character_visuals", map[string]any{"project_id": other.ID, "entity_ids": []string{e.ID}})
	if !res.IsError {
		t.Fatal("cross-work entity accepted")
	}
}
func TestVisualToolsRefuseMissingRepo(t *testing.T) {
	d := ToolDeps{}
	res, _, err := d.getCharacterVisuals(context.Background(), nil, characterVisualsInput{})
	if err != nil || !res.IsError {
		t.Fatalf("%+v %v", res, err)
	}
	res, _, err = d.buildIllustrationPrompt(context.Background(), nil, illustrationPromptInput{})
	if err != nil || !res.IsError {
		t.Fatalf("%+v %v", res, err)
	}
}
