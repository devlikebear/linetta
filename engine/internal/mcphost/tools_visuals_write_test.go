//go:build !mobile

package mcphost

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/appversion"
	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/linetta/engine/internal/storyops"
	"github.com/devlikebear/linetta/engine/internal/visual"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestVisualWritesTransport(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	d := ToolDeps{Projects: project.NewRepo(st), Entities: entity.NewRepo(st), Nodes: node.NewRepo(st), Visuals: visual.NewRepo(st)}
	d.Story = storyops.New(d.Projects, d.Nodes, nil, nil, d.Entities, nil)
	changed := 0
	d.Notify = func(method string, _ any) {
		if method == "mcp.changed" {
			changed++
		}
	}
	p, err := d.Projects.Create(ctx, 1, project.NewInput{Title: "T", LengthTarget: "short", DefaultPOV: "first"})
	if err != nil {
		t.Fatal(err)
	}
	e, err := d.Entities.Create(ctx, 1, entity.NewInput{ProjectID: p.ID, Name: "Hero"})
	if err != nil {
		t.Fatal(err)
	}
	aliases := []string{"Alias"}
	if err = d.Entities.Update(ctx, 1, entity.UpdateInput{ID: e.ID, Aliases: &aliases}); err != nil {
		t.Fatal(err)
	}
	other, err := d.Entities.Create(ctx, 1, entity.NewInput{ProjectID: p.ID, Name: "Place", Kind: entity.KindPlace})
	if err != nil {
		t.Fatal(err)
	}
	cs := connectedServer(t, d, settings.MCPModeFull, AllToolGroups())
	if got := cs.InitializeResult().ServerInfo.Version; got != appversion.Version {
		t.Fatalf("version %s", got)
	}
	call := func(tool string, args map[string]any, wantError string) map[string]any {
		t.Helper()
		res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "linetta_" + tool, Arguments: args})
		if err != nil {
			t.Fatal(err)
		}
		body := res.Content[0].(*mcp.TextContent).Text
		if wantError != "" {
			if !res.IsError || !strings.Contains(body, wantError) {
				t.Fatalf("%s: %s", tool, body)
			}
			return nil
		}
		if res.IsError {
			t.Fatalf("%s: %s", tool, body)
		}
		out := map[string]any{}
		if err = json.Unmarshal([]byte(body), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	undo := func(out map[string]any) {
		t.Helper()
		call("undo_last_change", map[string]any{"batch_id": out["undo_batch_id"]}, "")
	}
	out := call("set_character_visuals", map[string]any{"project_id": p.ID, "name": " alias ", "hair": "black", "outfit": "coat"}, "")
	if out["hair"] != "black" || out["entity_id"] != e.ID {
		t.Fatal(out)
	}
	undo(out)
	sheets, err := d.Visuals.ListSheetsByProject(ctx, p.ID)
	if err != nil || len(sheets) != 0 {
		t.Fatalf("undo absence: %v %v", sheets, err)
	}
	call("set_character_visuals", map[string]any{"entity_id": e.ID, "hair": "black", "outfit": "coat"}, "")
	out = call("set_character_visuals", map[string]any{"entity_id": e.ID, "hair": ""}, "")
	if out["hair"] != "" || out["outfit"] != "coat" {
		t.Fatal(out)
	}
	undo(out)
	sheet, _ := d.Visuals.GetSheet(ctx, e.ID)
	if sheet.Hair != "black" {
		t.Fatal(sheet)
	}
	call("set_art_style", map[string]any{"project_id": p.ID, "style": "ink", "negative_prompt": "text"}, "")
	out = call("set_art_style", map[string]any{"project_id": p.ID, "style": ""}, "")
	if out["style"] != "" || out["negative_prompt"] != "text" {
		t.Fatal(out)
	}
	undo(out)
	style, _ := d.Visuals.GetArtStyle(ctx, p.ID)
	if style.Style != "ink" {
		t.Fatal(style)
	}
	data := []byte("\x89PNG\r\n\x1a\nimage")
	imageArgs := func() map[string]any {
		return map[string]any{"entity_id": e.ID, "data_base64": base64.StdEncoding.EncodeToString(data), "mime": "image/png", "caption": "ref"}
	}
	out = call("add_reference_image", imageArgs(), "")
	id := out["id"].(string)
	undo(out)
	if _, err = d.Visuals.GetImage(ctx, id); err != visual.ErrImageNotFound {
		t.Fatal(err)
	}
	out = call("add_reference_image", imageArgs(), "")
	id = out["id"].(string)
	before, _ := d.Visuals.GetImage(ctx, id)
	deleted := call("delete_reference_image", map[string]any{"image_id": id}, "")
	if deleted["caption"] != "ref" {
		t.Fatal(deleted)
	}
	undo(deleted)
	restored, err := d.Visuals.GetImage(ctx, id)
	if err != nil || restored.ImageMeta != before.ImageMeta || !bytes.Equal(restored.Data, before.Data) {
		t.Fatalf("restore %v %v", restored, err)
	}
	// Pinning protects entity-id and image-id routes as well as named targets.
	unrestricted := cs
	pinned := storeWithGroups(t, AllToolGroups())
	otherProject := "another-work"
	if _, err := pinned.Set(ctx, settings.Patch{MCPProjectID: &otherProject}); err != nil {
		t.Fatal(err)
	}
	pinnedDeps := d
	pinnedDeps.Settings = pinned
	cs = connectedServer(t, pinnedDeps, settings.MCPModeFull, AllToolGroups())
	beforeRefusals := changed
	call("set_character_visuals", map[string]any{"entity_id": e.ID, "hair": "wrong"}, "restricted")
	call("set_art_style", map[string]any{"project_id": p.ID, "style": "wrong"}, "restricted")
	call("add_reference_image", imageArgs(), "restricted")
	call("delete_reference_image", map[string]any{"image_id": id}, "restricted")
	if changed != beforeRefusals {
		t.Fatal("refused writes emitted change notifications")
	}
	cs = unrestricted
	call("set_character_visuals", map[string]any{"entity_id": e.ID, "project_id": "wrong"}, "another work")
	placeImage, err := d.Visuals.AddImage(ctx, 1, other.ID, "image/png", "", data)
	if err != nil {
		t.Fatal(err)
	}
	call("delete_reference_image", map[string]any{"image_id": placeImage.ID}, "must be a character")
	fullSize := make([]byte, visual.MaxImageBytes)
	copy(fullSize, data)
	maxArgs := imageArgs()
	maxArgs["data_base64"] = base64.StdEncoding.EncodeToString(fullSize)
	undo(call("add_reference_image", maxArgs, ""))
	aliasArgs := imageArgs()
	delete(aliasArgs, "entity_id")
	aliasArgs["project_id"] = p.ID
	aliasArgs["name"] = "ALIAS"
	undo(call("add_reference_image", aliasArgs, ""))
	for _, tool := range []string{"set_character_visuals", "add_reference_image"} {
		args := imageArgs()
		if tool == "set_character_visuals" {
			args = map[string]any{}
		}
		delete(args, "entity_id")
		args["project_id"] = p.ID
		args["name"] = "missing"
		call(tool, args, "available characters: Hero")
		delete(args, "name")
		args["entity_id"] = other.ID
		call(tool, args, "must be a character")
		args["entity_id"] = e.ID
		args["name"] = "Hero"
		call(tool, args, "exactly one")
	}
	call("set_art_style", map[string]any{"project_id": "missing"}, "not found")
	call("delete_reference_image", map[string]any{"image_id": "missing"}, "not found")
	args := imageArgs()
	args["data_base64"] = "%%%"
	call("add_reference_image", args, "invalid data_base64")
	args = imageArgs()
	args["mime"] = "image/jpeg"
	call("add_reference_image", args, "mismatched")
	args = imageArgs()
	args["mime"] = "image/svg+xml"
	call("add_reference_image", args, "unsupported")
	args = imageArgs()
	args["data_base64"] = base64.StdEncoding.EncodeToString(make([]byte, visual.MaxImageBytes+1))
	call("add_reference_image", args, "5 MiB")
	for i := 1; i < visual.MaxImagesPerEntity; i++ {
		call("add_reference_image", imageArgs(), "")
	}
	call("add_reference_image", imageArgs(), "at most 8")
	// A failed restore stays in the undo window, and never breaks the image cap.
	deleted = call("delete_reference_image", map[string]any{"image_id": id}, "")
	added := call("add_reference_image", imageArgs(), "")
	call("undo_last_change", map[string]any{"batch_id": deleted["undo_batch_id"]}, "at most 8")
	undo(added)
	undo(deleted)
	if changed < 10 {
		t.Fatalf("missing notifications: %d", changed)
	}
	// Both MCP modes and optional group filtering must expose precisely ToolNames.
	for _, mode := range []string{settings.MCPModeReadOnly, settings.MCPModeFull} {
		g := AllToolGroups()
		g.DisableVisualWrites = true
		got := registeredToolNamesWithGroups(t, mode, g)
		for _, name := range VisualWriteToolNames {
			if slices.Contains(got, name) {
				t.Fatalf("disabled tool %s", name)
			}
		}
		if !slices.Equal(slices.Sorted(slices.Values(got)), slices.Sorted(slices.Values(ToolNames(mode, g)))) {
			t.Fatal("group inventory mismatch")
		}
	}
}
