package mcphost

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func allOn() ToolGroups { return ToolGroups{Memory: true, Skills: true, Review: true} }

func panelNames(t *testing.T, lang, mode string, groups ToolGroups) []string {
	t.Helper()
	got, err := PanelPrompts(context.Background(), lang, mode, groups)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(got))
	for i, p := range got {
		names[i] = p.Name
	}
	return names
}

// The list an external client sees, pinned: renaming a prompt breaks every
// writer's muscle memory for its slash command, so it should take editing
// this test.
func TestPromptListSnapshot(t *testing.T) {
	got, err := PanelPrompts(context.Background(), "ko", settings.MCPModeFull, allOn())
	if err != nil {
		t.Fatal(err)
	}
	want := []struct{ name, title, slots string }{
		{"draft_current_scene", "현재 씬 초고", "start"},
		{"check_continuity", "연속성 점검", "start,followup"},
		{"fill_story_world", "Story World 채우기", "start"},
		{"suggest_next_scene", "다음 씬 제안", "start"},
		{"forecast_next_episode", "다음 전개 갈래", "start,followup"},
		{"review_episode", "이 화 검수", "followup"},
		{"review_part", "부 전체 비평", "followup"},
		{"close_arc", "아크 결산", "longform"},
		{"rebuild_record", "정본 재구축", "longform"},
		{"find_impact", "최근 결정의 영향 찾기", "longform"},
	}
	if len(got) != len(want) {
		t.Fatalf("listed %d prompts, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		g := got[i]
		if g.Name != w.name || g.Title != w.title || strings.Join(g.Slots, ",") != w.slots || g.Text == "" {
			t.Errorf("prompt %d = %+v, want %s %q slots %s", i, g, w.name, w.title, w.slots)
		}
	}
}

// Every prompt takes the same two optional arguments, so a slash command in
// any client can name the work and scene without the app.
func TestPromptArgumentsAreOptionalProjectAndNode(t *testing.T) {
	srv := mcp.NewServer(&mcp.Implementation{Name: ServerName, Version: ServerVersion}, nil)
	RegisterPrompts(srv, "en", settings.MCPModeFull, allOn())
	ct, st := mcp.NewInMemoryTransports()
	ss, err := srv.Connect(context.Background(), st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "t", Version: "1"}, nil).Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	n := 0
	for p, err := range cs.Prompts(context.Background(), nil) {
		if err != nil {
			t.Fatal(err)
		}
		n++
		if len(p.Arguments) != 2 || p.Arguments[0].Name != "project_id" || p.Arguments[1].Name != "node_id" ||
			p.Arguments[0].Required || p.Arguments[1].Required {
			t.Errorf("%s arguments = %+v", p.Name, p.Arguments)
		}
	}
	if n != len(promptDefs) {
		t.Fatalf("listed %d, want %d", n, len(promptDefs))
	}
	got, err := cs.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name: "draft_current_scene", Arguments: map[string]string{"project_id": "p1", "node_id": "n1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Messages) != 1 || got.Messages[0].Role != "user" {
		t.Fatalf("messages = %+v", got.Messages)
	}
	want := "Write a draft of the scene that's currently open.\n\nWork id: p1\n\nScene id: n1"
	if text := got.Messages[0].Content.(*mcp.TextContent).Text; text != want {
		t.Fatalf("text = %q, want %q", text, want)
	}
}

// A prompt whose tools a server does not serve is not listed there.
func TestPromptsFollowTheToolsTheServerHas(t *testing.T) {
	// A read-only server keeps the two that only read: a critique is said,
	// not recorded.
	if got := panelNames(t, "ko", settings.MCPModeReadOnly, allOn()); !slices.Equal(got, []string{"suggest_next_scene", "review_part"}) {
		t.Errorf("read-only server lists %v", got)
	}
	noReview := allOn()
	noReview.Review = false
	if got := panelNames(t, "ko", settings.MCPModeFull, noReview); !slices.Equal(got, []string{"draft_current_scene", "suggest_next_scene"}) {
		t.Errorf("review tools off lists %v", got)
	}
}

func TestPromptTextFollowsTheAppLanguage(t *testing.T) {
	for lang, want := range map[string]string{
		"ko": "지금 열려 있는 씬의 초고를 써줘.",
		"en": "Write a draft of the scene that's currently open.",
		"ja": promptDefs[0].text["ja"],
		"":   "지금 열려 있는 씬의 초고를 써줘.",
		"fr": "지금 열려 있는 씬의 초고를 써줘.",
	} {
		if got, ok := PromptText("draft_current_scene", lang, "", ""); !ok || got != want {
			t.Errorf("%q: %q, want %q", lang, got, want)
		}
	}
	if _, ok := PromptText("no_such_prompt", "ko", "", ""); ok {
		t.Error("unknown prompt resolved")
	}
}

// Every definition says something in every language the app speaks.
func TestPromptDefinitionsAreComplete(t *testing.T) {
	for _, p := range promptDefs {
		for _, lang := range []string{"ko", "en", "ja"} {
			if strings.TrimSpace(p.title[lang]) == "" || strings.TrimSpace(p.text[lang]) == "" {
				t.Errorf("%s has no %s wording", p.name, lang)
			}
		}
		if len(p.slots) == 0 || len(p.tools) == 0 {
			t.Errorf("%s needs slots and tools", p.name)
		}
		for _, tool := range p.tools {
			if !slices.Contains(ToolNames(settings.MCPModeFull, allOn()), tool) {
				t.Errorf("%s names %s, which no server serves", p.name, tool)
			}
		}
	}
}
