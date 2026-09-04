//go:build !mobile

package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/companion"
)

type fakeScope struct {
	titles map[string]string
	labels map[string]string
}

func (f fakeScope) ProjectTitle(_ context.Context, id string) string { return f.titles[id] }
func (f fakeScope) NodeLabel(_ context.Context, id string) string    { return f.labels[id] }

func TestSystemPrompt_namesTheReplyLanguage(t *testing.T) {
	for _, lang := range []string{"ko", "en", "ja"} {
		got := systemPrompt(lang)
		if !strings.Contains(got, lang) {
			t.Errorf("systemPrompt(%q) does not name the language: %s", lang, got)
		}
	}
}

// The brief is fetched with a tool, never pasted in. If it ever appears here,
// the tool descriptions stop being exercised and start rotting.
func TestSystemPrompt_tellsTheAgentToReadContextFirst(t *testing.T) {
	got := systemPrompt("en")
	if !strings.Contains(got, "linetta_get_story_context") {
		t.Error("the prompt must point at the context tool")
	}
	if !strings.Contains(got, "linetta_create_checkpoint") {
		t.Error("the prompt must ask for a checkpoint before a large rewrite")
	}
}

func TestScopeLine_namesTheWorkAndTheOpenScene(t *testing.T) {
	s := fakeScope{
		titles: map[string]string{"p1": "은하수를 여행하는"},
		labels: map[string]string{"n1": "1장 · 출발"},
	}
	got := scopeLine(context.Background(), s, "p1", "n1")
	for _, want := range []string{"p1", "은하수를 여행하는", "n1", "1장 · 출발"} {
		if !strings.Contains(got, want) {
			t.Errorf("scope line %q is missing %q", got, want)
		}
	}
}

// With no scene open the line must not invent one; a bracket containing an
// empty id reads to the model as a scene it can address.
func TestScopeLine_omitsTheSceneWhenNoneIsOpen(t *testing.T) {
	s := fakeScope{titles: map[string]string{"p1": "제목"}}
	got := scopeLine(context.Background(), s, "p1", "")
	if strings.Contains(strings.ToLower(got), "scene") {
		t.Errorf("scope line %q mentions a scene that is not open", got)
	}
}

// Tool rows are not replayed: they are the bulkiest thing in the transcript
// and the model already saw their outcome in its own reply.
func TestPriorMessages_dropsToolRowsAndKeepsOrder(t *testing.T) {
	got := priorMessages([]companion.HistoryMessage{
		{Role: "user", Content: "first"},
		{Role: "tool", Content: `{"name":"linetta_write_scene"}`},
		{Role: "assistant", Content: "second"},
	})
	if len(got) != 2 {
		t.Fatalf("got %d messages, want 2: %+v", len(got), got)
	}
	if got[0].Content != "first" || got[1].Content != "second" {
		t.Errorf("order or content wrong: %+v", got)
	}
}

// The budget is filled from the most recent turn backwards, so what survives
// is the end of the conversation, not its beginning.
func TestPriorMessages_keepsTheNewestWithinBudget(t *testing.T) {
	big := strings.Repeat("a", historyBudget-10)
	got := priorMessages([]companion.HistoryMessage{
		{Role: "user", Content: "oldest"},
		{Role: "assistant", Content: big},
		{Role: "user", Content: "newest"},
	})
	joined := ""
	for _, m := range got {
		joined += m.Content
	}
	if !strings.Contains(joined, "newest") {
		t.Error("the newest turn was dropped")
	}
	if strings.Contains(joined, "oldest") {
		t.Error("the budget did not bite")
	}
}

func TestPriorMessages_mapsRolesForTheModel(t *testing.T) {
	got := priorMessages([]companion.HistoryMessage{
		{Role: "user", Content: "q"},
		{Role: "assistant", Content: "a"},
	})
	if got[0].Role != "user" || got[1].Role != "assistant" {
		t.Errorf("roles = %q,%q", got[0].Role, got[1].Role)
	}
}
