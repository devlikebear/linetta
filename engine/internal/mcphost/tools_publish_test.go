//go:build !mobile

package mcphost

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/publish"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/linetta/engine/internal/store"
)

type fakePublisher struct {
	status    publish.WorkStatus
	statusErr error
	result    publish.PublishResult
	err       error
	published []string
	upserts   []publish.UpsertInput
	work      publish.WorkView
	upsertErr error
}

func (f *fakePublisher) UpsertWork(_ context.Context, in publish.UpsertInput) (publish.WorkView, error) {
	f.upserts = append(f.upserts, in)
	return f.work, f.upsertErr
}

func (f *fakePublisher) WorkStatus(context.Context, string) (publish.WorkStatus, error) {
	return f.status, f.statusErr
}

func (f *fakePublisher) PublishEpisode(_ context.Context, nodeID string) (publish.PublishResult, error) {
	f.published = append(f.published, nodeID)
	return f.result, f.err
}

type publishFixture struct {
	deps      ToolDeps
	pub       *fakePublisher
	projectID string
	nodeID    string
	otherID   string
	otherNode string
	changed   []ChangedPayload
}

func newPublishFixture(t *testing.T) *publishFixture {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	projects := project.NewRepo(st)
	p, err := projects.Create(ctx, 1, project.NewInput{Title: "작품", Genres: []string{}, LengthTarget: "series", DefaultPOV: "first", OutlinePreset: project.OutlinePresetWebNovel})
	if err != nil {
		t.Fatal(err)
	}
	other, err := projects.Create(ctx, 2, project.NewInput{Title: "다른 작품", Genres: []string{}, LengthTarget: "series", DefaultPOV: "first"})
	if err != nil {
		t.Fatal(err)
	}
	f := &publishFixture{pub: &fakePublisher{}, projectID: p.ID, nodeID: *p.LastOpenedNodeID, otherID: other.ID, otherNode: *other.LastOpenedNodeID}
	f.deps = ToolDeps{
		Projects: projects,
		Nodes:    node.NewRepo(st),
		Activity: NewActivityRepo(st.DB()),
		Publish:  f.pub,
		Notify: func(method string, params any) {
			if c, ok := params.(ChangedPayload); ok && method == "mcp.changed" {
				f.changed = append(f.changed, c)
			}
		},
	}
	return f
}

func callPublishTool(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return res
}

func toolText(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

func TestPublishEpisode_refusesWithoutAConnectionAndSaysTheWriterMustConnect(t *testing.T) {
	f := newPublishFixture(t)
	f.pub.err = &rpc.ReasonError{Reason: rpc.ReasonPublishNotConnected, Err: errors.New("no token")}
	cs := connectedServer(t, f.deps, settings.MCPModeFull, AllToolGroups())
	res := callPublishTool(t, cs, "linetta_publish_episode", map[string]any{"node_id": f.nodeID})
	if !res.IsError || !strings.Contains(toolText(res), "Settings → Publishing") || !strings.Contains(toolText(res), "cannot do it for them") {
		t.Fatalf("result = %v %q", res.IsError, toolText(res))
	}
	if len(f.changed) != 0 {
		t.Fatal("a refused publish announced a change")
	}
}

func TestPublishEpisode_refusesAnUnregisteredWork(t *testing.T) {
	f := newPublishFixture(t)
	f.pub.err = &rpc.ReasonError{Reason: rpc.ReasonPublishWorkNotRegistered, Err: errors.New("404")}
	cs := connectedServer(t, f.deps, settings.MCPModeFull, AllToolGroups())
	res := callPublishTool(t, cs, "linetta_publish_episode", map[string]any{"node_id": f.nodeID})
	if !res.IsError || !strings.Contains(toolText(res), "linetta_register_work") {
		t.Fatalf("result = %q", toolText(res))
	}
}

func TestPublishEpisode_publishesAnnouncesAndIsLogged(t *testing.T) {
	f := newPublishFixture(t)
	f.pub.result = publish.PublishResult{Created: true, Episode: publish.EpisodeView{
		NodeID: f.nodeID, Number: 1, URL: "https://works.marvin-42.com/@a/w/1", State: publish.EpisodePublic,
	}}
	cs := connectedServer(t, f.deps, settings.MCPModeFull, AllToolGroups())
	res := callPublishTool(t, cs, "linetta_publish_episode", map[string]any{"node_id": f.nodeID})
	if res.IsError {
		t.Fatalf("publish failed: %q", toolText(res))
	}
	var out publishEpisodeOutput
	raw, _ := json.Marshal(res.StructuredContent)
	if err := json.Unmarshal(raw, &out); err != nil || out.URL == "" || out.Number != 1 || !out.Created {
		t.Fatalf("output = %s", raw)
	}
	if !slices.Equal(f.pub.published, []string{f.nodeID}) {
		t.Fatalf("published %v", f.pub.published)
	}
	if len(f.changed) != 1 || f.changed[0].Tool != "linetta_publish_episode" || f.changed[0].ProjectID != f.projectID {
		t.Fatalf("changed = %+v", f.changed)
	}
	entries, err := f.deps.Activity.List(context.Background(), 10)
	if err != nil || len(entries) != 1 || entries[0].Tool != "linetta_publish_episode" || !entries[0].OK || entries[0].TargetID != f.nodeID {
		t.Fatalf("activity = %+v, %v", entries, err)
	}
}

func TestPublishEpisode_respectsTheSingleWorkRestriction(t *testing.T) {
	f := newPublishFixture(t)
	st, err := settings.NewForHomeWithSecretStore(t.TempDir(), settings.NewMemorySecretStore())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Set(context.Background(), settings.Patch{MCPProjectID: &f.projectID}); err != nil {
		t.Fatal(err)
	}
	f.deps.Settings = st
	cs := connectedServer(t, f.deps, settings.MCPModeFull, AllToolGroups())
	res := callPublishTool(t, cs, "linetta_publish_episode", map[string]any{"node_id": f.otherNode})
	if !res.IsError || len(f.pub.published) != 0 {
		t.Fatalf("published a node outside the allowed work: %q", toolText(res))
	}
}

func TestPublishStatus_reportsStatesAndIsAReadTool(t *testing.T) {
	f := newPublishFixture(t)
	f.pub.status = publish.WorkStatus{
		Connected: true, Registered: true, Work: &publish.WorkView{URL: "https://works.marvin-42.com/@a/w"},
		Episodes: []publish.EpisodeView{{NodeID: f.nodeID, State: publish.EpisodeModified, Number: 1}},
		SiteOnly: []publish.EpisodeView{{NodeID: "gone", State: publish.EpisodeSiteOnly, Number: 2}},
	}
	// Read only: the status tool is there, the publish tool is not.
	cs := connectedServer(t, f.deps, settings.MCPModeReadOnly, AllToolGroups())
	list, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range list.Tools {
		names = append(names, tool.Name)
	}
	if !slices.Contains(names, "linetta_publish_status") || slices.Contains(names, "linetta_publish_episode") {
		t.Fatalf("read_only tools = %v", names)
	}
	res := callPublishTool(t, cs, "linetta_publish_status", map[string]any{"project_id": f.projectID})
	if res.IsError {
		t.Fatalf("status failed: %q", toolText(res))
	}
	var out publishStatusOutput
	raw, _ := json.Marshal(res.StructuredContent)
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if !out.Connected || out.WorkURL == "" || len(out.Episodes) != 1 || out.Episodes[0].State != "modified" || len(out.SiteOnly) != 1 {
		t.Fatalf("status = %s", raw)
	}
}

func TestPublishTools_refuseWhenNoPublisherIsWired(t *testing.T) {
	f := newPublishFixture(t)
	f.deps.Publish = nil
	cs := connectedServer(t, f.deps, settings.MCPModeFull, AllToolGroups())
	for _, call := range []struct {
		name string
		args map[string]any
	}{
		{"linetta_publish_status", map[string]any{"project_id": f.projectID}},
		{"linetta_publish_episode", map[string]any{"node_id": f.nodeID}},
		{"linetta_register_work", map[string]any{"project_id": f.projectID, "blurb": "소개"}},
	} {
		if res := callPublishTool(t, cs, call.name, call.args); !res.IsError {
			t.Errorf("%s without a publisher did not refuse", call.name)
		}
	}
}

func TestRegisterWork_registersWithTheBlurbAnnouncesAndIsLogged(t *testing.T) {
	f := newPublishFixture(t)
	f.pub.work = publish.WorkView{URL: "https://works.marvin-42.com/@a/w", SerialStatus: publish.SerialOngoing}
	cs := connectedServer(t, f.deps, settings.MCPModeFull, AllToolGroups())
	res := callPublishTool(t, cs, "linetta_register_work", map[string]any{"project_id": f.projectID, "blurb": "  회귀한 검사의 이야기  "})
	if res.IsError {
		t.Fatalf("register failed: %q", toolText(res))
	}
	if len(f.pub.upserts) != 1 || f.pub.upserts[0].ProjectID != f.projectID || f.pub.upserts[0].Blurb != "회귀한 검사의 이야기" {
		t.Fatalf("upserts = %+v", f.pub.upserts)
	}
	var out registerWorkOutput
	raw, _ := json.Marshal(res.StructuredContent)
	if err := json.Unmarshal(raw, &out); err != nil || out.WorkURL == "" || out.SerialStatus != "ongoing" {
		t.Fatalf("output = %s", raw)
	}
	if len(f.changed) != 1 || f.changed[0].Tool != "linetta_register_work" || f.changed[0].ProjectID != f.projectID {
		t.Fatalf("changed = %+v", f.changed)
	}
	entries, err := f.deps.Activity.List(context.Background(), 10)
	if err != nil || len(entries) != 1 || entries[0].Tool != "linetta_register_work" || !entries[0].OK {
		t.Fatalf("activity = %+v, %v", entries, err)
	}
}

func TestRegisterWork_keepsTheCurrentBlurbWhenOnlyTheSerialStatusChanges(t *testing.T) {
	f := newPublishFixture(t)
	f.pub.status = publish.WorkStatus{Connected: true, Registered: true,
		Work: &publish.WorkView{Blurb: "원래 소개", SerialStatus: publish.SerialOngoing}}
	cs := connectedServer(t, f.deps, settings.MCPModeFull, AllToolGroups())
	res := callPublishTool(t, cs, "linetta_register_work", map[string]any{"project_id": f.projectID, "serial_status": "completed"})
	if res.IsError {
		t.Fatalf("update failed: %q", toolText(res))
	}
	if len(f.pub.upserts) != 1 || f.pub.upserts[0].Blurb != "원래 소개" || f.pub.upserts[0].SerialStatus != "completed" {
		t.Fatalf("upserts = %+v", f.pub.upserts)
	}
}

func TestRegisterWork_asksForABlurbTheFirstTime(t *testing.T) {
	f := newPublishFixture(t)
	f.pub.upsertErr = &rpc.ReasonError{Reason: rpc.ReasonPublishBlurbRequired, Err: errors.New("no blurb")}
	cs := connectedServer(t, f.deps, settings.MCPModeFull, AllToolGroups())
	res := callPublishTool(t, cs, "linetta_register_work", map[string]any{"project_id": f.projectID})
	if !res.IsError || !strings.Contains(toolText(res), "blurb is required") {
		t.Fatalf("result = %q", toolText(res))
	}
	if len(f.changed) != 0 {
		t.Fatal("a refused registration announced a change")
	}
}

func TestRegisterWork_refusesAnOverlongBlurbBeforeSendingIt(t *testing.T) {
	f := newPublishFixture(t)
	cs := connectedServer(t, f.deps, settings.MCPModeFull, AllToolGroups())
	res := callPublishTool(t, cs, "linetta_register_work", map[string]any{
		"project_id": f.projectID, "blurb": strings.Repeat("가", publish.MaxBlurbRunes+1)})
	if !res.IsError || len(f.pub.upserts) != 0 {
		t.Fatalf("an overlong blurb was sent: %q", toolText(res))
	}
}

func TestRegisterWork_stillNeedsTheWriterToConnect(t *testing.T) {
	f := newPublishFixture(t)
	f.pub.upsertErr = &rpc.ReasonError{Reason: rpc.ReasonPublishNotConnected, Err: errors.New("no token")}
	cs := connectedServer(t, f.deps, settings.MCPModeFull, AllToolGroups())
	res := callPublishTool(t, cs, "linetta_register_work", map[string]any{"project_id": f.projectID, "blurb": "소개"})
	if !res.IsError || !strings.Contains(toolText(res), "Settings → Publishing") {
		t.Fatalf("result = %q", toolText(res))
	}
}

func TestRegisterWork_respectsTheSingleWorkRestrictionAndIsAWriteTool(t *testing.T) {
	f := newPublishFixture(t)
	ro := connectedServer(t, f.deps, settings.MCPModeReadOnly, AllToolGroups())
	list, err := ro.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range list.Tools {
		if tool.Name == "linetta_register_work" {
			t.Fatal("read_only lists linetta_register_work")
		}
	}
	st, err := settings.NewForHomeWithSecretStore(t.TempDir(), settings.NewMemorySecretStore())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Set(context.Background(), settings.Patch{MCPProjectID: &f.projectID}); err != nil {
		t.Fatal(err)
	}
	f.deps.Settings = st
	cs := connectedServer(t, f.deps, settings.MCPModeFull, AllToolGroups())
	res := callPublishTool(t, cs, "linetta_register_work", map[string]any{"project_id": f.otherID, "blurb": "소개"})
	if !res.IsError || len(f.pub.upserts) != 0 {
		t.Fatalf("registered a work outside the allowed one: %q", toolText(res))
	}
}
