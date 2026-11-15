package mcphost

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/linetta/engine/internal/store/storetest"
)

type citeFixture struct {
	d         ToolDeps
	work      project.Project
	sceneID   string
	episodeID string
	other     project.Project
}

// A work with an episode "1화" holding scene "씬 1" (비 오는 음악실), which reads,
// as linetta_read_scene numbered=true prints it:
// [1] 비가 내렸다.  [2] 수아는 은빛 열쇠를 쥐었다.  [3] 문이 열렸다.
func newCiteFixture(t *testing.T) (context.Context, *citeFixture) {
	t.Helper()
	ctx := context.Background()
	st, err := storetest.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	projects := project.NewRepo(st)
	nodes := node.NewRepo(st)
	mk := func(title string) project.Project {
		p, err := projects.Create(ctx, 1, project.NewInput{
			Title: title, Genres: []string{"fantasy"}, LengthTarget: "novel", DefaultPOV: "first",
		})
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	f := &citeFixture{d: ToolDeps{Projects: projects, Nodes: nodes}, work: mk("작품"), other: mk("다른 작품")}
	episode, err := nodes.CreateRoot(ctx, f.work.ID, node.KindContainer, "1화", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	scene, err := nodes.CreateChild(ctx, episode.ID, node.KindLeaf, "씬 1", "비 오는 음악실", 3)
	if err != nil {
		t.Fatal(err)
	}
	f.episodeID, f.sceneID = episode.ID, scene.ID
	var blocks []string
	for _, text := range []string{"비가 내렸다.", "", "수아는 은빛 열쇠를 쥐었다.", "문이 열렸다."} {
		if text == "" {
			blocks = append(blocks, `{"type":"paragraph"}`)
			continue
		}
		blocks = append(blocks, fmt.Sprintf(`{"type":"paragraph","content":[{"type":"text","text":%q}]}`, text))
	}
	doc := `{"type":"doc","content":[` + strings.Join(blocks, ",") + `]}`
	if err := nodes.UpdateContent(ctx, f.sceneID, doc, 10); err != nil {
		t.Fatal(err)
	}
	return ctx, f
}

func TestCiteSourcesNumbersVerifiedPassages(t *testing.T) {
	ctx, f := newCiteFixture(t)

	res, out, err := f.d.citeSources(ctx, nil, citeSourcesInput{
		ProjectID: f.work.ID,
		Citations: []citationInput{
			{NodeID: f.sceneID, ParagraphFrom: 2, Quote: "은빛  열쇠를"},
			{NodeID: f.sceneID, ParagraphFrom: 1, ParagraphTo: 3, Quote: "내렸다. 수아는"},
		},
	})
	if err != nil || res == nil || res.IsError {
		t.Fatalf("cite: res=%+v err=%v", res, err)
	}
	if len(out.Citations) != 2 {
		t.Fatalf("citations = %+v", out.Citations)
	}
	first, second := out.Citations[0], out.Citations[1]
	if first.N != 1 || first.ParagraphFrom != 2 || first.ParagraphTo != 2 || first.Quote != "은빛  열쇠를" {
		t.Fatalf("first = %+v; paragraph_to defaults to paragraph_from and the quote is kept as cited", first)
	}
	if second.N != 2 || second.ParagraphFrom != 1 || second.ParagraphTo != 3 {
		t.Fatalf("second = %+v", second)
	}
	if first.NodeID != f.sceneID || first.Breadcrumb != "1화 / 씬 1" || first.SceneTitle != "비 오는 음악실" {
		t.Fatalf("first = %+v, want the scene's outline path and title", first)
	}
	text := toolText(res)
	for _, want := range []string{"2 sources verified", "[1] 1화 / 씬 1 (비 오는 음악실), paragraph 2", "[2] ", "paragraphs 1-3"} {
		if !strings.Contains(text, want) {
			t.Fatalf("text %q lacks %q", text, want)
		}
	}
}

func TestCiteSourcesRefusesEveryBadCitationAtOnce(t *testing.T) {
	ctx, f := newCiteFixture(t)
	otherScene := *f.other.LastOpenedNodeID

	res, out, err := f.d.citeSources(ctx, nil, citeSourcesInput{
		ProjectID: f.work.ID,
		Citations: []citationInput{
			{NodeID: f.sceneID, ParagraphFrom: 2, Quote: "수아는 은빛 열쇠를 쥐었다."},
			{NodeID: f.sceneID, ParagraphFrom: 2, Quote: "금빛 열쇠"},
			{NodeID: f.sceneID, ParagraphFrom: 4, Quote: "문이"},
			{NodeID: otherScene, ParagraphFrom: 1, Quote: "무엇이든"},
			{NodeID: f.episodeID, ParagraphFrom: 1, Quote: "비가"},
			{NodeID: f.sceneID, ParagraphFrom: 1, Quote: "  "},
		},
	})
	if err != nil || res == nil || !res.IsError || len(out.Citations) != 0 {
		t.Fatalf("cite: res=%+v out=%+v err=%v", res, out, err)
	}
	text := toolText(res)
	for _, want := range []string{
		"citation 2: quote not found in paragraph 2",
		"linetta_read_scene numbered=true",
		"citation 3: paragraph 4 is out of range; scene " + fmt.Sprintf("%q", f.sceneID) + " has 3 paragraphs",
		"citation 4: scene " + fmt.Sprintf("%q", otherScene) + " is not in work",
		"citation 5: node",
		"not a scene",
		"citation 6: quote is required",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("refusal lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "citation 1") {
		t.Errorf("the good citation is not a problem:\n%s", text)
	}
}

func TestCiteSourcesBoundsTheCall(t *testing.T) {
	ctx, f := newCiteFixture(t)
	cases := map[string][]citationInput{
		"empty": nil,
		"seven": make([]citationInput, MaxCitations+1),
	}
	for name, citations := range cases {
		res, _, err := f.d.citeSources(ctx, nil, citeSourcesInput{ProjectID: f.work.ID, Citations: citations})
		if err != nil || res == nil || !res.IsError {
			t.Fatalf("%s: res=%+v err=%v", name, res, err)
		}
	}
	long := strings.Repeat("가", 301)
	res, _, _ := f.d.citeSources(ctx, nil, citeSourcesInput{
		ProjectID: f.work.ID,
		Citations: []citationInput{{NodeID: f.sceneID, ParagraphFrom: 1, Quote: long}},
	})
	if res == nil || !strings.Contains(toolText(res), "quote is 301 characters; the limit is 300") {
		t.Fatalf("long quote: %+v", res)
	}
}

// The agent reads the sources back off the structured result, so they have
// to travel there over a real server, beside the text an external client
// reads. A read-only server has the tool too: citing changes nothing.
func TestCiteSourcesReturnsStructuredCitationsOverTheWire(t *testing.T) {
	ctx, f := newCiteFixture(t)
	cs := connectedServer(t, f.d, settings.MCPModeReadOnly, AllToolGroups())
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "linetta_cite_sources", Arguments: map[string]any{
		"project_id": f.work.ID,
		"citations":  []map[string]any{{"node_id": f.sceneID, "paragraph_from": 3, "quote": "문이 열렸다"}},
	}})
	if err != nil || res.IsError {
		t.Fatalf("call: res=%+v err=%v", res, err)
	}
	structured, ok := res.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structured = %#v", res.StructuredContent)
	}
	list, _ := structured["citations"].([]any)
	if len(list) != 1 {
		t.Fatalf("citations = %#v", structured["citations"])
	}
	if c := list[0].(map[string]any); c["n"] != float64(1) || c["paragraph_from"] != float64(3) || c["quote"] != "문이 열렸다" {
		t.Fatalf("citation = %#v", c)
	}
	if !strings.Contains(toolText(res), "[1] ") {
		t.Fatalf("text = %q", toolText(res))
	}
}
