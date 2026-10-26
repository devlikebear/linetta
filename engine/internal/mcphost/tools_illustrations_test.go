//go:build !mobile

package mcphost

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/jpeg"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/illustration"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/linetta/engine/internal/storyops"
	"github.com/devlikebear/linetta/engine/internal/visual"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// noDrawing fails the test if anything is drawn: the tools never draw.
type noDrawing struct{ t *testing.T }

func (g noDrawing) Generate(context.Context, illustration.GenerateInput) (illustration.Meta, error) {
	g.t.Fatal("an MCP tool drew a picture")
	return illustration.Meta{}, nil
}

type illustrationToolsFixture struct {
	ctx      context.Context
	d        ToolDeps
	cs       *mcp.ClientSession
	project  string
	scene    string
	episode  string
	heroID   string
	picture  string
	requests []string // illustrations.changed statuses
	changed  []ChangedPayload
}

func newIllustrationToolsFixture(t *testing.T, source string) *illustrationToolsFixture {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	f := &illustrationToolsFixture{ctx: ctx}
	d := ToolDeps{Projects: project.NewRepo(st), Entities: entity.NewRepo(st), Nodes: node.NewRepo(st), Visuals: visual.NewRepo(st), Source: source}
	d.Story = storyops.New(d.Projects, d.Nodes, nil, nil, d.Entities, nil)
	d.Gallery = illustration.NewRepo(st)
	d.Illustrations = d.Gallery
	d.IllustrationRequests = illustration.NewRequestService(d.Gallery, noDrawing{t}, d.Nodes, func() int64 { return 7 },
		func(method string, params any) {
			f.requests = append(f.requests, params.(illustration.ChangedEvent).Status)
		})
	d.Notify = func(method string, params any) {
		if method == "mcp.changed" {
			f.changed = append(f.changed, params.(ChangedPayload))
		}
	}
	p, err := d.Projects.Create(ctx, 1, project.NewInput{Title: "비 오는 밤", LengthTarget: "short", DefaultPOV: "first"})
	if err != nil {
		t.Fatal(err)
	}
	f.project, f.scene = p.ID, *p.LastOpenedNodeID
	ep, err := d.Nodes.CreateRoot(ctx, p.ID, node.KindContainer, "1화", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	f.episode = ep.ID
	hero, err := d.Entities.Create(ctx, 1, entity.NewInput{ProjectID: p.ID, Name: "하린"})
	if err != nil {
		t.Fatal(err)
	}
	f.heroID = hero.ID
	if _, err := d.Visuals.SetSheet(ctx, 3, visual.Sheet{EntityID: hero.ID, Hair: "짧은 은발"}); err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, 4, 4)), nil); err != nil {
		t.Fatal(err)
	}
	pic, err := d.Gallery.Add(ctx, 4, illustration.NewInput{ProjectID: p.ID, SourceNodeID: f.scene, Caption: "우산", Prompt: "p", Provider: "x", Model: "m", Data: b.Bytes()})
	if err != nil {
		t.Fatal(err)
	}
	f.picture = pic.ID
	f.d = d
	f.cs = connectedServer(t, d, settings.MCPModeFull, AllToolGroups())
	return f
}

func (f *illustrationToolsFixture) call(t *testing.T, tool string, args map[string]any, wantError string) map[string]any {
	t.Helper()
	res, err := f.cs.CallTool(f.ctx, &mcp.CallToolParams{Name: "linetta_" + tool, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	body := res.Content[0].(*mcp.TextContent).Text
	if wantError != "" {
		if !res.IsError || !strings.Contains(body, wantError) {
			t.Fatalf("%s: want error %q, got %s", tool, wantError, body)
		}
		return nil
	}
	if res.IsError {
		t.Fatalf("%s: %s", tool, body)
	}
	out := map[string]any{}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestRequestIllustration_composesFromTheSheetsAndDrawsNothing(t *testing.T) {
	f := newIllustrationToolsFixture(t, SourceAgent)
	out := f.call(t, "request_illustration", map[string]any{
		"project_id": f.project, "node_id": f.scene, "purpose": "insert",
		"scene": "비 오는 골목에서 우산을 든다", "names": []string{"하린"}, "caption": "우산", "note": "첫 장면",
	}, "")
	if out["status"] != "waiting_for_writer" || out["request_id"] == "" {
		t.Fatalf("out = %v", out)
	}
	prompt, _ := out["prompt"].(string)
	if !strings.Contains(prompt, "짧은 은발") || !strings.Contains(prompt, "비 오는 골목") {
		t.Fatalf("prompt was not composed from the sheet and scene: %q", prompt)
	}
	list, err := f.d.IllustrationRequests.List(f.ctx, f.project, nil)
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %v, %v", list, err)
	}
	r := list[0]
	if r.Source != illustration.SourceAgent || r.Purpose != "insert" || r.Note != "첫 장면" ||
		!r.IncludeReferences || len(r.CharacterIDs) != 1 || r.CharacterIDs[0] != f.heroID {
		t.Fatalf("request = %+v", r)
	}
	if strings.Join(f.requests, ",") != "pending" {
		t.Fatalf("events = %v", f.requests)
	}
}

func TestRequestIllustration_externalClientsAreMarkedSo(t *testing.T) {
	f := newIllustrationToolsFixture(t, "")
	f.call(t, "request_illustration", map[string]any{
		"project_id": f.project, "purpose": "gallery", "prompt": "완성된 프롬프트",
	}, "")
	list, _ := f.d.IllustrationRequests.List(f.ctx, f.project, nil)
	if len(list) != 1 || list[0].Source != illustration.SourceExternal || list[0].Prompt != "완성된 프롬프트" || list[0].IncludeReferences {
		t.Fatalf("list = %+v", list)
	}
}

func TestRequestIllustration_refusesWhatTheWriterCouldNotApprove(t *testing.T) {
	f := newIllustrationToolsFixture(t, SourceAgent)
	f.call(t, "request_illustration", map[string]any{"project_id": f.project, "purpose": "gallery"}, "either scene")
	f.call(t, "request_illustration", map[string]any{"project_id": f.project, "purpose": "gallery", "scene": "s", "prompt": "p"}, "either scene")
	f.call(t, "request_illustration", map[string]any{"project_id": f.project, "node_id": f.episode, "purpose": "insert", "prompt": "p"}, "only be inserted into a scene")
	f.call(t, "request_illustration", map[string]any{"project_id": f.project, "purpose": "insert", "prompt": "p"}, "needs the scene")
	f.call(t, "request_illustration", map[string]any{"project_id": f.project, "purpose": "cover", "prompt": "p"}, "purpose must be")
	f.call(t, "request_illustration", map[string]any{"project_id": f.project, "purpose": "gallery", "prompt": "p", "names": []string{"없는 사람"}}, "unknown character")
	if list, _ := f.d.IllustrationRequests.List(f.ctx, f.project, nil); len(list) != 0 {
		t.Fatalf("stored %d refused requests", len(list))
	}
}

func TestRequestIllustration_isBounded(t *testing.T) {
	f := newIllustrationToolsFixture(t, SourceAgent)
	for i := 0; i < illustration.MaxPendingRequests; i++ {
		f.call(t, "request_illustration", map[string]any{"project_id": f.project, "purpose": "gallery", "prompt": "p"}, "")
	}
	f.call(t, "request_illustration", map[string]any{"project_id": f.project, "purpose": "gallery", "prompt": "p"}, "approve or reject some first")
}

func TestListIllustrations_showsTheGalleryAndRequestOutcomes(t *testing.T) {
	f := newIllustrationToolsFixture(t, SourceAgent)
	if _, err := f.d.Gallery.SetRepresentative(f.ctx, 5, f.episode, f.picture); err != nil {
		t.Fatal(err)
	}
	req := f.call(t, "request_illustration", map[string]any{"project_id": f.project, "node_id": f.episode, "purpose": "representative", "prompt": "p"}, "")
	if _, err := f.d.IllustrationRequests.Reject(f.ctx, req["request_id"].(string)); err != nil {
		t.Fatal(err)
	}
	out := f.call(t, "list_illustrations", map[string]any{"project_id": f.project}, "")
	pics := out["pictures"].([]any)
	if len(pics) != 1 {
		t.Fatalf("pictures = %v", pics)
	}
	pic := pics[0].(map[string]any)
	if pic["id"] != f.picture || pic["source_node_id"] != f.scene || pic["caption"] != "우산" {
		t.Fatalf("picture = %v", pic)
	}
	if reps := pic["representative_of"].([]any); len(reps) != 1 || reps[0] != f.episode {
		t.Fatalf("representative_of = %v", reps)
	}
	requests := out["requests"].([]any)
	if len(requests) != 1 {
		t.Fatalf("requests = %v", requests)
	}
	r := requests[0].(map[string]any)
	if r["request_id"] != req["request_id"] || r["status"] != "rejected" || r["purpose"] != "representative" || r["node_id"] != f.episode {
		t.Fatalf("request = %v", r)
	}
}

func TestSetRepresentativeIllustration_setUndoAndClear(t *testing.T) {
	f := newIllustrationToolsFixture(t, SourceAgent)
	out := f.call(t, "set_representative_illustration", map[string]any{"node_id": f.episode, "illustration_id": f.picture}, "")
	if out["illustration_id"] != f.picture || out["previous"] != "" || out["undo_batch_id"] == "" {
		t.Fatalf("out = %v", out)
	}
	if reps, _ := f.d.Gallery.Representatives(f.ctx, f.project); reps[f.episode] != f.picture {
		t.Fatalf("representatives = %v", reps)
	}
	if n := len(f.changed); n != 1 || !slices.Equal(f.changed[0].NodeIDs, []string{f.episode}) || f.changed[0].Source != SourceAgent {
		t.Fatalf("changed = %+v", f.changed)
	}
	f.call(t, "undo_last_change", map[string]any{"batch_id": out["undo_batch_id"]}, "")
	if reps, _ := f.d.Gallery.Representatives(f.ctx, f.project); len(reps) != 0 {
		t.Fatalf("after undo = %v", reps)
	}

	f.call(t, "set_representative_illustration", map[string]any{"node_id": f.scene, "illustration_id": f.picture}, "")
	cleared := f.call(t, "set_representative_illustration", map[string]any{"node_id": f.scene}, "")
	if cleared["previous"] != f.picture || cleared["illustration_id"] != "" {
		t.Fatalf("cleared = %v", cleared)
	}
	f.call(t, "set_representative_illustration", map[string]any{"node_id": f.scene, "illustration_id": "nope"}, "linetta_list_illustrations")
	f.call(t, "set_representative_illustration", map[string]any{"node_id": "nope", "illustration_id": f.picture}, "not found")
}

func TestIllustrationWriteToolsFollowTheVisualWritesSwitch(t *testing.T) {
	f := newIllustrationToolsFixture(t, SourceAgent)
	names := registeredToolNamesWithGroups(t, settings.MCPModeFull, ToolGroups{DisableVisualWrites: true, Memory: true, Skills: true, Review: true})
	for _, n := range IllustrationWriteToolNames {
		if slices.Contains(names, n) {
			t.Errorf("%s is served with visual writes off", n)
		}
	}
	if !slices.Contains(names, "linetta_list_illustrations") {
		t.Error("the read tool went with the write switch")
	}
	readOnly := registeredToolNamesWithGroups(t, settings.MCPModeReadOnly, AllToolGroups())
	if slices.Contains(readOnly, "linetta_request_illustration") || !slices.Contains(readOnly, "linetta_list_illustrations") {
		t.Errorf("read_only serves %v", readOnly)
	}
	_ = f
}
